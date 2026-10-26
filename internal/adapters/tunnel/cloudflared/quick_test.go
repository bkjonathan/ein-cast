package cloudflared

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"eincast/internal/adapters/tunnel/procsup"
	"eincast/internal/app"
	"eincast/internal/domain/server"
)

type fakeServer struct{ addrs []netip.AddrPort }

func newServer() fakeServer {
	return fakeServer{addrs: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:8765"), netip.MustParseAddrPort("192.168.1.9:8765")}}
}

func (f fakeServer) Addrs() []netip.AddrPort       { return f.addrs }
func (fakeServer) Bind(netip.Addr) (func(), error) { return func() {}, nil }

// stub writes a shell script standing in for cloudflared. The tests are about
// how the provider treats a process and its output, which a script reproduces
// faithfully; they do not need the real tool.
func stub(t *testing.T, body string) (locate func() (string, error), dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir = t.TempDir()
	path := filepath.Join(dir, "cloudflared")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return func() (string, error) { return path, nil }, dir
}

func newTestQuick(locate func() (string, error), tune func(*procsup.Spec)) *Quick {
	q := newQuick(locate, slog.New(slog.DiscardHandler))
	q.tune = func(s *procsup.Spec) {
		s.InitialBackoff, s.MaxBackoff = 5*time.Millisecond, 20*time.Millisecond
		s.GracePeriod = 2 * time.Second
		if tune != nil {
			tune(s)
		}
	}
	return q
}

func cleanup(t *testing.T, q *Quick) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = q.Stop(ctx)
	})
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestQuick_IsPublic(t *testing.T) {
	q := newQuick(locateCLI, slog.New(slog.DiscardHandler))
	if !q.Public() || q.ID() != "cloudflare-quick" {
		t.Errorf("public=%v id=%q: a quick tunnel is open to the whole Internet and must demand a PIN", q.Public(), q.ID())
	}
}

func TestQuick_StartReturnsTheAddressAndRunsTheRightCommand(t *testing.T) {
	locate, dir := stub(t, `
echo "$@" > "`+"$(dirname \"$0\")"+`/args"
echo '2026-05-01T10:00:00Z INF Thank you for trying Cloudflare Tunnel.' >&2
echo '2026-05-01T10:00:00Z ERR failed to request quick Tunnel: Post "https://api.trycloudflare.com/tunnel": EOF' >&2
echo '2026-05-01T10:00:01Z INF |  https://funny-lemon-tree.trycloudflare.com  |' >&2
exec sleep 60
`)
	q := newTestQuick(locate, nil)
	cleanup(t, q)

	url, err := q.Start(context.Background(), newServer())

	if err != nil {
		t.Fatal(err)
	}
	if url != "https://funny-lemon-tree.trycloudflare.com" {
		t.Errorf("url = %q: the API address in the error line must not be taken for the tunnel's", url)
	}
	if st := q.Status(); st.State != server.Running || st.URL != url {
		t.Errorf("status = %+v", st)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if got := strings.TrimSpace(string(args)); got != "tunnel --no-autoupdate --url http://127.0.0.1:8765" {
		t.Errorf("cloudflared was run as %q: it must forward to the loopback address, never a LAN one", got)
	}
}

func TestQuick_OnlyTheFirstAddressOfARunCounts(t *testing.T) {
	// After the banner, cloudflared may log things a stranger sent. Nothing they
	// can make it print may change the address we report.
	locate, _ := stub(t, `
echo '2026-05-01T10:00:01Z INF |  https://real-one.trycloudflare.com  |' >&2
echo '2026-05-01T10:00:02Z ERR request failed uri=https://attacker-chosen.trycloudflare.com/x' >&2
exec sleep 60
`)
	q := newTestQuick(locate, nil)
	cleanup(t, q)

	url, err := q.Start(context.Background(), newServer())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond) // let the second line arrive

	if st := q.Status(); st.URL != url || url != "https://real-one.trycloudflare.com" {
		t.Errorf("url=%q status=%+v: a later line changed the address", url, st)
	}
}

