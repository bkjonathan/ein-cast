# 0004 — Log fan-out via slog handler + ring buffer

## Context
The desktop UI must show a live, filterable log of everything the backend does:
scans, server lifecycle, and one line per HTTP request. The code already logs
with `log/slog` and the console output must keep working. Requirements:

- No change at the ~dozens of existing call sites.
- A UI that opens late (or reloads) still sees recent history.
- A slow or stuck UI must never slow down the server. A log line must not be
  able to stall a video stream.
- Bounded memory, however long the app runs.

## Decision
A decorator over `slog.Handler`, a bus, and a ring buffer, all in
`adapters/logbus`:

- **`Handler`** wraps the console handler. Each record becomes an
  `app.LogEntry` and is published to the bus; the console handler still gets it
  too. `WithAttrs`/`WithGroup` are implemented properly (groups flatten to
  `group.key`; `LogValuer`s are resolved), values are capped at 2000 bytes, and
  the `source` attribute is lifted into its own field.
- **`Bus`** stamps every entry with a global sequence number, stores it in the
  ring, and offers it to each subscriber with a non-blocking send
  (`select { case ch <- e: default: dropped++ }`). Publish, Subscribe and cancel
  share one mutex, so closing a channel can never race a send.
- **Ring buffer** keeps the last 2000 entries for `History()`.
- The **`app.LogStream`** port (entry type, `History`, `Subscribe`) lives in
  `app`, so the bridge depends on the port and never on an adapter.
- The bridge forwards entries as `log:line` events from one goroutine owned by
  the app context, and serves `GetLogHistory()`. The UI joins history and live
  lines by `Seq`, so there are no gaps or duplicates; a jump in `Seq` shows
  that a slow consumer lost lines.
- Each component tags its own logger (`log.With("source", "http")`, `library`,
  `server`) in its constructor, so call sites stay untouched.

The `Handler` accepts Debug and above even when the console is set to Info, so
the UI's level filter has something to filter.

## Consequences
- Memory is bounded at both ends: 2000 entries in Go, 5000 lines in the UI.
- Under overload the UI loses lines instead of the server losing throughput.
  That is the intended trade-off for a diagnostic view.
- The log path must never log. `Emitter.emitLog` stays silent when the UI is not
  ready; a log line that causes a log line would loop forever.
- Tokens must not reach the log (remote access phase). They will be kept out by
  never passing them to slog and by a redacting attribute helper, not by
  filtering here.

## Alternatives rejected
- **Tee to a log file and tail it in the UI**: needs file rotation and polling,
  and cannot filter by structured fields.
- **Unbuffered or blocking channel to the UI**: simplest, but a stuck webview
  would back-pressure into request handlers.
- **Replace slog with a bespoke logger**: forces every call site to change and
  loses the standard ecosystem (`slog.With`, groups, `LogValuer`).
- **One event per line plus a database/ring in the frontend only**: late
  subscribers would see nothing that happened before they connected.
