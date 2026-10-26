# 0008 — Public access requires authentication, enforced in the service

## Context
On the home network a guest or a compromised device is the threat, and an
optional access token ([ADR 0011](0011-lan-access-control.md)) is a fair answer.
Through a public tunnel the threat is anyone on the Internet. Quick-tunnel
addresses are random, but named-tunnel hostnames are listed in certificate
transparency logs and are found by scanners within minutes. If a public tunnel
ever carries an unauthenticated library, it will be read.

The brief is explicit: this must be enforced in code, not left to the UI.

## Decision
**1. The rule lives in `remote.Service.Enable`.** A provider whose `Public()` is
true is refused with `ErrPINRequired` unless a PIN is set. The check runs before
anything starts, and a test asserts the tunnel's `Start` is never called. The
PIN cannot be removed while a public tunnel is on (`ErrPublicActive`).

**2. The PIN is a long secret, stored as a hash.** At least 8 characters, at most
64. Stored as a salted scrypt hash in the OS credential store, never in the
settings file and never in logs (`app.Secret` prints `[redacted]` if one is
logged by mistake). If the credential store cannot be read, no PIN is set and
public access stays refused.

**3. A request is "tunnelled" by where it arrived, not by what it claims.** Tunnels
connect from this machine, so such requests arrive on loopback. A loopback
request counts as tunnelled when a public tunnel is up, or when it carries
forwarding headers (`CF-Connecting-IP`, `X-Forwarded-*`, `Forwarded`). A request
from any other address is never tunnelled, whatever headers it has: those are the
ones a neighbour on the Wi-Fi could forge. The cost of a wrong guess is that the
owner's own browser on `localhost` is asked for the PIN while a tunnel is up.

A private provider such as Tailscale is not tunnelled in this sense: its
requests arrive from tailnet addresses, so they follow the home-network rules
([ADR 0012](0012-tailscale-listens-on-the-tailnet-address.md)).

**4. Tunnelled requests accept two credentials and nothing else.**
- A **session cookie**, issued after a correct PIN. It is
  `<id>.<expiry>.<HMAC-SHA256>`, `HttpOnly`, `SameSite=Lax`, `Secure` when the
  tunnel reports https, valid for 24 hours. The id is also registered, so
  sessions can be counted and revoked.
- A **signed stream link** `/stream/{id}?exp=…&sig=…` for players that cannot hold
  cookies. The HMAC covers the media ID and the expiry, so a link opens one item
  until it expires (6 hours), and cannot be extended or moved to another item.
The LAN token is **not** accepted on a tunnelled request, so a token leaked
through a QR code does not open the public address. Static files are not exempt
either; only `POST /login` is open to a stranger.

**5. Keys are in memory and rotate.** Session and link keys are random, separate
from each other, and replaced whenever remote access is enabled or disabled or
the PIN changes. All sign-ins and links die together, and none survive a restart.

**6. Guessing is slowed twice.** A token bucket per client address (5 attempts,
one more per minute) and a global bucket across all addresses (30, one more per
10 seconds). An empty bucket refuses even the right PIN, with `429` and
`Retry-After`, so a lockout cannot be used to test guesses. A correct PIN refunds
its attempt; a refused request is not charged to the per-address bucket.

**7. The client address is trusted only from the tunnel.** `clientIP` reads
`CF-Connecting-IP`, else the last `X-Forwarded-For` entry, and only when the TCP
peer is loopback. The last entry, not the first, because the tunnel's edge appends
what it saw to whatever the client sent; this holds for exactly one trusted hop.

**8. The Host check learns the tunnel's name.** The DNS-rebinding guard from
ADR 0011 also accepts the address of the tunnel that is online, and nothing else.

**9. The sign-in form refuses other sites.** `POST /login` rejects a cross-site
`Sec-Fetch-Site` or an `Origin` that is neither the Host nor the tunnel. The page
sends a CSP that allows no script.

## Consequences
- Turning on a public provider is a two-step action (set a PIN, then enable),
  and the UI has to explain why rather than only disable a button.
- Signing in costs about 100 ms of CPU and 32 MiB per attempt (scrypt). That is
  deliberate and bounded by the rate limits.
- Restarting the app, toggling remote access or changing the PIN signs everyone
  out. For a personal library that is the right default.
- A signed link in a playlist is a bearer credential for one item for six hours.
  It will appear in a player's history; the expiry and the per-item scope are what
  limit the damage.
- The forwarding-header rules assume one proxy hop. A provider that chains
  proxies needs this revisited when it is added.

## Alternatives rejected
- **Enforce only in the UI** (disable the button): any other caller of the
  bridge, or a future UI bug, would bypass it.
- **Reuse the LAN token for public access**: it is placed in QR codes and
  playlists on the home network, so its exposure is much wider.
- **Stateless signed cookies only**: simpler, but sessions could not be counted
  for the "remote sessions" display or revoked one at a time.
- **A short numeric PIN**: friendlier to type, and guessable by a distributed
  attacker that rate limits per address cannot see.
- **Trusting `X-Forwarded-For` from anyone**: trivially forged, which would let
  an attacker choose their own rate-limit bucket.
- **Treating all loopback traffic as trusted**: the tunnel is loopback traffic.
