# 0020 — DLNA/UPnP: let smart TVs find and browse the library

## Context
Many TVs and media players show a "media servers" list and browse it, with no
browser and no address to type. They speak UPnP AV: they find servers with SSDP
(multicast UDP), read an XML device description over HTTP, and browse a
ContentDirectory service through SOAP. Files are then fetched with plain HTTP GET,
which the server already does well (`/stream/{id}`, ranges and all).

DLNA has no authentication. A TV cannot send an access token or a PIN.

## Decision
- **Opt-in, off by default, and exclusive with the access token.** The setting
  "Smart TVs" lists the library to the whole network without a password, so it must
  be asked for. `settings.Update` refuses `DLNA && RequireToken`
  (`ErrDLNAWithToken`), a settings file asking for both keeps the token, the
  bridge starts the server with `DLNA && !RequireToken`, and the HTTP layer sits
  behind the normal access control, so a token turned on later makes `/dlna/`
  answer 401 rather than serve. Four locks on one door, the first being the user's
  decision. A change applies at the next server start, like the port.
- **Served by the same HTTP server, under `/dlna/`**, so the Host check, the
  access log, graceful shutdown and the Tailscale/tunnel gating all apply. Mounted
  only for a run that asked for it. Through a public tunnel the PIN gate applies
  as to everything else.
- **`adapters/dlna` has three layers, each testable alone.**
  - *Content.* `buildTree` arranges the library for a remote control: Videos /
    Music / Photos, then the library's own folders, folders first, natural sort
    ("Episode 2" before "Episode 10"). Object IDs are opaque (`k-video`,
    `d-<hash>`, `i-<media id>`), looked up in the tree, never used as a path
    (ADR 0005). `didl` writes DIDL-Lite, `soap` reads Browse requests, answers or
    faults (UPnP error codes), `handler` routes by exact path. Browse supports
    paging and `BrowseMetadata`. The tree is rebuilt per call, so it cannot be
    stale.
  - *Protocol.* `ssdp_proto` is pure: parse an M-SEARCH, match the five targets
    (root device, UUID, MediaServer:1, ContentDirectory:1, ConnectionManager:1),
    build answers and NOTIFY messages, choose a LOCATION on the searcher's own
    network.
  - *Sockets.* `Responder` binds UDP 1900 with address sharing (other UPnP
    software and Windows' SSDP service listen there too), joins the multicast
    group per interface, announces (three times, then every 15 minutes), answers
    after a random delay within MX, and says goodbye on stop, after the announce
    loop has ended so no "alive" follows a "byebye".
- **SSDP written here rather than taken from a library.** The one evaluated
  (`koron/go-ssdp`) answers `ssdp:all` and `rootdevice` searches from one socket
  per target, with the `ST` header naming its own target rather than the one
  asked for, and cannot give each searcher the address on its own network. Picky
  TVs reject exactly that. The platform part is two small files (`SO_REUSEADDR`
  and, on Unix, `SO_REUSEPORT`).
- **Stable identity without storage.** The device UUID is a name-based (v5) UUID of
  the computer's host name, so a TV does not list a second copy after a restart.
- **DLNA headers on `/stream`.** A request with `getcontentFeatures.dlna.org: 1`
  gets `contentFeatures.dlna.org` and `transferMode.dlna.org: Streaming` back, which
  some TVs insist on before playing.
- **Abuse limits.** SOAP bodies are capped at 64 KB; SSDP packets at 2 KB; at most
  32 answers wait out their delay at once (a flood of searches, or searches forged
  to look like they came from a victim, cannot make the server send unbounded
  traffic); the file system is never reached through a request value.
- **Eventing is accepted but never delivered.** `SUBSCRIBE` and `UNSUBSCRIBE`
  succeed, because TVs refuse a server whose event URL fails, but nothing is sent to
  the callback address. Delivering would mean the server connects to any address a
  stranger on the network names. `SystemUpdateID` changes with the library, which
  a TV reads when it browses.

## Consequences
- **Not tried on a real TV.** What is verified: the real SSDP answer to a real
  multicast search from another process on this Mac, and a test that plays a TV's
  part (search, LOCATION, description, Browse) over the real network. TV
  firmwares differ in what they tolerate in DIDL-Lite and `protocolInfo`; expect
  to tune once one is available (ROADMAP).
- **No search, no sorting, no thumbnails or subtitles in the listing.** The
  capabilities say so honestly (`GetSearchCapabilities` and `GetSortCapabilities`
  are empty). Subtitle `<res>` entries differ per vendor (Samsung, LG, Sony).
- Formats are whatever the TV can decode: DLNA does not transcode here. A video
  the TV cannot play still needs the HLS route or another player.
- SSDP needs multicast: a Wi-Fi network with client isolation, or a firewall
  blocking UDP 1900, hides the server from TVs; the address still works.
- On a computer with several networks, each searcher gets the address on its own.
  A searcher on none gets the first.

## Alternatives rejected
- **DLNA always on.** Open library by default, to a network the user never vetted.
- **Make DLNA work with the token** by putting `?t=…` in each file URL. The listing
  itself would still be unauthenticated, and the token would travel in clear to any
  device that asks.
- **A separate port for DLNA.** More to bind, more to firewall, and its own copy of
  the lifecycle; nothing needed it.
- **Deliver event notifications.** See above.
- **Open-source DLNA servers as a subprocess** (MiniDLNA, gmrender): a second program
  to install, configure and secure, with its own idea of the library.
