# 0031 — Browsers without HLS get hls.js; audio and subtitle tracks inside a video can be chosen

## Context
ADR 0018 converts a video a device cannot play into HLS, but only for devices that
play HLS themselves: Safari, iOS, many TVs. Firefox cannot, so an MKV or an HEVC
file in Firefox failed with "this browser can't play this file". (The Chrome used to
check this phase, 156 on macOS, answers `canPlayType` for HLS with "maybe" and played
the stream natively; I did not check which versions got that, or Edge. Firefox is the
browser known to need the script, and the page decides by asking the browser, not by
its name.)

Two more gaps sat in the same place. A converted video always used the **first**
audio stream, so a film with English first and Japanese second could not be heard in
Japanese. And **subtitles stored inside** an MKV were ignored: only `.srt` and `.vtt`
files next to the video worked (ADR 0017), but a downloaded MKV usually carries its
subtitles inside.

Constraints: the library can hold tens of thousands of files and a person opens a few,
so nothing may be looked at ahead of time; the page's policy allows scripts and
media from its own origin only; a conversion is expensive and shared (ADR 0018).

## Decision
**hls.js, vendored, loaded only when needed.**
- `web/hls.min.js` is hls.js **1.6.16** (Apache-2.0; its licence ships as
  `web/hls.js.LICENSE`), taken from the npm tarball, whose SHA-1 matched the
  registry's and whose SHA-256 is pinned in `static_hls_test.go`. Upgrading means
  changing that line and this ADR together. 1.6.16 was chosen because it had been out
  for months when picked; the 1.7 line was weeks old.
- The page asks `canPlayType` for native HLS first. Without it, and with Media Source
  Extensions present, it appends a `<script src="/static/hls.min.js">` the first time
  a converted video is played. A browser with its own HLS never downloads the 540 KB.
  `canPlayHLS()` stayed the one decision to change.
- The policy gained exactly one thing: `media-src 'self' blob:`. hls.js feeds the
  video element through a MediaSource, which the browser exposes as a `blob:` URL. It
  is allowed for media only, and a test fails if `blob:` appears in any other
  directive. hls.js runs with `enableWorker: false`, so no `worker-src` is needed.
- hls.js is told `startPosition: 0`: our playlist grows while ffmpeg works, which
  hls.js takes for a live stream and would start at its end. It also sends the ngrok
  header on segment requests, like `getJSON`.
- While a playlist is still growing, `lengthOf(player)` reports "unknown", as Safari
  already did, so a position is never saved against a length that is only the
  converted part.

**Tracks inside a video, on demand.**
- A new port pair in `app`: `StreamProber` (audio and subtitle tracks, from ffprobe)
  and `SubtitleExtractor` (one text track as WebVTT, from ffmpeg), both implemented
  by the `ffmpeg` adapter beside the thumbnail grabber and the transcoder, and a new
  use case, `app/tracks`. `GET /api/library/{id}/tracks` lists the tracks (labels such
  as "English · 5.1 · Dolby Digital" are built by the HTTP layer from the file's
  language tag); the page asks when a video's detail page opens, and `/api/health`
  says whether it can.
- The result of a probe is remembered in memory per file version (ID, size, modTime),
  bounded, so opening a page twice costs one ffprobe.
- An embedded subtitle is served at `/subtitle/{id}/embedded/{n}`, next to the sidecar
  route. Extracting it means reading the whole file, so the WebVTT is kept in the
  per-user cache (`subtitles/`, one file per ID, track, size and modTime), requests
  for the same track share one ffmpeg run, and at most two run at once. The same
  `thumbcache` folder cache serves both (`WithFiles("subtitle", ".vtt")`); a second
  cache package for the same ten lines would have been worse.
- Only **text** tracks (SRT, ASS/SSA, WebVTT, `mov_text`, a few more) are served.
  Picture tracks (Blu-ray PGS, DVD) are listed so the person sees they exist, are
  greyed out in the menu with "pictures, can't be shown", and answer 404.
