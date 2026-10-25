package netinfo

import (
	"errors"
	"net"
	"net/netip"
	"slices"
	"testing"
)

const up = net.FlagUp | net.FlagBroadcast | net.FlagMulticast

func pfx(s string) []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix(s)} }

func strs(as []netip.Addr) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.String()
	}
	return out
}

func TestPick(t *testing.T) {
	tests := []struct {
		name   string
		ifaces []Iface
		want   []string
	}{
		{"typical home Wi-Fi", []Iface{{"en0", up, pfx("192.168.1.20/24")}}, []string{"192.168.1.20"}},
		{"nothing", nil, []string{}},
		{"loopback flag", []Iface{{"lo0", up | net.FlagLoopback, pfx("127.0.0.1/8")}}, []string{}},
		{"interface down", []Iface{{"en0", net.FlagBroadcast, pfx("192.168.1.20/24")}}, []string{}},
		{"point-to-point is a VPN", []Iface{{"en5", up | net.FlagPointToPoint, pfx("10.8.0.2/24")}}, []string{}},
		{"public address is not LAN", []Iface{{"en0", up, pfx("203.0.113.9/24")}}, []string{}},
		{"link-local means no DHCP", []Iface{{"en0", up, pfx("169.254.10.2/16")}}, []string{}},
		{"CGNAT range (Tailscale)", []Iface{{"en0", up, pfx("100.101.102.103/10")}}, []string{}},
		{"IPv6 only", []Iface{{"en0", up, pfx("fe80::1/64")}}, []string{}},
		{"IPv4-mapped IPv6 is unmapped", []Iface{{"en0", up, pfx("::ffff:192.168.1.9/120")}}, []string{"192.168.1.9"}},

		// Virtual adapters, by name.
		{"docker bridge", []Iface{{"docker0", up, pfx("172.17.0.1/16")}}, []string{}},
		{"user-defined docker bridge", []Iface{{"br-3f2a9c", up, pfx("172.18.0.1/16")}}, []string{}},
		{"veth pair", []Iface{{"veth12ab", up, pfx("172.19.0.1/16")}}, []string{}},
		{"macOS utun VPN", []Iface{{"utun3", up, pfx("10.9.0.5/24")}}, []string{}},
		{"wireguard", []Iface{{"wg0", up, pfx("10.7.0.2/24")}}, []string{}},
		{"tailscale", []Iface{{"tailscale0", up, pfx("10.64.0.1/16")}}, []string{}},
		{"vmware", []Iface{{"vmnet8", up, pfx("192.168.233.1/24")}}, []string{}},
		{"virtualbox", []Iface{{"vboxnet0", up, pfx("192.168.56.1/24")}}, []string{}},
		{"Windows WSL", []Iface{{"vEthernet (WSL)", up, pfx("172.28.0.1/20")}}, []string{}},
		{"Windows VMware", []Iface{{"VMware Network Adapter VMnet1", up, pfx("192.168.40.1/24")}}, []string{}},
		{"Windows Hyper-V", []Iface{{"vEthernet (Default Switch)", up, pfx("172.20.0.1/20")}}, []string{}},
		{"macOS bridge", []Iface{{"bridge100", up, pfx("192.168.2.1/24")}}, []string{}},

		// Real adapters on other platforms must survive the name filter.
		{"Windows Ethernet", []Iface{{"Ethernet 2", up, pfx("192.168.0.50/24")}}, []string{"192.168.0.50"}},
		{"Windows Wi-Fi", []Iface{{"Wi-Fi", up, pfx("192.168.0.51/24")}}, []string{"192.168.0.51"}},
		{"Linux predictable names", []Iface{{"enp0s31f6", up, pfx("10.0.0.4/24")}, {"wlp3s0", up, pfx("10.0.0.5/24")}}, []string{"10.0.0.4", "10.0.0.5"}},
		{"Linux eth0", []Iface{{"eth0", up, pfx("192.168.1.2/24")}}, []string{"192.168.1.2"}},

		{
			"ranks 192.168 before 10 before 172.16, and keeps interface order within a rank",
			[]Iface{
				{"en2", up, pfx("172.16.5.5/16")},
				{"en1", up, pfx("10.1.1.1/8")},
				{"en0", up, pfx("192.168.1.20/24")},
				{"en3", up, pfx("192.168.9.9/24")},
			},
			[]string{"192.168.1.20", "192.168.9.9", "10.1.1.1", "172.16.5.5"},
		},
		{
			"several addresses on one interface, and duplicates across interfaces",
			[]Iface{
				{"en0", up, append(pfx("192.168.1.20/24"), netip.MustParsePrefix("192.168.1.21/24"))},
				{"en1", up, pfx("192.168.1.20/24")},
			},
			[]string{"192.168.1.20", "192.168.1.21"},
		},
		{
			"a real LAN survives alongside a VPN and Docker",
			[]Iface{
				{"utun4", up | net.FlagPointToPoint, pfx("10.8.0.2/32")},
				{"docker0", up, pfx("172.17.0.1/16")},
				{"en0", up, pfx("192.168.1.20/24")},
			},
			[]string{"192.168.1.20"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strs(Pick(tt.ifaces)); !slices.Equal(got, tt.want) {
				t.Fatalf("Pick() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSystem_LANAddrs_PropagatesErrors(t *testing.T) {
	boom := errors.New("boom")
	s := &System{interfaces: func() ([]Iface, error) { return nil, boom }}
	if _, err := s.LANAddrs(); !errors.Is(err, boom) {
		t.Fatalf("LANAddrs() error = %v, want it to wrap %v", err, boom)
	}
}

// Whatever machine runs the tests, the invariants hold.
func TestSystem_RealInterfacesOnlyYieldPrivateIPv4(t *testing.T) {
	addrs, err := New().LANAddrs()
	if err != nil {
		t.Fatalf("LANAddrs() error = %v", err)
	}
	for _, a := range addrs {
		if !a.Is4() || !a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
			t.Errorf("%v is not a private, non-loopback IPv4 address", a)
		}
	}
	t.Logf("this machine reports: %v", addrs)
}
