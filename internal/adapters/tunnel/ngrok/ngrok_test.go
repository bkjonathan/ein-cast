package ngrok

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/providerconfig"
	"eincast/internal/domain/server"
)

const testToken = "2xYzSECRETauthtoken_abcdefghijklmnop"

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) Get(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[k]; ok {
		return v, nil
	}
	return "", app.ErrSecretNotFound
}
func (s *memSecrets) Set(k, v string) error { s.mu.Lock(); defer s.mu.Unlock(); s.m[k] = v; return nil }
func (s *memSecrets) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, k)
	return nil
}

type fakeServer struct{ addrs []netip.AddrPort }

func newServer() fakeServer {
	return fakeServer{addrs: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:8765"), netip.MustParseAddrPort("192.168.1.9:8765")}}
}
func (f fakeServer) Addrs() []netip.AddrPort       { return f.addrs }
func (fakeServer) Bind(netip.Addr) (func(), error) { return func() {}, nil }

// fakeEndpoint is a published endpoint we can end from "ngrok's side".
type fakeEndpoint struct {
	url      string
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	closes   int
	closeErr error
}

func newEndpoint(url string) *fakeEndpoint { return &fakeEndpoint{url: url, done: make(chan struct{})} }

func (e *fakeEndpoint) URL() string           { return e.url }
func (e *fakeEndpoint) Done() <-chan struct{} { return e.done }
func (e *fakeEndpoint) Close(context.Context) error {
	e.mu.Lock()
	e.closes++
	err := e.closeErr
	e.mu.Unlock()
	if err == nil {
		e.end()
	}
	return err
}
func (e *fakeEndpoint) end() { e.once.Do(func() { close(e.done) }) }
func (e *fakeEndpoint) closeCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closes
}

// fakeConnector records what it was asked and returns what it is told to.
type fakeConnector struct {
	ep  *fakeEndpoint
	err error
	// block makes Forward wait for its context, like a connection that hangs.
	block bool

	mu       sync.Mutex
	calls    int
	gotToken string
	gotUp    string
	onEvent  func(event)
}

func (c *fakeConnector) Forward(ctx context.Context, token, upstream string, onEvent func(event)) (endpoint, error) {
	c.mu.Lock()
	c.calls++
	c.gotToken, c.gotUp, c.onEvent = token, upstream, onEvent
	block, err, ep := c.block, c.err, c.ep
	c.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return ep, nil
}

func (c *fakeConnector) emit(e event) {
	c.mu.Lock()
	f := c.onEvent
	c.mu.Unlock()
	f(e)
}

