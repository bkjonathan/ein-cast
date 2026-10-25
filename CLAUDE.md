# Build Prompt: "EinCast" — Local Media Server (Wails + Go + React)

> Paste this whole file into your coding agent (or save it as `CLAUDE.md` in the repo root).

**App name: EinCast**. "Ein" (အိမ်) is Burmese for *home*, "Cast" is casting media to screens: *your home media, on every screen.*
- Display name: `EinCast` · Binary/module: `eincast` · Bundle ID: `com.bkthomas.eincast` · mDNS host: `eincast.local`
- Tagline: "Your home media, on every screen."
- Use the name consistently in window title, about screen, device player page, HTTP `Server` header (`EinCast/<version>`), and logs.

---

## 0. Role and goal

You are a senior Go + TypeScript engineer building a production-quality desktop app with me.
I am a senior backend engineer (Node/NestJS, learning Go deeply) and **I am using this project to learn**.
So your code must be the kind a staff engineer would approve: clear, idiomatic, tested, and boring in the good way.

**Senior ≠ complicated.** Use a design pattern only when it solves a real problem in this app, and record why in an ADR (see §9). If a simpler solution works, use it.

### What the app does
1. Desktop app for **macOS and Windows**.
2. On launch, the user picks a media folder (native directory dialog).
3. The app scans the folder and starts an **HTTP server on the LAN**.
4. The app shows the **connection URL + QR code** so TVs, iPads and phones can connect.
5. The app shows a **live, filterable log** of everything the server does (requests, scans, errors).
6. Clients get: a minimal player page, a JSON API, and an **M3U playlist** (so VLC / Infuse / TV players can open it directly).
7. **Remote access (optional):** the user can switch on a tunnel (Cloudflare Tunnel, ngrok, or Tailscale) to reach their library from outside the home network. See §4b.

---

## 1. Tech stack (fixed)

**Backend**
- Latest stable Go
- Wails v2 (`wails init -n eincast -t react-ts`)
- Standard library HTTP: `net/http` with Go 1.22+ `ServeMux` method/path patterns. **No chi, no gin.**
- `log/slog` for structured logging
- `golang.org/x/sync/errgroup` for lifecycle management
- Tests: standard `testing` package, table-driven, hand-written fakes (no mocking frameworks)

**Frontend**
- React + TypeScript (strict mode), upgrade the template's Vite to the latest version
- Tailwind CSS v4 via `@tailwindcss/vite`
- shadcn/ui (components copied into the repo, then restyled for glass)
- `motion` (formerly Framer Motion), imported from `motion/react`
- `lucide-react` icons
- `zustand` for client state
- `@tanstack/react-virtual` for the log list
- `qrcode.react` for the QR code

---

## 2. Architecture: hexagonal (ports & adapters)

Core rule: **the domain and application layers never import Wails, `net/http`, or the filesystem directly.** They depend on small interfaces. Adapters implement those interfaces. `main.go` wires everything together (composition root).

