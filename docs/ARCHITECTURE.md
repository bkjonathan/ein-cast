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

## What exists today (through Phase 4)

| Layer | Contents |
|---|---|
| `domain/media` | `Item`, `ID` (hash of relative path), `Kind` + extension detection, `Stats`, sentinel errors |
| `domain/server` | `State`, the transition table, and `Machine` (atomic check-and-set) |
| `app` | Ports (`Scanner`, `LibraryRepo`, `EventPublisher`, `WebServer`, `RunningServer`, `LogStream`), `LogEntry`, sealed `Event` type |
| `app/library` | `Service`: `ScanFolder`, `Summary`, `Root`, `List`, `Get` |
| `app/streaming` | `Config` + functional options, `Service`: `Start`, `Stop`, `Status`, crash watcher |
| `adapters/fsscan` | `filepath.WalkDir` scanner (skips hidden, system files, symlinks) |
| `adapters/memrepo` | `sync.RWMutex`-guarded snapshot repository |
| `adapters/httpapi` | Server lifecycle (errgroup), route table, middleware chain, handlers |
| `adapters/logbus` | `Handler` (slog decorator), `Bus` (non-blocking pub/sub), ring buffer |
| `bridge` | `App` facade, `Emitter` (event → Wails runtime), log forwarder, DTOs, startup/shutdown hooks |

### HTTP API

| Route | Response |
|---|---|
| `GET /` | Placeholder landing page (the embedded player arrives in the Connect phase) |
| `GET /api/health` | `{"status":"ok","version":"…","name":"…"}` |
| `GET /api/library` | `[{id, name, kind, size, modTime}]` (never a path) |
| `GET /api/library/{id}` | One item, or 404 |
| `GET /stream/{id}` | File bytes via `http.ServeContent`: `Range` → `206`, `HEAD`, correct `Content-Type` |
| `GET /playlist.m3u` | Extended M3U of video + audio with absolute URLs built from the request `Host` |

Errors always look like `{"error":{"code":"not_found","message":"…"}}`.
Middleware, outermost first: request ID → `Server: EinCast/<version>` → access
log → panic recovery → LAN-only CORS.

The server binds `127.0.0.1` only for now. The Connect phase adds the LAN
interface addresses (never `0.0.0.0`).

### Flow: a log line reaches the screen

```mermaid
sequenceDiagram
    participant Code as any component (slog)
    participant H as logbus.Handler
    participant Con as console handler
    participant Bus as logbus.Bus
    participant Fwd as bridge.forwardLogs
    participant UI as React (useLogSync)

    Code->>H: log.Info("request", attrs…)
    H->>Bus: Publish(entry)
    Bus->>Bus: stamp Seq, push to ring
    Bus-->>Fwd: non-blocking send (dropped if its buffer is full)
    H->>Con: Handle(record)
    Fwd-->>UI: event "log:line"
    UI->>UI: batch 50 ms → store.append (ignores seq already seen)
    Note over UI: on load: GetLogHistory() → store.hydrate (fills only older seq)
```

The logger never waits for the UI. If the UI cannot keep up, the bus drops
entries for that subscriber and `Seq` shows the gap. `forwardLogs` is owned by
the app context and unsubscribes when it ends.

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

The UI changes screens because of the *event*, not because the command
resolved: Go is the source of truth. On a webview reload the UI re-reads state
with `GetLibrarySummary()`.

### Flow: user clicks Start

```mermaid
sequenceDiagram
    actor User
    participant UI as React (ServerPanel)
    participant Bridge as bridge.App
    participant Svc as streaming.Service
    participant SM as server.Machine
    participant Web as httpapi.Server
    participant Emit as bridge.Emitter

    User->>UI: click "Start server"
    UI->>Bridge: StartServer()
    Bridge->>Svc: Start(appCtx, config)
    Svc->>SM: Transition(Starting)
    Svc-->>Emit: Publish(ServerStateChanged: starting)
    Emit-->>UI: "server:state" → pill shows Starting…
    Svc->>Web: Start(ctx, ServeConfig)
    Web->>Web: net.Listen (port conflicts fail here)
    Web-->>Svc: RunningServer (serving in the background)
    Svc->>SM: Transition(Running)
    Svc-->>Emit: Publish(ServerStateChanged: running + addr)
    Emit-->>UI: "server:state" → pill shows Running
    Svc->>Svc: go watch(run) until run.Done()
```

If `net.Listen` fails, the service does `Fail(err)` instead and the UI shows the
reason with a "Try again" button. A server that dies later is caught by the
watcher, which moves Running → Failed.

### Flow: an iPad requests /stream/{id}

```mermaid
sequenceDiagram
    participant iPad
    participant MW as Middleware chain
    participant H as handleStream
    participant Lib as library.Service
    participant FS as filesystem

    iPad->>MW: GET /stream/ab12… (Range: bytes=0-)
    MW->>MW: request ID, Server header, start access log
    MW->>H: route match "GET /stream/{id}"
    H->>Lib: Get(id)
    Lib-->>H: Item (absolute path stays server-side)
    H->>H: resolveInside(root, path): symlinks resolved, must stay in root
    H->>FS: open file
    H->>iPad: http.ServeContent → 206 Partial Content
    MW->>MW: access log: method, path, status, bytes, duration, client IP
```

An unknown ID, a path-like ID, a deleted file, or a file that now resolves
outside the library root all end in the same JSON `404`, with the real reason
only in the log.

## Frontend

Feature-sliced: `app/` (shell), `features/` (one folder per feature),
`shared/` (bridge, glass UI, motion). Only `shared/bridge` imports the
generated `wailsjs/` code (see [ADR 0006](adr/0006-bridge-only-wails-dependency.md)).
Theming is CSS-only via `prefers-color-scheme`.
