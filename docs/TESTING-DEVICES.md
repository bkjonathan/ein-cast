# Device test matrix

Every claim in the README should be true on a real device. This page records
what was actually tried. It is Phase 13 of [PLAN-v1.1.md](PLAN-v1.1.md).

## How to read it

Each cell holds one of:

| Mark | Meaning |
|---|---|
| ✅ | Worked. |
| ⚠️ | Worked with a problem. Put the issue number in the log below. |
| ❌ | Did not work. Put the issue number in the log below. |
| — | Does not apply to this device (for example, a phone has no UPnP list). |
| *(empty)* | Not tried yet. |

Rows marked **must** have to be filled in before v1.1.1 is tagged
(release gate 2). Failures go to GitHub issues, one per failure, and are either
fixed or listed under "Known limits" in the README.

## The matrix

Columns: **QR** connect by scanning the QR code · **MP4** play an MP4 ·
**MKV** play an MKV through the converted (HLS) stream · **Subs** a sidecar
`.srt` shows · **Seek** jumping forward and back · **Resume** reopening
continues where it stopped · **M3U** `/playlist.m3u` opens in the device's player ·
**DLNA** the device lists EinCast and browses the library.

| Device | Must | QR | MP4 | MKV | Subs | Seek | Resume | M3U | DLNA |
|---|---|---|---|---|---|---|---|---|---|
| iPhone, Safari | **must** | | | | | | | — | — |
| iPad, Safari | **must** | | | | | | | — | — |
| Android phone, Chrome | | | | | | | | — | — |
| Windows, Chrome | **must** | | | | | | | — | — |
| Windows, Edge | | | | | | | | — | — |
| macOS, Safari | **must** | | | | | | | — | — |
| macOS, Chrome | | | | | | | | — | — |
| Samsung or LG TV, built-in browser | | | | | | | | — | — |
| Samsung or LG TV, DLNA / media player app | | — | | — | | | — | — | |
| Android TV or Chromecast with Google TV | | | | | | | | | — |
| VLC desktop (UPnP list and opening the M3U) | **must** | — | | — | | | — | | |
| VLC mobile | | — | | — | | | — | | |
| Infuse | | — | | — | | | — | | |

"MKV" on a device that plays Matroska natively is still worth recording, but the
interesting case is a file the device cannot decode (HEVC in MKV is the usual
one).

## Test material

Keep the same small set of files for every run so results compare:

- `sample-h264.mp4`: H.264 + AAC, about 2 minutes.
- `sample-hevc.mkv`: HEVC + AAC in Matroska, about 2 minutes.
- `sample-h264.srt` next to the MP4, named `sample-h264.en.srt`.
- One folder two levels deep, so the DLNA browse has a tree.

## Run log

One line per run. Add a line, do not edit old ones.

| Date | EinCast version | Device and OS version | Network (Wi-Fi / tunnel) | Result summary | Issues |
|---|---|---|---|---|---|
| | | | | | |

## Other Phase 13 checks

These are not device rows but belong to the same reality check.

| Check | Where | Status |
|---|---|---|
| 13.4 Windows 11: install, start, firewall prompt, Free a port, Mica look, uninstall | real Windows machine | not run |
| 16 Profiles: pick Kids on an iPad (no PIN), Adults with a PIN on a phone; confirm the kids' iPad cannot see, search, stream, get a playlist entry or HLS for adult titles, and that "Continue watching" follows a profile between phone and TV browser | iPad, phone, TV browser | not run |
| 17 HEVC MKV with two audio languages and embedded subtitles, played from the matrix devices: the Audio and Subtitles menus list the tracks, the chosen language is heard, the chosen subtitle shows, switching audio mid-film resumes within a few seconds | Windows Chrome and Edge, Firefox, iPhone Safari, iPad Safari, a TV browser | macOS Chrome 156 and Firefox 158.0.1: **done** on a 40 s file (native, and hls.js). All others: not run |
| 17 hls.js on a TV browser: does the page play a converted MKV at all (Media Source Extensions vary), and does the Subtitles menu work with the remote | a TV browser | not run |
| 17 NVENC: the "video encoder failed" line is absent from the log and the conversion is faster than with Hardware acceleration off | Windows with an NVIDIA card | not run |
| 17 QuickSync: same, on an Intel iGPU; and a Windows computer with neither confirms the fallback to `libx264` (log line, video still plays) | Windows, Intel and neither | not run |
| 17 VideoToolbox on an Intel Mac and a long film (an hour or more): quality and speed with Hardware acceleration on and off | an Intel Mac | Apple silicon, 14 s and 40 s files: **done**; long film: not run |
| 17 Low disk: with the cache disk nearly full, pressing play on an MKV shows the "not enough free disk space" message and no ffmpeg runs | any machine, a small volume | page message **done** (stand-in server); real full disk: not run |
| 18 Burmese on the player page: open it with the browser language set to Burmese (or `?lang=my`); marks are not clipped, nothing is tofu, the grid, detail page, PIN picker and a playback hint read correctly | iPhone Safari, Windows Chrome, a TV browser | macOS Chrome 156 (headless) only |
| 18 Burmese sign-in pages: through a tunnel, the PIN page and, on the LAN with the token on and no token, the "access token needed" page, in Burmese | a phone | not run |
| 18 Burmese desktop window: every screen, in the Wails webview, for clipped stacked marks and cut-off buttons | macOS and Windows | not run |
| 18.3 A Burmese speaker reads `my.json` (window), `web/locales/my.json` (player) and `README.my.md`, with the checklist in TRANSLATING.md | a person | not done |
| 17 Embedded subtitle on a network share: the first read of a big MKV's subtitle (seconds to a minute) and that the picker is usable meanwhile | a NAS or SMB share | not run |
| 16 Smart TV with profiles: a library marked "not shown to TVs" does not appear in the TV's browse list or play by a remembered ID | a real TV or VLC's UPnP list | not run |
| 14.6 Keep awake: `pmset -g assertions` shows `caffeinate` while sharing (macOS); `powercfg /requests` shows EinCast (Windows); gone after Stop | real machines | macOS: not run on a Mac laptop; Windows: not run |
| 14.7 Windows tray icon: Open / Quit, close-to-tray, relaunch shows the window | real Windows machine | not run |
| 14.7 Open at login: LaunchAgent after a real macOS sign-in; `Run` key after a real Windows sign-in; starts hidden only with hide-on-close | real machines | not run |
| 14.4 First-run wizard: stopwatch from install to first video, three people, both systems | real people | not run |
| 13.5 Tailscale, second tailnet device | real tailnet | not run |
| 13.5 Cloudflare quick tunnel (banner parser, `~/.cloudflared/config.yml`) | real `cloudflared` | not run |
| 13.5 Cloudflare named tunnel | real token | not run |
| 13.5 ngrok | real authtoken | not run |
| 13.6 Visual pass of every screen, light and dark | running window | not run |
| 13.7 Large-library check, 50,000 files | `EINCAST_SCALE_FILES=50000 go test ./internal/adapters/fsscan ./internal/adapters/fswatch -run TestScale -v` | **done**, numbers in [ROADMAP.md](ROADMAP.md) |
