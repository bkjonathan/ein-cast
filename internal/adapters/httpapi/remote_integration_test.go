package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"

	"eincast/internal/adapters/tunnel"
	"eincast/internal/app"
	"eincast/internal/app/remote"
)

// The tests above use a fake Remote to pin down what the HTTP layer does with
// each answer. This one wires in the real remote.Service, to prove the two agree
// on the credentials' wire format: the cookie a login sets, the link a playlist
// carries.

type stubTunnel struct{}

func (stubTunnel) ID() app.ProviderID { return "stub-public" }
func (stubTunnel) Public() bool       { return true }
func (stubTunnel) Start(context.Context, app.LocalServer) (app.PublicURL, error) {
	return app.PublicURL("https://" + publicHost), nil
}
func (stubTunnel) Stop(context.Context) error { return nil }
func (stubTunnel) Status() app.TunnelStatus   { return app.TunnelStatus{} }

// loopbackServer is an app.LocalServer for tests that never bind anything.
type loopbackServer struct{}

func (loopbackServer) Addrs() []netip.AddrPort {
	return []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:8765")}
}
func (loopbackServer) Bind(netip.Addr) (func(), error) { return func() {}, nil }

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
func (s *memSecrets) Set(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}
func (s *memSecrets) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, k)
	return nil
}

type discardEvents struct{}

func (discardEvents) Publish(app.Event) {}

func TestRemoteAccess_EndToEndWithTheRealService(t *testing.T) {
	reg, err := tunnel.NewRegistry(stubTunnel{})
	if err != nil {
		t.Fatal(err)
	}
	svc := remote.NewService(reg, &memSecrets{m: map[string]string{}}, discardEvents{}, slog.New(slog.DiscardHandler))
	if err := svc.SetPIN(goodPIN); err != nil {
		t.Fatal(err)
	}
	if err := svc.Enable(context.Background(), "stub-public", loopbackServer{}); err != nil {
		t.Fatal(err)
	}
	f := newFixtureFull(t, &fakeAccess{}, svc)
	atTunnel := func(r *http.Request) { viaProxy(r); r.Host = publicHost }

	// A stranger gets nowhere.
	assertErrorShape(t, f.do("GET", "/api/library", atTunnel), 401, "unauthorized")

	// A wrong PIN is refused; the right one signs in.
	if resp := f.postForm("/login", url.Values{"pin": {"not the pin"}}, atTunnel); resp.StatusCode != 401 {
		t.Fatalf("wrong PIN: status = %d", resp.StatusCode)
	}
	login := f.postForm("/login", url.Values{"pin": {goodPIN}}, atTunnel)
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("right PIN: status = %d", login.StatusCode)
	}
	var session *http.Cookie
	for _, c := range login.Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session cookie")
	}
	signedIn := func(r *http.Request) { atTunnel(r); r.AddCookie(session) }

	if resp := f.do("GET", "/api/library", signedIn); resp.StatusCode != 200 {
		t.Fatalf("with the session: status = %d, want 200", resp.StatusCode)
	}

	// A player that cannot hold cookies plays from the playlist's own links.
	resp := f.do("GET", "/playlist.m3u", func(r *http.Request) { signedIn(r); r.Header.Set("X-Forwarded-Proto", "https") })
	var link string
	for _, line := range strings.Split(string(readBody(t, resp)), "\n") {
		if strings.HasPrefix(line, "https://"+publicHost+"/stream/"+string(f.movie.ID)) {
			link = strings.TrimPrefix(line, "https://"+publicHost)
		}
	}
	if link == "" || !strings.Contains(link, "sig=") || !strings.Contains(link, "exp=") {
		t.Fatalf("playlist has no signed link for the film: %q", link)
	}
	played := f.do("GET", link, atTunnel) // no cookie
	if played.StatusCode != 200 || string(readBody(t, played)) != string(f.movieBy) {
		t.Fatalf("playing the signed link: status = %d", played.StatusCode)
	}
	if resp := f.do("GET", link+"0", atTunnel); resp.StatusCode != 401 {
		t.Errorf("tampered link: status = %d, want 401", resp.StatusCode)
	}

	// Rebinding: a name that is neither local nor the tunnel is refused.
	if resp := f.do("GET", "/api/health", func(r *http.Request) { signedIn(r); r.Host = "evil.example" }); resp.StatusCode != 403 {
		t.Errorf("foreign Host: status = %d, want 403", resp.StatusCode)
	}

	// Turning access off ends everything at once. Two layers refuse: the Host
	// check no longer knows the tunnel's name, and, for a request that arrives
	// by the local Host instead, the credentials themselves are dead.
	if err := svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	local := func(r *http.Request) { viaProxy(r) } // httptest's own 127.0.0.1 Host
	cases := []struct {
		name   string
		path   string
		mod    func(*http.Request)
		status int
	}{
		{"session, tunnel Host", "/api/library", signedIn, 403},
		{"signed link, tunnel Host", link, atTunnel, 403},
		{"session, local Host", "/api/library", func(r *http.Request) { local(r); r.AddCookie(session) }, 401},
		{"signed link, local Host", link, local, 401},
	}
	for _, c := range cases {
		if resp := f.do("GET", c.path, c.mod); resp.StatusCode != c.status {
			t.Errorf("%s after Disable: status = %d, want %d", c.name, resp.StatusCode, c.status)
		}
	}
}

func TestRemoteAccess_LockoutReachesTheClient(t *testing.T) {
	reg, _ := tunnel.NewRegistry(stubTunnel{})
	svc := remote.NewService(reg, &memSecrets{m: map[string]string{}}, discardEvents{}, slog.New(slog.DiscardHandler))
	if err := svc.SetPIN(goodPIN); err != nil {
		t.Fatal(err)
	}
	if err := svc.Enable(context.Background(), "stub-public", loopbackServer{}); err != nil {
		t.Fatal(err)
	}
	f := newFixtureFull(t, &fakeAccess{}, svc)

	var last *http.Response
	for i := range 8 {
		last = f.postForm("/login", url.Values{"pin": {fmt.Sprintf("guess number %d", i)}}, func(r *http.Request) { r.Host = publicHost })
		if last.StatusCode == http.StatusTooManyRequests {
			break
		}
	}
	if last.StatusCode != http.StatusTooManyRequests || last.Header.Get("Retry-After") == "" {
		t.Fatalf("status %d, Retry-After %q; want 429 with a wait time", last.StatusCode, last.Header.Get("Retry-After"))
	}

	// Even the right PIN is refused while locked out.
	if resp := f.postForm("/login", url.Values{"pin": {goodPIN}}, func(r *http.Request) { r.Host = publicHost }); resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("right PIN while locked out: status = %d, want 429", resp.StatusCode)
	}
}
