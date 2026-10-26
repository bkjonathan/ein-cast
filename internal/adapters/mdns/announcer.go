// Package mdns is the multicast-DNS implementation of app.Discovery: it makes a
// running server answer to http://eincast.local:<port> and show up as an HTTP
// service in Bonjour/Avahi browsers.
package mdns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"unicode/utf8"

	"github.com/libp2p/zeroconf/v2"

	"eincast/internal/app"
)

// Host is the name the server answers to: "eincast" → eincast.local.
const Host = "eincast"

// maxInstanceBytes is the DNS label limit for the service's instance name.
const maxInstanceBytes = 63

// Announcer announces the server over mDNS. It implements app.Discovery.
type Announcer struct {
	log  *slog.Logger
	host string // a field so tests can use a name that cannot clash with a real EinCast
	// register publishes the records; zeroconf.RegisterProxy in production, a
	// recorder in tests of what gets published.
	register registerFunc
}

// registerFunc has the shape of zeroconf.RegisterProxy, reduced to the
// arguments this package varies.
type registerFunc func(instance, service, domain string, port int, host string, ips, text []string) (shutdowner, error)

// shutdowner is the part of *zeroconf.Server that withdraws the records.
type shutdowner interface{ Shutdown() }

var _ app.Discovery = (*Announcer)(nil)

// New returns an Announcer for eincast.local.
func New(log *slog.Logger) *Announcer {
	return &Announcer{log: log.With(slog.String("source", "mdns")), host: Host, register: zeroconfRegister}
}

// zeroconfRegister publishes on every multicast-capable interface. Which
// addresses are announced is decided by the caller's ips, not by the interfaces.
func zeroconfRegister(instance, service, domain string, port int, host string, ips, text []string) (shutdowner, error) {
	return zeroconf.RegisterProxy(instance, service, domain, port, host, ips, text, nil)
}

// Name labels this discovery in logs and the UI.
func (a *Announcer) Name() string { return "mdns" }

// Announce registers two things: the hostname (A records for eincast.local, one
// per LAN address) and an _http._tcp service named after the server. It returns
// once both are registered; stop withdraws them, with a goodbye packet so
// caches forget the name at once instead of after its time to live.
//
// Only the addresses in the announcement are published, never "all of this
// computer's addresses": VPN and container networks stay out of it, as they do
// for the listener itself.
func (a *Announcer) Announce(_ context.Context, ann app.Announcement) (func(), []string, error) {
	if len(ann.Addrs) == 0 {
		return nil, nil, errors.New("no network address to announce")
	}
	ips := make([]string, len(ann.Addrs))
	for i, addr := range ann.Addrs {
		ips[i] = addr.String()
	}

	srv, err := a.register(
		instanceName(ann.Name), "_http._tcp", "local.",
		ann.Port, a.host, ips, []string{"path=/", "app=EinCast"},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("register %s.local: %w", a.host, err)
	}
	stop := func() { srv.Shutdown() }
	return stop, []string{fmt.Sprintf("http://%s.local:%d", a.host, ann.Port)}, nil
}

// instanceName fits a display name into a DNS label (63 bytes), without
// cutting a character in half.
func instanceName(name string) string {
	if len(name) <= maxInstanceBytes {
		return name
	}
	cut := maxInstanceBytes
	for cut > 0 && !utf8.RuneStart(name[cut]) {
		cut--
	}
	return name[:cut]
}
