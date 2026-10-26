package dlna

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
)

// SSDP, the Simple Service Discovery Protocol, is HTTP-shaped messages in UDP
// packets. A control point (a TV) multicasts an M-SEARCH asking what is out
// there; each device answers the sender directly. Devices also multicast NOTIFY
// messages when they appear and disappear. This file is the message layer:
// parsing, matching, and building, with no sockets.

const (
	ssdpGroup = "239.255.255.250"
	ssdpPort  = 1900
	// maxAgeSeconds is how long listeners may remember an announcement.
	maxAgeSeconds = 1800
	// maxSearchBytes bounds a packet we agree to parse. M-SEARCH is under 300.
	maxSearchBytes = 2048
)

// target is one thing the device announces itself as: a notification type (NT in
// NOTIFY, ST in a search) and the unique service name (USN) that goes with it.
type target struct {
	nt, usn string
}

// targets lists everything a MediaServer:1 announces, in the order the UPnP
// architecture asks for: the root device, the device's own UUID, the device type,
// then each service.
func targets(uuid string) []target {
	u := "uuid:" + uuid
	return []target{
		{"upnp:rootdevice", u + "::upnp:rootdevice"},
		{u, u},
		{deviceType, u + "::" + deviceType},
		{contentDirectory, u + "::" + contentDirectory},
		{connectionManager, u + "::" + connectionManager},
	}
}

// search is a parsed M-SEARCH request.
type search struct {
	st string
	mx int // seconds the searcher will wait, clamped to 1..5
}

// parseSearch reads an M-SEARCH packet. Anything else (NOTIFY from other
// devices, garbage, a search with the wrong MAN) is not for us and returns false.
func parseSearch(packet []byte) (search, bool) {
	if len(packet) > maxSearchBytes || !bytes.HasPrefix(packet, []byte("M-SEARCH ")) {
		return search{}, false
	}
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		return search{}, false
	}
	if strings.Trim(req.Header.Get("MAN"), `"`) != "ssdp:discover" {
		return search{}, false
	}
	st := strings.TrimSpace(req.Header.Get("ST"))
	if st == "" {
		return search{}, false
	}
	mx, err := strconv.Atoi(strings.TrimSpace(req.Header.Get("MX")))
	if err != nil || mx < 1 {
		mx = 1 // a missing or silly MX is answered promptly
	}
	return search{st: st, mx: min(mx, 5)}, true
}

// matching returns the targets a search for st should be answered with: all of
// them for ssdp:all, otherwise the one with exactly that type. Version 1 devices
// also answer for a lower version of their own types; this server has only
// version 1, so exact matching is complete.
func matching(all []target, st string) []target {
	if st == "ssdp:all" {
		return all
	}
	for _, t := range all {
		if strings.EqualFold(t.nt, st) {
			return []target{t}
		}
	}
	return nil
}

// searchResponse is the unicast answer to an M-SEARCH. ST echoes the target
// that matched, as a search for that type asked.
func searchResponse(t target, location, server string) []byte {
	var b bytes.Buffer
	b.WriteString("HTTP/1.1 200 OK\r\n")
	fmt.Fprintf(&b, "CACHE-CONTROL: max-age=%d\r\n", maxAgeSeconds)
	b.WriteString("EXT:\r\n")
	fmt.Fprintf(&b, "LOCATION: %s\r\n", location)
	fmt.Fprintf(&b, "SERVER: %s\r\n", server)
	fmt.Fprintf(&b, "ST: %s\r\n", t.nt)
	fmt.Fprintf(&b, "USN: %s\r\n", t.usn)
	b.WriteString("\r\n")
	return b.Bytes()
}

// Values of the NTS header.
const (
	ntsAlive = "ssdp:alive"
	ntsBye   = "ssdp:byebye"
)

// notify builds the multicast announcement that a target has appeared (alive) or
// is going away (byebye). Byebye carries no location.
func notify(t target, nts, location, server string) []byte {
	var b bytes.Buffer
	b.WriteString("NOTIFY * HTTP/1.1\r\n")
	fmt.Fprintf(&b, "HOST: %s:%d\r\n", ssdpGroup, ssdpPort)
	if nts == ntsAlive {
		fmt.Fprintf(&b, "CACHE-CONTROL: max-age=%d\r\n", maxAgeSeconds)
		fmt.Fprintf(&b, "LOCATION: %s\r\n", location)
		fmt.Fprintf(&b, "SERVER: %s\r\n", server)
	}
	fmt.Fprintf(&b, "NT: %s\r\n", t.nt)
	fmt.Fprintf(&b, "NTS: %s\r\n", nts)
	fmt.Fprintf(&b, "USN: %s\r\n", t.usn)
	b.WriteString("\r\n")
	return b.Bytes()
}

// localAddr is one of this computer's LAN addresses with the network it is on.
type localAddr struct {
	addr   netip.Addr
	prefix netip.Prefix
	iface  net.Interface
}

// locationFor is the device description URL to give a searcher at from: the one
// on the address that shares a network with it, so a TV on Wi-Fi is not sent the
// Ethernet address it cannot reach. A searcher on no known network (a router, a
// relay) gets the first address.
func locationFor(from netip.Addr, locals []localAddr, httpPort int) (string, bool) {
	if len(locals) == 0 {
		return "", false
	}
	chosen := locals[0]
	for _, l := range locals {
		if l.prefix.IsValid() && l.prefix.Contains(from.Unmap()) {
			chosen = l
			break
		}
	}
	return locationURL(chosen.addr, httpPort), true
}

func locationURL(addr netip.Addr, httpPort int) string {
	return "http://" + netip.AddrPortFrom(addr, uint16(httpPort)).String() + devicePath
}