func newProvider(t *testing.T, conn *fakeConnector, withToken bool) *Provider {
	t.Helper()
	cfg := providerconfig.New(&memSecrets{m: map[string]string{}})
	if withToken {
		if err := cfg.Set(ID, authtokenField(), testToken); err != nil {
			t.Fatal(err)
		}
	}
	p := newWith(cfg, conn, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	return p
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestProvider_IsPublicAndNeedsAnAuthtoken(t *testing.T) {
	p := newProvider(t, &fakeConnector{}, false)
	if !p.Public() || p.ID() != "ngrok" {
		t.Errorf("public=%v id=%q: it is open to the whole Internet and must demand a PIN", p.Public(), p.ID())
	}
	f := p.Fields()
	if len(f) != 1 || f[0].Key != "authtoken" || f[0].Kind != app.FieldSecret || !f[0].Required {
		t.Errorf("fields = %+v: the authtoken is a required secret", f)
	}
}

func TestStart_PublishesAnEndpointForwardingToLoopbackOnly(t *testing.T) {
	conn := &fakeConnector{ep: newEndpoint("https://abc-123.ngrok-free.app")}
	p := newProvider(t, conn, true)

	url, err := p.Start(context.Background(), newServer())

	if err != nil {
		t.Fatal(err)
	}
	if url != "https://abc-123.ngrok-free.app" {
		t.Errorf("url = %q", url)
	}
	if conn.gotToken != testToken {
		t.Errorf("connector got token %q, want the saved one", conn.gotToken)
	}
	if conn.gotUp != "http://127.0.0.1:8765" {
		t.Errorf("forwarding to %q: it must be the loopback address, never a LAN one", conn.gotUp)
	}
	if st := p.Status(); st.State != server.Running || st.URL != url {
		t.Errorf("status = %+v", st)
	}
}

func TestStart_WithoutAnAuthtokenItDoesNotConnect(t *testing.T) {
	conn := &fakeConnector{ep: newEndpoint("https://x")}
	p := newProvider(t, conn, false)

	_, err := p.Start(context.Background(), newServer())

	if !errors.Is(err, app.ErrNotConfigured) {
		t.Fatalf("error = %v", err)
	}
	if conn.calls != 0 {
		t.Error("connected to ngrok without a token")
	}
	if st := p.Status(); st.State != server.Failed {
		t.Errorf("status = %+v", st)
	}
}

func TestStart_FailuresExplainThemselvesAndNeverContainTheToken(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"rejected token":     {errors.New("authentication failed: The authtoken you specified (" + testToken + ") is invalid. ERR_NGROK_105"), "did not accept the authtoken"},
		"no network":         {errors.New("failed to dial ngrok server with address \"connect.ngrok-agent.com:443\": dial tcp: lookup: no such host"), "could not start ngrok: failed to dial"},
		"token in a message": {errors.New("something odd with " + testToken), "[redacted]"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProvider(t, &fakeConnector{err: c.err}, true)

			_, err := p.Start(context.Background(), newServer())

			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want one containing %q", err, c.want)
			}
			if strings.Contains(err.Error(), testToken) || strings.Contains(p.Status().Error, testToken) {
				t.Errorf("the token is in the error or the status:\n%v\n%+v", err, p.Status())
			}
			if st := p.Status(); st.State != server.Failed {
				t.Errorf("status = %+v", st)
			}
		})
	}
}

func TestStart_CancellingWhileConnectingReturnsTheCancellation(t *testing.T) {
	p := newProvider(t, &fakeConnector{block: true}, true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := p.Start(ctx, newServer()); done <- err }()
	time.Sleep(30 * time.Millisecond)

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want the cancellation, not a made-up failure", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return")
	}
}

func TestStart_TwiceIsRefusedAndOnlyConnectsOnce(t *testing.T) {
	conn := &fakeConnector{ep: newEndpoint("https://a")}
	p := newProvider(t, conn, true)
	if _, err := p.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Start(context.Background(), newServer()); err == nil {
		t.Error("second Start succeeded")
	}
	if conn.calls != 1 {
		t.Errorf("connected %d times", conn.calls)
	}
}

func TestStop_ClosesTheEndpointOnceAndCanStartAgain(t *testing.T) {
	ep := newEndpoint("https://a")
	conn := &fakeConnector{ep: ep}
	p := newProvider(t, conn, true)
	if _, err := p.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if err := p.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	if ep.closeCount() != 1 {
		t.Errorf("closed %d times, want 1", ep.closeCount())
	}
	if st := p.Status(); st.State != server.Idle || st.URL != "" {
		t.Errorf("status = %+v, want off", st)
	}

	conn.mu.Lock()
	conn.ep = newEndpoint("https://b")
	conn.mu.Unlock()
	if url, err := p.Start(context.Background(), newServer()); err != nil || url != "https://b" {
		t.Errorf("restart: %q, %v", url, err)
	}
}

func TestStop_BeforeStartIsFine(t *testing.T) {
	if err := newProvider(t, &fakeConnector{}, true).Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStop_ThatCannotCloseKeepsTheEndpointSoItCanBeRetried(t *testing.T) {
	ep := newEndpoint("https://a")
	ep.closeErr = errors.New("connection reset")
	p := newProvider(t, &fakeConnector{ep: ep}, true)
	if _, err := p.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}

	if err := p.Stop(context.Background()); err == nil {
		t.Fatal("Stop reported success although the endpoint could not be closed")
	}
	if p.Status().State != server.Running {
		t.Errorf("status = %+v: it is not confirmed stopped, so it must not say so", p.Status())
	}

	ep.mu.Lock()
	ep.closeErr = nil
	ep.mu.Unlock()
	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if p.Status().State != server.Idle {
		t.Errorf("status = %+v", p.Status())
	}
}

