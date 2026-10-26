package httpapi

import (
	"context"
	"errors"
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
	"eincast/internal/app/providerconfig"
	"eincast/internal/app/remote"
	"eincast/internal/domain/server"
)

// A person's path through the named tunnel, with only the cloudflared program
// itself replaced by a script: nothing configured, then each setting, then the
// PIN, then on, then a remote browser signs in, then off. Every piece in between
// is real: the setup store, the credential handling, the provider, the
// supervisor, the service and the HTTP layer.
func TestNamedTunnel_EndToEnd_FromNothingConfiguredToASignedInBrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	const token = "eyJhIjoiRU5ELVRPLUVORC1TRUNSRVQifQ"
	dir := t.TempDir()
	script := filepath.Join(dir, "cloudflared")
	body := `#!/bin/sh
dir="$(dirname "$0")"
echo "$@" > "$dir/args"
echo "$TUNNEL_TOKEN" > "$dir/env-token"
echo "2026-05-01T10:00:00Z INF Starting tunnel with token $TUNNEL_TOKEN" >&2
echo '2026-05-01T10:00:01Z INF Registered tunnel connection connIndex=0 location=sin01' >&2
exec sleep 60
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	secrets := &memSecrets{m: map[string]string{}}
	logs := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := providerconfig.New(secrets)
	reg, err := tunnel.NewRegistry(cloudflared.NewNamed(cfg, log))
	if err != nil {
		t.Fatal(err)
	}
	svc := remote.NewService(reg, secrets, discardEvents{}, log, remote.WithPollInterval(10*time.Millisecond))
	svc.Load()
	t.Cleanup(func() { _ = svc.Disable(context.Background()) })
	ctx := context.Background()
	id := cloudflared.NamedID

	// 1. Nothing is set up: it says what is missing and starts nothing.
	if st := svc.Status(); len(st.NeedsSetup) != 1 || st.NeedsSetup[0] != id {
		t.Fatalf("NeedsSetup = %v", st.NeedsSetup)
	}
	err = svc.Enable(ctx, id, loopbackServer{})
	var missing *app.NotConfiguredError
	if !errors.As(err, &missing) || strings.Join(missing.Missing, ",") != "Tunnel token,Hostname" {
		t.Fatalf("Enable before setup: %v", err)
	}

	// 2. Bad values are refused with advice; good ones are tidied and kept.
	if err := svc.SetProviderField(id, "hostname", "media.example.com:8080"); err == nil {
		t.Error("a hostname with a port was accepted")
	}
	if err := svc.SetProviderField(id, "hostname", "  HTTPS://Media.Example.com/ "); err != nil {
		t.Fatalf("hostname: %v", err)
	}
	if err := svc.SetProviderField(id, "token", token); err != nil {
		t.Fatalf("token: %v", err)
	}
	if err := svc.SetProviderField(id, "binary", script); err != nil {
		t.Fatalf("binary: %v", err)
	}
	if st := svc.Status(); len(st.NeedsSetup) != 0 {
		t.Fatalf("still needs setup: %v", st.NeedsSetup)
	}
	states, err := svc.ProviderConfig(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		switch s.Field.Key {
		case "hostname":
			if s.Value != "media.example.com" {
				t.Errorf("hostname saved as %q, want it tidied", s.Value)
			}
		case "token":
			if !s.Set || s.Value != "" {
				t.Errorf("token state %+v: set, without its value", s)
			}
		}
	}

	// 3. It is public, so the PIN comes next.
	if err := svc.Enable(ctx, id, loopbackServer{}); !errors.Is(err, app.ErrPINRequired) {
		t.Fatalf("Enable without a PIN: %v, want ErrPINRequired", err)
	}
	if err := svc.SetPIN(goodPIN); err != nil {
		t.Fatal(err)
	}

	// 4. On.
	if err := svc.Enable(ctx, id, loopbackServer{}); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if st := svc.Status(); st.State != server.Running || st.URL != "https://media.example.com" {
		t.Fatalf("status = %+v", st)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if strings.Contains(string(args), token) || strings.TrimSpace(string(args)) != "tunnel --no-autoupdate run" {
		t.Errorf("run as %q: the token must not be on the command line", args)
	}
	if env, _ := os.ReadFile(filepath.Join(dir, "env-token")); strings.TrimSpace(string(env)) != token {
		t.Errorf("cloudflared saw TUNNEL_TOKEN=%q", env)
	}

	// 5. Its settings are frozen while it is on.
	if err := svc.SetProviderField(id, "hostname", "other.example.com"); err == nil {
		t.Error("a setting changed while the tunnel was on")
	}

	// 6. A remote browser: turned away, signs in, gets the library.
	f := newFixtureFull(t, &fakeAccess{}, svc)
	at := func(r *http.Request) { viaProxy(r); r.Host = "media.example.com" }
	assertErrorShape(t, f.do("GET", "/api/library", at), 401, "unauthorized")
	login := f.postForm("/login", url.Values{"pin": {goodPIN}}, at)
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %d", login.StatusCode)
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
	if resp := f.do("GET", "/api/library", func(r *http.Request) { at(r); r.AddCookie(session) }); resp.StatusCode != 200 {
		t.Errorf("signed in: %d", resp.StatusCode)
	}

	// 7. The token appears nowhere it should not, even though cloudflared echoed it.
	if out := logs.String(); strings.Contains(out, token) {
		t.Errorf("the token is in the log:\n%s", out)
	}
	if st := svc.Status(); strings.Contains(st.Error, token) || strings.Contains(string(st.URL), token) {
		t.Errorf("the token is in the status: %+v", st)
	}

	// 8. Off: the settings can be changed again.
	if err := svc.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetProviderField(id, "hostname", "other.example.com"); err != nil {
		t.Errorf("after turning it off: %v", err)
	}
}
