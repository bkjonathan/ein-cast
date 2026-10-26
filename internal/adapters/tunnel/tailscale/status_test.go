package tailscale

import (
	"net/netip"
	"strings"
	"testing"
)

// running is shaped like real `tailscale status --json` output: far more than
// we read, to show unknown fields are ignored.
const running = `{
  "Version": "1.80.0-t1234",
  "TUN": true,
  "BackendState": "Running",
  "HaveNodeKey": true,
  "AuthURL": "",
  "TailscaleIPs": ["100.101.102.103", "fd7a:115c:a1e0::1"],
  "Self": {
    "ID": "nABC123CNTRL",
    "HostName": "den",
    "DNSName": "den.tail1234.ts.net.",
    "OS": "macOS",
    "TailscaleIPs": ["100.101.102.103", "fd7a:115c:a1e0::1"],
    "Online": true,
    "Capabilities": ["https"]
  },
  "Health": [],
  "MagicDNSSuffix": "tail1234.ts.net",
  "CurrentTailnet": {"Name": "me@example.com", "MagicDNSSuffix": "tail1234.ts.net", "MagicDNSEnabled": true},
  "Peer": {"nodekey:abc": {"HostName": "phone", "DNSName": "phone.tail1234.ts.net.", "TailscaleIPs": ["100.64.0.9"]}}
}`

func TestParseStatus_Running(t *testing.T) {
	n, err := parseStatus([]byte(running))
	if err != nil {
		t.Fatal(err)
	}
	if n.ip != netip.MustParseAddr("100.101.102.103") {
		t.Errorf("ip = %v, want the IPv4 tailnet address", n.ip)
	}
	if n.host != "den.tail1234.ts.net" {
		t.Errorf("host = %q, want the MagicDNS name without its trailing dot", n.host)
	}
}

func TestParseStatus_AddressAndNameChoices(t *testing.T) {
	status := func(self, tailnet string) string {
		return `{"BackendState":"Running","Self":` + self + `,"CurrentTailnet":` + tailnet + `}`
	}
	const dnsOn, dnsOff = `{"MagicDNSEnabled":true}`, `{"MagicDNSEnabled":false}`

	cases := []struct {
		name     string
		json     string
		wantIP   string
		wantHost string
	}{
		{"MagicDNS off: use the address", status(`{"DNSName":"den.t.ts.net.","TailscaleIPs":["100.64.0.1"]}`, dnsOff), "100.64.0.1", "100.64.0.1"},
		{"MagicDNS on but no name yet", status(`{"DNSName":"","TailscaleIPs":["100.64.0.1"]}`, dnsOn), "100.64.0.1", "100.64.0.1"},
		{"IPv6 listed first, IPv4 wins", status(`{"DNSName":"d.ts.net.","TailscaleIPs":["fd7a:115c:a1e0::5","100.64.0.2"]}`, dnsOff), "100.64.0.2", "100.64.0.2"},
		{"IPv6 only", status(`{"DNSName":"","TailscaleIPs":["fd7a:115c:a1e0:ab12::7"]}`, dnsOff), "fd7a:115c:a1e0:ab12::7", "fd7a:115c:a1e0:ab12::7"},
		{"top of the tailnet range", status(`{"TailscaleIPs":["100.127.255.254"]}`, dnsOff), "100.127.255.254", "100.127.255.254"},
		{"mapped IPv6 form", status(`{"TailscaleIPs":["::ffff:100.64.0.3"]}`, dnsOff), "100.64.0.3", "100.64.0.3"},
		{"older Tailscale: legacy suffix means MagicDNS", `{"BackendState":"Running","MagicDNSSuffix":"t.ts.net","Self":{"DNSName":"den.t.ts.net.","TailscaleIPs":["100.64.0.1"]}}`, "100.64.0.1", "den.t.ts.net"},
		{"older Tailscale without MagicDNS", `{"BackendState":"Running","Self":{"DNSName":"den.t.ts.net.","TailscaleIPs":["100.64.0.1"]}}`, "100.64.0.1", "100.64.0.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, err := parseStatus([]byte(c.json))
			if err != nil {
				t.Fatal(err)
			}
			if n.ip != netip.MustParseAddr(c.wantIP) || n.host != c.wantHost {
				t.Errorf("got ip=%v host=%q, want ip=%s host=%q", n.ip, n.host, c.wantIP, c.wantHost)
			}
		})
	}
}

func TestParseStatus_NeverReturnsAnAddressOutsideTheTailnet(t *testing.T) {
	// Whatever the status says, the server must not be told to listen on a LAN,
	// public or loopback address.
	for _, ip := range []string{"192.168.1.5", "10.0.0.2", "8.8.8.8", "127.0.0.1", "100.63.255.255", "100.128.0.0", "0.0.0.0", "fe80::1", "2001:db8::1", "fd7b:115c:a1e0::1"} {
		t.Run(ip, func(t *testing.T) {
			raw := `{"BackendState":"Running","Self":{"TailscaleIPs":["` + ip + `"]}}`
			if n, err := parseStatus([]byte(raw)); err == nil {
				t.Errorf("accepted %s: %+v", ip, n)
			}
		})
	}
}

func TestParseStatus_ExplainsWhatToDo(t *testing.T) {
	selfOK := `"Self":{"DNSName":"d.ts.net.","TailscaleIPs":["100.64.0.1"]}`
	cases := []struct {
		name string
		json string
		want string
	}{
		{"signed out", `{"BackendState":"NeedsLogin"}`, "not signed in"},
		{"no state yet", `{"BackendState":"NoState"}`, "not signed in"},
		{"turned off", `{"BackendState":"Stopped"}`, "turned off"},
		{"starting", `{"BackendState":"Starting"}`, "still starting"},
		{"needs approval", `{"BackendState":"NeedsMachineAuth"}`, "admin console"},
		{"unknown state", `{"BackendState":"Hibernating"}`, "Hibernating"},
		{"state missing", `{` + selfOK + `}`, "not running"},
		{"not running, even with an address", `{"BackendState":"Stopped",` + selfOK + `}`, "turned off"},
		{"no self", `{"BackendState":"Running"}`, "did not report this device"},
		{"no addresses", `{"BackendState":"Running","Self":{"TailscaleIPs":[]}}`, "no Tailscale address"},
		{"not JSON", `Tailscale is stopped.`, "could not read"},
		{"empty", ``, "could not read"},
		{"bad address", `{"BackendState":"Running","Self":{"TailscaleIPs":["nope"]}}`, "could not read"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseStatus([]byte(c.json))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want one mentioning %q", err, c.want)
			}
		})
	}
}
