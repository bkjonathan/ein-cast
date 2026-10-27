# 0034 — Reset EinCast: erase everything it saved, then open again as a first run

## Context
A person who hands the computer on, wants to try the first-run setup again, or has got
into a state they cannot explain needs a way to start over that is not "find five folders
and a Keychain entry by hand". EinCast keeps state in several places:

- `<config dir>/EinCast/`: `settings.json`, `profiles.json`, `resume.json` (and `.bad`
  files set aside by earlier corrupt loads);
- `<cache dir>/EinCast/`: thumbnails, extracted subtitles, HLS output;
- the OS credential store, service `com.bkthomas.eincast`: the access token, the remote
  PIN, the profile signing secret and each tunnel provider's token;
- the open-at-login entry (a LaunchAgent on macOS, a `Run` key value on Windows);
- the window's own browser storage: theme, language, "read this once" notices.

"Start over" has to mean all of it, or the first run is not a first run (a left-over token
would make the wizard skip nothing but still show the old link).

## Decision
- **One use case, `app/reset.Service`.** `Erase()` removes the login entry, the secrets and
  the folders, and runs every step even when an earlier one fails, returning all failures
  together. A half-erased install that says what is left is better than one that stopped at
  the first error. `Relaunch()` asks for a new copy.
- **Three small ports** (`DataEraser`, `SecretEraser`, `Relauncher`), implemented by
  `adapters/eraser`, `adapters/secrets` and `adapters/relaunch`. The login entry reuses the
  existing `LoginItem` port.
- **The credential store is emptied by service name** (`go-keyring`'s `DeleteAll`), not by
  a list of keys. A list would have to be kept in step with every secret a later phase
  adds, and a forgotten key would survive a "reset". The cost is that the adapter erases
  by the service name alone; the test checks that another program's secret survives.
- **The eraser removes whole directories and refuses to remove anything else**: an empty or
  relative path, a disk root, or the home folder is refused. `main` passes it exactly two
  paths, both of which it built by joining `EinCast` to the user's config and cache dirs.
  Media folders are never on the list.
- **Order, in `bridge.App.ResetApp`:** stop remote access, the server and the conversions,
  run the shutdown closers (resume positions are written *now*), then erase, then relaunch,
  then quit. Doing it in this order means nothing can write a file back after it is gone,
  and an erase failure can be shown in the window (the app stays open) instead of being
  lost in a shutdown hook. The stop-and-close part is guarded by a `sync.Once`, so the
  shutdown hook that follows the quit does not run the closers a second time.
- **Relaunching waits for the old process.** Wails' single-instance lock is an `flock` held
  until the process exits; a copy started earlier would hand over to the running one and
  exit. So the new copy is started detached with `--relaunch-after=<pid>`, and `main`
  waits for that process to be gone (up to 15 s) before it reads a file or takes the lock.
  On macOS inside a bundle the copy is started with `open -n <bundle> --args …`, as the
  Finder would (Dock icon, menu bar); elsewhere the program is run directly.
- **If the new copy cannot be started the app still quits.** The data is gone; staying open
  would show a library that no longer exists. The log says why.
- **The window forgets its own preferences first.** When Go succeeds the app quits and the
  promise never settles, so there is no "after" for the page to clean up in. It removes the
  `eincast.*` keys from its storage before calling Go. If Go then fails, the loss is a
  theme or language the person can choose again.
- **Guarded by a second click.** Settings → Reset EinCast… shows what is erased and what is
  not (the media files), then asks once more.

## Consequences
- A reset cannot be undone. The confirmation text says so.
- Devices using the old link stop working: the token is new after the reset (the first-run
  defaults turn it on), and so are the profile PIN hashes (gone with `profiles.json`).
- The wait-for-exit handshake depends on the new process seeing the old one's id. It was
  tested with a throwaway program on macOS (the new copy started 22 ms after the old one
  exited). `open -n` inside a real `.app`, and everything on Windows (`OpenProcess` +
  `WaitForSingleObject`, `DETACHED_PROCESS`), compile but have not run.
- In `wails dev` the relaunched program is the dev binary, which may not find the dev
  server; use a built app to try a reset.
- Reset does not remove the app itself, nor the `.app` or installer, nor downloads of
  updates the person made.

## Alternatives rejected
- **Reload in place** (erase, then re-create every service and reload the page). Every
  service keeps state in memory (the library repo, profile caches, the PIN lockout table, a
  cached token); resetting each of them is more code, and more ways to miss one, than
  starting a fresh process.
- **A marker file that makes the next start erase everything.** Crash-safe and race-free,
  but the person sees nothing wrong until a later launch, and a failure cannot be shown.
  It would still be the right design if erasing in the running process turns out to be
  unreliable on Windows (open files cannot be deleted there).
- **Erase inside the shutdown hook.** No place to show an error, and it runs after the
  window is gone.
- **`sh -c "sleep 1; open …"` as the relaunch.** A fixed delay is a guess, and the shell
  dialects differ between macOS and Windows. Passing the process id removes the guess.
- **A list of secret keys.** See above.
