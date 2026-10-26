# 0016 — A manual light/dark choice, on top of a backdrop the page cannot change

## Context
Until now the theme followed the OS through `prefers-color-scheme`, and the
roadmap promised a manual toggle in v0.2. The window is translucent: the glass
panels sit on macOS vibrancy or Windows Mica, which belong to the native window
and follow the OS appearance. Wails 2.12 exposes `WindowSetLightTheme` /
`WindowSetDarkTheme` / `WindowSetSystemDefaultTheme`, but on macOS all three are
empty functions. So on a Mac in light mode the app can show dark panels, but not a
dark backdrop behind them.

Near-white glass (`rgb(255 255 255 / .08)`) with near-white text on a light
backdrop is unreadable, so "just flip the colors" is not enough.

## Decision
- **Three modes: System (default), Light, Dark.** The choice is kept in the
  webview's `localStorage` (guarded like the provider notices) and applied as
  `data-theme="light|dark"` on `<html>` before the first render; System removes
  the attribute. It is a preference of this window, not of the server, so it does
  not go into the Go settings file.
- **Tokens by attribute and by media query.** The dark values exist twice, behind
  `@media (prefers-color-scheme: dark) :root:not([data-theme="light"])` and
  behind `:root[data-theme="dark"]` (CSS cannot share a block across the two). A
  Tailwind `@custom-variant dark` follows the same two rules so shadcn's `dark:`
  classes agree with the tokens. `light-dark()` was rejected: it would break the
  whole UI on macOS versions whose WebKit lacks it.
- **A tint where the choice disagrees with the OS.** Dark chosen on a light OS
  puts an 80%-opaque dark layer over the window; light on a dark OS, a light one.
  Where they agree there is no tint, so the vibrancy is untouched. `color-scheme`
  follows the mode, so native controls match.
- **Windows is told as well.** The bridge's `SetWindowTheme(mode)` calls the Wails
  runtime, which on Windows really changes the Mica/title-bar theme; a hook calls
  it at startup and on every change. On macOS it is a no-op, which is why the tint
  exists.

## Consequences
- On macOS, forced themes that disagree with the OS look like a tinted frosted
  window rather than true dark/light vibrancy. Readable, not identical.
- The two dark blocks in `globals.css` must change together; a comment says so.
- The choice does not follow the person to another machine, and clearing the app's
  web data resets it to System.
- The device player page keeps following each device's own setting.

## Alternatives rejected
- **Store the theme in `settings.json`.** It would let Go set the native window
  before the page loads, but it makes a pure view preference part of the server's
  settings, its validation and its events.
- **Reopen the window with `mac.Options.Appearance`.** Only read at startup, so a
  change would need a restart.
- **Give up on macOS and offer the toggle on Windows only.** Inconsistent, and the
  tint solves the actual problem.
