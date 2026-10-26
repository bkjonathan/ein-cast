package cloudflared

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/adapters/tunnel/procsup"
	"eincast/internal/app"
	"eincast/internal/app/providerconfig"
	"eincast/internal/domain/server"
)

const (
	testToken = "eyJhIjoiU0VDUkVULVRPS0VOIiwidCI6IjEyMyJ9"
	testHost  = "media.example.com"
)

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func newSecrets() *memSecrets { return &memSecrets{m: map[string]string{}} }

func (s *memSecrets) Get(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[k]
	if !ok {
		return "", app.ErrSecretNotFound
	}
	return v, nil
}
func (s *memSecrets) Set(k, v string) error { s.mu.Lock(); defer s.mu.Unlock(); s.m[k] = v; return nil }
func (s *memSecrets) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, k)
	return nil
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// newTestNamed returns a Named with its token and hostname saved, a stub
// cloudflared, a fast supervisor, and a log we can search.
func newTestNamed(t *testing.T, script string, tune func(*procsup.Spec)) (n *Named, dir string, logs *syncBuf, cfg *providerconfig.Store) {
	t.Helper()
	locate, dir := stub(t, script)
	cfg = providerconfig.New(newSecrets())
	for _, saved := range []struct {
		field app.ConfigField
		value string
	}{{tokenField(), testToken}, {hostnameField(), testHost}} {
		if err := cfg.Set(NamedID, saved.field, saved.value); err != nil {
			t.Fatal(err)
		}
	}
	logs = &syncBuf{}
	n = NewNamed(cfg, slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	n.locate = locate
	n.tune = func(s *procsup.Spec) {
		s.InitialBackoff, s.MaxBackoff = 5*time.Millisecond, 20*time.Millisecond
		s.GracePeriod = 2 * time.Second
		if tune != nil {
			tune(s)
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = n.Stop(ctx)
	})
	return n, dir, logs, cfg
}

func TestNamed_IsPublicAndAsksForWhatItNeeds(t *testing.T) {
	n := NewNamed(providerconfig.New(newSecrets()), slog.New(slog.DiscardHandler))
	if !n.Public() || n.ID() != "cloudflare-named" {
		t.Errorf("public=%v id=%q: it is open to the whole Internet and must demand a PIN", n.Public(), n.ID())
	}
	fields := n.Fields()
	if len(fields) != 3 {
		t.Fatalf("fields = %+v", fields)
	}
	byKey := map[string]app.ConfigField{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	if f := byKey["token"]; f.Kind != app.FieldSecret || !f.Required {
		t.Errorf("token = %+v: it must be a required secret", f)
	}
	if f := byKey["hostname"]; f.Kind != app.FieldText || !f.Required {
		t.Errorf("hostname = %+v", f)
	}
	if f := byKey["binary"]; f.Kind != app.FieldFile || f.Required || f.Shared != "cloudflared" {
		t.Errorf("binary = %+v: optional, and shared with the quick tunnel", f)
	}
}

func TestNamed_TheTokenGoesInTheEnvironmentNeverTheCommandLine(t *testing.T) {
	n, dir, _, _ := newTestNamed(t, `
echo "$@" > "$(dirname "$0")/args"
echo "$TUNNEL_TOKEN" > "$(dirname "$0")/env-token"
echo '2026-05-01T10:00:00Z DBG Registering tunnel connection connIndex=0' >&2
echo '2026-05-01T10:00:01Z INF Registered tunnel connection connIndex=0 connection=abc location=sin01 protocol=quic' >&2
exec sleep 60
`, nil)

	url, err := n.Start(context.Background(), newServer())

	if err != nil {
		t.Fatal(err)
	}
	if url != "https://media.example.com" {
		t.Errorf("url = %q: it should be the hostname the person gave", url)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if got := strings.TrimSpace(string(args)); got != "tunnel --no-autoupdate run" {
		t.Errorf("run as %q", got)
	}
	if strings.Contains(string(args), testToken) {
		t.Error("the token is on the command line, where any user of the machine can read it")
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env-token"))
	if strings.TrimSpace(string(env)) != testToken {
		t.Errorf("cloudflared saw TUNNEL_TOKEN=%q, want the saved token", env)
	}
	if st := n.Status(); st.State != server.Running || st.URL != url {
		t.Errorf("status = %+v", st)
	}
}

func TestNamed_IsNotUpUntilACloudflareConnectionIsRegistered(t *testing.T) {
	// The attempt (Debug) and noise do not count; the tunnel serves nothing yet.
	n, _, _, _ := newTestNamed(t, `
echo '2026-05-01T10:00:00Z DBG Registering tunnel connection connIndex=0' >&2
echo '2026-05-01T10:00:00Z INF Starting tunnel tunnelID=abc' >&2
exec sleep 60
`, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	_, err := n.Start(ctx, newServer())

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v: Start must wait for a registered connection", err)
	}
}

func TestNamed_TheTokenNeverReachesTheLogTheStatusOrAnErrorEvenIfCloudflaredEchoesIt(t *testing.T) {
	n, _, logs, _ := newTestNamed(t, `
echo "2026-05-01T10:00:00Z INF using token $TUNNEL_TOKEN" >&2
echo "2026-05-01T10:00:00Z ERR failed to authenticate with $TUNNEL_TOKEN" >&2
exit 1
`, func(s *procsup.Spec) { s.MaxRestarts = -1 })

	_, err := n.Start(context.Background(), newServer())

	if err == nil {
		t.Fatal("expected a failure")
	}
	for name, text := range map[string]string{
		"error":  err.Error(),
		"status": fmt.Sprintf("%+v", n.Status()),
		"log":    logs.String(),
	} {
		if strings.Contains(text, testToken) {
			t.Errorf("the token appears in the %s:\n%s", name, text)
		}
	}
	if !strings.Contains(err.Error(), "failed to authenticate") {
		t.Errorf("error = %v: the reason should still be shown", err)
	}
}

func TestNamed_WithoutItsSettingsItDoesNotStart(t *testing.T) {
	locate, dir := stub(t, `echo ran > "$(dirname "$0")/ran"; exec sleep 60`)
	n := NewNamed(providerconfig.New(newSecrets()), slog.New(slog.DiscardHandler))
	n.locate = locate

	_, err := n.Start(context.Background(), newServer())

	if !errors.Is(err, app.ErrNotConfigured) {
		t.Fatalf("error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, statErr := os.Stat(filepath.Join(dir, "ran")); statErr == nil {
		t.Error("cloudflared was run without a token")
	}
	if st := n.Status(); st.State != server.Failed {
		t.Errorf("status = %+v", st)
	}
}

func TestNamed_AfterARestartTheSameAddressComesBack(t *testing.T) {
	n, _, _, _ := newTestNamed(t, `
dir="$(dirname "$0")"
c=$(cat "$dir/runs" 2>/dev/null || echo 0)
echo $((c+1)) > "$dir/runs"
echo '2026-05-01T10:00:01Z INF Registered tunnel connection connIndex=0' >&2
echo '2026-05-01T10:00:01Z INF Registered tunnel connection connIndex=1' >&2
if [ "$c" = 0 ]; then sleep 0.1; exit 1; fi
exec sleep 60
`, nil)

	url, err := n.Start(context.Background(), newServer())
	if err != nil {
		t.Fatal(err)
	}

	// The second run is up again, at the same address: a named tunnel's is stable.
	deadline := time.Now().Add(5 * time.Second)
	for {
		runs, _ := os.ReadFile(filepath.Join(filepath.Dir(mustLocate(t, n)), "runs"))
		if strings.TrimSpace(string(runs)) == "2" && n.Status().State == server.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never came back up; status = %+v", n.Status())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if st := n.Status(); st.URL != url {
		t.Errorf("address after restart = %q, want %q", st.URL, url)
	}
}

func mustLocate(t *testing.T, n *Named) string {
	t.Helper()
	p, err := n.locate()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCleanHostname(t *testing.T) {
	good := map[string]string{
		"media.example.com":             "media.example.com",
		"  Media.Example.COM \n":        "media.example.com",
		"https://media.example.com":     "media.example.com",
		"HTTPS://media.example.com/":    "media.example.com",
		"http://media.example.com/path": "media.example.com",
		"media.example.com.":            "media.example.com",
		"a-b.c-d.example.co.uk":         "a-b.c-d.example.co.uk",
		"x1.example.com?x=1":            "x1.example.com",
	}
	for in, want := range good {
		if got, err := cleanHostname(in); err != nil || got != want {
			t.Errorf("cleanHostname(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "   ", "https://", "localhost", "media", "media.example.com:8080", "user@media.example.com",
		"-media.example.com", "media-.example.com", "media..example.com", ".example.com", "med_ia.example.com",
		"media.exam ple.com", "203.0.113.7", "media.example.123", "例え.jp", strings.Repeat("a", 64) + ".example.com",
		strings.Repeat("a.", 130) + "com",
	}
	for _, in := range bad {
		if got, err := cleanHostname(in); err == nil {
			t.Errorf("cleanHostname(%q) accepted as %q", in, got)
		}
	}
}

func TestCleanBinaryPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix permissions")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "cloudflared")
	plain := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plain, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, err := cleanBinaryPath(exe); err != nil || got != exe {
		t.Errorf("a good path: %q, %v", got, err)
	}
	if got, err := cleanBinaryPath(dir + "/./sub/../cloudflared"); err != nil || got != exe {
		t.Errorf("an untidy path: %q, %v; want it cleaned to %q", got, err, exe)
	}
	for name, in := range map[string]string{
		"relative":       "cloudflared",
		"relative dots":  "../cloudflared",
		"missing":        filepath.Join(dir, "nope"),
		"a folder":       dir,
		"not executable": plain,
	} {
		if got, err := cleanBinaryPath(in); err == nil {
			t.Errorf("%s: accepted as %q", name, got)
		}
	}
}

func TestLocator_PrefersTheSavedLocationAndNeverQuietlySubstitutesAnother(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix permissions")
	}
	dir := t.TempDir()
	saved := filepath.Join(dir, "my-cloudflared")
	onPath := filepath.Join(t.TempDir(), "cloudflared")
	for _, p := range []string{saved, onPath} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", filepath.Dir(onPath))
	cfg := providerconfig.New(newSecrets())
	find := locator(cfg, NamedID)

	if got, err := find(); err != nil || got != onPath {
		t.Fatalf("nothing saved: %q, %v; want the one on PATH", got, err)
	}

	if err := cfg.Set(NamedID, binaryField(), saved); err != nil {
		t.Fatal(err)
	}
	if got, err := find(); err != nil || got != saved {
		t.Errorf("saved: %q, %v; want the chosen one even though another is on PATH", got, err)
	}

	if err := os.Remove(saved); err != nil {
		t.Fatal(err)
	}
	if got, err := find(); err == nil || !strings.Contains(err.Error(), "no longer works") {
		t.Errorf("saved but gone: %q, %v; want an error, not a silent fall back to PATH", got, err)
	}
}

func TestLocator_TheLocationIsSharedBetweenBothTunnels(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix permissions")
	}
	p := filepath.Join(t.TempDir(), "cloudflared")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // nothing there
	cfg := providerconfig.New(newSecrets())
	if err := cfg.Set(QuickID, binaryField(), p); err != nil {
		t.Fatal(err)
	}

	if got, err := locator(cfg, NamedID)(); err != nil || got != p {
		t.Errorf("named: %q, %v: set once for the quick tunnel, it should apply to both", got, err)
	}
}

func TestLocator_NothingSavedAndNothingInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if _, err := os.Stat("/opt/homebrew/bin/cloudflared"); err == nil {
			t.Skip("cloudflared is installed in a standard place on this machine")
		}
	}
	if _, err := locator(providerconfig.New(newSecrets()), QuickID)(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("error = %v, want ErrNotInstalled", err)
	}
}

func TestQuick_OffersOnlyTheLocationSetting(t *testing.T) {
	f := NewQuick(providerconfig.New(newSecrets()), slog.New(slog.DiscardHandler)).Fields()
	if len(f) != 1 || f[0].Key != "binary" || f[0].Required {
		t.Errorf("fields = %+v: a quick tunnel needs nothing from the person", f)
	}
}