func TestQuick_StopKillsCloudflared(t *testing.T) {
	locate, dir := stub(t, `
echo $$ > "$(dirname "$0")/pid"
echo 'INF |  https://abc.trycloudflare.com  |' >&2
exec sleep 60
`)
	q := newTestQuick(locate, nil)
	if _, err := q.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}

	if err := q.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	assertGone(t, filepath.Join(dir, "pid"))
	if st := q.Status(); st.State != server.Idle || st.URL != "" {
		t.Errorf("status = %+v, want off", st)
	}
	if err := q.Stop(context.Background()); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestQuick_EndingTheLifetimeKillsCloudflared(t *testing.T) {
	locate, dir := stub(t, `
echo $$ > "$(dirname "$0")/pid"
echo 'INF |  https://abc.trycloudflare.com  |' >&2
exec sleep 60
`)
	q := newTestQuick(locate, nil)
	cleanup(t, q)
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := q.Start(ctx, newServer()); err != nil {
		t.Fatal(err)
	}

	cancel()

	eventually(t, "cloudflared to be stopped", func() bool { return q.Status().State == server.Idle })
	assertGone(t, filepath.Join(dir, "pid"))
}

func assertGone(t *testing.T, pidfile string) {
	t.Helper()
	b, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	proc, _ := os.FindProcess(pid)
	eventually(t, "process "+strconv.Itoa(pid)+" to be gone", func() bool {
		return proc.Signal(syscall.Signal(0)) != nil
	})
}

func TestQuick_ARestartedCloudflaredGetsANewAddress(t *testing.T) {
	// The first run reports an address and dies; the second reports another.
	locate, _ := stub(t, `
dir="$(dirname "$0")"
n=$(cat "$dir/runs" 2>/dev/null || echo 0)
echo $((n+1)) > "$dir/runs"
if [ "$n" = 0 ]; then
  echo 'INF |  https://first-address.trycloudflare.com  |' >&2
  sleep 0.1
  exit 1
fi
echo 'INF |  https://second-address.trycloudflare.com  |' >&2
exec sleep 60
`)
	q := newTestQuick(locate, nil)
	cleanup(t, q)

	first, err := q.Start(context.Background(), newServer())
	if err != nil {
		t.Fatal(err)
	}
	if first != "https://first-address.trycloudflare.com" {
		t.Fatalf("first = %q", first)
	}

	eventually(t, "the new address", func() bool { return q.Status().URL == "https://second-address.trycloudflare.com" })
	if q.Status().State != server.Running {
		t.Errorf("status = %+v, want Running with the new address", q.Status())
	}
}

func TestQuick_ShowsReconnectingWhileItRestarts(t *testing.T) {
	locate, _ := stub(t, `
echo 'INF |  https://abc.trycloudflare.com  |' >&2
sleep 0.05
exit 1
`)
	q := newTestQuick(locate, func(s *procsup.Spec) {
		s.InitialBackoff, s.MaxBackoff = 400*time.Millisecond, 400*time.Millisecond
	})
	cleanup(t, q)
	if _, err := q.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}

	eventually(t, "a Starting status between runs", func() bool {
		st := q.Status()
		return st.State == server.Starting && st.URL == ""
	})
}

func TestQuick_ReportsFailureWhenItGivesUp(t *testing.T) {
	locate, _ := stub(t, `
echo 'INF |  https://abc.trycloudflare.com  |' >&2
echo '2026-05-01T10:00:09Z ERR connection refused by edge' >&2
exit 1
`)
	q := newTestQuick(locate, func(s *procsup.Spec) { s.MaxRestarts = 1 })
	cleanup(t, q)
	if _, err := q.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}

	eventually(t, "Failed", func() bool { return q.Status().State == server.Failed })
	if msg := q.Status().Error; !strings.Contains(msg, "could not be restarted") || !strings.Contains(msg, "connection refused by edge") {
		t.Errorf("error = %q, want it to say what cloudflared last reported", msg)
	}
}