- The URL carries an ID and a number, never a path (ADR 0005), and goes through the
  same profile check as every other route.

**Choosing the audio.**
- An audio track is part of the conversion's identity: `/hls/{id}/audio/{n}/index.m3u8`
  and its segments (named relative to the playlist, so they land under the same
  folder). `transcode.Service` keys sessions by video **and** track, and passes the
  track to the transcoder. Without a track in the URL the file's own default (the
  stream it marks as default, else the first) is used; **this changes the behaviour
  of ADR 0018**, which always took the first stream, and matches what a browser does
  when it plays the file directly.
- Switching the track while watching replaces the source and seeks back to where the
  viewer was (`seekWhenPossible` waits for the conversion to reach that point). A
  browser cannot choose the audio of a file it plays as it is, so choosing a track
  other than the default turns conversion on for that video.
- A third audio track of the same video takes a slot from another track **of the
  same video** even if it was used a moment ago (the person who switches has left the
  old one); another video still cannot take a slot from a conversion in use.
- A track number the video does not have is an error (`422`), not a video without
  sound.

**The page after a failure.** A `<video>` error says nothing about why, so the page
asks the server again: `507` is the full disk (ADR 0032), `503` is "busy", and anything
else keeps the general message.

## Consequences
- **Firefox plays MKV and HEVC**, with the audio and subtitles the person chose.
  Checked with Firefox 158.0.1 on macOS against a throwaway server: an HEVC MKV with two
  audio languages and two subtitle tracks played through hls.js, the English subtitle
  showed, French stayed off, and switching audio resumed within a second of the same
  place. The same file played in Chrome through its native HLS and, with
  `canPlayType` hidden, through hls.js.
- **Not checked** on Safari, an iPhone or iPad, a smart-TV browser, Edge, or any
  Windows machine. A TV browser's Media Source Extensions are the unknown; the page
  keeps playing files directly when it can.
- **Switching audio starts a second conversion** of the same video (a re-encode, if
  it is HEVC). It is quick when the picture is copied or the chip encodes (ADR 0032)
  and takes a while when the processor does, and it is not seekable past what is done
  (the same limit as ADR 0018).
- **Chrome re-enables subtitle tracks** when a source is set, whatever their
  attributes say, and hls.js replaces the source after the page added the tracks. The
  page therefore re-applies the choice at `loadstart` and `loadedmetadata`, and, for
  hls.js, adds the `<track>` elements again once the stream is attached. Both were
  found by running a real browser, not by reading the specification.
- The first read of an embedded subtitle takes as long as reading the file: seconds
  on a local disk, a minute on a slow network share (the extraction times out at
  three minutes). Later reads come from the cache.
- A file ffprobe cannot read lists no tracks and plays or fails as before.
- Tracks are not shown in the desktop app's Library screen, only on the player page.

## Alternatives rejected
- **One HLS stream with every audio track as an alternative rendition** (`EXT-X-MEDIA`),
  switched in the player without restarting anything. It is the better end state, and
  Safari would switch natively. It makes every conversion mux, and for a non-AAC
  source encode, every audio track whether or not anyone switches, and ffmpeg's
  `var_stream_map` output is a larger change to a working pipeline than this phase
  warranted. Recorded in the roadmap.
- **Probing every file during the scan.** 50,000 files would mean 50,000 processes,
  mostly for videos nobody opens.
- **Reading picture subtitles** (OCR) or **burning them into the video.** Both are
  projects of their own; the first needs a model, the second a conversion per choice.
- **Loading hls.js with the page, or from a CDN.** The first costs every device the
  download; the second breaks the "no third-party content" policy and works only
  online. The library is the user's, and so is the network.
- **Taking the newest hls.js (1.7.x).** Weeks old at the time, with no feature we need.
- **A bundled copy through npm and Vite.** The player page is plain files with no build
  step on purpose; a vendored file with a checksum test keeps it that way.
