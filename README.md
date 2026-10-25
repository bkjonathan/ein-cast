# EinCast

*Your home media, on every screen.* "Ein" (အိမ်) is Burmese for *home*.

A desktop app (macOS + Windows) that serves a folder of media over your LAN so TVs, iPads and phones can play it. Built with Go, Wails v2 and React.

> Status: **Phase 5 — connect.** Choose a media folder, start the server, and open it from a TV, iPad or phone by scanning the QR code or typing the address; the page streams video, music and photos, and an M3U playlist works in VLC and Infuse. The Logs view shows everything the app does. **There is no password yet** (the access token is Phase 6), so anyone on your Wi-Fi can open the library while the server runs. Features land phase by phase (see [docs/ROADMAP.md](docs/ROADMAP.md)).

## Requirements

- Go 1.26+
- Node 20+ and npm
- [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation) (`wails doctor` should report success)

## Run, build, test

```sh
wails dev                       # live-reloading dev window
wails build -ldflags "-X main.version=0.1.0"   # → build/bin/eincast.app

go test ./...                   # Go tests
scripts/check-deps.sh           # hexagonal dependency rules
cd frontend && npm run typecheck && npm test
```

## Try the API

Choose a folder, press **Start server**, then open the **Connect a device** card. On the same machine, or any device on your network, use the address it shows (default port 8765):

```sh
curl http://127.0.0.1:8765/api/health
curl http://127.0.0.1:8765/api/library
curl -H "Range: bytes=0-99" -i http://127.0.0.1:8765/stream/<id>
curl http://127.0.0.1:8765/playlist.m3u      # open in VLC / Infuse
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full route table.

## Folder map

| Path | What lives there |
|---|---|
| `main.go` | Composition root: builds dependencies, configures the Wails window. No logic. |
| `internal/domain` | Pure types and rules. Imports nothing. |
| `internal/app` | Use cases, talking to the outside through small interfaces (ports). |
| `internal/adapters` | Implementations of those ports (filesystem, HTTP, logging, ...). |
| `internal/bridge` | The only package that may import Wails. Facade + DTOs for the frontend. |
| `frontend/src/app` | Layout shell: title bar, sidebar, view switching. |
| `frontend/src/features` | One folder per feature slice. |
| `frontend/src/shared` | Bridge to Go, glass UI kit, motion presets, helpers. |
| `scripts/` | Repo tooling (`check-deps.sh`). |
| `docs/` | Architecture, roadmap, and decision records (`docs/adr`). |
