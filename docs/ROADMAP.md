# Roadmap

Build only the current phase. When a later feature needs a seam, note it here
instead of adding it early.

| Version | Features | Status |
|---|---|---|
| **v0.1 MVP** | Pick folder, scan, LAN HTTP server, device player page, M3U playlist, QR connect, live logs, glass UI | In progress — Phase 1 done |
| v0.2 | Settings persistence, rescan + file watcher (`fsnotify`), library search/filter, thumbnails (ffmpeg), dark/light toggle | |
| v0.3 Remote access | Tunnel providers (Tailscale, Cloudflare quick/named, ngrok), PIN auth, signed stream links, public-access badge, remote session list | |
| v0.4 Playback | ffmpeg HLS transcoding, subtitles (.srt → WebVTT), resume position per device | |
| v0.5 TV | DLNA/UPnP, mDNS `eincast.local` | |
| v1.0 | Multiple libraries, user profiles, auto-update, signed + notarized builds | |

## v0.1 phases

1. **Skeleton** — done
2. Domain + library
3. Server lifecycle
4. Log pipeline
5. Connect
6. Hardening
7. Remote access (v0.3)

## Seams needed later

- `Transcoder` port (v0.4) — not added until transcoding starts.
- Windows title bar: Phase 1 keeps the native frame on Windows (Mica backdrop only). A frameless window needs custom min/max/close controls; decide in Phase 6 when the cross-platform build is checked.
- Manual dark/light toggle (v0.2) — theming is CSS-only until then.
