# 0002 — Stdlib ServeMux instead of a router library

## Context
EinCast serves six routes with one path parameter (`/stream/{id}`) and needs
method matching. Historically that pushed Go projects toward chi or gin. Since
Go 1.22, `net/http.ServeMux` supports method-qualified patterns
(`GET /stream/{id}`), wildcards (`r.PathValue("id")`) and exact-root matching
(`GET /{$}`).

## Decision
Use `http.ServeMux` and plain `http.Handler` middleware (`func(http.Handler)
http.Handler`) composed by a small `chain` helper. All routes live in one
function, `routes.go`.

## Consequences
- No third-party router to learn, audit or upgrade; handlers are ordinary
  `http.HandlerFunc`s and work with `httptest` unchanged.
- The mux redirects unclean paths (`/stream/../../x` → `/x`) before any handler
  runs, a free layer of path-traversal protection on top of ID lookup.
- A catch-all `/` pattern is needed to keep 404s in the API's JSON error shape.
  A side effect: a wrong method on a known path gets that 404 rather than a 405.
- No route groups or per-route middleware; we apply middleware to the whole mux.
  If sub-routers with different auth rules are needed (remote access), revisit.

## Alternatives rejected
- **chi**: pleasant API, but everything it gives us we now get from the stdlib.
- **gin / echo**: bring their own context types and binding magic, which would
  leak into handlers and make them harder to test and to reason about.
