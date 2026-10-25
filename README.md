# EinCast

*Your home media, on every screen.* "Ein" (အိမ်) is Burmese for *home*.

A desktop app (macOS + Windows) that serves a folder of media over your LAN so TVs, iPads and phones can play it. Built with Go, Wails v2 and React.

> Status: **Phase 2 — library.** You can choose a media folder and EinCast scans it and shows what it found. The HTTP server comes next; features land phase by phase (see [docs/ROADMAP.md](docs/ROADMAP.md)).

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
