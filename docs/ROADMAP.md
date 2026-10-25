# Roadmap

Build only the current phase. When a later feature needs a seam, note it here
instead of adding it early.

| Version | Features | Status |
|---|---|---|
| **v0.1 MVP** | Pick folder, scan, LAN HTTP server, device player page, M3U playlist, QR connect, live logs, glass UI | In progress — Phases 1–6 done; Phase 7 is the remote-access phase (v0.3) |
| v0.2 | ~~Settings persistence~~ and ~~manual rescan~~ (pulled forward into Phase 6), file watcher (`fsnotify`), library search/filter, thumbnails (ffmpeg), dark/light toggle | Partly done |
| v0.3 Remote access | Tunnel providers (Tailscale, Cloudflare quick/named, ngrok), PIN auth, signed stream links, public-access badge, remote session list | |
| v0.4 Playback | ffmpeg HLS transcoding, subtitles (.srt → WebVTT), resume position per device | |
| v0.5 TV | DLNA/UPnP, mDNS `eincast.local` | |
| v1.0 | Multiple libraries, user profiles, auto-update, signed + notarized builds | |

## v0.1 phases

1. **Skeleton** — done
2. **Domain + library** — done
3. **Server lifecycle** — done
4. **Log pipeline** — done
5. **Connect** — done
6. **Hardening** — done
7. Remote access (v0.3)

## Seams needed later

- **Secret redaction (Phase 7):** nothing secret is logged today. When tunnel tokens and PINs exist, add a redacting `slog.Attr` helper and keep secrets out of log calls; `logbus.Handler` deliberately does not try to scrub text after the fact.
- **Log gap marker (optional):** `Seq` already reveals dropped lines; the viewer does not yet render a "N lines dropped" row.
- **Log level control (Phase 6):** the console handler is fixed at Info and the UI sees Debug and up. A setting can raise or lower the console level without touching `logbus`.

- **Rate limiting (Phase 7):** the LAN token is 192 bits and needs none; the PIN for public tunnels does (token bucket per IP, lockout). The `requireAccess` middleware is where it goes.
- **Short-lived signed stream links (Phase 7):** playlists currently embed the long-lived token so VLC can play. Signed, expiring `?sig=&exp=` links for tunnels replace that for public access.
- **Token expiry:** the cookie lasts 30 days and the token until replaced. A "sign out all devices" action is the existing token rotation.
- **Windows file attributes:** the scanner still skips by name only (dot-files and known system names), not the Windows "hidden" attribute.
- **Cross-platform tests:** `scripts/check-cross.sh` proves compilation for macOS and Windows from one machine; behaviour on Windows is checked only by `.github/workflows/ci.yml`, which has not run yet because nothing is pushed.
- **Network changes:** the bound addresses are fixed at start; a new Wi-Fi/Ethernet address needs a restart. A watcher (or a "refresh addresses" action) can come with Settings.
- **Tailscale and mDNS addresses (Phase 7 / v0.5):** `netinfo` deliberately skips the Tailscale interface (CGNAT range); the tailscale provider reports its own URL. `eincast.local` arrives with mDNS.
- **Player page on old TVs:** `app.js` uses `fetch`, `async/await`, template-free string concatenation and `replaceChildren`. A very old smart-TV browser may need a lighter fallback; test on real devices before v1.0.
- **Settings (Phase 6):** `StartServer()` uses `streaming.NewConfig()` defaults (port 8765, name "EinCast"). Port and name come from persisted settings later; `Config` and its options are already the seam.
- **Player page (Phase 5):** `GET /` serves a placeholder string; the real page goes in `adapters/httpapi/web` via `embed.FS`.
- **Forwarded headers (Phase 7):** the access log uses the TCP peer address and ignores `X-Forwarded-For` / `CF-Connecting-IP` until requests are known to come from a tunnel's loopback connection.
- **Tailscale CORS (Phase 7):** `isLANOrigin` does not yet accept the 100.64.0.0/10 range.

- `Transcoder` port (v0.4) — not added until transcoding starts.
- Windows title bar: Phase 1 keeps the native frame on Windows (Mica backdrop only). A frameless window needs custom min/max/close controls; decide in Phase 6 when the cross-platform build is checked.
- Multiple libraries (v1.0): `media.NewID` hashes only the relative path, so the same relative path in two roots collides. Add a library identifier to the hash input then (ADR 0005).
- Windows "hidden" file attribute: the scanner skips dot-prefixed and well-known system names only. Honouring the Windows hidden attribute needs a build-tagged helper; decide in Phase 6.
- Folder drag-and-drop onto the onboarding card (Wails `EnableFileDrop`): the button covers v0.1.
- Manual dark/light toggle (v0.2) — theming is CSS-only until then.
