package tailscale

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/server"
)

type fakeRunner struct {
	out []byte
	err error
}

func (f fakeRunner) Status(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.out, f.err
}

// fakeServer records what the provider asked of the server.
type fakeServer struct {
	addrs   []netip.AddrPort
	bindErr error

	mu      sync.Mutex
	bound   []netip.Addr
	unbinds int
}

func newFakeServer() *fakeServer {
	return &fakeServer{addrs: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:8765")}}
}

func (f *fakeServer) Addrs() []netip.AddrPort { return f.addrs }

func (f *fakeServer) Bind(a netip.Addr) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bindErr != nil {
		return nil, f.bindErr
	}
	f.bound = append(f.bound, a)
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.unbinds++
	}, nil
}

func (f *fakeServer) counts() (binds, unbinds int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bound), f.unbinds
}

func newProvider(out string, err error) *Provider {
	return newWith(fakeRunner{out: []byte(out), err: err}, slog.New(slog.DiscardHandler))
}

func TestProvider_IsPrivate(t *testing.T) {
	p := newProvider(running, nil)
	if p.Public() {
		t.Error("a tailnet is reachable only by the user's own devices; calling it public would demand a PIN for nothing")
	}
	if p.ID() != "tailscale" {
		t.Errorf("ID = %q", p.ID())
	}
}

func TestStart_ListensOnTheTailnetAddressAndReportsTheURL(t *testing.T) {
	p := newProvider(running, nil)
	srv := newFakeServer()

	url, err := p.Start(context.Background(), srv)

	if err != nil {
		t.Fatal(err)
	}
	if url != "http://den.tail1234.ts.net:8765" {
		t.Errorf("url = %q", url)
	}
	if len(srv.bound) != 1 || srv.bound[0] != netip.MustParseAddr("100.101.102.103") {
		t.Errorf("bound %v, want only the IPv4 tailnet address", srv.bound)
	}
	if st := p.Status(); st.State != server.Running || st.URL != url {
		t.Errorf("status = %+v", st)
	}
}

func TestStart_URLsForEachAddressForm(t *testing.T) {
	v6Only := `{"BackendState":"Running","Self":{"TailscaleIPs":["fd7a:115c:a1e0::9"]}}`
	noDNS := `{"BackendState":"Running","Self":{"DNSName":"den.t.ts.net.","TailscaleIPs":["100.64.0.7"]},"CurrentTailnet":{"MagicDNSEnabled":false}}`
	cases := map[string]struct{ json, want string }{
		"IPv4 without MagicDNS": {noDNS, "http://100.64.0.7:8765"},
		"IPv6 is bracketed":     {v6Only, "http://[fd7a:115c:a1e0::9]:8765"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			url, err := newProvider(c.json, nil).Start(context.Background(), newFakeServer())
			if err != nil || string(url) != c.want {
				t.Errorf("url = %q, err = %v; want %q", url, err, c.want)
			}
		})
	}
}

func TestStart_UsesTheServersPort(t *testing.T) {
	srv := newFakeServer()
	srv.addrs = []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:54321"), netip.MustParseAddrPort("192.168.1.9:54321")}
	url, err := newProvider(running, nil).Start(context.Background(), srv)
	if err != nil || !strings.HasSuffix(string(url), ":54321") {
		t.Errorf("url = %q, err = %v", url, err)
	}
}

func TestStart_FailuresListenOnNothingAndSayWhy(t *testing.T) {
	cases := []struct {
		name    string
		p       *Provider
		srv     func() *fakeServer
		wantErr string
	}{
		{"not installed", newProvider("", ErrNotInstalled), newFakeServer, "was not found"},
		{"Tailscale stopped", newProvider(`{"BackendState":"Stopped"}`, nil), newFakeServer, "turned off"},
		{"unreadable output", newProvider("garbage", nil), newFakeServer, "could not read"},
		{"server has no address", newProvider(running, nil), func() *fakeServer { s := newFakeServer(); s.addrs = nil; return s }, "no address"},
		{"the server refuses the address", newProvider(running, nil), func() *fakeServer { s := newFakeServer(); s.bindErr = errors.New("address in use"); return s }, "address in use"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := c.srv()
			_, err := c.p.Start(context.Background(), srv)

			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want one mentioning %q", err, c.wantErr)
			}
			if binds, _ := srv.counts(); binds != 0 {
				t.Error("the server was told to listen although Start failed")
			}
			if st := c.p.Status(); st.State != server.Failed || !strings.Contains(st.Error, c.wantErr) {
				t.Errorf("status = %+v, want Failed with the reason", st)
			}
		})
	}
}

