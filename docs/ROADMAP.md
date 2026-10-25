# Roadmap

Build only the current phase. When a later feature needs a seam, note it here
instead of adding it early.

| Version | Features | Status |
|---|---|---|
| **v0.1 MVP** | Pick folder, scan, LAN HTTP server, device player page, M3U playlist, QR connect, live logs, glass UI | In progress — Phases 1–2 done |
| v0.2 | Settings persistence, rescan + file watcher (`fsnotify`), library search/filter, thumbnails (ffmpeg), dark/light toggle | |
| v0.3 Remote access | Tunnel providers (Tailscale, Cloudflare quick/named, ngrok), PIN auth, signed stream links, public-access badge, remote session list | |
| v0.4 Playback | ffmpeg HLS transcoding, subtitles (.srt → WebVTT), resume position per device | |
| v0.5 TV | DLNA/UPnP, mDNS `eincast.local` | |
| v1.0 | Multiple libraries, user profiles, auto-update, signed + notarized builds | |

## v0.1 phases

1. **Skeleton** — done
2. **Domain + library** — done
3. Server lifecycle
4. Log pipeline
5. Connect
6. Hardening
7. Remote access (v0.3)

## Seams needed later

- `Transcoder` port (v0.4) — not added until transcoding starts.
- Windows title bar: Phase 1 keeps the native frame on Windows (Mica backdrop only). A frameless window needs custom min/max/close controls; decide in Phase 6 when the cross-platform build is checked.
- Multiple libraries (v1.0): `media.NewID` hashes only the relative path, so the same relative path in two roots collides. Add a library identifier to the hash input then (ADR 0005).
- Windows "hidden" file attribute: the scanner skips dot-prefixed and well-known system names only. Honouring the Windows hidden attribute needs a build-tagged helper; decide in Phase 6.
- Folder drag-and-drop onto the onboarding card (Wails `EnableFileDrop`): the button covers v0.1.
- Manual dark/light toggle (v0.2) — theming is CSS-only until then.
