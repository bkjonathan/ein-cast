package remote

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/server"
)

const testPIN = "correct horse battery"

// fakeTunnel is a controllable app.Tunnel.
type fakeTunnel struct {
	id     app.ProviderID
	public bool
	url    app.PublicURL

	mu          sync.Mutex
	startErr    error
	stopErr     error
	block       bool // Start waits for its context instead of returning
	started     int
	stopped     int
	gotSrv      app.LocalServer
	life        context.Context
	live        app.TunnelStatus
	statusCalls int
	fields      []app.ConfigField
}

func publicTunnel() *fakeTunnel {
	return &fakeTunnel{id: "fake-public", public: true, url: "https://abc.trycloudflare.com"}
}

func privateTunnel() *fakeTunnel {
	return &fakeTunnel{id: "fake-private", url: "http://den.tail1234.ts.net:8765"}
}

func (f *fakeTunnel) ID() app.ProviderID { return f.id }

// Fields makes a fakeTunnel Configurable when it has any; with none it behaves
// like a provider that needs nothing.
func (f *fakeTunnel) Fields() []app.ConfigField { return f.fields }
func (f *fakeTunnel) Public() bool              { return f.public }

func (f *fakeTunnel) Start(ctx context.Context, srv app.LocalServer) (app.PublicURL, error) {
	f.mu.Lock()
	f.started++
	f.gotSrv, f.life = srv, ctx
	err, block, url := f.startErr, f.block, f.url
	f.mu.Unlock()

	if block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	return url, nil
}

func (f *fakeTunnel) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
	return f.stopErr
}

func (f *fakeTunnel) Status() app.TunnelStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusCalls++
	return f.live
}

func (f *fakeTunnel) polls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statusCalls
}

func (f *fakeTunnel) counts() (started, stopped int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started, f.stopped
}

func (f *fakeTunnel) lifetime() context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.life
}

func (f *fakeTunnel) set(mod func(*fakeTunnel)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mod(f)
}

// fakeServer is an app.LocalServer listening on loopback.
type fakeServer struct{}

func localServer() *fakeServer { return &fakeServer{} }

func (*fakeServer) Addrs() []netip.AddrPort {
	return []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:8765")}
}

func (*fakeServer) Bind(netip.Addr) (func(), error) { return func() {}, nil }

type fakeRegistry struct{ byID map[app.ProviderID]app.Tunnel }

func newRegistry(ts ...*fakeTunnel) *fakeRegistry {
	r := &fakeRegistry{byID: map[app.ProviderID]app.Tunnel{}}
	for _, t := range ts {
		r.byID[t.id] = t
	}
	return r
}

func (r *fakeRegistry) Lookup(id app.ProviderID) (app.Tunnel, error) {
	t, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("provider %q: %w", id, app.ErrUnknownProvider)
	}
	return t, nil
}

func (r *fakeRegistry) List() []app.ProviderInfo {
	var out []app.ProviderInfo
	for id, t := range r.byID {
		info := app.ProviderInfo{ID: id, Public: t.Public()}
		if c, ok := t.(app.Configurable); ok {
			info.Fields = c.Fields()
		}
		out = append(out, info)
	}
	return out
}

// fakeSecrets is an in-memory app.SecretStore that can be made to fail.
type fakeSecrets struct {
	mu     sync.Mutex
	m      map[string]string
	getErr error
	setErr error
}

func newSecrets() *fakeSecrets { return &fakeSecrets{m: map[string]string{}} }

func (f *fakeSecrets) Get(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.m[key]
	if !ok {
		return "", fmt.Errorf("get %q: %w", key, app.ErrSecretNotFound)
	}
	return v, nil
}

func (f *fakeSecrets) Set(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.m[key] = value
	return nil
}

func (f *fakeSecrets) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, key)
	return nil
}

func (f *fakeSecrets) stored(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.m[key]
}

type fakeEvents struct {
	mu  sync.Mutex
	got []app.RemoteStatus
}

func (f *fakeEvents) Publish(ev app.Event) {
	if e, ok := ev.(app.RemoteStatusChanged); ok {
		f.mu.Lock()
		f.got = append(f.got, e.Status)
		f.mu.Unlock()
	}
}

// liveStates is the provider's own state as each event carried it.
func (f *fakeEvents) liveStates() []server.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]server.State, len(f.got))
	for i, s := range f.got {
		out[i] = s.Live.State
	}
	return out
}

func (f *fakeEvents) states() []server.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]server.State, len(f.got))
	for i, s := range f.got {
		out[i] = s.State
	}
	return out
}

// clock is a manually advanced time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)} }

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fastPINs lowers the scrypt cost so tests do not wait on it.
func fastPINs() Option {
	return func(s *Service) { s.pins.cost = 1 << 4 }
}

// env bundles a Service with the fakes around it.
type env struct {
	svc     *Service
	secrets *fakeSecrets
	events  *fakeEvents
	clock   *clock
	logs    *syncBuf
}

type syncBuf struct {
	mu sync.Mutex
	b  []byte
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b = append(s.b, p...)
	return len(p), nil
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.b)
}

func newEnv(t *testing.T, tunnels ...*fakeTunnel) *env {
	return newEnvWith(t, newSecrets(), tunnels)
}

func newEnvWith(t *testing.T, secrets *fakeSecrets, tunnels []*fakeTunnel, opts ...Option) *env {
	t.Helper()
	e := &env{secrets: secrets, events: &fakeEvents{}, clock: newClock(), logs: &syncBuf{}}
	log := slog.New(slog.NewTextHandler(e.logs, nil))
	all := append([]Option{WithClock(e.clock.now), fastPINs(), WithPollInterval(5 * time.Millisecond)}, opts...)
	e.svc = NewService(newRegistry(tunnels...), secrets, e.events, log, all...)
	e.svc.Load()
	t.Cleanup(func() { _ = e.svc.Disable(context.Background()) })
	return e
}

// withPIN returns an env whose PIN is already set.
func withPIN(t *testing.T, tunnels ...*fakeTunnel) *env {
	t.Helper()
	e := newEnv(t, tunnels...)
	if err := e.svc.SetPIN(testPIN); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) enable(t *testing.T, tn *fakeTunnel) {
	t.Helper()
	if err := e.svc.Enable(context.Background(), tn.id, localServer()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
}

func (e *env) login(t *testing.T, client string) app.RemoteSession {
	t.Helper()
	s, err := e.svc.Login(client, testPIN)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return s
}

func is(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want one wrapping %v", err, target)
	}
}
