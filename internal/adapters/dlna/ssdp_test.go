package dlna

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// A responder tested over unicast: the "multicast group" is a socket the test
// owns, and searches are sent straight to the responder's port. That exercises
// every code path except the operating system's multicast delivery.

type rig struct {
	r      *Responder
	client *net.UDPConn // receives announcements, sends searches
	port   int
}

func loopbackLocals(t *testing.T) []localAddr {
	t.Helper()
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			return []localAddr{{addr: netip.MustParseAddr("127.0.0.1"), prefix: netip.MustParsePrefix("127.0.0.0/8"), iface: ifc}}
		}
	}
	t.Skip("no loopback interface")
	return nil
}

func newRig(t *testing.T, mutate func(*ResponderConfig)) *rig {
	t.Helper()
	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })

	cfg := ResponderConfig{
		UUID: "11111111-2222-5333-8444-555555555555", HTTPPort: 8765, Server: "test/1.0 UPnP/1.0 EinCast/0",
		Locals: loopbackLocals(t), Log: slog.New(slog.DiscardHandler),
		ListenPort: -1, NoMulticast: true,
		Group:  netip.MustParseAddrPort(client.LocalAddr().String()),
		Delay:  func(int) time.Duration { return 0 },
		Repeat: time.Hour,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	r := NewResponder(cfg)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(r.Stop)
	return &rig{r: r, client: client, port: r.Port()}
}

// read returns the next packet as an HTTP-style header map plus its first line.
func (g *rig) read(t *testing.T, wait time.Duration) (first string, h http.Header, ok bool) {
	t.Helper()
	_ = g.client.SetReadDeadline(time.Now().Add(wait))
	buf := make([]byte, 4096)
	n, _, err := g.client.ReadFromUDP(buf)
	if err != nil {
		return "", nil, false
	}
	tp := bufio.NewReader(bytes.NewReader(buf[:n]))
	line, _ := tp.ReadString('\n')
	h = http.Header{}
	for {
		l, err := tp.ReadString('\n')
		l = strings.TrimRight(l, "\r\n")
		if l == "" || err != nil {
			break
		}
		k, v, _ := strings.Cut(l, ":")
		h.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	return strings.TrimSpace(line), h, true
}

func (g *rig) search(t *testing.T, st string) {
	t.Helper()
	msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: " + st + "\r\n\r\n"
	if _, err := g.client.WriteToUDP([]byte(msg), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: g.port}); err != nil {
		t.Fatal(err)
	}
}

// drainFor discards whatever arrives during d, however busy the responder is.
func (g *rig) drainFor(t *testing.T, d time.Duration) {
	t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); {
		g.read(t, 20*time.Millisecond)
	}
}

// collect reads until nothing more arrives for a while.
func (g *rig) collect(t *testing.T, wait time.Duration) []http.Header {
	t.Helper()
	var out []http.Header
	for {
		_, h, ok := g.read(t, wait)
		if !ok {
			return out
		}
		out = append(out, h)
	}
}

func TestResponder_AnnouncesItselfOnStart(t *testing.T) {
	g := newRig(t, nil)

	var alive []http.Header
	deadline := time.Now().Add(2 * time.Second)
	for len(alive) < 5 && time.Now().Before(deadline) {
		first, h, ok := g.read(t, 500*time.Millisecond)
		if ok && first == "NOTIFY * HTTP/1.1" && h.Get("NTS") == "ssdp:alive" {
			alive = append(alive, h)
		}
	}
	if len(alive) < 5 {
		t.Fatalf("got %d alive messages, want one for each of the 5 targets", len(alive))
	}
	nts := map[string]bool{}
	for _, h := range alive {
		nts[h.Get("NT")] = true
		if h.Get("Location") != "http://127.0.0.1:8765/dlna/device.xml" || h.Get("Server") == "" {
			t.Errorf("alive headers = %v", h)
		}
	}
	for _, want := range []string{"upnp:rootdevice", "uuid:11111111-2222-5333-8444-555555555555", deviceType, contentDirectory, connectionManager} {
		if !nts[want] {
			t.Errorf("no alive message for %s (got %v)", want, nts)
		}
	}
}

func TestResponder_AnswersASearchForEverything(t *testing.T) {
	g := newRig(t, func(c *ResponderConfig) { c.Repeat = time.Hour })
	g.collect(t, 400*time.Millisecond) // the startup announcements

	g.search(t, "ssdp:all")
	var got []http.Header
	for _, h := range g.collect(t, 400*time.Millisecond) {
		got = append(got, h)
	}
	if len(got) != 5 {
		t.Fatalf("got %d answers to ssdp:all, want 5", len(got))
	}
	sts := map[string]string{}
	for _, h := range got {
		sts[h.Get("ST")] = h.Get("USN")
		if h.Get("Location") != "http://127.0.0.1:8765/dlna/device.xml" {
			t.Errorf("Location = %q", h.Get("Location"))
		}
	}
	if sts["upnp:rootdevice"] != "uuid:11111111-2222-5333-8444-555555555555::upnp:rootdevice" || len(sts) != 5 {
		t.Errorf("answers = %v", sts)
	}
}

func TestResponder_AnswersASpecificSearchWithThatTypeOnly(t *testing.T) {
	g := newRig(t, nil)
	g.collect(t, 400*time.Millisecond)

	g.search(t, contentDirectory)
	got := g.collect(t, 400*time.Millisecond)
	if len(got) != 1 || got[0].Get("ST") != contentDirectory || !strings.HasSuffix(got[0].Get("USN"), "::"+contentDirectory) {
		t.Fatalf("answers = %v", got)
	}
}

