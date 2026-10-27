# 0018 — Videos a device cannot play are converted to HLS, once, on demand

> Extended in Phase 17: browsers without HLS get hls.js, the audio track can be chosen
> (the file's own default is now used, not always the first), and hardware encoders and a
> free-space check were added. See [0031](0031-playback-gaps-hlsjs-and-embedded-tracks.md)
> and [0032](0032-hardware-encoding-and-disk-safety.md).

## Context
An iPhone or iPad opening `/stream/{id}` for an MKV, an AVI, or an HEVC file in
MP4 gets an error. The files are fine; the device does not support the container
or codec. Safari, iOS and many smart TVs, however, all play **HLS** natively when
it carries 8-bit H.264 video and AAC audio, which ffmpeg can produce from almost
anything. The roadmap asks for exactly this ("MKV/HEVC → H.264 for iPhone/iPad").

Constraints: ffmpeg is the user's own installation (ADR 0015); a library has
thousands of films and nobody watches most of them; a conversion costs a CPU core
and writes gigabytes.

## Decision
- **On demand, per video, shared.** Nothing is converted ahead of time. The first
  request for `/hls/{id}/index.m3u8` starts ffmpeg; later requests, from any
  device, reuse it. `transcode.Service` keeps one session per video, and
  restarts it if the source file changed.
- **Only what is needed.** ffprobe reads the streams first. 8-bit H.264 is
  **copied** and AAC is **copied**: converting an MKV into HLS then takes about as
  long as copying the file, with no quality loss. HEVC, 10-bit, AC-3, FLAC and the
  like are **encoded** (`libx264 -preset veryfast`, AAC stereo, at most 1920 px
  wide, a keyframe every 6 s). With no ffprobe, everything is encoded: slower, never
  wrong. `plan.go` is a pure function and table-tested.
- **An "event" playlist.** ffmpeg writes `index.m3u8` as it goes, with every
  segment kept; the stream is playable once the second segment exists, long
  before the end, and the finished playlist is seekable. The adapter closes
  `Ready` at that point and the handler waits for it (up to 60 s).
- **Ports:** `app.Transcoder` (`Available`, `Start`) and `app.HLSStream`
  (`Ready`, `Done`, `Err`, `Open(name)`, `Stop`). The adapter owns the working
  folder, so the application layer never touches the filesystem. `Open` accepts
  only `index.m3u8` and `seg00000.ts`-shaped names.
- **Bounded and self-cleaning.** At most 2 conversions at once. A third video
  may take the place of one that has been idle for 30 s, never of one being
  watched; otherwise it gets `503` with `Retry-After`. A conversion no one has
  asked for in 2 minutes is stopped and its files deleted. Everything is stopped
  when the app quits, and the working folder is emptied at every start, so a crash
  leaves nothing behind. A failed conversion is remembered (until it is cleaned
  up) so an unreadable file costs one process, not one per request.
- **Same access control and path guard as streaming.** Behind the LAN token or the
  tunnel sign-in; `safepath.Inside` runs before ffmpeg opens anything; ffmpeg reads
  `file:<path>` with only the `file` and `pipe` protocols.
- **The player decides, and only when it can work.** `/api/health` reports
  `transcoding`. The page converts a video when the server can, the device plays
  HLS natively (`canPlayType`), and the container is not one browsers play as is
  (MP4, M4V, MOV, WebM). It also retries a failed direct playback once as HLS.

## Consequences
- **Safari, iOS and HLS-capable TVs only.** Chrome and Firefox on a computer do not
  play HLS natively; for them the page keeps the direct stream and the playlist
  for VLC. Adding `hls.js` would cover them, at the cost of a vendored script.
- **Not seekable past the converted part** while an encode is in progress. A
  remux is finished in seconds; a full encode of a film runs at several times real
  time, so this matters only for jumping far ahead right after pressing play.
- **Resume does not start until the stream's length is known** (a still-converting
  stream reports no duration), so a long encode saves positions only once it ends.
- **Not tested on a real iPhone, iPad or TV.** The playlist, segments and headers
  are checked against the real ffmpeg and a handwritten player, not Apple's
  decoder. Cookies on segment requests are assumed to work as they do for
  Safari's other media requests; the token-in-URL form of access does not carry to
  segments, only the cookie does.
- Software encoding competes with whatever else the computer is doing, and a
  library on a network drive is read twice (probe and convert).

## Alternatives rejected
- **Convert whole files ahead of time into a second library.** Doubles the disk and
  converts what nobody watches.
- **Per-segment, seekable on-demand encoding** (generate segment N when asked,
  like Jellyfin): the right answer for a media server proper, a large state
  machine for a v0.4. The event playlist covers the common case.
- **Always re-encode.** Needlessly slow and lossy for the majority of files, which
  are already H.264/AAC in an unsupported container.
- **Hardware encoders** (VideoToolbox, NVENC, QuickSync): faster, platform-specific
  and quirky across ffmpeg builds. A seam, not a requirement.
- **fMP4 segments** (needed for HEVC in HLS): HEVC is converted to H.264 instead,
  which every HLS device plays.
- **Serve the playlist before ffmpeg has produced anything** and let the player
  retry: players handle missing segments badly; waiting server-side is simpler.
