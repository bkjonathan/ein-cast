<p align="center"><img src="docs/assets/logo.png" alt="EinCast logo" width="128"></p>

# EinCast

*Your home media, on every screen.* "Ein" (အိမ်) is Burmese for *home*.

[![CI](https://github.com/bkjonathan/ein-cast/actions/workflows/ci.yml/badge.svg)](https://github.com/bkjonathan/ein-cast/actions/workflows/ci.yml)
[![Licence: MIT](https://img.shields.io/badge/licence-MIT-blue.svg)](LICENSE)
[![Latest release](https://img.shields.io/github/v/release/bkjonathan/ein-cast)](https://github.com/bkjonathan/ein-cast/releases/latest)

**မြန်မာဘာသာ:** [README.my.md](README.my.md) (install, first run, troubleshooting; an unreviewed draft).

**EinCast turns a folder on your Mac or Windows PC into a media server for the TVs, tablets and phones in your home.** Pick a folder, scan a QR code on any screen, and play. No accounts, no cloud, nothing to install on the TV.

<!-- TODO (Phase 20.6 and 20.7): demo GIF (about 20 seconds) and screenshots: light, dark, Burmese, the player on a phone and on a TV. Put them in docs/assets/screenshots/ and show them here. -->

## Download

Get the latest build from the **[Releases page](https://github.com/bkjonathan/ein-cast/releases/latest)**:

| System | File |
|---|---|
| macOS (Intel and Apple silicon) | `eincast-<version>-macos-universal.zip` |
| Windows 10/11 | `eincast-<version>-windows-amd64-installer.exe`, or `…-windows-amd64.exe` (portable, no install) |

Each release lists the SHA-256 of every file (`SHA256SUMS.txt`). EinCast can also tell you when a newer version exists; it never installs anything by itself.

**These builds are not code-signed yet**, so your system warns the first time:

- **macOS:** unzip, then run `xattr -dr com.apple.quarantine eincast.app` (or right-click, Open, the first time).
- **Windows:** SmartScreen warns; choose **More info, Run anyway**.

## Quick start

1. Open EinCast and **choose a media folder**. (A short wizard walks you through it.)
2. EinCast scans it and **starts sharing**. The Dashboard shows an address and a **QR code**.
3. On a phone, tablet or TV browser, **scan the code** (or type the address). The library opens.
4. Press play. Or open `…/playlist.m3u` in **VLC** or **Infuse**. Smart TVs can find it by themselves if you turn on *Smart TVs* in Settings.

Videos a device cannot play as they are (MKV, HEVC…) are converted on the fly if [ffmpeg](https://ffmpeg.org) is installed (`brew install ffmpeg`, `winget install ffmpeg`). Without it everything else still works. More in the [guide](docs/GUIDE.md).

### What you get

- Video, music and photos from any number of folders, in a page that works on every browser.
- Subtitles (`.srt`, `.vtt`, and text tracks inside MKV/MP4), several audio languages, and **continue watching** that follows a person from phone to TV.
- **Profiles** for a household: limit a profile to some libraries, kinds or folders, with an optional PIN.
- Titles, years, series and posters read from your file names and the files beside the video. Nothing is looked up online.
- Watch away from home through **Tailscale** (private) or a Cloudflare or ngrok tunnel, which always requires a PIN.
- The app and the player page in **English and Burmese**; a live log of everything the server does.

## Why not Jellyfin?

Often you should use Jellyfin or Plex. EinCast is for the household that wants *"play this folder on that TV, now"* with no accounts, no database and nothing to maintain.

| | EinCast | A media server (Jellyfin, Plex) |
|---|---|---|
| Setup | Pick a folder, scan a QR code | Install a server, set up users and libraries |
| Accounts | None | Server users (Jellyfin); a company account (Plex) |
| Catalogue with posters from the internet | No: names and local files only | Yes |
| Native apps on TVs | No: browser, DLNA, VLC, Infuse | Yes |
| Always on | While the app is open | Yes, on a NAS or spare computer |

The honest long version, including when to pick something else: **[docs/WHY-NOT-JELLYFIN.md](docs/WHY-NOT-JELLYFIN.md)**.

## Privacy and security

- **No accounts, no analytics, no phoning home.** The only request EinCast makes by itself is a once-a-day look at this project's public release list, which you can turn off. Tunnels connect only when you turn them on. Details, and how CI keeps it true: [docs/PRIVACY.md](docs/PRIVACY.md).
- A new install has the **access token on**: a device needs the link or QR code to open your library. Public access (a tunnel) is refused without a PIN. Devices can never ask for a file path, only an ID.
- Anyone on your Wi-Fi can open the library if you turn the token off or turn on *Smart TVs*; Settings says so beside each switch.
- Found a vulnerability? Please report it privately: [SECURITY.md](SECURITY.md).

## Known limits

- **Not code-signed yet** (see Download). Signing needs certificates; it is planned.
- **Young software.** Only some devices have been tried: [docs/TESTING-DEVICES.md](docs/TESTING-DEVICES.md) lists what has, and what has not. Please add yours.
- **It runs while the app is open.** EinCast can open at login and keep sharing with its window closed, but there is no headless or NAS mode yet.
- **macOS and Windows only.** No live TV, recording or plugins, on purpose.
- Remote viewing is limited by your home internet's *upload* speed, and Cloudflare's terms restrict heavy video through their proxy; Tailscale is the recommended way to watch away from home.
- The Burmese text is a draft waiting for a native reviewer ([docs/TRANSLATING.md](docs/TRANSLATING.md)). Old Zawgyi-encoded file names are shown as stored.

## Help and community

- Something wrong? Settings → **Help and feedback** → **Copy diagnostics**, then [open a bug report](https://github.com/bkjonathan/ein-cast/issues/new?template=bug_report.yml).
- Ideas and questions: [Discussions](https://github.com/bkjonathan/ein-cast/discussions) and [issues](https://github.com/bkjonathan/ein-cast/issues).
- Want to help? Read **[CONTRIBUTING.md](CONTRIBUTING.md)**. Translations, device reports and small fixes are all welcome. Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).

## For developers

Built with Go, [Wails v2](https://wails.io) and React, as ports and adapters ([architecture](docs/ARCHITECTURE.md), [decisions](docs/adr/), [roadmap and where things stand](docs/ROADMAP.md)).

Requirements: Go 1.26+, Node 20+ and npm, and the [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation) (`wails doctor` should report success).

```sh
wails dev                       # live-reloading dev window
wails build -ldflags "-X main.version=1.1.1"   # → build/bin/eincast.app

go test ./...                   # Go tests
scripts/check-deps.sh           # hexagonal dependency rules
scripts/check-network.sh        # outbound connections only where docs/PRIVACY.md says
scripts/check-cross.sh          # compiles for macOS and Windows; add --full for a complete Windows Wails build
scripts/make-icons.sh           # rebuilds every icon from build/logo.png (needs ImageMagick)
cd frontend && npm run typecheck && npm test
```

Releases: the git tag is the version. Push `v1.1.1-rc1` (a pre-release; the apps are unsigned) and the `release` workflow runs the tests, then builds and publishes the apps; the checklist is in [docs/RELEASING.md](docs/RELEASING.md).

| Path | What lives there |
|---|---|
| `main.go` | Composition root: builds dependencies, configures the Wails window. No logic. |
| `internal/domain` | Pure types and rules. Imports nothing. |
| `internal/app` | Use cases, talking to the outside through small interfaces (ports). |
| `internal/adapters` | Implementations of those ports (filesystem, HTTP, logging, ...). |
| `internal/bridge` | The only package that may import Wails. Facade + DTOs for the frontend. |
| `frontend/src` | `app` (shell), `features` (one folder per feature), `shared` (bridge to Go, glass UI kit, helpers). |
| `build/` | Wails build assets. `logo.png` is the master logo; the app icons are generated from it. |
| `scripts/` | Repo tooling. |
| `docs/` | Guide, architecture, roadmap, privacy, and decision records (`docs/adr`). |

## Licence

[MIT](LICENSE) © 2026 BK Thomas. Dependencies keep their own licences (MIT, BSD, ISC, Apache-2.0, and MPL-2.0 for `hashicorp/yamux`, which the ngrok SDK uses); the bundled hls.js is Apache-2.0 (`internal/adapters/httpapi/web/hls.js.LICENSE`).
