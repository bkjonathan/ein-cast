# 0026 — Titles, series and posters from names and local files

## Context
A library of `Heat.1995.1080p.BluRay.x264-GROUP.mkv` and `S02E05.mkv` is hard to
browse. Plex and Jellyfin solve it by looking every file up on the internet. EinCast
promises "no account, no server, nothing leaves your home" (Phase 15 goal), so the
first version must work with nothing but what is on the disk, and cost almost
nothing per file: a scan of 50,000 files takes a third of a second today.

## Decision
- **Parse the name, pure and table-tested** (`media.ParseMeta`). Input is the
  slash path inside the library, so the folders count. It reads:
  - a movie's title and year: the words before the first release-noise word
    (`1080p`, `BluRay`, `x265`, `DTS`, …), and the *last* year-shaped word before that
    noise, so "Blade Runner 2049 2017 1080p" is *Blade Runner 2049* from 2017, and a
    name that is only a year ("2012") is a title;
  - an episode, from `S02E05`, `2x05`, `Season 2 Episode 5`, `Episode 5`, or a bare
    number in a `Season 2` folder; the series from the text before the marker, else the
    nearest folder that is not a season folder; the episode's own name from the text
    after it.
  - Bracketed tags (`[YTS]`) are dropped; a bracketed year (`[2010]`) is kept. Text is
    never lower- or title-cased: names in any script survive untouched.
  - Only videos are parsed. Music and photos keep their file names.
- **A guess, never a loss.** `Item.Name` stays the file name. `Meta.Title` falls back
  to the cleaned name, and to the raw name if even that is empty, so a row is never
  blank. A name the parser misreads is wrong in the title only, and a `.nfo` can fix
  it.
- **`.nfo` files correct the guess** (`media.ParseNFO`, `Meta.Apply`): `Name.nfo` next
  to `Name.mkv`, or `movie.nfo` when a folder holds one video. Title, year, plot and,
  for an episode file, series, season and episode. A field the file leaves empty keeps
  the parsed value. The decoder is lenient (these files are written by many tools and
  often are not valid XML), reads at most 256 KiB, does not load external entities or
  DTDs (Go's decoder never does), bounds every string, strips control characters, and
  clients show the text with `textContent` only.
- **Posters are found, not fetched** (`media.MatchArtwork`), next to the video:
  `<video>-poster.jpg`, then `<video>.jpg`, then `poster.jpg` / `folder.jpg` /
  `cover.jpg`. The folder-level names count only when they can be this video's: the
  folder holds one video, or the video is an episode (then the series folder above a
  `Season N` folder is searched too). In a folder of unrelated movies, `poster.jpg` is
  nobody's. A picture that became a poster leaves the list of photos; one that did not
  stays a photo.
- **Posters travel on the thumbnail route.** `/thumb/{id}` and the desktop thumbnail
  already exist, are guarded by the same path check (`safepath.Inside`) and cache. The
  thumbnail service makes the picture from the poster when there is one, with its own
  cache key (poster size and time), so adding or replacing a poster makes a new
  picture. Without ffmpeg, a poster that is a JPEG of at most 512 KiB is served as it
  is; larger ones and PNGs need ffmpeg to be shrunk. The API says `poster: true` so a
  client knows the picture is a portrait poster and not a frame.
- **The cost is in the scanner, where the files already are.** Names are parsed once
  per scan. `.nfo` files are read only when present. `sameFiles` (does a watcher-
  triggered rescan need to tell the UI?) compares metadata and poster too, and the
  watcher treats `.nfo` as relevant, so editing one updates the library.
- **Series grouping is a view, not a data model.** Items carry `Show`, `Season`,
  `Episode`; both UIs gather them. The desktop list stays a flat virtual list of rows
  (header, season label, episode, item) so a library of thousands costs a screenful. The
  player page builds a catalog (films, one card per series, music, photos).
- **Search** is a plain substring match over path, title, series and year, every word
  in any order, NFC-normalised. No fuzzy matching and no plot search yet.
- **Not in 1.1: online metadata (15.7).** A `Metadata` port with a TMDB adapter, the
  person's own API key in the keychain and a privacy line is a reasonable 1.2 feature,
  after local metadata proves its worth. It needs a privacy statement that stays true
  ("turning this on sends titles to themoviedb.org") and nothing here blocks it:
  `Meta` is where its answer would go, and `.nfo` is already the precedent for
  "metadata that came from somewhere else".

## Consequences
- Misparses happen, mostly with titles that look like release noise (a film called
  "Cam"), or a year that is part of the title. The fix is a `.nfo` or a rename, and the
  parser's tests list the known hard cases.
- Year is not taken from a series folder for its episodes ("Fargo (2014)" gives
  episodes of *Fargo* with no year); it is not shown for episodes.
- DLNA listings and the M3U playlist still use file names; titles there are a seam
  (see the roadmap).
- Memory per item grows by a `Meta` (a few strings) and an `Artwork` (a path and two
  numbers).
- `episode` is 0 for anything that is not an episode, and an item with an episode
  number but no series is deliberately not an episode, so nothing is filed under an
  empty header.

## Alternatives rejected
- **Look everything up online by default.** Best results, and the opposite of the
  promise. Kept as an opt-in for 1.2.
- **Parse with one big regular expression or a ported parser library** (guessit,
  PTN). Either is opaque and hard to extend case by case. A tokenise-and-cut parser
  with a table of cases is readable and every failure becomes a new row.
- **Resize posters in Go.** Needs an image-resampling dependency or a slow
  nearest-neighbour; ffmpeg is already the thumbnail tool, and small JPEGs need no
  resizing.
- **A database of series and seasons.** The items already carry what grouping needs;
  adding a second source of truth would let them disagree.