```
eincast/
├── main.go                      # Composition root ONLY: build deps, configure Wails, run
├── wails.json
├── internal/
│   ├── domain/                  # Pure types + rules. Zero external deps.
│   │   ├── media/
│   │   │   ├── item.go          # MediaItem, MediaID, Kind (Video/Audio/Image)
│   │   │   ├── kind.go          # Kind detection from extension (strategy table)
│   │   │   └── errors.go        # ErrNotFound, ErrUnsupported (sentinel errors)
│   │   └── server/
│   │       ├── state.go         # State machine: Idle → Starting → Running → Stopping → Idle / Failed
│   │       └── state_test.go    # Every valid + invalid transition tested
│   │
│   ├── app/                     # Use cases. Orchestrates domain via ports.
│   │   ├── ports.go             # Interfaces the app NEEDS (Scanner, LibraryRepo, EventPublisher, NetInfo)
│   │   ├── library/
│   │   │   ├── service.go       # ScanFolder(ctx, path), List(), Get(id)
│   │   │   └── service_test.go
│   │   ├── streaming/
│   │   │   ├── service.go       # Start(ctx, Config), Stop(ctx), Status()
│   │   │   ├── config.go        # Config + functional options (WithPort, WithName, ...)
│   │   │   └── service_test.go
│   │   └── remote/
│   │       ├── service.go       # Enable(ctx, provider), Disable(ctx), Status(); enforces auth-on-public rule
│   │       └── service_test.go
│   │
│   ├── adapters/                # Implementations of ports
│   │   ├── fsscan/              # filepath.WalkDir scanner, skips hidden/system files
│   │   ├── memrepo/             # In-memory library repository (sync.RWMutex)
│   │   ├── httpapi/
│   │   │   ├── server.go        # http.Server lifecycle, graceful shutdown
│   │   │   ├── routes.go        # Route table in ONE place
│   │   │   ├── middleware.go    # RequestID, AccessLog, Recover, CORS(LAN)
│   │   │   ├── handlers_library.go
│   │   │   ├── handlers_stream.go   # http.ServeContent → free Range/seek support
│   │   │   ├── handlers_playlist.go # GET /playlist.m3u
│   │   │   └── web/             # embed.FS minimal player page for devices
│   │   ├── netinfo/             # Detect LAN IPv4 (skip loopback, docker, VPN ifaces)
│   │   ├── tunnel/
│   │   │   ├── provider.go      # Registry of providers (strategy), keyed by ID
│   │   │   ├── cloudflared/     # Supervises the cloudflared binary (quick tunnel + named tunnel w/ token)
│   │   │   ├── ngrok/           # Embedded via the ngrok-go SDK (no external binary)
│   │   │   ├── tailscale/       # Detects a running Tailscale node, exposes the tailnet URL
│   │   │   └── procsup/         # Generic subprocess supervisor: start, parse stdout, restart w/ backoff, kill on ctx cancel
│   │   ├── secrets/             # OS keychain (macOS Keychain / Windows Credential Manager) for tunnel tokens
│   │   └── logbus/
│   │       ├── handler.go       # Custom slog.Handler (decorator) → fans out records
│   │       ├── ringbuffer.go    # Fixed-size buffer: last N log lines for late subscribers
│   │       └── bus.go           # Pub/sub with non-blocking send (slow subscriber never blocks server)
│   │
│   └── bridge/                  # The ONLY package that imports Wails runtime
│       ├── app.go               # Wails-bound facade: methods the frontend can call
│       ├── dto.go               # JSON-safe DTOs (never expose domain types to JS)
│       └── events.go            # Event names as constants + domain event → runtime.EventsEmit
│
├── frontend/                    # See §5
└── docs/
    ├── ARCHITECTURE.md          # Layer diagram (Mermaid) + request flow + event flow
    └── adr/                     # One file per significant decision
```

### Dependency direction (must hold, verify it)
```
bridge ──► app ──► domain
adapters ──► app (implement ports), adapters ──► domain
main ──► everything (wiring only)
domain ──► nothing
```
Add a small test or `go list` check script (`scripts/check-deps.sh`) that fails if `internal/domain` or `internal/app` imports `wailsapp`, `net/http`, or `os`.

---

## 3. Go design patterns to use (and WHY)

| Pattern | Where | Why it earns its place |
|---|---|---|
| Composition root + constructor DI | `main.go`, all `New...` funcs | No globals, no `init()` magic, everything testable |
| Consumer-side interfaces | `app/ports.go` | Idiomatic Go: "accept interfaces, return structs" |
| State machine | `domain/server/state.go` | Start/Stop can be clicked anytime; illegal transitions must be impossible |
| Functional options | `streaming.Config` | Readable defaults, easy to extend |
| Decorator | `logbus.Handler` wraps a base `slog.Handler` | Logs go to console AND the UI without changing call sites |
| Observer / pub-sub | `logbus.Bus`, server state events | UI reacts to backend; backend never knows about UI |
| Ring buffer | `logbus/ringbuffer.go` | UI opened late still sees recent history; bounded memory |
| Middleware chain | `httpapi/middleware.go` | Cross-cutting concerns in one place |
| Facade | `bridge/app.go` | Frontend sees a small, stable API instead of internals |
| Anti-corruption layer | `bridge/dto.go` | Domain can change without breaking JS bindings |

### Hard rules for Go code
- `context.Context` is the first param of every blocking or IO function.
- Graceful shutdown: `http.Server.Shutdown(ctx)` with timeout; background goroutines owned by an `errgroup` and stopped via context.
- **No goroutine leaks.** Every goroutine has a clear owner and exit path.
- Errors: wrap with `fmt.Errorf("scan %q: %w", path, err)`; check with `errors.Is/As`; sentinel errors in domain.
- Concurrency: `sync.RWMutex` for the repo; buffered channels + `select { default: }` for non-blocking fan-out.
- No `panic` except truly impossible states; `Recover` middleware for handlers.
- Logging: structured `slog` attributes (`slog.String("path", p)`), never `fmt.Println`.
- Comments explain **why**, not what. Every exported identifier has a doc comment.
- Keep functions short; prefer early returns; no `else` after `return`.

