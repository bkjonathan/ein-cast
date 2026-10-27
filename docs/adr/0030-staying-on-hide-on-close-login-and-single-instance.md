# 0030 — Staying on: hide on close, open at login, one instance

(Numbered 0030 because 0023–0029 are reserved by the v1.1 plan: signing,
token default, power, filename parser, profiles, i18n, updates. This one was
written during Phase 14.7, and ADR 0024 and 0025 took their planned numbers.)

## Context
"The server only runs while the app is open" is a weakness for the use case
(a TV in another room), not a bug: closing the window quits the app. For most
people the fix is not a background service but an app that survives its window,
opens by itself, and does not start twice.

Wails v2 has `HideWindowOnClose` and `SingleInstanceLock` but no tray or
menu-bar API.

## Decision
- **Closing the window can hide it instead of quitting** (`Settings.HideOnClose`,
  Wails `HideWindowOnClose`). It is read once, at startup, because Wails takes it
  as an option, not a runtime switch; the Settings text says "takes effect the
  next time EinCast opens".
- **Where the window comes back from decides the default.**
  - macOS: closing hides the whole app (`NSApp hide:`), and a click on the Dock
    icon shows it again. That works with no code of ours. **On for new installs.**
  - Windows: a hidden window needs a tray icon to come back from. The adapter
    (`adapters/tray`, `fyne.io/systray`, Windows only) shows an icon with "Open
    EinCast" and "Quit EinCast". It compiles but has never run on a real Windows
    machine, and a hidden window with no way back is a worse failure than the one
    being fixed. **Off for new installs** until a real run has proved it
    (`docs/TESTING-DEVICES.md`, row 13.4); whoever turns it on gets the tray.
  - Because nothing in `app/` consumes a tray, there is no `Tray` port. `main`
    wires the adapter straight to two functions of the bridge (`ShowWindow`,
    `Quit`). A port for one caller and no use case would be the "abstraction for
    the future" the project rules rule out.
- **Quit really quits.** `bridge.Quit` calls the Wails quit, which runs the
  shutdown hook that turns off remote access, stops the server and the
  conversions. Hiding never skips that path.
- **One instance.** `SingleInstanceLock` with the bundle ID. A second launch (the
  Dock icon, the Start menu, the login item firing while the app is already open)
  shows the first window instead. Two EinCasts would fight over the port, and
  "Address already in use" would be the person's first impression.
- **Open at login** (`Settings.OpenAtLogin`) behind a `LoginItem` port, since
  `settings.Service` must make the computer match the setting and cannot touch the
  filesystem or the registry. macOS writes a per-user LaunchAgent that runs
  `open -a EinCast.app --args --background`; Windows writes a value under the
  current user's `Run` key. Both start the app with `--background`, which
  starts it hidden, but only where the window can be brought back (hide-on-close is
  on); otherwise the person would have a running app they cannot see.
- **The login entry follows the app.** `Load` writes the entry again when the
  setting is on, so a moved or updated app does not leave a dead path. A failed save
  rolls the entry back, so the computer and `settings.json` cannot disagree.

## Consequences
- With hide-on-close on, the Dock (or tray) shows EinCast is still running, and
  ⌘Q or "Quit" is the way to stop sharing. The Settings text says so on each
  platform.
- A login launch on macOS goes through `open -a`, which only brings an
  already-running app forward, so the login item firing while EinCast is open is
  harmless. On Windows the single-instance lock handles the same case.
- The tray library is Windows-only code behind a build tag; macOS and the CI
  cross-compile do not link it.
- Unverified on real hardware: the tray on Windows, the Run key, and the
  LaunchAgent on a fresh macOS login. Each is a row in the Phase 13 matrix.

## Alternatives rejected
- **A real background service / launch daemon.** Needs installer privileges and a
  second process talking to the UI; it is the headless mode (v1.2 candidate in the
  plan), which should be built once, as `eincastd`, not half-built here.
- **A menu-bar item on macOS too.** Wails owns the NSApplication loop and the
  popular systray libraries fight it there. The Dock does the job.
- **Always hide on close, on both systems.** Fine on macOS; on Windows it ships an
  unverified tray as the only way back.
- **No single-instance lock.** Cheaper, but two instances is the commonest way to
  get "address already in use".
