package tailscale

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// The fields of `tailscale status --json` we read. They are the Go field names
// of Tailscale's ipnstate.Status, which has no JSON tags, so they appear
// verbatim. Everything else in the output is ignored, which is what lets a
// newer Tailscale add fields without breaking us.
type statusJSON struct {
	BackendState   string
	MagicDNSSuffix string // legacy; CurrentTailnet.MagicDNSSuffix replaces it
	CurrentTailnet *struct {
		MagicDNSEnabled bool
	}
	Self *struct {
		DNSName      string
		TailscaleIPs []netip.Addr
	}
}

// node is what Start needs to know about this machine on the tailnet.
type node struct {
	// ip is the tailnet address to listen on.
	ip netip.Addr
	// host is how other devices reach this one: its MagicDNS name when the
	// tailnet has MagicDNS, otherwise the address.
	host string
}

var (
	// tailnetV4 and tailnetV6 are where Tailscale assigns addresses from. Only
	// these are ever bound: a bug or a surprising status must not be able to
	// make the server listen on, say, a public address.
	tailnetV4 = netip.MustParsePrefix("100.64.0.0/10")
	tailnetV6 = netip.MustParsePrefix("fd7a:115c:a1e0::/48")
)

// parseStatus reads the output of `tailscale status --json`. The errors are
// written for the person who sees them in the app.
func parseStatus(raw []byte) (node, error) {
	var st statusJSON
	if err := json.Unmarshal(raw, &st); err != nil {
		return node{}, fmt.Errorf("could not read Tailscale's status: %w", err)
	}
	if err := checkBackend(st.BackendState); err != nil {
		return node{}, err
	}
	if st.Self == nil {
		return node{}, errors.New("Tailscale did not report this device; try again in a moment")
	}

	ip, ok := pickAddr(st.Self.TailscaleIPs)
	if !ok {
		return node{}, errors.New("this device has no Tailscale address yet; try again in a moment")
	}

	n := node{ip: ip, host: ip.String()}
	if name := strings.TrimSuffix(st.Self.DNSName, "."); name != "" && magicDNSOn(st) {
		n.host = name
	}
	return n, nil
}

func magicDNSOn(st statusJSON) bool {
	if st.CurrentTailnet != nil {
		return st.CurrentTailnet.MagicDNSEnabled
	}
	return st.MagicDNSSuffix != "" // older Tailscale
}

// checkBackend turns Tailscale's state names into advice.
func checkBackend(state string) error {
	switch state {
	case "Running":
		return nil
	case "NeedsLogin", "NoState":
		return errors.New("Tailscale is not signed in. Sign in from the Tailscale app, then try again")
	case "Stopped":
		return errors.New("Tailscale is turned off. Turn it on in the Tailscale app, then try again")
	case "Starting":
		return errors.New("Tailscale is still starting. Try again in a moment")
	case "NeedsMachineAuth":
		return errors.New("this device is waiting for approval in your Tailscale admin console")
	default:
		return fmt.Errorf("Tailscale is not running (state %q)", state)
	}
}

// pickAddr prefers the IPv4 tailnet address, which every client understands, and
// falls back to IPv6. Addresses outside Tailscale's ranges are skipped.
func pickAddr(ips []netip.Addr) (netip.Addr, bool) {
	var v6 netip.Addr
	for _, ip := range ips {
		ip = ip.Unmap()
		switch {
		case tailnetV4.Contains(ip):
			return ip, true
		case tailnetV6.Contains(ip) && !v6.IsValid():
			v6 = ip
		}
	}
	return v6, v6.IsValid()
}