### Security (non-negotiable)
- **Never accept a filesystem path from a URL.** Clients request `/stream/{id}`; the repo maps ID → absolute path. This blocks path traversal.
- IDs: stable hash of relative path (e.g. first 16 hex chars of SHA-256).
- Verify resolved paths stay inside the library root (defense in depth).
- Listen on LAN interfaces only; show the URL from `netinfo`.
- Phase 6: optional access token in the connect URL (`?t=...`), checked by middleware.

---

## 4. HTTP API (served to devices)

```
GET  /                      → embedded minimal player page (list + <video> element)
GET  /api/health            → { "status": "ok", "version": "..." }
GET  /api/library           → [{ id, name, kind, size, modTime }]
GET  /api/library/{id}      → single item
GET  /stream/{id}           → file bytes via http.ServeContent (Range support)
GET  /playlist.m3u          → M3U of all video/audio with absolute LAN URLs
```
- Correct `Content-Type` via `mime.TypeByExtension` with fallbacks (`.mkv` → `video/x-matroska`).
- JSON errors use one shape: `{ "error": { "code": "not_found", "message": "..." } }`.
- Every request gets a request ID and an access log line (method, path, status, bytes, duration, client IP) → shows in the desktop log panel.

---

## 4b. Remote access via tunnels

### Port (in `app/ports.go`)
```go
// Tunnel exposes the local HTTP server on a public or private remote URL.
type Tunnel interface {
    ID() ProviderID                       // "cloudflare-quick", "cloudflare-named", "ngrok", "tailscale"
    Start(ctx context.Context, localAddr string) (PublicURL, error)
    Stop(ctx context.Context) error
    Status() TunnelStatus                 // Off, Connecting, Online(url), Failed(err)
}
```
- **Strategy pattern:** `remote.Service` doesn't know which provider runs; it gets one from the registry.
- Tunnel lifecycle reuses the same state-machine idea as the server. Stopping the server stops the tunnel first.

