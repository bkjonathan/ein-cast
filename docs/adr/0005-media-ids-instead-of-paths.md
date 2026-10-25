# 0005 — Media IDs instead of paths in URLs

## Context
Devices on the LAN (and later, the internet) request files from EinCast. If a
URL carried a filesystem path (`/stream?file=../../etc/passwd`), every handler
would have to defend against path traversal, and URLs would leak the host's
directory layout.

## Decision
Clients only ever see an opaque ID: `/stream/{id}`.

- `media.NewID(relPath)` = first 8 bytes (16 hex chars) of SHA-256 over the
  slash-separated path *relative to the library root*.
- The repository maps ID → `media.Item`, which holds the absolute path. The
  absolute path never leaves the server (`LibrarySummaryDTO` carries only the
  root folder for display).
- An unknown ID, including `../../etc/passwd`, is simply a missing map key →
  `media.ErrNotFound` → 404. There is no path parsing to get wrong.
- The scanner skips symlinks, so a link inside the library cannot make an
  ID resolve to a file outside it.
- Phase 3 adds a second check at serve time: the resolved path must still be
  inside the library root (defense in depth).

## Consequences
- IDs are stable across rescans and machines, so bookmarks, playlists and
  playback positions (v0.4) survive a restart.
- Renaming or moving a file changes its ID. Acceptable: it is a different path.
- 64 bits makes accidental collisions negligible for a home library. If one
  ever occurred, the later item would shadow the earlier one in the repo.
- With multiple libraries (v1.0), the same relative path in two roots would
  collide; the hash input will need a library identifier then (see ROADMAP).

## Alternatives rejected
- **Path in the URL, sanitized**: relies on getting sanitization right in every
  handler, forever.
- **Random UUID per scan**: safe, but IDs change on every rescan and break
  bookmarks and resume positions.
- **Sequential integers**: trivially enumerable, and unstable across rescans.
