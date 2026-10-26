# 0017 — Subtitles as sidecar files, resume position in the browser

## Context
v0.4 asks for `.srt` subtitles shown as WebVTT, and for each device to resume
where it stopped. Browsers show subtitles natively through `<track>`, but only
WebVTT; most subtitle files in the wild are `.srt`, often with a byte-order mark,
Windows line endings, or a legacy 8-bit encoding. And the server has no notion of
"a device": no accounts, and a phone is just whoever has the address.

## Decision
**Subtitles**
- **Sidecars only.** `Movie.srt`, `Movie.en.srt` and `Movie.pt-BR.forced.srt`
  belong to `Movie.mkv` (same folder, same name, optional dot-separated tags).
  `Movie2.srt` and `Movie Extras.srt` do not. The rule is `media.MatchSubtitles`,
  in the domain, table-tested. Subtitles that live *inside* an MKV, and image
  formats (`.sub/.idx`, PGS), are out of scope: they need ffmpeg and rendering.
- **They are part of the item, not items.** The scanner attaches them to the
  video (`media.Item.Subtitles`) instead of listing them, so stats, the playlist
  and the library list are unchanged. The library's change detection counts them,
  and the watcher wakes for `.srt`/`.vtt`, so adding a file to a running library
  shows up.
- **Converted on the way out.** `GET /subtitle/{id}/{n}` reads the file, decodes it
  (UTF-8, UTF-16 with a mark, otherwise Windows-1252), converts SRT to WebVTT and
  answers `text/vtt`. The file on disk is never modified. The conversion is
  forgiving (missing numbering, `1:00:01,000`, position hints, stray blocks),
  because a browser rejects a whole track over one bad line. It lives in
  `domain/subtitle` as a pure function.
- **Addressed by position, never by path** (ADR 0005). `/api/library` lists
  `subtitles: [{label, lang}]`; track `n` is the n-th. File names and disk paths
  are not exposed, the path guard (`safepath.Inside`) runs before opening, and
  files over 4 MB are refused.
- **Off by default.** The page adds the tracks; the viewer picks one in the
  player's own menu.

**Resume**
- **In the browser's own storage, per device by construction.** The page saves
  `{position, duration, modTime}` per item in `localStorage` every 5 s, on pause,
  and when switching titles, and restores it when the title loads, with a "Resumed
  from 12:34. Start over" line. The list shows "12:34 in" on part-watched titles.
- **Rules:** not before the first 10 s, forgotten in the last 30 s or at the end,
  ignored if the file changed since, and never written for a stream of unknown
  length.

## Consequences
- No server state, no identity, no privacy question: what you watched stays in the
  browser that watched it. Clearing site data forgets it; a different browser,
  or the same device through a different address (LAN IP versus tunnel URL), has
  its own positions.
- VLC and Infuse, which open the M3U playlist, keep their own positions.
- A subtitle file whose name does not follow the video's is invisible. That is the
  convention players and media servers share, and the scanner has no way to guess.
- Timing offsets between a subtitle and a different cut of the film are not
  corrected.

## Alternatives rejected
- **Resume on the server, keyed by a cookie.** Works across addresses only if the
  cookie does, adds a store to persist and migrate, and gives the server
  something to leak. The benefit (the same position on two devices) is not what
  "per device" asked for.
- **Convert `.srt` in the browser.** Moves a tested pure function into untested
  page script, and every device would repeat the work.
- **Extract embedded subtitles with ffmpeg.** Useful, a different feature: it
  needs a probe of every file and a cache. Left for later.
- **List subtitle files as library items.** They are not something one plays, and
  it would break the "every item is a video, song or photo" assumption of the UI.
