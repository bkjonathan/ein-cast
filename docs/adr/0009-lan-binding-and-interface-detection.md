# 0009 — LAN binding and interface detection

## Context
Phase 3 bound loopback only, so no other device could connect. Devices need an
address to open, and the choice of what to listen on is a security decision: a
home media server that listens on every interface also listens on the VPN, the
Docker bridge and any virtual machine network.

## Decision
- **Bind explicit addresses, never `0.0.0.0`.** `httpapi.Server` opens one
  listener per address: loopback (always) plus the LAN addresses the app layer
  passes in `ServeConfig.Hosts`. An unspecified address (`0.0.0.0`, `::`) is
  refused in code, with a test, so a future change cannot quietly widen exposure.
- **All or nothing.** If any bind fails, listeners already opened are closed and
  `Start` fails with the reason (the state machine moves to Failed). A partly
  reachable server would show a URL that does not work.
- **One port for every address.** With port 0 the first listener picks a port
  and the rest reuse it.
- **`app.NetInfo` port, `adapters/netinfo` implementation.** Selection is the
  pure function `Pick` over a small `Iface` struct, so it is table-tested with
  no network. It keeps interfaces that are up, not loopback, not point-to-point
  (VPNs), and not virtual by name (Docker, VMware, Hyper-V/WSL, utun, Tailscale
  and similar), and from those only private IPv4 addresses. Link-local
  (169.254/16: "DHCP failed"), CGNAT (100.64/10: Tailscale) and public addresses
  are not LAN addresses. Results are ranked 192.168/16, then 10/8, then
  172.16/12 (which Docker favours).
- **URLs come from what was bound.** `streaming.Service` builds the local and
  LAN URLs from `RunningServer.Addrs()`, not from what `NetInfo` returned, so the
  UI never advertises an address that is not actually listening.
- **A missing network is not an error.** If detection fails or finds nothing the
  server starts on loopback and the UI says no network was found.
- **The device page is embedded** (`embed.FS`), served with a strict
  Content-Security-Policy (`default-src 'none'`, same-origin script, style,
  media, images), `nosniff` and `no-referrer` on every response. Its JavaScript
  inserts file names only as text; a Go test fails if an HTML-injecting API
  appears in it.

## Consequences
- Anyone on the same network can now reach the library. That is the product, but
  it also means there is **no authentication until the access token in Phase 6**
  (and PIN auth for public tunnels in Phase 7).
- The bound addresses are fixed at start. If the machine changes network (Wi-Fi
  to Ethernet, a new DHCP lease) the server must be restarted to pick the new
  address; the UI does not watch for changes yet.
- IPv4 only. Home TVs and phones overwhelmingly use IPv4 on the LAN, and
  IPv6 link-local addresses need zone identifiers that complicate URLs.
- Name-based filtering is a heuristic. An unusual adapter name can be wrongly
  kept or dropped; the table of cases in the tests is where to add a fix.

## Alternatives rejected
- **Listen on `0.0.0.0`**: one line, and exposes every interface. Rejected
  outright; see the project's security rules.
- **Listen on one chosen LAN address only**: breaks when the machine has both
  Wi-Fi and Ethernet and the device is on the other one.
- **Use the request `Host` header to build URLs in the UI**: the UI has no
  request; it must know the address before any device connects.
- **mDNS name (`eincast.local`) instead of an IP**: the right long-term answer
  for TVs, but it needs a responder and does not resolve on every Android
  device. It arrives in v0.5; IP URLs keep working alongside it.
