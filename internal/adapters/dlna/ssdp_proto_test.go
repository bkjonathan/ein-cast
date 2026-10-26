package dlna

import (
	"net"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func TestTargets(t *testing.T) {
	got := targets("abc")
	nts := make([]string, len(got))
	for i, g := range got {
		nts[i] = g.nt
	}
	want := []string{"upnp:rootdevice", "uuid:abc", deviceType, contentDirectory, connectionManager}
	if !slices.Equal(nts, want) {
		t.Fatalf("NTs = %v, want %v", nts, want)
	}
	if got[0].usn != "uuid:abc::upnp:rootdevice" || got[1].usn != "uuid:abc" || got[3].usn != "uuid:abc::"+contentDirectory {
		t.Errorf("USNs = %+v", got)
	}
}

func msearch(headers ...string) []byte {
	return []byte("M-SEARCH * HTTP/1.1\r\n" + strings.Join(headers, "\r\n") + "\r\n\r\n")
}

func TestParseSearch(t *testing.T) {
	tests := []struct {
		name   string
		packet []byte
		want   search
		ok     bool
	}{
		{"typical", msearch("HOST: 239.255.255.250:1900", `MAN: "ssdp:discover"`, "MX: 3", "ST: upnp:rootdevice"), search{"upnp:rootdevice", 3}, true},
		{"ssdp:all", msearch(`MAN: "ssdp:discover"`, "MX: 2", "ST: ssdp:all"), search{"ssdp:all", 2}, true},
		{"MX too big is capped", msearch(`MAN: "ssdp:discover"`, "MX: 120", "ST: ssdp:all"), search{"ssdp:all", 5}, true},
		{"MX missing", msearch(`MAN: "ssdp:discover"`, "ST: ssdp:all"), search{"ssdp:all", 1}, true},
		{"MX nonsense", msearch(`MAN: "ssdp:discover"`, "MX: soon", "ST: ssdp:all"), search{"ssdp:all", 1}, true},
		{"MX zero or negative", msearch(`MAN: "ssdp:discover"`, "MX: -4", "ST: ssdp:all"), search{"ssdp:all", 1}, true},
		{"lower-case header names", msearch(`man: "ssdp:discover"`, "mx: 2", "st: ssdp:all"), search{"ssdp:all", 2}, true},
		{"wrong MAN", msearch(`MAN: "ssdp:other"`, "ST: ssdp:all"), search{}, false},
		{"no ST", msearch(`MAN: "ssdp:discover"`, "MX: 2"), search{}, false},
		{"a NOTIFY from another device", []byte("NOTIFY * HTTP/1.1\r\nNT: upnp:rootdevice\r\nNTS: ssdp:alive\r\n\r\n"), search{}, false},
		{"garbage", []byte("hello"), search{}, false},
		{"empty", nil, search{}, false},
		{"oversized", append(msearch(`MAN: "ssdp:discover"`, "ST: ssdp:all"), make([]byte, maxSearchBytes)...), search{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSearch(tt.packet)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("parseSearch() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestMatching(t *testing.T) {
	all := targets("abc")
	if got := matching(all, "ssdp:all"); len(got) != 5 {
		t.Errorf("ssdp:all matched %d targets, want all 5", len(got))
	}
	for _, st := range []string{"upnp:rootdevice", "uuid:abc", deviceType, contentDirectory, connectionManager} {
		got := matching(all, st)
		if len(got) != 1 || got[0].nt != st {
			t.Errorf("search for %q matched %+v, want exactly that target", st, got)
		}
	}
	for _, st := range []string{"uuid:other", "urn:schemas-upnp-org:device:MediaRenderer:1", "", "upnp:rootdevice:extra"} {
		if got := matching(all, st); len(got) != 0 {
			t.Errorf("search for %q matched %+v, want nothing", st, got)
		}
	}
	if got := matching(all, "UPNP:ROOTDEVICE"); len(got) != 1 {
		t.Error("matching should ignore case")
	}
}

func TestSearchResponse(t *testing.T) {
	got := string(searchResponse(target{"upnp:rootdevice", "uuid:abc::upnp:rootdevice"}, "http://192.168.1.20:8765/dlna/device.xml", "darwin/1.0 UPnP/1.0 EinCast/1"))
	for _, want := range []string{
		"HTTP/1.1 200 OK\r\n", "CACHE-CONTROL: max-age=1800\r\n", "EXT:\r\n",
		"LOCATION: http://192.168.1.20:8765/dlna/device.xml\r\n", "SERVER: darwin/1.0 UPnP/1.0 EinCast/1\r\n",
		"ST: upnp:rootdevice\r\n", "USN: uuid:abc::upnp:rootdevice\r\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("response lacks %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\r\n\r\n") {
		t.Error("response does not end with an empty line")
	}
}

func TestNotify(t *testing.T) {
	tg := target{contentDirectory, "uuid:abc::" + contentDirectory}
	alive := string(notify(tg, ntsAlive, "http://10.0.0.5:8765/dlna/device.xml", "srv"))
	for _, want := range []string{
		"NOTIFY * HTTP/1.1\r\n", "HOST: 239.255.255.250:1900\r\n", "NT: " + contentDirectory + "\r\n",
		"NTS: ssdp:alive\r\n", "LOCATION: http://10.0.0.5:8765/dlna/device.xml\r\n", "CACHE-CONTROL: max-age=1800\r\n",
	} {
		if !strings.Contains(alive, want) {
			t.Errorf("alive lacks %q:\n%s", want, alive)
		}
	}
	bye := string(notify(tg, ntsBye, "ignored", "srv"))
	if !strings.Contains(bye, "NTS: ssdp:byebye\r\n") || strings.Contains(bye, "LOCATION") || strings.Contains(bye, "CACHE-CONTROL") {
		t.Errorf("byebye must say who is leaving and nothing else:\n%s", bye)
	}
}

func local(addr, cidr string) localAddr {
	return localAddr{addr: netip.MustParseAddr(addr), prefix: netip.MustParsePrefix(cidr), iface: net.Interface{Name: "en0"}}
}

func TestLocationFor(t *testing.T) {
	locals := []localAddr{local("192.168.1.20", "192.168.1.0/24"), local("10.0.0.7", "10.0.0.0/24")}
	tests := []struct {
		name string
		from string
		want string
	}{
		{"a TV on the first network", "192.168.1.55", "http://192.168.1.20:8765/dlna/device.xml"},
		{"a TV on the second network", "10.0.0.99", "http://10.0.0.7:8765/dlna/device.xml"},
		{"a stranger gets the first address", "172.16.4.4", "http://192.168.1.20:8765/dlna/device.xml"},
		{"an IPv4-mapped sender", "::ffff:10.0.0.99", "http://10.0.0.7:8765/dlna/device.xml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := locationFor(netip.MustParseAddr(tt.from), locals, 8765)
			if !ok || got != tt.want {
				t.Fatalf("locationFor(%s) = %q, %v; want %q", tt.from, got, ok, tt.want)
			}
		})
	}
	if _, ok := locationFor(netip.MustParseAddr("192.168.1.55"), nil, 8765); ok {
		t.Error("with no addresses there is no location to give")
	}
}
