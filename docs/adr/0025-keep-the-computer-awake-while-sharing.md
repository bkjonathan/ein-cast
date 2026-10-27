# 0025 — Keeping the computer awake while sharing

## Context
The server runs inside a laptop app. When the laptop sleeps, the server is gone: the
TV shows "can't connect" and the person blames EinCast. Closing the lid is the
usual cause, but macOS and Windows also sleep an idle machine after a few minutes,
which is exactly what a machine serving a film to another room looks like (nobody
is touching it).

## Decision
- **A `PowerGuard` port in `app/ports.go`**: `Hold(reason) (release, error)`. The
  app layer cannot call IOKit or Win32, and the streaming service needs to be
  testable without sleeping a computer.
- **`streaming.Service` holds it for exactly one run.** `Config.KeepAwake` (from
  the setting, default on) asks for it; the hold is taken after the server is
  Running and given back with the other things a run set up (discovery
  announcements) when the server is stopped or dies. A crash releases it too, so a
  dead server never keeps the machine awake. Failing to hold is logged and the
  server runs regardless.
- **The hold prevents idle sleep, not display sleep.** The screen can turn off;
  that is what people expect of a media server and it saves power. It does not
  override closing the lid on a laptop on battery; no API reliably does, and
  pretending otherwise would be a promise EinCast cannot keep. The setting text
  says "the screen may still turn off".
- **macOS: run `caffeinate -i -w <our pid>`** instead of calling
  `IOPMAssertionCreateWithName` through cgo. It is the system's own tool and
  creates the same assertion, shows in `pmset -g assertions`, and `-w` ties its
  lifetime to ours: if EinCast is killed, `caffeinate` exits and the Mac may sleep
  again. No cgo keeps the build and the universal binary simple.
- **Windows: `SetThreadExecutionState(ES_CONTINUOUS | ES_SYSTEM_REQUIRED)`** from a
  goroutine pinned to one OS thread with `runtime.LockOSThread`. The call is
  per-thread, not per-process: a request made from a thread the Go scheduler later
  moves away from is lost when that thread ends. The goroutine asks, waits for
  release, withdraws the request, and ends.
- **Other systems get an adapter that always fails** with a clear message, so
  nothing builds differently and the failure is the logged warning above.

## Consequences
- One extra process on macOS while sharing, a few kilobytes, visible in Activity
  Monitor as `caffeinate`.
- The setting applies when the server next starts, not to a running one. The panel
  says so.
- The Windows adapter compiles (checked for amd64 and arm64) but has not run on a
  real Windows machine; that is on the Phase 13 list (`docs/TESTING-DEVICES.md`).

## Alternatives rejected
- **IOKit through cgo.** Same effect, but a C dependency in an otherwise pure-Go
  build, and a process that dies without releasing leaves the assertion until exit
  anyway.
- **Hold it for as long as the app is open.** Simpler, but a person who stopped
  sharing would still be unable to let the laptop sleep.
- **Always hold, with no setting.** Some people run on battery and want the laptop
  to sleep; the setting is cheap.
