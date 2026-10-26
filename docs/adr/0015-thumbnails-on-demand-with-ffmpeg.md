# 0015 — Thumbnails: made on demand by the ffmpeg already installed, kept on disk

## Context
v0.2 adds preview pictures: a frame of each video, the cover of each song, a
shrunken photo. They appear in the desktop Library list and on the device player
page. Making one means decoding media, which Go's standard library cannot do for
video or most audio; `ffmpeg` can do all three with one command.

Constraints: no bundled binaries in v1 (as for `cloudflared`); a library may hold
tens of thousands of files, so nothing may happen per file at scan time; and a
file path is something a client must never choose (ADR 0005).

## Decision
- **Lazy, per item.** Nothing is generated when a library is scanned. A thumbnail
  is made the first time someone looks at the item, then kept.
- **Ports:** `app.FrameGrabber` (`Available`, `Grab(ctx, GrabRequest) → JPEG`) and
  `app.ThumbnailCache` (`Get`/`Put` by key). `app/thumbnails.Service` is the use
  case: look up the item, serve the cached picture or make one.
- **The service guarantees four things.** Concurrent requests for the same item
  share one attempt (singleflight by hand: a `map` of jobs and a `done` channel).
  At most 2 ffmpeg processes run at once. Each attempt has its own 30 s timeout
  and runs on a detached context, so one impatient client cannot cancel work
  others wait for. A failure is remembered for the rest of the run, so a file
  ffmpeg cannot read costs one process, not one per page load.
- **Cache key = item ID + size + modification time + version** (e.g.
  `a1b2…-1048576-1700000000000000000-v1`). An edited or replaced file therefore
  gets a new picture, and bumping `version` regenerates everything. The cache is a
  folder of `<key>.jpg` files under `os.UserCacheDir()/EinCast/thumbs`, written
  through a temp file and rename, and pruned of files older than 90 days at start.
  The OS may clear it; the only cost is making pictures again.
- **The ffmpeg adapter** picks a frame 8 s into a video (past logos and black
  frames), falling back to the first frame for shorter clips; takes cover art from
  songs (ffmpeg treats it as a one-frame video stream); and scales to at most 320
  px wide without enlarging. The input is passed as `file:<path>` with
  `-protocol_whitelist file,pipe`, so a file *named* `concat:…` or `http:…` is
  still read as a file.
- **Same path guard as streaming.** `safepath.Inside(root, path)` (moved out of
  `httpapi` so both use one implementation) runs before ffmpeg opens anything:
  a file swapped for a symlink after the scan cannot be thumbnailed.
- **Finding ffmpeg:** `PATH` first, then the places installers use (Homebrew's
  folders on macOS, WinGet and Program Files on Windows). A macOS app started from
  the Finder does not inherit the shell's `PATH`, so without the second step a
  Homebrew install would never be found. The search repeats until it succeeds, so
  installing ffmpeg while EinCast runs works without a restart. On Windows the
  child is started without a console window.
- **Two ways in, one service.** `GET /thumb/{id}` serves the JPEG with
  `Last-Modified` and `Cache-Control: private`, behind the same access control as
  `/stream/{id}`; a missing picture is a plain `404`. `/api/health` reports
  `thumbnails: true|false` so the player page requests pictures only when they can
  exist. The desktop UI calls the bridge's `GetThumbnail(id)`, which returns a
  `data:` URL (or `""` for none), so thumbnails work with the server stopped.
- **Without ffmpeg everything still works,** with icons instead of pictures and a
  one-line hint in the Library screen.

## Consequences
- The first view of a large folder shows icons that fill in as ffmpeg works
  through the queue, two at a time. The desktop list only asks for a row once it
  has stayed on screen for 150 ms, so scrolling past thousands of files costs
  nothing.
- Exactly one thing is unverifiable here: how a given ffmpeg *build* handles HEIC,
  AVIF or exotic containers. Those items get no thumbnail rather than an error.
- Requests that queue behind slow files hold an HTTP connection open for up to
  30 s each. The player page marks pictures `loading="lazy"` so a browser does not
  open dozens at once.
- The pictures of a deleted file stay in the cache until pruned.

## Alternatives rejected
- **Generate during the scan.** Makes opening a library as slow as decoding all of
  it, and wastes work on files nobody opens.
- **Go libraries for decoding** (cgo bindings to libav, or pure-Go video
  decoders): heavy builds, patchy codec coverage, and a bigger attack surface
  than running one well-known tool on a file the person already owns.
- **Bundle ffmpeg.** Several tens of megabytes per platform, codec-licensing
  questions, and an update burden. Detecting the installed one costs nothing.
- **Serve full-size images and let the client shrink them.** A 6 MB photo per row
  defeats the point, especially over a tunnel.
- **Use `ffprobe` to seek to a percentage of the duration.** A second process per
  file for a marginal gain; a fixed offset with a fallback works for nearly
  everything.
- **A "Locate ffmpeg…" setting like cloudflared's.** The usual install locations
  cover almost everyone; it can be added the same way if reports say otherwise.
