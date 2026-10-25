// Package netinfo finds the addresses other devices can use to reach this
// machine. It implements app.NetInfo.
package netinfo

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// Iface is the part of a network interface that address selection looks at.
// Selection is a pure function over this struct so it can be tested without a
// network; System fills it in from the operating system.
type Iface struct {
	Name     string
	Flags    net.Flags
	Prefixes []netip.Prefix
}

// virtualPrefixes are Unix-style interface names (docker0, utun3, wg0, ...)
// that never carry the home LAN.
var virtualPrefixes = []string{
	"lo", "docker", "br-", "veth", "virbr", "vmnet", "vboxnet", "utun", "tun",
	"tap", "ppp", "wg", "awdl", "llw", "bridge", "anpi", "gif", "stf", "ap",
	"p2p", "tailscale", "zt", "cni", "flannel", "cali", "kube",
}

// virtualWords are fragments of the descriptive names Windows gives virtual
// adapters ("vEthernet (WSL)", "VMware Network Adapter VMnet8", ...).
var virtualWords = []string{
	"vethernet", "hyper-v", "wsl", "vpn", "tailscale", "zerotier", "virtual",
	"vmware", "vmnet", "virtualbox", "loopback", "pseudo", "teredo", "isatap",
	"npcap", "bluetooth",
}

func isVirtual(name string) bool {
	n := strings.ToLower(name)
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	for _, w := range virtualWords {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// rank orders private ranges by how likely they are to be the home network:
// 192.168/16 first (consumer routers), then 10/8, then 172.16/12 (which Docker
// and some VPNs favour).
func rank(a netip.Addr) int {
	b := a.As4()
	switch {
	case b[0] == 192 && b[1] == 168:
		return 0
	case b[0] == 10:
		return 1
	default:
		return 2
	}
}

// Pick returns the LAN IPv4 addresses among ifaces: interfaces that are up,
// not loopback, not point-to-point (VPNs) and not virtual by name, keeping only
// private IPv4 addresses. Link-local (169.254/16, "no DHCP"), CGNAT
// (100.64/10, Tailscale) and public addresses are not LAN addresses. The
// result is ordered by rank, then interface order, without duplicates.
func Pick(ifaces []Iface) []netip.Addr {
	type candidate struct {
		addr  netip.Addr
		rank  int
		order int
	}
	var found []candidate
	seen := map[netip.Addr]bool{}

	for i, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
			continue
		}
		if isVirtual(ifc.Name) {
			continue
		}
		for _, p := range ifc.Prefixes {
			a := p.Addr().Unmap()
			if !a.Is4() || !a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || seen[a] {
				continue
			}
			seen[a] = true
			found = append(found, candidate{addr: a, rank: rank(a), order: i})
		}
	}

	slices.SortStableFunc(found, func(x, y candidate) int {
		if x.rank != y.rank {
			return x.rank - y.rank
		}
		return x.order - y.order
	})

	out := make([]netip.Addr, len(found))
	for i, c := range found {
		out[i] = c.addr
	}
	return out
}

// System reads the real interfaces from the operating system.
type System struct {
	// interfaces is replaceable in tests.
	interfaces func() ([]Iface, error)
}

// New returns a System that inspects the machine it runs on.
func New() *System {
	return &System{interfaces: readInterfaces}
}

// LANAddrs implements app.NetInfo.
func (s *System) LANAddrs() ([]netip.Addr, error) {
	ifaces, err := s.interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	return Pick(ifaces), nil
}

func readInterfaces() ([]Iface, error) {
	sys, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]Iface, 0, len(sys))
	for _, ifc := range sys {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue // one unreadable interface should not hide the others
		}
		ifc2 := Iface{Name: ifc.Name, Flags: ifc.Flags}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipnet.IP)
			if !ok {
				continue
			}
			ones, _ := ipnet.Mask.Size()
			ifc2.Prefixes = append(ifc2.Prefixes, netip.PrefixFrom(ip.Unmap(), ones))
		}
		out = append(out, ifc2)
	}
	return out, nil
}
