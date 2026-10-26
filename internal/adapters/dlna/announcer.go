package dlna

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"

	"eincast/internal/app"
)

// Announcer makes the server findable by smart TVs over SSDP. It implements
// app.Discovery.
type Announcer struct {
	log     *slog.Logger
	uuid    string
	version string
	// interfaces lists the computer's networks; net.Interfaces in production.
	interfaces func() ([]ifaceInfo, error)
}

var _ app.Discovery = (*Announcer)(nil)

// ifaceInfo is a network interface and the networks it is attached to.
type ifaceInfo struct {
	iface    net.Interface
	prefixes []netip.Prefix
}

// NewAnnouncer returns an Announcer for the device identified by uuid.
func NewAnnouncer(uuid, version string, log *slog.Logger) *Announcer {
	return &Announcer{log: log.With(slog.String("source", "dlna")), uuid: uuid, version: version, interfaces: systemInterfaces}
}

// Name labels this discovery in logs and the UI.
func (a *Announcer) Name() string { return "dlna" }

// Announce starts the SSDP responder for the announced addresses. It does
// nothing (app.ErrNotAnnounced) unless Smart TV discovery is on for this run.
func (a *Announcer) Announce(ctx context.Context, ann app.Announcement) (func(), []string, error) {
	if !ann.DLNA {
		return nil, nil, app.ErrNotAnnounced
	}
	ifaces, err := a.interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("list network interfaces: %w", err)
	}
	locals := localsFor(ann.Addrs, ifaces)
	if len(locals) == 0 {
		return nil, nil, errors.New("none of the server's addresses belongs to a network interface")
	}

	r := NewResponder(ResponderConfig{
		UUID:     a.uuid,
		HTTPPort: ann.Port,
		Server:   ServerHeader(a.version),
		Locals:   locals,
		Log:      a.log,
	})
	if err := r.Start(ctx); err != nil {
		return nil, nil, err
	}
	return r.Stop, nil, nil
}

// localsFor pairs each announced address with the interface and network it is
// on. An address that no interface owns is dropped: it cannot be announced from.
func localsFor(addrs []netip.Addr, ifaces []ifaceInfo) []localAddr {
	var out []localAddr
	for _, addr := range addrs {
		for _, in := range ifaces {
			for _, p := range in.prefixes {
				if p.Addr() == addr {
					out = append(out, localAddr{addr: addr, prefix: p.Masked(), iface: in.iface})
				}
			}
		}
	}
	return out
}

func systemInterfaces() ([]ifaceInfo, error) {
	list, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []ifaceInfo
	for _, ifc := range list {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		info := ifaceInfo{iface: ifc}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				if p, ok := netip.AddrFromSlice(n.IP); ok {
					ones, _ := n.Mask.Size()
					info.prefixes = append(info.prefixes, netip.PrefixFrom(p.Unmap(), ones))
				}
			}
		}
		out = append(out, info)
	}
	return out, nil
}
