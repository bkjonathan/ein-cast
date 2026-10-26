package dlna

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"testing"

	"eincast/internal/app"
)

func iface(index int, name string, prefixes ...string) ifaceInfo {
	in := ifaceInfo{iface: net.Interface{Index: index, Name: name}}
	for _, p := range prefixes {
		in.prefixes = append(in.prefixes, netip.MustParsePrefix(p))
	}
	return in
}

func TestLocalsFor(t *testing.T) {
	ifaces := []ifaceInfo{
		iface(1, "lo0", "127.0.0.1/8"),
		iface(4, "en0", "192.168.1.20/24", "fe80::1/64"),
		iface(9, "utun3", "100.64.0.5/10"),
	}
	got := localsFor([]netip.Addr{netip.MustParseAddr("192.168.1.20"), netip.MustParseAddr("10.9.9.9")}, ifaces)

	if len(got) != 1 {
		t.Fatalf("locals = %+v, want only the address an interface really owns", got)
	}
	l := got[0]
	if l.addr != netip.MustParseAddr("192.168.1.20") || l.iface.Name != "en0" || l.prefix != netip.MustParsePrefix("192.168.1.0/24") {
		t.Errorf("local = %+v, want the address with its interface and its network (masked)", l)
	}
	if got := localsFor(nil, ifaces); len(got) != 0 {
		t.Errorf("no addresses gave %v", got)
	}
}

func newTestAnnouncer(ifaces []ifaceInfo, err error) *Announcer {
	a := NewAnnouncer("11111111-2222-5333-8444-555555555555", "1.2.3", slog.New(slog.DiscardHandler))
	a.interfaces = func() ([]ifaceInfo, error) { return ifaces, err }
	return a
}

func TestAnnouncer_IsOptIn(t *testing.T) {
	a := newTestAnnouncer(nil, nil)
	_, _, err := a.Announce(context.Background(), app.Announcement{Name: "Den", Port: 8765, Addrs: []netip.Addr{netip.MustParseAddr("192.168.1.20")}})
	if !errors.Is(err, app.ErrNotAnnounced) {
		t.Fatalf("error = %v, want ErrNotAnnounced when DLNA is off: TVs must not be told about a server that did not ask for it", err)
	}
	if a.Name() != "dlna" {
		t.Errorf("Name() = %q", a.Name())
	}
}

func TestAnnouncer_Failures(t *testing.T) {
	ann := app.Announcement{Name: "Den", Port: 8765, DLNA: true, Addrs: []netip.Addr{netip.MustParseAddr("192.168.1.20")}}

	t.Run("cannot list interfaces", func(t *testing.T) {
		_, _, err := newTestAnnouncer(nil, errors.New("boom")).Announce(context.Background(), ann)
		if err == nil || errors.Is(err, app.ErrNotAnnounced) {
			t.Fatalf("error = %v, want a real failure", err)
		}
	})
	t.Run("address on no interface", func(t *testing.T) {
		_, _, err := newTestAnnouncer([]ifaceInfo{iface(4, "en0", "10.0.0.2/24")}, nil).Announce(context.Background(), ann)
		if err == nil || errors.Is(err, app.ErrNotAnnounced) {
			t.Fatalf("error = %v, want a real failure", err)
		}
	})
}

func TestAnnouncer_StartsAndStopsTheResponder(t *testing.T) {
	// The real port 1900 is shared, so this works even where something else
	// (Windows' SSDP service, another media server) already listens on it.
	// Loopback has no multicast, so the join may fail; then there is nothing to
	// test here beyond the error being reported.
	ifaces, err := systemInterfaces()
	if err != nil {
		t.Skip(err)
	}
	var lan netip.Addr
	for _, in := range ifaces {
		if in.iface.Flags&net.FlagLoopback != 0 || in.iface.Flags&net.FlagMulticast == 0 || in.iface.Flags&net.FlagUp == 0 {
			continue
		}
		for _, p := range in.prefixes {
			if p.Addr().Is4() && p.Addr().IsPrivate() {
				lan = p.Addr()
			}
		}
	}
	if !lan.IsValid() {
		t.Skip("no private IPv4 address on a multicast-capable interface")
	}

	a := NewAnnouncer("11111111-2222-5333-8444-555555555555", "1.2.3", slog.New(slog.DiscardHandler))
	stop, urls, err := a.Announce(context.Background(), app.Announcement{Name: "Den", Port: 18765, DLNA: true, Addrs: []netip.Addr{lan}})
	if err != nil {
		t.Skipf("cannot announce here: %v", err)
	}
	if len(urls) != 0 {
		t.Errorf("urls = %v: TVs find the server by themselves, there is no address to show", urls)
	}
	stop()
	stop()
}
