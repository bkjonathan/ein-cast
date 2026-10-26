# 0014 — The library follows its folder: watch, wait, rescan quietly

## Context
Until v0.2 the library changed only when the person pressed Rescan. Adding an
episode to the folder and not seeing it on the iPad is the first thing anyone
trips over. The brief asks for a file watcher (`fsnotify`).

`fsnotify` has three properties that shape the design. It watches folders one at
a time, not trees, so a folder created later is invisible until someone adds a
watch for it. It reports many events for one human action (copying a file is a
create followed by dozens of writes). And on macOS it uses kqueue, which holds an
open file handle for **every file** in a watched folder, so a very large library
can hit the system's limit.

## Decision
- **Port `app.FolderWatcher`.** `Watch(ctx, root)` returns a channel that carries
  one meaning: "something may have changed, look again". No paths, no event
  kinds. The answer to "what changed" is always a rescan, so the app never
  reasons about individual events. The channel holds at most one pending signal,
  so bursts coalesce, and it is closed when the context ends or watching fails.
- **Policy lives in `library.Service`, mechanism in `adapters/fswatch`.** The
  adapter turns raw events into signals: it adds a watch for each new folder (and
  signals, because files may have landed before the watch did), tracks watched
  folders so a removed folder is recognised, and ignores what the scanner would
  skip. The service decides *when* to scan: wait until the folder has been quiet
  for 2 s (`WatchTiming.Quiet`), but never longer than 30 s after the first change
  (`MaxWait`), so a ten-minute download still refreshes the list now and then.
- **One rule for "not library content".** `media.Ignored(name)` (hidden entries,
  `Thumbs.db`, `@eaDir`, …) moved from the scanner into the domain; the scanner
  and the watcher both call it, so a file the scan skips can never trigger a scan.
- **Watch-triggered scans are quiet.** They show no "Scanning…" state, and when the
  files turn out to be identical (same IDs, sizes and modification times) they
  publish nothing at all. A failed one does show up in the status, with the same
  "Showing the last successful scan" wording as a manual rescan. A manual scan or
  rescan is as loud as before.
- **Which folder is watched follows the scans.** Each successful scan announces
  its root on a one-slot channel (the newest announcement replaces an unread one,
  so scanning never waits for the watcher). `Service.Watch(ctx)` runs for the life
  of the app, started by the bridge's startup hook beside the log forwarder; it
  moves the watch when the root changes and ends it when the context ends.
- **All or nothing.** If any folder cannot be watched (the system limit), `Watch`
  returns an error and nothing is watched. The service logs a warning, reports
  `LibraryStatus.Watching = false`, and the Library screen says changes need
  Rescan. Scanning the same folder again retries the watch.

## Consequences
- The list updates a couple of seconds after files are added, removed or renamed,
  with no flicker and no log noise when nothing really changed.
- A very large library on macOS may fall back to manual rescans. The limit is the
  process's file-handle allowance (Go raises the soft limit to the hard one at
  startup), roughly one handle per media file. The person is told, not misled.
- A rescan walks the whole folder. That is fine for home libraries; a watcher that
  applies deltas would only pay off at hundreds of thousands of files.
- A network share that does not deliver change notifications (some SMB/NFS
  mounts) reports `Watching = true` and never signals. The OS gives no way to tell.
- Events from the first moments of a watch are possible while the initial walk is
  still adding folders; the initial scan that precedes every watch covers that.

## Alternatives rejected
- **Poll on a timer (a cheap stat-walk every few seconds).** Needs no file
  handles and works on network shares, but wastes work 99% of the time and adds
  latency. Kept in mind as the fallback if kqueue limits bite in practice.
- **Interpret each event and patch the repository.** Rename, move and
  copy-in-progress semantics differ per platform, and a bug would make the
  library drift from the disk with nothing to correct it. A rescan is
  self-healing.
- **Debounce inside the adapter.** Would hide the policy where the service tests
  cannot reach it, and the adapter tests would need real timers for every case.
- **Watch partially when the limit is hit.** A half-watched library looks fine
  and silently misses changes in the unwatched part.
- **A setting to turn the watcher off.** Nothing needs it yet; if the file-handle
  cost proves a problem for someone, it is a one-line option on the service.
