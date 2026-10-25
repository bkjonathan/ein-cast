# Architecture

EinCast is a hexagonal (ports & adapters) application. See
[ADR 0001](adr/0001-hexagonal-architecture.md) for why.

## Layers

```mermaid
flowchart LR
    main["main.go<br/>(composition root)"]
    bridge["internal/bridge<br/>Wails facade + DTOs"]
    app["internal/app<br/>use cases + ports"]
    domain["internal/domain<br/>pure types & rules"]
    adapters["internal/adapters<br/>fs, http, logbus, tunnel..."]
    ui["frontend/<br/>React"]

    ui -- "Wails bindings / events" --> bridge
    bridge --> app
    app --> domain
    adapters -- "implement ports" --> app
    adapters --> domain
    main -. "wires" .-> bridge
    main -. "wires" .-> adapters
```

Arrows point in the direction of the import. `domain` imports nothing; `app`
never imports Wails, `net/http` or `os`. `scripts/check-deps.sh` fails if that
changes.

## What exists today (through Phase 2)

| Layer | Contents |
|---|---|
| `domain/media` | `Item`, `ID` (hash of relative path), `Kind` + extension detection, `Stats`, sentinel errors |
| `app` | Ports (`Scanner`, `LibraryRepo`, `EventPublisher`), sealed `Event` type, `LibraryScanned` |
| `app/library` | `Service`: `ScanFolder`, `Summary`, `List`, `Get` |
| `adapters/fsscan` | `filepath.WalkDir` scanner (skips hidden, system files, symlinks) |
| `adapters/memrepo` | `sync.RWMutex`-guarded snapshot repository |
| `bridge` | `App` facade (`AppInfo`, `ChooseFolder`, `GetLibrarySummary`), `Emitter` (event → Wails runtime), DTOs |

### Flow: user chooses a folder

```mermaid
sequenceDiagram
    actor User
    participant UI as React (useChooseFolder)
    participant Bridge as bridge.App
    participant Svc as library.Service
    participant Scan as fsscan.Scanner
    participant Repo as memrepo.Repo
    participant Emit as bridge.Emitter

    User->>UI: click "Choose Media Folder"
    UI->>Bridge: ChooseFolder()
    Bridge->>Bridge: native directory dialog
    Bridge->>Svc: ScanFolder(ctx, path)
    Svc->>Scan: Scan(ctx, path)
    Scan-->>Svc: []media.Item
    Svc->>Repo: Replace(path, items)
    Svc->>Emit: Publish(LibraryScanned)
    Emit-->>UI: event "library:scanned" (DTO)
    UI->>UI: store.applyScanned → onboarding gives way to the shell
    Svc-->>Bridge: summary
    Bridge-->>UI: resolves (or rejects with the error)
```

Note the UI changes screens because of the *event*, not because the command
resolved: Go is the source of truth. On a webview reload the UI re-reads state
with `GetLibrarySummary()`.

The sequence diagrams for "user clicks Start" and "iPad requests /stream/{id}"
are added in Phase 3, when there is a flow to draw.

## Frontend

Feature-sliced: `app/` (shell), `features/` (one folder per feature),
`shared/` (bridge, glass UI, motion). Only `shared/bridge` imports the
generated `wailsjs/` code (see [ADR 0006](adr/0006-bridge-only-wails-dependency.md)).
Theming is CSS-only via `prefers-color-scheme`.
