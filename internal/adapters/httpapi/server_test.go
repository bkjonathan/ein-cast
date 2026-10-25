package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

func startServer(t *testing.T, ctx context.Context, lib Library, port int) app.RunningServer {
	t.Helper()
	srv := New(lib, "test", slog.New(slog.DiscardHandler))
	run, err := srv.Start(ctx, app.ServeConfig{Port: port, Name: "Den"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = run.Stop(sctx)
	})
	return run
}

func waitDone(t *testing.T, run app.RunningServer) {
	t.Helper()
	select {
	case <-run.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("server did not finish within 3s")
	}
}

func TestServer_StartServeStop(t *testing.T) {
	run := startServer(t, context.Background(), &fakeLibrary{}, 0)

	if !strings.HasPrefix(run.Addr(), "127.0.0.1:") {
		t.Fatalf("Addr() = %q, want a loopback address (never 0.0.0.0)", run.Addr())
	}
	resp, err := http.Get("http://" + run.Addr() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("health status = %d", resp.StatusCode)
	}

	if err := run.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	waitDone(t, run)
	if run.Err() != nil {
		t.Errorf("Err() = %v after a clean stop, want nil", run.Err())
	}
	if _, err := http.Get("http://" + run.Addr() + "/api/health"); err == nil {
		t.Error("server still answers after Stop")
	}
}

func TestServer_PortInUseFailsAtStart(t *testing.T) {
	first := startServer(t, context.Background(), &fakeLibrary{}, 0)
	_, portStr, _ := net.SplitHostPort(first.Addr())
	port, _ := net.LookupPort("tcp", portStr)

	_, err := New(&fakeLibrary{}, "test", slog.New(slog.DiscardHandler)).
		Start(context.Background(), app.ServeConfig{Port: port, Name: "x"})
	if err == nil {
		t.Fatal("second Start() on the same port succeeded")
	}
}

func TestServer_ParentContextCancelShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	run := startServer(t, ctx, &fakeLibrary{}, 0)

	cancel()
	waitDone(t, run)
	if run.Err() != nil {
		t.Errorf("Err() = %v, want nil for a cancelled-context shutdown", run.Err())
	}
}

// blockingLibrary hangs List() so a request stays in flight.
type blockingLibrary struct {
	fakeLibrary
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingLibrary) List() []media.Item {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return nil
}

func TestServer_StopWaitsThenDropsStuckRequests(t *testing.T) {
	lib := &blockingLibrary{entered: make(chan struct{}), release: make(chan struct{})}
	run := startServer(t, context.Background(), lib, 0)
	t.Cleanup(func() { close(lib.release) })

	reqErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + run.Addr() + "/api/library")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		reqErr <- err
	}()
	<-lib.entered // the request is now in flight

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := run.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v; hitting the deadline is expected, not a failure", err)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("Stop took %s; want it to wait for the deadline (~150ms) and then give up", elapsed)
	}

	select {
	case err := <-reqErr:
		if err == nil {
			t.Error("stuck request completed normally; it should have been dropped")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stuck request was never dropped")
	}
}
