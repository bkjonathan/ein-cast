# Using EinCast: settings, data and the API

The reference that used to live in the README. For the short version, see the [README](../README.md).

## Settings, token and data

| What | Where |
|---|---|
| Port, display name, library folders (and their names, and whether each is shown to smart TVs), token on/off, Smart TVs on/off, start/awake/login/close-window choices, the ffmpeg location, the update check on/off | `settings.json` in `~/Library/Application Support/EinCast` (macOS) or `%AppData%\EinCast` (Windows) |
| Profiles and their PIN hashes (`profiles.json`) and where each profile stopped (`resume.json`) | The same folder as `settings.json`, readable by you only |
| The access token | The system Keychain / Credential Manager, service `com.bkthomas.eincast`. Never in `settings.json` or the logs |

Thumbnails are made with `ffmpeg` if it is installed (for example `brew install ffmpeg`); without it EinCast shows icons instead. They are cached in `~/Library/Caches/EinCast/thumbs` (macOS) or `%LocalAppData%\EinCast\thumbs` (Windows) and are safe to delete. The Light/Dark/System choice is remembered by the app window itself.

Subtitles: put `.srt` (or `.vtt`) files next to the video with the same name, optionally with a language, e.g. `Film.mkv` and `Film.en.srt`. Pick one in the player's menu. Text subtitles stored inside an MKV or MP4 are listed too (needs `ffmpeg`); picture subtitles (Blu-ray, DVD) are listed but can't be shown. A video with several audio languages gets an Audio menu on its page.

Videos a device can't play as they are (MKV, AVI, HEVC…) are converted on the fly with `ffmpeg` for Safari, iPhone, iPad, Firefox (through the bundled hls.js) and TVs that play HLS. The computer's video chip does the work when it can (Settings → ffmpeg → Use hardware acceleration, on by default), and EinCast refuses to start a conversion that would fill the disk. Conversions are temporary files in `~/Library/Caches/EinCast/hls` (macOS) or `%LocalAppData%\EinCast\hls` (Windows), deleted when you stop watching and when EinCast quits.

Smart TVs (Settings → Smart TVs) is off by default. A TV can't send the access token, so while it is on the whole library is visible, without a password, to everyone on your network; it can't be on together with the access token.

If the server won't start because another program already holds its port, Settings → Free a port shows which program that is (using `lsof` on macOS, `netstat` on Windows) and ends it when you confirm. It never ends EinCast itself, and only offers ports 1024–65535.

Burmese file names: names in Myanmar Unicode are listed, searched and read for titles like any other, and search treats the two spellings of a letter (for example U+1026 and U+1025 U+102E) as the same text. Names in the old **Zawgyi** encoding are shown as they are stored, which can look garbled (overlapping or misordered letters); EinCast does not convert them. Rename such files in Unicode with any Zawgyi-to-Unicode converter, and rescan.

Updates: about once a day EinCast asks GitHub whether a newer release exists (Settings → Updates). If one does, a line appears in the sidebar and the Updates card shows what's new and each download's SHA-256; **Open download page** takes you to the release, and you install it yourself. EinCast never downloads or installs anything on its own. The request is one anonymous read of this project's public release list: no account, no cookie, nothing about you, this computer or the version you run (GitHub sees your IP address, as any website does). Turn **Check for updates automatically** off and EinCast makes no request until you press **Check now**. This is the only request EinCast makes to the internet unprompted (finding your devices on your own Wi-Fi is separate); tunnels and the like happen only when you turn them on.

Deleting `settings.json` resets EinCast to its defaults. A damaged one is moved to `settings.json.bad` and EinCast starts with defaults.

## Start over: Reset EinCast

Settings → **Reset EinCast…** forgets everything this install knows (libraries, profiles, where each person
stopped, settings, the access token and PINs, saved tunnel tokens, caches, opening at login), restarts, and
shows the first-run setup again. Your media files are not touched. It cannot be undone, and devices using the
old link need the new one. Design: [ADR 0034](adr/0034-reset-erases-and-relaunches.md).

## Report a problem

Settings → **Help and feedback** → **Copy diagnostics** puts a report on your clipboard (version, system, which
settings are on, the last 200 log lines; folder names and secrets hidden). Read it, then paste it into a
[bug report](https://github.com/bkjonathan/ein-cast/issues/new?template=bug_report.yml). Nothing is sent by EinCast
itself ([PRIVACY.md](PRIVACY.md)).

## Try the API

Choose a folder, press **Start server**, then open the **Connect a device** card. On the same machine, or any device on your network, use the address it shows (default port 8765):

```sh
curl http://127.0.0.1:8765/api/health
curl http://127.0.0.1:8765/api/library
curl -H "Range: bytes=0-99" -i http://127.0.0.1:8765/stream/<id>
curl http://127.0.0.1:8765/playlist.m3u      # open in VLC / Infuse
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full route table.