### Providers
| Provider | How | Notes for the UI |
|---|---|---|
| Cloudflare quick tunnel | Run `cloudflared tunnel --url http://127.0.0.1:<port>`, parse the `trycloudflare.com` URL from output | No account. Random URL each start. Good for testing. |
| Cloudflare named tunnel | `cloudflared tunnel run --token <token>`, user supplies token + hostname | Stable custom domain. Show a ToS notice (below). |
| ngrok | Embedded `ngrok-go` SDK with user's authtoken | Free plan has a very small bandwidth allowance: warn it's for testing, not movie streaming. Send `ngrok-skip-browser-warning` header from our player page. |
| Tailscale | Detect local Tailscale; show `http://<tailnet-ip-or-magicdns>:<port>` | Private (only the user's own devices), no public exposure. **Recommended default for personal streaming.** |

- `cloudflared` binary: don't bundle in v1. Detect it on `PATH`, else show a one-click link to the official download and a "Locate binary…" picker.
- Subprocess supervision (`procsup`): owned by an errgroup, killed on context cancel, restart with exponential backoff (max 5), stdout/stderr piped into the log bus with a `source=tunnel` attribute.
- Tokens are stored in the **OS keychain**, never in the JSON settings file, never in logs (add a redacting slog attribute helper).

### Security rule: public means authenticated (enforced in code, not UI)
- `remote.Service.Enable()` **refuses** to start a public provider unless an access PIN/token is configured. Test this.
- Auth middleware: PIN login page → signed, HttpOnly session cookie (HMAC, rotating secret). Stream URLs for external players (VLC, Infuse) use short-lived signed links (`/stream/{id}?sig=…&exp=…`).
- Rate-limit login attempts per IP (token bucket); lock out after repeated failures.
- Behind a tunnel, real client IP comes from `CF-Connecting-IP` / `X-Forwarded-For`, but trust those headers **only** for requests arriving from the tunnel's loopback connection.
- Bind the server to `127.0.0.1` + LAN only; tunnels connect to loopback. Never bind `0.0.0.0` "for convenience".
- UI shows a persistent amber **"Public access ON"** badge in the title bar while any public tunnel runs, plus the count of active remote sessions.

### Honest warnings to show in the app
- **Cloudflare:** Cloudflare's service terms restrict serving lots of video through their proxy outside paid video products. Show a one-time notice: "Fine for light personal use; heavy streaming may violate Cloudflare's terms. Tailscale is recommended for regular remote viewing."
- **Upload speed:** remote playback is limited by the home internet *upload* speed. Show measured throughput in the remote panel; later, transcoding to lower bitrate solves this.

---

## 4c. Feature roadmap

| Version | Features |
|---|---|
| **v0.1 MVP** | Pick folder, scan, LAN HTTP server, device player page, M3U playlist, QR connect, live logs, glass UI |
| **v0.2** | Settings persistence, rescan + file watcher (`fsnotify`), library search/filter, thumbnails (ffmpeg frame grab), dark/light |
| **v0.3 Remote access** | Tunnel providers (Tailscale, Cloudflare quick/named, ngrok), PIN auth, signed stream links, public-access badge, remote session list |
| **v0.4 Playback** | ffmpeg on-the-fly HLS transcoding (MKV/HEVC → H.264 for iPhone/iPad), subtitle support (.srt → WebVTT), resume position per device |
| **v0.5 TV** | DLNA/UPnP server so smart TVs discover EinCast automatically, mDNS `eincast.local` |
| **v1.0** | Multiple libraries (folders), user profiles (kids/adults), auto-update, signed + notarized builds (macOS) and code-signed installer (Windows) |
| **Later ideas** | Burmese + English UI (i18n), watch-together sync, Chromecast/AirPlay casting from the device page, menu-bar/tray mode |

Rules for the agent: build only the current phase's features. When a later feature needs a seam (e.g. transcoding needs a `Transcoder` port), **don't add it early**: note it in `docs/ROADMAP.md` instead.

---

## 5. Frontend architecture: feature-sliced

```
frontend/src/
├── main.tsx
├── app/
│   ├── App.tsx                  # Layout shell: title bar, sidebar, content
│   ├── providers.tsx            # Theme, MotionConfig (reducedMotion="user")
│   └── routes.ts                # Simple view enum (no router lib needed)
├── features/
│   ├── onboarding/              # First run: pick folder (big glass drop target)
│   ├── server/                  # Status pill, start/stop, state store
│   │   ├── components/
│   │   ├── hooks/useServerStatus.ts
│   │   └── store.ts
│   ├── connect/                 # URL card, copy button, QR code (LAN + public URL tabs)
│   ├── remote/                  # Provider picker, token entry, public-access warning, status
│   ├── library/                 # Stats + item list
│   └── logs/                    # Virtualized log viewer: level filter, search, pause autoscroll
└── shared/
    ├── bridge/                  # THE ONLY place that imports ../wailsjs
    │   ├── commands.ts          # Typed wrappers around Go methods
    │   ├── events.ts            # Typed event map + subscribe helpers
    │   └── types.ts
    ├── ui/
    │   ├── glass/               # GlassPanel, GlassButton, GlassToolbar, GlassPill
    │   └── (shadcn components)
    ├── motion/presets.ts        # Named springs + variants
    ├── hooks/useWailsEvent.ts   # Subscribe/unsubscribe safely in useEffect
    └── lib/cn.ts
```

### Frontend rules
- Components **never** import `wailsjs/*` directly. Only `shared/bridge` does. (Same anti-corruption idea as the Go side.)
- **Go is the source of truth.** Frontend stores are projections updated from events; UI never assumes a command succeeded until the state event arrives.
- Typed event map:
  ```ts
  type EventMap = {
    "server:state": ServerStateDTO;
    "log:line": LogLineDTO;
    "library:scanned": LibrarySummaryDTO;
  };
  ```
- One Zustand store per feature; selectors to avoid re-renders.
- Strict TS: no `any`, no non-null `!` without a comment explaining why.
- Feature folders export through an `index.ts` (public API of the feature).

---

## 6. Design system: macOS glass

### Native window (in `main.go`)
- macOS: `mac.Options{ TitleBar: mac.TitleBarHiddenInset(), WebviewIsTransparent: true, WindowIsTranslucent: true }`
- Windows: `windows.Options{ WebviewIsTransparent: true, WindowIsTranslucent: true, BackdropType: windows.Mica }`
- `BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0}`
- Draggable title area via CSS `--wails-draggable: drag` (and `no-drag` on buttons).
- Leave space for the macOS traffic lights (≈ 80px left padding in the title bar).

### Glass tokens (Tailwind v4 `@theme` in `globals.css`)
- Surfaces: `rgba(255,255,255,0.08)` dark / `rgba(255,255,255,0.55)` light
- `backdrop-filter: blur(24px) saturate(180%)`
- 1px inner highlight border (`rgba(255,255,255,0.18)`) + soft shadow
- Radii: 10 / 14 / 20px; system font stack (`-apple-system, "SF Pro Text", "Segoe UI Variable", sans-serif`)
- Support light + dark via `prefers-color-scheme`

### Motion presets (`shared/motion/presets.ts`)
```ts
export const spring = {
  snappy: { type: "spring", stiffness: 500, damping: 32 },
  smooth: { type: "spring", stiffness: 260, damping: 26 },
  gentle: { type: "spring", stiffness: 140, damping: 20 },
} as const;
```
- Use for: status pill morph (`layout`), panel enter/exit, log line insert, QR card reveal.
- Respect reduced motion (`MotionConfig reducedMotion="user"`).
- No animation longer than ~400ms; nothing bounces on every log line.

### Screens
1. **Onboarding**: centered glass card, folder icon, "Choose Media Folder" button → native dialog.
2. **Dashboard**: left sidebar (Dashboard / Library / Logs / Settings); main area shows status pill (Idle / Running / Failed), Start/Stop, connect card (URL + copy + QR), library stats (counts by kind, total size).
3. **Logs**: virtualized list, monospace, level chips (debug/info/warn/error), search, "pause autoscroll", clear view.
4. **Settings**: port, server display name, change folder, rescan.

---

## 7. Build in phases — STOP after each phase

After each phase: make sure it builds (`wails build`), tests pass (`go test ./...`, `tsc --noEmit`), then give me:
1. A short summary of what you built and which patterns appeared.
2. The files I should read first, in order.
3. **Three questions that check whether I understood the phase.** Don't continue until I say "next".

- **Phase 1 — Skeleton:** folder structure, composition root, dependency-check script, Tailwind v4 + shadcn + glass tokens, translucent window, empty shell layout with title bar + sidebar.
- **Phase 2 — Domain + library:** `media` types, kind detection, `fsscan`, `memrepo`, `library.Service` with tests; bridge method `ChooseFolder()` + `library:scanned` event; onboarding screen.
- **Phase 3 — Server lifecycle:** state machine + tests, `streaming.Service`, `httpapi` with health + library + stream + playlist, graceful shutdown; status pill + start/stop UI.
- **Phase 4 — Log pipeline:** custom slog handler, ring buffer, bus, access-log middleware, `log:line` events; virtualized log viewer with filters.
- **Phase 5 — Connect:** `netinfo`, connect card with URL, copy, QR; embedded device player page.
- **Phase 6 — Hardening:** access token, settings persistence (JSON in `os.UserConfigDir()`), error states in UI, rescan, cross-platform build check for macOS + Windows.
- **Phase 7 — Remote access (v0.3):** `Tunnel` port + registry, `procsup` supervisor, Tailscale + Cloudflare quick tunnel first, then named tunnel + ngrok; keychain secrets; PIN auth + signed stream links; "Public access ON" badge; remote panel in UI. Tests: refuses public without PIN, supervisor restarts and stops cleanly, header-trust rules.
- **After that:** follow the roadmap in §4c, one version per phase, same stop-and-quiz rule.

---

## 8. Testing expectations
- Domain: 100% of state transitions and kind detection, table-driven.
- App services: tested with hand-written fakes for every port.
- HTTP: `httptest` for each route, including Range requests (`206 Partial Content`) and path-traversal attempts (`/stream/../../etc/passwd` → 404).
- Log bus: a slow subscriber must not block publishers (test with a timeout).
- Frontend: at minimum, type-check passes; add Vitest for `shared/bridge` and stores.

---

## 9. Documentation (this is how I learn)
- `docs/ARCHITECTURE.md`: Mermaid diagram of layers, plus two sequence diagrams: "user clicks Start" and "iPad requests /stream/{id}".
- `docs/adr/NNNN-title.md` for each real decision, format: **Context → Decision → Consequences → Alternatives rejected**. Expected ADRs at minimum:
  - 0001 Hexagonal architecture
  - 0002 Stdlib ServeMux instead of a router library
  - 0003 Server lifecycle as an explicit state machine
  - 0004 Log fan-out via slog handler + ring buffer
  - 0005 Media IDs instead of paths in URLs
  - 0006 Bridge layer as the only Wails dependency
  - 0007 Tunnel providers as strategies + subprocess supervision
  - 0008 Public access requires authentication (enforced in the service layer)
- `docs/ROADMAP.md`: the §4c table, kept current, with "seams needed later" notes.
- `README.md`: how to run, build, test; folder map.

---

## 10. What NOT to do
- No global variables or singletons (except the `main` wiring).
- No pattern without a reason in an ADR. No "manager", "helper", "utils" grab-bag packages.
- No generic repository abstractions "for the future".
- No importing Wails outside `internal/bridge` and `main.go`.
- No business logic in React components or HTTP handlers — handlers parse, call the service, write the response.
- Don't generate everything at once. Follow the phases.