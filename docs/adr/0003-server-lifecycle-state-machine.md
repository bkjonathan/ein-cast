# 0003 — Server lifecycle as an explicit state machine

## Context
The user can click Start and Stop at any moment, including twice in a row, while
a start is still binding the port, or after the server died on its own. The UI
must show an accurate status, and the app must never end up with two servers, a
half-stopped server, or a "running" label on a dead one.

## Decision
Model the lifecycle in `domain/server` as `Idle → Starting → Running → Stopping
→ Idle`, with `Failed` reachable from Starting, Running and Stopping, and
`Failed → Starting | Idle` for retry and acknowledge.

- `State.CanTransitionTo` is the single transition table (a `switch`, not a
  mutable map). Tests check all 25 `(from, to)` pairs.
- `Machine` checks and changes state under one mutex, so of two racing callers
  exactly one wins and the other gets `ErrInvalidTransition`.
- `streaming.Service` is the only code that drives the machine. It serializes
  Start/Stop with a mutex, publishes `ServerStateChanged` on every transition,
  and runs one watcher goroutine per run that turns an unexpected exit into
  `Failed`.
- The UI is a projection: buttons are enabled from the state, and the state only
  changes when the event arrives.

## Consequences
- Double-clicks, races and stray crashes all resolve to a defined state.
- `Failed` carries a reason the UI can show ("address already in use"), and
  retrying is a normal transition.
- `Stop` holds the service mutex while shutting down (bounded by a 5 s timeout),
  so a concurrent `Start` waits; `Status()` does not, so the UI never freezes.
- Slightly more code than a `running bool`. Accepted: the bool cannot express
  Starting, Stopping or Failed, which are exactly the states users see.

## Alternatives rejected
- **`running bool` plus a mutex**: cannot represent in-progress or failed states
  and invites check-then-act races.
- **Library (e.g. a generic FSM package)**: five states and nine edges do not
  justify a dependency; a `switch` is easier to read and to test exhaustively.
