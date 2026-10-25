# 0006 — Bridge layer as the only Wails dependency

## Context
Wails generates JS bindings from exported methods on bound Go structs and
provides a runtime (dialogs, events) imported as a Go package. If domain or
use-case code imported it, business logic would need a running window to test,
and every domain refactor would silently change the JS API.

## Decision
Only `internal/bridge` (and `main.go`, as composition root) import Wails.

- `bridge.App` is a facade: a small, stable set of methods the UI may call.
- `bridge/dto.go` defines flat, JSON-safe DTOs; domain types never cross to JS.
- On the frontend, only `shared/bridge` imports `wailsjs/`, mapping generated
  classes into frontend-owned types.

`scripts/check-deps.sh` fails if any other Go package imports `github.com/wailsapp/`.

## Consequences
- Domain and use cases stay testable and Wails-free.
- Domain can change without breaking the frontend; the DTO mapping absorbs it.
- Each new capability costs a little boilerplate (method + DTO + TS wrapper).

## Alternatives rejected
- **Bind use-case services directly**: less code, but exposes domain shapes to JS
  and couples the app layer to Wails' binding rules.
- **Import `wailsjs` from components**: convenient, but scatters the Go contract
  across the UI and makes components hard to test.