func TestQuick_StartFailsWhenItNeverComesUp(t *testing.T) {
	locate, _ := stub(t, `
echo '2026-05-01T10:00:00Z ERR failed to request quick Tunnel: dial tcp: lookup api.trycloudflare.com: no such host' >&2
exit 1
`)
	q := newTestQuick(locate, func(s *procsup.Spec) { s.MaxRestarts = -1 })
	cleanup(t, q)

	_, err := q.Start(context.Background(), newServer())

	if err == nil || !strings.Contains(err.Error(), "no such host") || !strings.Contains(err.Error(), "before the tunnel came up") {
		t.Fatalf("error = %v, want the reason cloudflared gave", err)
	}
	if st := q.Status(); st.State != server.Failed || st.Error == "" {
		t.Errorf("status = %+v", st)
	}
}

func TestQuick_CancellingWhileConnectingStopsIt(t *testing.T) {
	locate, dir := stub(t, `
echo $$ > "$(dirname "$0")/pid"
exec sleep 60
`)
	q := newTestQuick(locate, nil)
	cleanup(t, q)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := q.Start(ctx, newServer()); done <- err }()
	eventually(t, "the process to start", func() bool { _, err := os.Stat(filepath.Join(dir, "pid")); return err == nil })

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancellation")
	}
	assertGone(t, filepath.Join(dir, "pid"))
}

func TestQuick_NotInstalled(t *testing.T) {
	q := newQuick(func() (string, error) { return "", ErrNotInstalled }, slog.New(slog.DiscardHandler))

	_, err := q.Start(context.Background(), newServer())

	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "github.com/cloudflare/cloudflared") {
		t.Errorf("the message should say where to get it: %v", err)
	}
	if st := q.Status(); st.State != server.Failed {
		t.Errorf("status = %+v", st)
	}
}

func TestQuick_ServerWithoutALoopbackAddress(t *testing.T) {
	locate, _ := stub(t, "exec sleep 60\n")
	q := newTestQuick(locate, nil)
	srv := fakeServer{addrs: []netip.AddrPort{netip.MustParseAddrPort("192.168.1.9:8765")}}
	// Bounded, so a regression that starts cloudflared anyway fails instead of
	// waiting for an address that never comes.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := q.Start(ctx, srv)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want a refusal to start without a loopback address to forward to", err)
	}
}

func TestQuick_StartTwiceIsRefused(t *testing.T) {
	locate, _ := stub(t, "echo 'INF |  https://abc.trycloudflare.com  |' >&2\nexec sleep 60\n")
	q := newTestQuick(locate, nil)
	cleanup(t, q)
	if _, err := q.Start(context.Background(), newServer()); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Start(context.Background(), newServer()); err == nil {
		t.Error("second Start succeeded")
	}
}

func TestQuick_CanBeStartedAgainAfterStop(t *testing.T) {
	locate, _ := stub(t, "echo 'INF |  https://abc.trycloudflare.com  |' >&2\nexec sleep 60\n")
	q := newTestQuick(locate, nil)
	cleanup(t, q)
	for range 2 {
		if _, err := q.Start(context.Background(), newServer()); err != nil {
			t.Fatal(err)
		}
		if err := q.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInstallLocations(t *testing.T) {
	if got := installLocations("darwin"); len(got) != 2 || got[0] != "/opt/homebrew/bin/cloudflared" {
		t.Errorf("darwin: %v", got)
	}
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramFiles(x86)", "")
	if got := installLocations("windows"); len(got) != 1 || filepath.Base(got[0]) != "cloudflared.exe" {
		t.Errorf("windows: %v", got)
	}
	if got := installLocations("plan9"); got != nil {
		t.Errorf("other: %v", got)
	}
}

var _ app.LocalServer = fakeServer{}
