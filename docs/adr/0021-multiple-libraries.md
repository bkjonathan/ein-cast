# 0021 — Several libraries, each with its own identity and root

## Context
Until v1.0 EinCast served exactly one folder. "A library on the internal disk and
another on the external drive" is the normal home setup, so v1.0 asks for several.
That touches everything that assumed one root: item IDs (ADR 0005 flagged that the
same relative path in two roots would collide), the path guard that keeps served
files inside the library, the scanner and repository, the watcher, the saved
settings, and the screens.

## Decision
- **A library has an identity derived from its folder.** `media.LibraryID` is the
  first 8 bytes of SHA-256 over `"library|" + cleaned root`. No stored random IDs:
  adding the same folder again, even after removing it, gives the same library, so
  item IDs, bookmarks and resume positions come back. Moving a folder makes it a
  new library, which is what happened to its files' IDs anyway.
- **Item IDs include the library**: `NewID(libraryID, relPath)`. This replaces the
  scheme of ADR 0005 (its other points stand). **Upgrading from a single-library
  version renumbers every item once**, so resume positions saved by the player page
  start over; nothing else keeps IDs.
- **Every item carries its library's ID and root** (`Item.LibraryID`, `Item.Root`;
  server-side only, like `Path`). The path guard becomes
  `safepath.Inside(item.Root, item.Path)`: a file must be inside *the library it
  claims to belong to*, not merely inside some library. The "which root?" question
  no longer exists, so `Root()` is gone from the HTTP, thumbnail and conversion
  ports.
- **The service owns the list.** `library.Service` keeps the configured libraries,
  separately from what the repository holds, so a library whose folder has
  vanished (an unplugged drive) stays listed with its error and comes back when the
  drive does. The repository stores items per library (`Replace(id, items)`,
  `Remove(id)`, `Items(id)`, `List()`); `Scanner.Scan` takes a `media.Library`.
- **Folders may not overlap.** The same folder, a folder inside a library, or one
  that contains a library is refused (`media.Overlaps`, case-insensitive, since both
  platforms' file systems are by default): otherwise one file would be listed twice
  under two IDs.
- **Operations**: `Add` (the library appears at once, scanning; a failed first scan
  removes it again, because a bad choice should not leave a broken entry), `Remove`
  (the folder on disk is untouched; a scan still running for it is discarded),
  `Rename` (names are 1–64 characters, no control characters), `Rescan(id)`,
  `RescanAll`, `Restore` (scan all remembered libraries; one failing does not stop
  the others). Scans are serialized **per library**: scanning A never blocks
  scanning B, and two scans of the same library are refused.
- **Watching is per library.** `Watch` is a supervisor: each library that has been
  scanned gets its own child watcher with its own debounce, started and stopped as
  libraries are added and removed. A child that ends (the watch could not start, or
  stopped) is restarted by the next successful scan of that library, as before.
- **Events carry the whole picture.** `LibraryScanned` ("what is in the libraries
  changed") and `LibraryStatusChanged` ("what is happening changed") both carry the
  state of every library, so the UI replaces its list instead of piecing it together
  from deltas; the first also tells it to re-read the files.
- **Settings**: `Settings.Libraries` replaces `LibraryRoot`. The file stores
  `{name, root}` per library; the ID is recomputed on load, so it cannot disagree
  with the folder. A file with only the old `libraryRoot` becomes a one-library list
  on load, deterministically and without writing anything back.
- **UI**: onboarding is "no libraries yet" (and "the first scan" while it runs);
  Settings gets a Libraries panel to add, rename, rescan and remove; the Library
  screen can filter by library; the Dashboard shows totals across all of them.

## Consequences
- One-time renumbering of item IDs on upgrade (see above), and resume positions
  saved in browsers are lost.
- DLNA merges libraries: a folder called "Shows" in two libraries is one folder on
  the TV (its items stay distinct). The device player page lists all files flat. A
  per-library view for both is a seam.
- `State()` computes statistics from every library's items on each event. That is
  cheap for home libraries and would want caching at hundreds of thousands of files.
- Two libraries on removable drives with the same mount path would be one library.
  That is the right answer for the same drive, and a surprise for two different
  drives mounted in turn at the same place.
- "Change folder" no longer exists; remove and add instead (the same folder returns
  the same IDs, a different one is a different library).

## Alternatives rejected
- **Random library IDs stored in settings.** Another thing to persist and to lose;
  adding a folder back would renumber everything.
- **One `library.Service` per library with an aggregating reader.** Looks tidy, but
  every consumer (HTTP, DLNA, thumbnails, conversion, the UI) would need to know
  about the aggregate, and add/remove would need a registry anyway.
- **Keep a global `Root()` and look up the library per request.** An extra lookup on
  every request and a way to get the pair wrong. Carrying the root on the item makes
  the right check the only possible one.
- **Allow nested libraries and de-duplicate files.** De-duplication needs to pick a
  winner; refusing overlap is simple and explainable.
- **Keep the old ID scheme for the first library to avoid renumbering.** An
  unwritten special case ("the library with no ID") that every future change would
  have to remember; renumbering once before 1.0 is cheaper.
