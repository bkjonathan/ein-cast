package httpapi

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"eincast/internal/adapters/netinfo"
	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// baseURL is where the loopback listener of run answers.
func baseURL(run app.RunningServer) string {
	return "http://" + run.Addrs()[0].String()
}

func startServer(t *testing.T, ctx context.Context, lib Library, port int, hosts ...netip.Addr) app.RunningServer {
	t.Helper()
	srv := New(lib, &fakeAccess{}, &fakeRemote{}, "test", slog.New(slog.DiscardHandler))
	run, err := srv.Start(ctx, app.ServeConfig{Port: port, Name: "Den", Hosts: hosts})
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

	if got := run.Addrs(); len(got) != 1 || !got[0].Addr().IsLoopback() {
		t.Fatalf("Addrs() = %v, want exactly the loopback address when no hosts are given", got)
	}
	resp, err := http.Get(baseURL(run) + "/api/health")
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
	if _, err := http.Get(baseURL(run) + "/api/health"); err == nil {
		t.Error("server still answers after Stop")
	}
}

func TestServer_PortInUseFailsAtStart(t *testing.T) {
	first := startServer(t, context.Background(), &fakeLibrary{}, 0)
	port := int(first.Addrs()[0].Port())

	_, err := New(&fakeLibrary{}, &fakeAccess{}, &fakeRemote{}, "test", slog.New(slog.DiscardHandler)).
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
		resp, err := http.Get(baseURL(run) + "/api/library")
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

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestServer_RefusesUnspecifiedAddresses(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::"} {
		t.Run(host, func(t *testing.T) {
			_, err := New(&fakeLibrary{}, &fakeAccess{}, &fakeRemote{}, "test", slog.New(slog.DiscardHandler)).
				Start(context.Background(), app.ServeConfig{Port: 0, Hosts: []netip.Addr{netip.MustParseAddr(host)}})
			if err == nil || !strings.Contains(err.Error(), "refusing") {
				t.Fatalf("Start() error = %v, want a refusal to listen on every interface", err)
			}
		})
	}
}

func TestServer_LoopbackIsAlwaysBoundAndDuplicatesAreIgnored(t *testing.T) {
	loop := netip.MustParseAddr("127.0.0.1")
	run := startServer(t, context.Background(), &fakeLibrary{}, 0, loop, loop)

	if got := run.Addrs(); len(got) != 1 || got[0].Addr() != loop {
		t.Fatalf("Addrs() = %v, want a single loopback listener", got)
	}
}

// A failed bind must not leave the earlier listeners open: the server starts
// completely or not at all.
func TestServer_PartialBindRollsBack(t *testing.T) {
	port := freePort(t)
	unassignable := netip.MustParseAddr("203.0.113.77") // TEST-NET-3: not on any interface here

	_, err := New(&fakeLibrary{}, &fakeAccess{}, &fakeRemote{}, "test", slog.New(slog.DiscardHandler)).
		Start(context.Background(), app.ServeConfig{Port: port, Hosts: []netip.Addr{unassignable}})
	if err == nil {
		t.Fatal("Start() succeeded with an address this machine does not have")
	}

	ln, lerr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if lerr != nil {
		t.Fatalf("the loopback listener was left open after the failed start: %v", lerr)
	}
	ln.Close()
}

func TestServer_ListensOnLANAddressesOnOnePort(t *testing.T) {
	lan, err := netinfo.New().LANAddrs()
	if err != nil || len(lan) == 0 {
		t.Skip("no LAN address on this machine")
	}
	run := startServer(t, context.Background(), &fakeLibrary{}, 0, lan[0])

	addrs := run.Addrs()
	if len(addrs) != 2 || !addrs[0].Addr().IsLoopback() || addrs[1].Addr() != lan[0] {
		t.Fatalf("Addrs() = %v, want loopback then %v", addrs, lan[0])
	}
	if addrs[0].Port() != addrs[1].Port() || addrs[0].Port() == 0 {
		t.Fatalf("ports differ: %v", addrs)
	}

	for _, a := range addrs {
		resp, err := http.Get("http://" + a.String() + "/api/health")
		if err != nil {
			t.Fatalf("GET via %v: %v", a, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("GET via %v: status %d", a, resp.StatusCode)
		}
	}
}

func TestBind_RefusesAddressesItShouldNeverListenOn(t *testing.T) {
	run := startServer(t, context.Background(), &fakeLibrary{}, 0)

	cases := map[string]netip.Addr{
		"every IPv4 interface": netip.MustParseAddr("0.0.0.0"),
		"every IPv6 interface": netip.MustParseAddr("::"),
		"loopback (bound)":     netip.MustParseAddr("127.0.0.1"),
		"another loopback":     netip.MustParseAddr("127.0.0.2"),
		"IPv6 loopback":        netip.MustParseAddr("::1"),
		"no address":           {},
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if unbind, err := run.Bind(addr); err == nil {
				unbind()
				t.Errorf("Bind(%v) succeeded", addr)
			}
		})
	}
}

