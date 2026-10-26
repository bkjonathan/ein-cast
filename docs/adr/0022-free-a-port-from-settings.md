# 0022 — Freeing a port from Settings

## Context
The server listens on a fixed port (8765 by default). When something else already
holds it, often a forgotten run of the same app or a dev server, `Start` fails with
"address already in use" and the person has to open a terminal, find the process id
with `lsof` or `netstat`, and kill it. A Settings button that does this is a
convenience, but it ends other people's programs, so the way it is built matters
more than the feature.

## Decision
- **Look first, end second.** `portfree.Service.Check` lists the programs listening
  on a port; nothing is ended by it. `Free(port, pid)` is a separate call, and the
  screen asks for a second click that names the program.
- **The caller names a process, not a command.** `Free` looks again and ends `pid`
  only if it is listening on `port` right now. A stale screen, or a process id the
  system has since reused for something else, gets `ErrNotHolding` and nothing
  happens. The UI can never ask for an arbitrary process to be ended.
- **EinCast never ends itself.** The service is given its own process id (`main`
  passes `os.Getpid()`, since `app/` may not import `os`). If the server's own
  listener holds the port, the answer is "stop the server", not a kill.
- **Same range as the port setting: 1024–65535.** Ports below that belong to system
  services; this is not the tool to stop them. Process ids 0 and 1 are refused in
  the adapter as well.
- **Polite first, forced second.** `Terminate` (SIGTERM; on Windows, where console
  programs have no such request, it ends the process) is followed by a wait of up
  to 3 s for the port to be released, then `Kill`, then up to 2 s more. Success means
  the port is free, not that a signal was sent. A cancelled request does not
  escalate to a forced end.
- **A port in `app/ports.go`, an adapter for the system tools.** `app.PortInspector`
  (`Holders`, `Terminate`, `Kill`) is implemented by `adapters/portproc`: `lsof` on
  macOS and Linux, `netstat -ano` plus `tasklist` on Windows. The output parsers are
  pure functions, tested on sample text from both systems. The port number is an
  `int`, so only digits ever reach a command line.
- **Windows output is parsed without the word "LISTENING".** Windows translates it
  into the display language. A listening socket has no peer, so its foreign address
  ends in `:0`; that is what is matched.
- **Refusals are explained.** The operating system refusing (another user's or a
  system process) becomes `app.ErrNotPermitted`, which the bridge turns into advice
  in plain words. A process that is already gone counts as success.
- **The action is logged** (`source=portfree`, port, pid, name), so the Logs screen
  shows what was ended and when.

## Consequences
- One more Settings card and two bridge methods (`CheckPort`, `FreePort`).
- It depends on `lsof` being installed on macOS (it ships with the system) and on
  `netstat`/`tasklist` on Windows. If the tool is missing, Check fails with its
  error and nothing is ended.
- Ending a process that belongs to the same user needs no extra rights. For another
  user's process the person gets the explanation, not an elevation prompt; EinCast
  does not ask for administrator rights.
- Only TCP listeners are considered. A UDP socket on the same number does not block
  the HTTP server.

## Alternatives rejected
- **A single "Kill port N" button.** Fewer clicks, but it kills whatever is there
  without saying what, and it can be wrong after the machine has changed state.
- **Killing by process name** (`pkill node`). Hits unrelated programs.
- **Using a Go library for process and socket tables** (such as gopsutil). Reads the
  tables directly and avoids parsing text, but adds a large dependency and per-OS
  cgo or syscall surface for one feature; the two tools' outputs are stable and
  small.
- **Asking for administrator rights to end other users' processes.** Out of scope
  and a bigger security surface than this feature is worth.
