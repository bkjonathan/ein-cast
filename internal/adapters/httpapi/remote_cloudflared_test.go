package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"eincast/internal/adapters/tunnel"
	"eincast/internal/adapters/tunnel/cloudflared"
	"eincast/internal/app"
	"eincast/internal/app/remote"
	"eincast/internal/domain/server"
)

// This test wires the real pieces together, with only the cloudflared program
// itself replaced by a script: the supervisor that runs it, the provider that
// reads its output, the remote service that watches the provider, and the HTTP
// layer that serves requests. It follows one quick tunnel through a restart that
// changes its address, which is the case no single unit sees.
func TestQuickTunnel_EndToEnd_AcrossARestartThatChangesTheAddress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "cloudflared")
	body := `#!/bin/sh
dir="$(dirname "$0")"
n=$(cat "$dir/runs" 2>/dev/null || echo 0)
echo $((n+1)) > "$dir/runs"
echo $$ > "$dir/pid$n"
if [ "$n" = 0 ]; then
  echo '2026-05-01T10:00:00Z INF |  https://first-run.trycloudflare.com  |' >&2
  sleep 0.4
  exit 1
fi
echo '2026-05-01T10:00:00Z INF |  https://second-run.trycloudflare.com  |' >&2
exec sleep 60
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	quick := cloudflared.NewQuickAt(script, slog.New(slog.DiscardHandler))
	reg, err := tunnel.NewRegistry(quick)
	if err != nil {
		t.Fatal(err)
	}
	svc := remote.NewService(reg, &memSecrets{m: map[string]string{}}, discardEvents{}, slog.New(slog.DiscardHandler),
		remote.WithPollInterval(10*time.Millisecond))
	if err := svc.SetPIN(goodPIN); err != nil {
		t.Fatal(err)
	}
	if err := svc.Enable(context.Background(), cloudflared.QuickID, loopbackServer{}); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	t.Cleanup(func() { _ = svc.Disable(context.Background()) })

	f := newFixtureFull(t, &fakeAccess{}, svc)
	at := func(host string) func(*http.Request) {
		return func(r *http.Request) { viaProxy(r); r.Host = host }
	}

	// The first address: strangers are turned away; the PIN signs in.
	const first, second = "first-run.trycloudflare.com", "second-run.trycloudflare.com"
	assertErrorShape(t, f.do("GET", "/api/library", at(first)), 401, "unauthorized")
	login := f.postForm("/login", url.Values{"pin": {goodPIN}}, at(first))
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: status = %d", login.StatusCode)
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
	signedIn := func(host string) func(*http.Request) {
		return func(r *http.Request) { at(host)(r); r.AddCookie(session) }
	}
	if resp := f.do("GET", "/api/library", signedIn(first)); resp.StatusCode != 200 {
		t.Fatalf("signed in on the first address: status = %d", resp.StatusCode)
	}

	// cloudflared crashes, is restarted, and has a new address.
	deadline := time.Now().Add(15 * time.Second)
	for svc.Status().URL != "https://"+second {
		if time.Now().After(deadline) {
			t.Fatalf("the service never followed the restart; status = %+v", svc.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}

	if resp := f.do("GET", "/api/health", signedIn(first)); resp.StatusCode != 403 {
		t.Errorf("the dead address: status = %d, want 403 from the Host check", resp.StatusCode)
	}
	if resp := f.do("GET", "/api/library", signedIn(second)); resp.StatusCode != 200 {
		t.Errorf("the new address with the same session: status = %d, want 200 (a restart is not a new sign-in)", resp.StatusCode)
	}
	assertErrorShape(t, f.do("GET", "/api/library", at(second)), 401, "unauthorized")
	if st := svc.Status(); st.State != server.Running || !st.Public {
		t.Errorf("status = %+v", st)
	}

	// Turning it off kills the process and ends the sign-in.
	if err := svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertProcessGone(t, filepath.Join(dir, "pid1"))
	if resp := f.do("GET", "/api/library", func(r *http.Request) { viaProxy(r); r.AddCookie(session) }); resp.StatusCode != 401 {
		t.Errorf("after Disable: status = %d, want 401", resp.StatusCode)
	}
}

func assertProcessGone(t *testing.T, pidfile string) {
	t.Helper()
	b, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatal(err)
	}
	pid := strings.TrimSpace(string(b))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := probeProcess(pid); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("process %s is still running after Disable", pid)
}

var _ app.Tunnel = (*cloudflared.Quick)(nil)
