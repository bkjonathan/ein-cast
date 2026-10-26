# 0012 — Tailscale: listen on the tailnet address while it is on

## Context
Every provider so far in the plan (Cloudflare, ngrok) is a process that connects
*out* to a relay and forwards to `127.0.0.1`, so the server never listens on
anything but loopback and the LAN. Tailscale is different: other devices connect
*in*, to this machine's tailnet address (`100.x.y.z`, or its MagicDNS name).
`netinfo` deliberately skips that interface (ADR 0009), so today nothing listens
there.

Two things must hold. The server must not listen on `0.0.0.0` (it would also
open the Docker and VPN networks). And turning Tailscale off must actually close
the door, including streams already open over it.

## Decision
- **The server gains one capability: `Bind(addr)`.** `app.LocalServer` is the
  running server as a tunnel sees it: `Addrs()` plus `Bind(addr) (unbind func(),
  error)`. `Bind` opens an additional listener on the server's own port, serves
  the same handler on it, and returns a function that closes it and drops the
  connections that arrived through it. `RunningServer` embeds `LocalServer`.
- **`Tunnel.Start` receives the server**, not a "host:port" string (ADR 0007
  said otherwise at first). Process-based providers use
  `app.LoopbackAddr(srv)`; Tailscale uses `Bind`. One signature serves both.
- **The same rules as `Start`**: `Bind` refuses unspecified addresses, loopback
  and addresses already bound, and refuses once the server is stopping.
- **The Tailscale provider runs `tailscale status --json`** to learn this device's
  address and name. It binds only an address inside Tailscale's own ranges
  (`100.64.0.0/10`, `fd7a:115c:a1e0::/48`), whatever the status says, so a bug
  or an odd status cannot make the server listen on a public or LAN address. It
  returns `http://<MagicDNS name>:<port>` when the tailnet has MagicDNS, else the
  address.
- **Private, so no PIN, and the home-network rules apply.** Requests arrive from
  tailnet addresses, not loopback, so they are not "tunnelled" in the sense of
  ADR 0008: the LAN access token protects them if the user turned it on, and the
  access log shows the real device address. The Host check allows the MagicDNS
  name because the provider's URL is registered with the service; CORS accepts
  `100.64.0.0/10` and `*.ts.net`.
- **Closing means closing.** Connections are tracked through
  `http.Server.ConnState` by the local address they were accepted on, and
  `unbind` closes those of that listener only. Wrapping connections instead
  would hide the TCP connection's `ReadFrom` and cost multi-gigabyte downloads
  their `sendfile` fast path.
- **The provider's lifetime is its context**, as for every tunnel:
  `context.AfterFunc` releases the listener when it ends, and `Stop` cancels that
  watcher, so no goroutine outlives the tunnel.

## Consequences
- The server listens on the tailnet address only while Tailscale is switched on in
  EinCast, not whenever Tailscale happens to be running.
- The user must have Tailscale installed and signed in. A missing, signed-out or
  stopped Tailscale produces a specific message and leaves nothing bound.
- A Tailscale ACL or the macOS firewall can still block the port; EinCast cannot
  see that and will show a working address that does not work. Worth a line in the
  UI if it proves common.
- The JSON field names are Tailscale's `ipnstate.Status` Go field names (it has no
  JSON tags). They have been stable for years but are not a documented contract;
  the parser reads few fields and ignores the rest, and fails with a readable
  message if the ones it needs are missing.
- A connection accepted just as `unbind` runs can survive until it ends (the
  `ConnState` callback and the close are not one atomic step). The window is
  milliseconds and the listener is already closed.

## Alternatives rejected
- **Bind the tailnet address whenever the server starts**: the toggle would mean
  nothing, and the library would be reachable on the tailnet without being asked.
- **Forward from a separate TCP proxy to loopback**: the server would then see
  loopback requests with no forwarding headers and could not tell tailnet traffic
  from the owner's own browser. The real client address would be lost from the log.
- **`tailscale serve`**: needs HTTPS and MagicDNS enabled on the tailnet, hands the
  address and certificate to another process, and still needs a way to be undone.
- **Parsing `tailscale ip -4` and `tailscale status` separately**: two processes
  and two chances to disagree; one `status --json` has everything.
- **Opening the Tailscale local API socket directly**: platform-specific (a Unix
  socket, a named pipe, a localhost port with a token on macOS). The command-line
  tool hides all of that.
