# 0001 — Hexagonal architecture

## Context
EinCast has several volatile edges: a desktop shell (Wails), an HTTP server,
the filesystem, OS keychains and tunnel providers. The core rules (what a media
item is, when the server may start or stop) should not change when an edge does,
and should be testable without any of them.

## Decision
Use ports & adapters.

- `internal/domain` — pure types and rules; imports nothing.
- `internal/app` — use cases; depend on small, consumer-side interfaces (ports).
- `internal/adapters/*` — implement ports (fs scanner, repo, HTTP, log bus, tunnels).
- `internal/bridge` — the Wails-facing facade.
- `main.go` — composition root; the only place concrete types are wired together.

`scripts/check-deps.sh` enforces the direction by inspecting direct imports.

## Consequences
- Use cases are tested with hand-written fakes; no Wails, network or disk needed.
- Swapping an edge (e.g. a different scanner) touches one adapter and `main.go`.
- More packages and some indirection than a flat `main` package. Accepted: the
  app is meant to grow (tunnels, transcoding, DLNA) and to teach clean Go structure.

## Alternatives rejected
- **Flat package / everything in `main`**: fastest to start, but Wails, HTTP and
  business rules would tangle and tests would need a running window.
- **Classic layered (handlers → services → repo) with concrete deps**: allows
  inner layers to import outer ones, which is what the import check forbids.
