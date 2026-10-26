package mdns

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"eincast/internal/app"
)

func TestInstanceName(t *testing.T) {
	long := strings.Repeat("မ", 30) // 3 bytes each: 90 bytes
	got := instanceName(long)
	if len(got) > maxInstanceBytes || !utf8.ValidString(got) {
		t.Errorf("instanceName() = %d bytes, valid=%v; want at most %d bytes of valid UTF-8", len(got), utf8.ValidString(got), maxInstanceBytes)
	}
	if instanceName("Living room") != "Living room" {
		t.Error("a short name must be left alone")
	}
}

type recorder struct {
	instance, service, domain, host string
	port                            int
	ips, text                       []string
	shutdowns                       int
	err                             error
}

func (r *recorder) register(instance, service, domain string, port int, host string, ips, text []string) (shutdowner, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.instance, r.service, r.domain, r.port, r.host, r.ips, r.text = instance, service, domain, port, host, ips, text
	return r, nil
}

func (r *recorder) Shutdown() { r.shutdowns++ }

func newTestAnnouncer(r *recorder) *Announcer {
	a := New(slog.New(slog.DiscardHandler))
	a.register = r.register
	return a
}

func TestAnnounce_PublishesTheHostnameAndTheHTTPService(t *testing.T) {
	r := &recorder{}
	stop, urls, err := newTestAnnouncer(r).Announce(context.Background(), app.Announcement{
		Name: "Living room", Port: 8765,
		Addrs: []netip.Addr{netip.MustParseAddr("192.168.1.20"), netip.MustParseAddr("10.0.0.7")},
	})
	if err != nil {
		t.Fatal(err)
	}

	if r.host != "eincast" || r.service != "_http._tcp" || r.domain != "local." || r.port != 8765 || r.instance != "Living room" {
		t.Errorf("registered %+v", r)
	}
	if !slices.Equal(r.ips, []string{"192.168.1.20", "10.0.0.7"}) {
		t.Errorf("published addresses = %v, want exactly the announced ones", r.ips)
	}
	if !slices.Equal(urls, []string{"http://eincast.local:8765"}) {
		t.Errorf("urls = %v", urls)
	}

	stop()
	if r.shutdowns != 1 {
		t.Errorf("shut down %d times, want 1", r.shutdowns)
	}
}

func TestAnnounce_Failures(t *testing.T) {
	t.Run("no address", func(t *testing.T) {
		r := &recorder{}
		if _, _, err := newTestAnnouncer(r).Announce(context.Background(), app.Announcement{Name: "Den", Port: 8765}); err == nil {
			t.Fatal("Announce() = nil error with no address")
		}
	})
	t.Run("registration fails", func(t *testing.T) {
		r := &recorder{err: errors.New("no multicast interface")}
		_, _, err := newTestAnnouncer(r).Announce(context.Background(), app.Announcement{
			Name: "Den", Port: 8765, Addrs: []netip.Addr{netip.MustParseAddr("192.168.1.20")},
		})
		if err == nil || !strings.Contains(err.Error(), "eincast.local") {
			t.Fatalf("error = %v, want one that names what could not be registered", err)
		}
	})
}

// TestAnnounce_RealRegistrationStartsAndStops uses the real library once, so a
// change in its arguments or a missing multicast interface shows up here. It
// asserts only that registering and withdrawing work; whether other machines
// hear it is a property of the network, checked by hand (see ADR 0019).
func TestAnnounce_RealRegistrationStartsAndStops(t *testing.T) {
	a := New(slog.New(slog.DiscardHandler))
	a.host = "eincast-test"
	stop, _, err := a.Announce(context.Background(), app.Announcement{
		Name: "EinCast Test", Port: 18765, Addrs: []netip.Addr{netip.MustParseAddr("192.0.2.10")},
	})
	if err != nil {
		t.Skipf("cannot register here (no multicast-capable interface?): %v", err)
	}
	stop()
	stop() // a second call is harmless
}