func TestResponder_StaysQuietForWhatIsNotItsBusiness(t *testing.T) {
	g := newRig(t, nil)
	g.collect(t, 400*time.Millisecond)

	g.search(t, "urn:schemas-upnp-org:device:MediaRenderer:1")
	g.search(t, "uuid:somebody-else")
	_, _ = g.client.WriteToUDP([]byte("garbage"), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: g.port})
	_, _ = g.client.WriteToUDP([]byte("NOTIFY * HTTP/1.1\r\nNT: upnp:rootdevice\r\nNTS: ssdp:alive\r\n\r\n"), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: g.port})
	if got := g.collect(t, 400*time.Millisecond); len(got) != 0 {
		t.Fatalf("answered %d times to searches for other things: %v", len(got), got)
	}
}

func TestResponder_AnswersAfterTheRandomDelay(t *testing.T) {
	g := newRig(t, func(c *ResponderConfig) {
		c.Delay = func(mx int) time.Duration { return 300 * time.Millisecond }
	})
	g.collect(t, 400*time.Millisecond)

	g.search(t, "upnp:rootdevice")
	if _, _, ok := g.read(t, 100*time.Millisecond); ok {
		t.Fatal("answered before the delay")
	}
	if _, _, ok := g.read(t, time.Second); !ok {
		t.Fatal("never answered")
	}
}

func TestResponder_RepeatsItsAnnouncement(t *testing.T) {
	g := newRig(t, func(c *ResponderConfig) { c.Repeat = 80 * time.Millisecond })
	// 3 startup rounds of 5, then a round every 80 ms: well over 15 in a second.
	deadline := time.Now().Add(1500 * time.Millisecond)
	count := 0
	for time.Now().Before(deadline) {
		if _, h, ok := g.read(t, 200*time.Millisecond); ok && h.Get("NTS") == "ssdp:alive" {
			count++
		}
	}
	if count <= 15 {
		t.Errorf("%d alive messages, want repeats beyond the 15 sent at start", count)
	}
}

func TestResponder_SaysGoodbyeOnStopAndNothingAfter(t *testing.T) {
	g := newRig(t, func(c *ResponderConfig) { c.Repeat = 50 * time.Millisecond })
	g.drainFor(t, 400*time.Millisecond) // the responder keeps repeating, so "wait for quiet" would never end

	g.r.Stop()
	var kinds []string
	for _, h := range g.collect(t, 400*time.Millisecond) {
		kinds = append(kinds, h.Get("NTS"))
	}
	byes := 0
	for i, k := range kinds {
		if k == "ssdp:byebye" {
			byes++
		} else if byes > 0 || i > 20 {
			// An "alive" arriving after a goodbye would tell TVs the server is back.
			t.Fatalf("messages after Stop = %v: an alive followed the goodbye", kinds)
		}
	}
	if byes != 5 {
		t.Fatalf("got %d byebye messages, want one for each of the 5 targets (%v)", byes, kinds)
	}
	g.r.Stop() // a second call is harmless
}

func TestResponder_StopDoesNotWaitForDelayedAnswers(t *testing.T) {
	g := newRig(t, func(c *ResponderConfig) { c.Delay = func(int) time.Duration { return time.Hour } })
	for range 100 { // far more than may be pending
		g.search(t, "ssdp:all")
	}
	time.Sleep(200 * time.Millisecond)

	done := make(chan struct{})
	go func() { g.r.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop is stuck behind answers waiting out their delay")
	}
}

func TestResponder_TheNumberOfWaitingAnswersIsBounded(t *testing.T) {
	g := newRig(t, func(c *ResponderConfig) { c.Delay = func(int) time.Duration { return 500 * time.Millisecond } })
	g.collect(t, 400*time.Millisecond)

	for range 100 {
		g.search(t, "ssdp:all")
	}
	got := len(g.collect(t, 1500*time.Millisecond))
	if got > maxPendingReplies+5 { // a few may have been sent and replaced slots meanwhile
		t.Errorf("sent %d answers to a flood of 100 searches, want about %d at most", got, maxPendingReplies)
	}
	if got == 0 {
		t.Error("answered none")
	}
}

func TestResponder_NeedsAnAddress(t *testing.T) {
	r := NewResponder(ResponderConfig{UUID: "x", Log: slog.New(slog.DiscardHandler)})
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("Start() = nil with no address to announce on")
	}
}

func TestResponder_SharesThePortWithAnotherListener(t *testing.T) {
	// Another UPnP program already on the port must not stop us: the real port
	// is 1900, shared by every SSDP participant on the computer.
	lc := net.ListenConfig{Control: shareAddress}
	first, err := lc.ListenPacket(context.Background(), "udp4", ":0")
	if err != nil {
		t.Skipf("cannot open a shareable socket: %v", err)
	}
	defer first.Close()
	port := first.LocalAddr().(*net.UDPAddr).Port

	r := NewResponder(ResponderConfig{
		UUID: "x", HTTPPort: 1, Locals: loopbackLocals(t), Log: slog.New(slog.DiscardHandler),
		ListenPort: port, NoMulticast: true, Group: netip.MustParseAddrPort("127.0.0.1:9"),
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start() on a port someone else listens on = %v", err)
	}
	r.Stop()
}