func TestBind_AddsAndRemovesAListenerOnTheServersPort(t *testing.T) {
	lan, err := netinfo.New().LANAddrs()
	if err != nil || len(lan) == 0 {
		t.Skip("no LAN address on this machine to stand in for a tailnet address")
	}
	run := startServer(t, context.Background(), &fakeLibrary{}, 0) // not given the LAN address
	extra := netip.AddrPortFrom(lan[0], run.Addrs()[0].Port())
	health := "http://" + extra.String() + "/api/health"

	if _, err := http.Get(health); err == nil {
		t.Fatal("the server answered on an address it was never asked to listen on")
	}

	unbind, err := run.Bind(lan[0])
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	resp, err := http.Get(health)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("after Bind: %v %v", resp, err)
	}
	resp.Body.Close()

	if _, err := run.Bind(lan[0]); err == nil {
		t.Error("binding the same address twice succeeded")
	}
	if got := run.Addrs(); len(got) != 1 {
		t.Errorf("Addrs() = %v: extra listeners are not part of the server's own addresses", got)
	}

	unbind()
	unbind() // safe to repeat
	client := http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 2 * time.Second}
	if resp, err := client.Get(health); err == nil {
		resp.Body.Close()
		t.Fatal("still answering after unbind")
	}
	// Loopback is untouched.
	if resp, err := http.Get(baseURL(run) + "/api/health"); err != nil || resp.StatusCode != 200 {
		t.Errorf("loopback after unbind: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}
}

func TestBind_UnbindDropsConnectionsThatCameInThroughIt(t *testing.T) {
	lan, err := netinfo.New().LANAddrs()
	if err != nil || len(lan) == 0 {
		t.Skip("no LAN address on this machine to stand in for a tailnet address")
	}
	run := startServer(t, context.Background(), &fakeLibrary{}, 0)
	unbind, err := run.Bind(lan[0])
	if err != nil {
		t.Fatal(err)
	}

	// A keep-alive connection through the extra listener, and one through loopback.
	extraConn, err := net.Dial("tcp", netip.AddrPortFrom(lan[0], run.Addrs()[0].Port()).String())
	if err != nil {
		t.Fatal(err)
	}
	defer extraConn.Close()
	loopConn, err := net.Dial("tcp", run.Addrs()[0].String())
	if err != nil {
		t.Fatal(err)
	}
	defer loopConn.Close()
	for _, c := range []net.Conn{extraConn, loopConn} {
		fmt.Fprintf(c, "GET /api/health HTTP/1.1\r\nHost: x\r\n\r\n")
		if _, err := bufio.NewReader(c).ReadString('\n'); err != nil {
			t.Fatalf("no response on %v: %v", c.RemoteAddr(), err)
		}
	}

	unbind()

	_ = extraConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := extraConn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET) {
		t.Errorf("the tailnet connection survived unbind: read error = %v", err)
	}
	// The loopback connection is not ours to drop.
	_ = loopConn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, err := loopConn.Read(make([]byte, 1)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("unbind disturbed an unrelated connection: read error = %v", err)
	}
}

func TestBind_ListenersEndWithTheServer(t *testing.T) {
	lan, err := netinfo.New().LANAddrs()
	if err != nil || len(lan) == 0 {
		t.Skip("no LAN address on this machine to stand in for a tailnet address")
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := startServer(t, ctx, &fakeLibrary{}, 0)
	if _, err := run.Bind(lan[0]); err != nil {
		t.Fatal(err)
	}
	port := run.Addrs()[0].Port()

	cancel()
	waitDone(t, run)

	ln, err := net.Listen("tcp", netip.AddrPortFrom(lan[0], port).String())
	if err != nil {
		t.Fatalf("the extra listener outlived the server: %v", err)
	}
	ln.Close()
	if _, err := run.Bind(lan[0]); err == nil {
		t.Error("Bind succeeded on a server that has stopped")
	}
}