func TestEndingTheLifetimeClosesTheEndpoint(t *testing.T) {
	ep := newEndpoint("https://a")
	p := newProvider(t, &fakeConnector{ep: ep}, true)
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := p.Start(ctx, newServer()); err != nil {
		t.Fatal(err)
	}

	cancel()

	eventually(t, "the endpoint to be closed", func() bool { return ep.closeCount() == 1 })
	eventually(t, "status to be off", func() bool { return p.Status().State == server.Idle })
}

func TestWhenNgrokEndsTheEndpointItIsReportedAsFailed(t *testing.T) {
	ep := newEndpoint("https://a")
	p := newProvider(t, &fakeConnector{ep: ep}, true)
	if _, err := p.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}

	ep.end() // ngrok closes it on its side

	eventually(t, "Failed", func() bool { return p.Status().State == server.Failed })
	if msg := p.Status().Error; !strings.Contains(msg, "ngrok closed the tunnel") {
		t.Errorf("error = %q", msg)
	}
}

func TestOurOwnStopIsNotReportedAsNgrokClosingIt(t *testing.T) {
	ep := newEndpoint("https://a")
	p := newProvider(t, &fakeConnector{ep: ep}, true)
	if _, err := p.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond) // the watcher had every chance to misreport
	if st := p.Status(); st.State != server.Idle {
		t.Errorf("status = %+v", st)
	}
}

// slowCloseEndpoint ends (Done fires) and then takes a while to finish closing,
// as a real endpoint does: the watcher gets to run in between.
type slowCloseEndpoint struct{ *fakeEndpoint }

func (e slowCloseEndpoint) Close(ctx context.Context) error {
	e.end()
	time.Sleep(80 * time.Millisecond)
	return e.fakeEndpoint.Close(ctx)
}

func TestOurOwnStopNeverFlashesAsFailedWhileItIsClosing(t *testing.T) {
	conn := &fakeConnector{}
	p := newWith(providerconfig.New(&memSecrets{m: map[string]string{}}), conn, slog.New(slog.DiscardHandler))
	if err := p.cfg.Set(ID, authtokenField(), testToken); err != nil {
		t.Fatal(err)
	}
	// Wrap so Forward hands back the slow-closing endpoint.
	p.conn = connectorFunc(func(ctx context.Context, token, up string, f func(event)) (endpoint, error) {
		return slowCloseEndpoint{newEndpoint("https://a")}, nil
	})
	if _, err := p.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}

	stopped := make(chan struct{})
	go func() { _ = p.Stop(context.Background()); close(stopped) }()
	for {
		if st := p.Status(); st.State == server.Failed {
			t.Fatalf("status = %+v: a tunnel we are stopping was reported as having failed", st)
		}
		select {
		case <-stopped:
			if st := p.Status(); st.State != server.Idle {
				t.Errorf("after Stop: %+v", st)
			}
			return
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

type connectorFunc func(ctx context.Context, token, upstream string, onEvent func(event)) (endpoint, error)

func (f connectorFunc) Forward(ctx context.Context, token, upstream string, onEvent func(event)) (endpoint, error) {
	return f(ctx, token, upstream, onEvent)
}

func TestConnectionEventsShowReconnecting(t *testing.T) {
	conn := &fakeConnector{ep: newEndpoint("https://a")}
	p := newProvider(t, conn, true)
	if _, err := p.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}

	conn.emit(event{err: errors.New("heartbeat timeout")})
	if st := p.Status(); st.State != server.Starting {
		t.Errorf("after a disconnect: %+v, want reconnecting", st)
	}

	conn.emit(event{connected: true})
	if st := p.Status(); st.State != server.Running || st.URL != "https://a" {
		t.Errorf("after reconnecting: %+v", st)
	}
}

func TestEventsOutsideARunAreIgnored(t *testing.T) {
	conn := &fakeConnector{ep: newEndpoint("https://a")}
	p := newProvider(t, conn, true)
	if _, err := p.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	conn.emit(event{connected: true}) // a late event from the closed session

	if st := p.Status(); st.State != server.Idle {
		t.Errorf("status = %+v: a stale event revived a stopped tunnel", st)
	}
}

var _ app.LocalServer = fakeServer{}
