# 0011 — Access control on the LAN: token, Host check, no secrets in logs

## Context
Since the Connect phase the server listens on the local network (ADR 0009), so
anyone on the same Wi-Fi can open the library. Two further threats exist even on
a trusted home network:

1. **Guests and compromised devices** on the same network.
2. **DNS rebinding.** A malicious website the user visits can make its own
   domain resolve to this machine's LAN address. The browser then treats the
   server as "the same site" and lets the page read the library.

## Decision
Optional **access token**, always enforced in the HTTP middleware, plus an
unconditional **Host check**.

- **Token (opt-in in Settings).** 192 random bits (`crypto/rand`, URL-safe
  base64), compared in constant time. `Verify` fails closed: an empty or missing
  token never matches while protection is on. A token is accepted from
  `Authorization: Bearer`, the `?t=` query parameter, or the `eincast_access`
  cookie.
- **QR flow.** The QR code encodes `/?t=…`. The server sets an `HttpOnly`,
  `SameSite=Lax` cookie and redirects to `/`, so the token does not remain in
  the address bar or history. After that, requests carry only the cookie.
- **Players without cookies** (VLC, Infuse, TVs) get the token in each playlist
  entry (`/stream/{id}?t=…`), taken from the token the playlist request itself
  authenticated with.
- **Everything is protected except** `/static/*` (the player page's own files,
  which hold nothing private). The 401 for a browser is a short instruction
  page; for everything else it is the standard JSON error. The message is the
  same whether the token was missing or wrong.
- **Host check, always on.** The `Host` header must be an IP address, a
  single-label name (`localhost`, `my-pc`) or an mDNS name (`*.local`). A public
  domain name can never satisfy that, so a rebinding page is refused with 403
  before any data is read, even if it somehow had the token.
- **CORS** answers preflights before authentication (a preflight cannot carry
  credentials) and only for LAN origins.
- **Logging.** The access log records the path, never the query string. A test
  sends tokens in the query, a cookie and a header, then asserts none appears in
  the logs. Refusals are logged with who asked and why, not what they sent.
- The setting takes effect immediately and rotating the token locks out devices
  holding the old one; the UI asks for confirmation first.

## Consequences
- Turning the token on is a one-click change that closes the LAN exposure; the
  Settings screen and README say the default is off.
- Links and QR codes are credentials. The Connect card says so, and the token can
  be replaced at any time.
- Tokens in URLs can end up in a player's own history or a shared screenshot.
  That is the price of working with players that cannot hold cookies; the
  `no-referrer` header stops them leaking to other sites.
- There is no per-IP rate limiting yet. A 192-bit token cannot be guessed at LAN
  speeds, so this matters for the PIN-based public access in the remote phase,
  not here.

## Alternatives rejected
- **HTTP Basic auth**: players and TVs handle it inconsistently, and the browser
  login dialog is a poor experience.
- **Short PIN for the LAN token**: friendlier to type, but guessable; PINs belong
  behind the rate limiter planned for public tunnels.
- **Always-on token with no off switch**: safest default, but a home network with
  one family is the main use case, and a locked TV nobody can unlock is worse.
- **Skipping the Host check because the token exists**: with the token off (the
  default) the Host check is the only thing standing between a hostile web page
  and the library.