func TestStart_CanBeRetriedAfterAFailure(t *testing.T) {
	srv := newFakeServer()
	p := newProvider(`{"BackendState":"Stopped"}`, nil)
	if _, err := p.Start(context.Background(), srv); err == nil {
		t.Fatal("expected a failure")
	}
	p.run = fakeRunner{out: []byte(running)}
	if _, err := p.Start(context.Background(), srv); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestStart_TwiceIsRefusedAndListensOnce(t *testing.T) {
	p := newProvider(running, nil)
	srv := newFakeServer()
	if _, err := p.Start(context.Background(), srv); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Start(context.Background(), srv); err == nil {
		t.Error("second Start succeeded")
	}
	if binds, _ := srv.counts(); binds != 1 {
		t.Errorf("binds = %d, want 1", binds)
	}
}

func TestStart_RespectsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := newFakeServer()
	if _, err := newProvider(running, nil).Start(ctx, srv); err == nil {
		t.Error("Start ignored a cancelled context")
	}
	if binds, _ := srv.counts(); binds != 0 {
		t.Error("listened although the context was already cancelled")
	}
}

func TestStop_StopsListeningOnce(t *testing.T) {
	p := newProvider(running, nil)
	srv := newFakeServer()
	if _, err := p.Start(context.Background(), srv); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if err := p.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	if _, unbinds := srv.counts(); unbinds != 1 {
		t.Errorf("unbinds = %d, want exactly 1", unbinds)
	}
	if st := p.Status(); st.State != server.Idle || st.URL != "" {
		t.Errorf("status = %+v, want off", st)
	}
}

func TestStop_BeforeStartIsFine(t *testing.T) {
	if err := newProvider(running, nil).Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCancellingTheLifetimeStopsListening(t *testing.T) {
	p := newProvider(running, nil)
	srv := newFakeServer()
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := p.Start(ctx, srv); err != nil {
		t.Fatal(err)
	}

	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, unbinds := srv.counts(); unbinds == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("still listening after the context ended")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, unbinds := srv.counts(); unbinds != 1 {
		t.Errorf("unbinds = %d after cancel then Stop, want 1", unbinds)
	}
}

func TestInstallLocations(t *testing.T) {
	if got := installLocations("darwin"); len(got) == 0 || got[0] != "/Applications/Tailscale.app/Contents/MacOS/Tailscale" {
		t.Errorf("darwin: %v", got)
	}
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramFiles(x86)", "")
	got := installLocations("windows")
	if len(got) != 1 || filepath.Base(got[0]) != "tailscale.exe" {
		t.Errorf("windows: %v", got)
	}
	if got := installLocations("plan9"); got != nil {
		t.Errorf("unsupported OS: %v", got)
	}
}

// The remaining tests run a real child process: a shell script standing in for
// the Tailscale tool, to exercise the exec path without Tailscale installed.

func fakeCLI(t *testing.T, script string) cliRunner {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	path := filepath.Join(t.TempDir(), "tailscale")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return cliRunner{locate: func() (string, error) { return path, nil }}
}

func TestCLIRunner_PassesTheArgumentsAndReturnsStdout(t *testing.T) {
	c := fakeCLI(t, `[ "$1 $2" = "status --json" ] && echo '{"BackendState":"Running"}' || { echo "unexpected: $*" >&2; exit 2; }`)
	out, err := c.Status(context.Background())
	if err != nil || !strings.Contains(string(out), `"Running"`) {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestCLIRunner_FailureCarriesTailscalesOwnMessage(t *testing.T) {
	c := fakeCLI(t, `echo "failed to connect to local tailscaled; it doesn't appear to be running" >&2; exit 1`)
	_, err := c.Status(context.Background())
	if err == nil || !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Errorf("error = %v, want Tailscale's explanation", err)
	}
}

func TestCLIRunner_NotInstalled(t *testing.T) {
	c := cliRunner{locate: func() (string, error) { return "", ErrNotInstalled }}
	if _, err := c.Status(context.Background()); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("error = %v", err)
	}
}

func TestCLIRunner_GivesUpWhenContextEnds(t *testing.T) {
	c := fakeCLI(t, `sleep 30`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Status(ctx); err == nil {
		t.Fatal("expected an error")
	}
	if time.Since(start) > 5*time.Second {
		t.Error("a hung Tailscale held Start for far longer than its deadline")
	}
}

func TestStart_WithTheRealRunnerAgainstAScript(t *testing.T) {
	c := fakeCLI(t, "cat <<'EOF'\n"+running+"\nEOF\n")
	p := newWith(c, slog.New(slog.DiscardHandler))
	url, err := p.Start(context.Background(), newFakeServer())
	if err != nil || url != "http://den.tail1234.ts.net:8765" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	_ = app.PublicURL(url)
}
