# 0019 — Discovery behind one port; mDNS gives the server a name

## Context
So far a device needed the server's address typed in or scanned from a QR code.
v0.5 asks for two ways of being found without that: by name (`eincast.local`),
and by smart TVs (DLNA/UPnP, ADR 0020). Both are "tell the local network I exist"
protocols with the same lifecycle: announce when the server is running, withdraw
before it stops, never take the server down if they fail.

macOS already advertises `<computer-name>.local`; `eincast.local` is a *different*
name that has to be published on purpose, pointing at the LAN addresses the
server really listens on.

## Decision
- **One port, `app.Discovery`.** `Name()` and
  `Announce(ctx, Announcement) (stop, urls, err)`. An `Announcement` is the display
  name, the port, and the LAN addresses the server is bound to (never loopback,
  never "every interface"). `urls` are friendlier addresses the announcement
  creates, such as `http://eincast.local:8765`. `app.ErrNotAnnounced` means "this
  kind is not wanted this run" and is not logged as a failure.
- **`streaming.Service` owns the lifecycle** (`WithDiscovery(...)`). It announces
  after the state machine reaches Running, and withdraws *first* when stopping or
  when the server crashes: stop telling people where to find a server that is
  going away, then drop the tunnel, then the server. A discovery that fails is
  logged at Warn and skipped; the others still run and the server still serves.
  `ServerStatus` gains `NameURLs` and `Discovery` so the UI can show what is
  active.
- **mDNS through `libp2p/zeroconf`**, in proxy mode: it publishes `eincast.local`
  (A records for exactly the announced addresses) and an `_http._tcp` service named
  after the server. The library carries the DNS wire format, which is the tedious
  and error-prone part; the registration call is behind a small function type so
  tests assert what is published without needing multicast.
- **mDNS is always on while the server runs** and has no setting: it reveals that a
  server exists, not what is in it, and the access token still guards the content.
- **The Connect card shows `eincast.local` as a second address** after the numeric
  ones. Numbers first, because they work on every device; a `.local` name needs
  mDNS, which Apple and Windows 10+ devices have and some Android versions and
  older TVs lack. The Host check already accepted `*.local`.

## Consequences
- Verified by hand on macOS: with the announcer running, `dns-sd -G v4
  eincast-probe.local` resolved to the announced address and `dns-sd -B _http._tcp`
  listed the service. The library's own browser does not hear a responder on the
  same machine, so the repository's test asserts the published records rather than
  looping through multicast.
- **Two EinCasts on one network both claim `eincast.local`.** zeroconf does not
  probe for conflicts or rename; devices will get either. A conflict-aware name
  (`eincast-2.local`) is a seam, not built.
- mDNS announcements go out on every multicast-capable interface, but carry only
  the announced addresses, so VPN and container networks are not advertised.
- A firewall that blocks UDP 5353 silently defeats it. The numeric address and QR
  code keep working, and the log says what was announced.

## Alternatives rejected
- **Hand-write an mDNS responder.** Needs DNS packet encoding and decoding, name
  compression, probing and goodbye packets; the library is the less risky place to
  spend that effort.
- **Rely on the OS's own `<computer>.local`.** Not under the app's control and not
  "eincast".
- **One interface per protocol in `streaming`.** Would make the service know about
  mDNS and SSDP by name; the port keeps it ignorant of both.
- **A setting to turn mDNS off.** No known reason to; it is one more switch that
  would need explaining.
