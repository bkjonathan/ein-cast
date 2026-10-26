package dlna

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/ipv4"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// TestE2E_TVFindsAndBrowsesTheServer plays a TV's part over the real network:
// it multicasts a search, reads the LOCATION from the answer, fetches the device
// description from it, and browses the library. Everything on the way is real
// except the library. It needs a multicast-capable interface with a private IPv4
// address, and is skipped where nothing answers (some sandboxes and CI machines).
func TestE2E_TVFindsAndBrowsesTheServer(t *testing.T) {
	ifaces, err := systemInterfaces()
	if err != nil {
		t.Skip(err)
	}
	var lan netip.Addr
	var via net.Interface
	for _, in := range ifaces {
		if in.iface.Flags&net.FlagLoopback != 0 || in.iface.Flags&net.FlagMulticast == 0 || in.iface.Flags&net.FlagUp == 0 {
			continue
		}
		for _, p := range in.prefixes {
			if p.Addr().Is4() && p.Addr().IsPrivate() {
				lan, via = p.Addr(), in.iface
			}
		}
	}
	if !lan.IsValid() {
		t.Skip("no private IPv4 address on a multicast-capable interface")
	}

	// The HTTP side, listening where the announcement will say it is.
	ln, err := net.Listen("tcp4", net.JoinHostPort(lan.String(), "0"))
	if err != nil {
		t.Skipf("cannot listen on %s: %v", lan, err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	uuid := DeviceUUID("e2e-test-" + time.Now().Format("150405.000000"))
	lib := &fakeLib{items: []media.Item{libItem("films/Heat.mkv", media.KindVideo, 4242)}}
	srv := &http.Server{Handler: NewHandler(Config{Library: lib, Name: "E2E <Den>", UUID: uuid, Version: "test"})}
	go srv.Serve(ln)
	defer srv.Close()

	a := NewAnnouncer(uuid, "test", slog.New(slog.DiscardHandler))
	stop, _, err := a.Announce(context.Background(), app.Announcement{Name: "E2E", Port: port, DLNA: true, Addrs: []netip.Addr{lan}})
	if err != nil {
		t.Skipf("cannot announce here: %v", err)
	}
	defer stop()

	// The TV's search.
	pc, err := net.ListenPacket("udp4", net.JoinHostPort(lan.String(), "0"))
	if err != nil {
		t.Skipf("cannot open a search socket: %v", err)
	}
	defer pc.Close()
	conn := ipv4.NewPacketConn(pc)
	if err := conn.SetMulticastInterface(&via); err != nil {
		t.Skipf("cannot send multicast on %s: %v", via.Name, err)
	}
	search := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: " + deviceType + "\r\n\r\n"
	group := &net.UDPAddr{IP: net.ParseIP(ssdpGroup), Port: ssdpPort}

	location := ""
	buf := make([]byte, 4096)
	deadline := time.Now().Add(6 * time.Second)
	for location == "" && time.Now().Before(deadline) {
		if _, err := conn.WriteTo([]byte(search), nil, group); err != nil {
			t.Skipf("cannot send the search: %v", err)
		}
		_ = pc.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		for {
			n, _, err := pc.ReadFrom(buf)
			if err != nil {
				break
			}
			msg := string(buf[:n])
			if strings.Contains(msg, "uuid:"+uuid) { // other media servers on the LAN answer too
				m := regexp.MustCompile(`(?i)\r\nLOCATION:\s*(\S+)`).FindStringSubmatch(msg)
				if m != nil {
					location = m[1]
					break
				}
			}
		}
	}
	if location == "" {
		t.Skip("no answer to a multicast search: multicast does not work on this machine")
	}

	want := "http://" + net.JoinHostPort(lan.String(), strconv.Itoa(port)) + devicePath
	if location != want {
		t.Fatalf("LOCATION = %q, want %q", location, want)
	}
	resp, err := http.Get(location)
	if err != nil {
		t.Fatalf("fetching the advertised description: %v", err)
	}
	desc, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(desc), "<friendlyName>E2E &lt;Den&gt;</friendlyName>") || !strings.Contains(string(desc), contentDirectory) {
		t.Fatalf("description = %s", desc)
	}

	// Browse the root the way the description says to.
	body := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:Browse xmlns:u="` + contentDirectory + `">` +
		`<ObjectID>0</ObjectID><BrowseFlag>BrowseDirectChildren</BrowseFlag><Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria/></u:Browse></s:Body></s:Envelope>`
	req, _ := http.NewRequest(http.MethodPost, "http://"+net.JoinHostPort(lan.String(), strconv.Itoa(port))+cdControlPath, strings.NewReader(body))
	req.Header.Set("SOAPACTION", `"`+contentDirectory+`#Browse"`)
	br, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(br.Body)
	br.Body.Close()
	if br.StatusCode != 200 || !strings.Contains(string(out), "Videos") {
		t.Fatalf("Browse: %d %s", br.StatusCode, out)
	}
}
