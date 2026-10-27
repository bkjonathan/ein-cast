# Roadmap

Build only the current phase. When a later feature needs a seam, note it here
instead of adding it early.

| Version | Features | Status |
|---|---|---|
| **v0.1 MVP** | Pick folder, scan, LAN HTTP server, device player page, M3U playlist, QR connect, live logs, glass UI | In progress — Phases 1–6 done; Phase 7 is the remote-access phase (v0.3) |
| v0.2 | Settings persistence and manual rescan (Phase 6), file watcher (`fsnotify`), library search/filter, thumbnails (ffmpeg), dark/light toggle | Done in Phase 8 (see below) |
| v0.3 Remote access | Tunnel providers (Tailscale, Cloudflare quick/named, ngrok), PIN auth, signed stream links, public-access badge, remote session list | In progress — see phase 7 steps below |
| v0.4 Playback | ffmpeg HLS transcoding, subtitles (.srt → WebVTT), resume position per device | Done in Phase 9 (see below) |
| v0.5 TV | DLNA/UPnP, mDNS `eincast.local` | Done in Phase 10 (see below) |
| v1.0 | Multiple libraries, user profiles, auto-update, signed + notarized builds | In progress — multiple libraries done in Phase 11; the rest below |

## v0.1 phases

1. **Skeleton** — done
2. **Domain + library** — done
3. **Server lifecycle** — done
4. **Log pipeline** — done
5. **Connect** — done
6. **Hardening** — done
7. Remote access (v0.3), in four steps, each ending with a review:
   - **7a** — `Tunnel` port and registry, `remote.Service`, PIN, sessions, signed links, rate limiting, forwarding-header trust, Host check: **done** (no provider yet)
   - **7b** — Tailscale provider (extra tailnet listener), bridge methods, Remote screen, Connect-card tab and "Public access ON" badge: **done**
   - **7c** — `procsup` and the Cloudflare quick tunnel: **done**, with the open points listed under "Seams" below
   - **7d** — Cloudflare named tunnel, ngrok, keychain-held provider settings, and the "locate cloudflared…" picker: **done**

## v0.2 phase

8. **Library polish** — one phase, three steps:
   - **8a** — Library item list with kind chips, search and sort; System/Light/Dark appearance (ADR 0016): **done**
   - **8b** — `fsnotify` folder watcher with quiet, debounced rescans (ADR 0014): **done**
   - **8c** — On-demand ffmpeg thumbnails in the desktop list and on the player page, disk cache (ADR 0015): **done**

## v0.4 phase

9. **Playback** — one phase, two steps:
   - **9a** — Sidecar `.srt`/`.vtt` subtitles served as WebVTT; resume position per device, in the browser (ADR 0017): **done**
   - **9b** — On-demand ffmpeg HLS conversion for Safari/iOS/TVs, shared sessions, bounded and self-cleaning (ADR 0018): **done**

## v0.5 phase

10. **TV** — one phase, three steps:
   - **10a** — `Discovery` port, announced by `streaming.Service`; mDNS `eincast.local` and an `_http._tcp` service; the Connect card offers the name (ADR 0019): **done**
   - **10b** — DLNA MediaServer over HTTP: device and service descriptions, ContentDirectory Browse with a folder tree, ConnectionManager, DIDL-Lite, `/stream` DLNA headers (ADR 0020): **done**
   - **10c** — SSDP responder (announce, answer, goodbye), opt-in "Smart TVs" setting, exclusive with the access token: **done**

## v1.0 phases

11. **Multiple libraries** (done): a list of folders with their own identity, scanning and watching; IDs include the library; per-item path guard; Libraries panel in Settings; library filter (ADR 0021).

12. **Free a port** (done): Settings card that shows which program holds a port and ends it on confirmation; `portfree` service, `portproc` adapter (`lsof` / `netstat`), `CheckPort` / `FreePort` bridge methods (ADR 0022).

The next release (v1.1.1, "make it adoptable") is planned phase by phase in [PLAN-v1.1.md](PLAN-v1.1.md): real-device checks, signing, token on by default, local metadata, profiles, playback gaps, Burmese, auto-update and a community launch kit. Profiles, auto-update and signing below are scheduled there.

Still to do for v1.0, none of it started:
- **User profiles (kids/adults), done in Phase 16 (ADR 0027):** profiles restrict libraries, kinds and folders; a device chooses one (PIN optional) and holds a signed cookie; every route that takes an ID enforces it with a 404; Continue watching is kept on the server per profile.
- **Auto-update, notice done in Phase 19 (ADR 0029):** EinCast checks GitHub Releases about once a day (setting, on by default), shows a sidebar notice and an Updates card with the release notes and each file's SHA-256, and opens the release page. It does not install: a silent self-update waits for signing (below).
- **Signed and notarized builds** (macOS notarization, a Windows code-signing certificate): need credentials, and a CI job to use them (`.github/workflows/ci.yml` only builds and tests today).

## Seams needed later

- **Updates (Phase 19, ADR 0029), what is not done:** nothing installs by itself; the person downloads and runs the new version. The checksums catch a damaged download, not a replaced one: real authenticity needs signing (14.1, 14.2). A pre-release is never offered (GitHub's `latest` skips them), so someone on a release candidate hears only about the final. The check ran for real against the public API (it read v1.0.1 and its three files) but a 1.1.2 has not been published, so the "available" path has only run against a fake server and in unit tests; the checksum job in `release.yml` and `scripts/release-notes.sh`'s use in CI have not run on GitHub yet (the script was run locally). A "skip this version" button, an update notice in the title bar, and Burmese text for the messages Go writes (the failure reasons) are not built. If a platform's file name stops containing `macos` or `windows`, `assetsFor` lists every file. Hand-edit `.github/release-known-limits.md` when signing lands.

- **Profiles (Phase 16), what they do not do:** a PIN is a household gate, not a vault (4–6 digits, hash in `profiles.json`, owner-only file). A child with an adult profile already selected, or who can read this computer's files, is not stopped. Smart TVs cannot say who is watching: they get the libraries marked visible to TVs and nothing profile-specific, and DLNA search/browse stay profile-blind. Profiles are not seen by the desktop Library screen (the owner's view). A profile's folder rules are name patterns, not tags or ratings; there is no age-rating metadata. Resume positions are not migrated between profiles when one is deleted (they go with it). The player page was checked in desktop Chrome only (picker, wrong PIN, restriction, server resume after clearing storage), not on an iPad, Safari or a TV; the PIN field uses `inputmode=numeric` and should be confirmed on real keyboards. Per-library views on DLNA (above) is the neighbouring seam.
- **Metadata (Phase 15), what is not done:** online metadata (15.7) is deliberately 1.2 (ADR 0026). Titles are not yet used in the DLNA listing (`dc:title`) or the M3U playlist (`#EXTINF`), which still show file names; `upnp:albumArtURI` pointing at `/thumb/{id}` would show posters on a TV (this was already a seam under "Thumbnails in the DLNA listing"). Search does not look in the plot. `tvshow.nfo` (series-level title and plot) and per-season posters named `season01-poster.jpg` are not read. A year in a series folder name is not given to its episodes. The poster for an episode in a flat folder of several shows is not guessed.
- **Phase 15 on real screens:** the player page was checked in desktop Chrome (headless, driven over the DevTools protocol against a throwaway server) and by Go tests, not on an old TV browser or in Safari. It avoids `aspect-ratio`, `inset`, optional chaining and `Array.prototype.flat` for that reason; confirm on a TV (`TESTING-DEVICES.md`).
- **Scan cost with metadata (Phase 15):** the 50,000-file measurement went from 0.26 s to 0.34 s for the scan and from 1.8 ms to 2.7 ms for one `State()` pass (the items are larger).
- **Phase 14 (v1.1.1), built but never run on the target hardware:** the Windows tray icon and hide-on-close (ADR 0030), the Windows `Run` key, the macOS LaunchAgent after a real sign-in, and `SetThreadExecutionState` (ADR 0025) have been compiled for macOS and Windows and unit-tested where the platform allowed (the `caffeinate` adapter and the plist run for real on macOS). Each has a row in [TESTING-DEVICES.md](TESTING-DEVICES.md). Until the tray has run on Windows, hide-on-close is off by default there.
- **Phase 14 not done (needs the outside world):** 14.1 macOS signing + notarization, 14.2 Windows code signing (accounts, certificates, CI secrets; ADR 0023 is reserved for the decision).
- **First device in the wizard (14.4):** read from the access log (a non-loopback client with an answered request). A device behind a tunnel appears with the tunnel's loopback address and is not counted; the wizard is about the home network.
- **Keep-awake (14.6):** prevents idle sleep only. Closing a laptop lid on battery still sleeps it, and nothing portable overrides that. A running server does not pick up a changed setting until it restarts.
- **Hide-on-close is read at startup (14.7):** Wails takes it as a window option, so the Settings switch applies the next time EinCast opens.

- **Free a port (12):** parsers are tested on sample output; the real `lsof` path was run against a live listener on macOS. The Windows path (`netstat -ano`, `tasklist`, `TerminateProcess`) has been compiled and unit-tested on samples but never run on Windows; CI runs the unit tests there. Before relying on it, run it once against a real listener on Windows, including a localized one if possible.
- **Offer it where the failure happens (12), done in Phase 14.8:** when `Start` fails with "address already in use" (or Windows' "Only one usage of each socket address"), the Dashboard offers "Find what is using port N", which opens Settings → Free a port and looks straight away.

- **Upgrade renumbers item IDs (11):** a one-time loss of resume positions saved in browsers (ADR 0021). Nothing to do unless it matters to someone.
- **Per-library views on DLNA and the device page (11):** both list all libraries merged (DLNA merges folders of the same name). A top-level folder per library when there are several is the seam.
- **Library order (11):** libraries appear in the order they were added; there is no reordering.
- **Libraries on removable drives (11):** a missing folder stays listed with its error and is picked up again by a rescan (or a change event once it is back, if it was being watched). There is no "check again when a drive is mounted".
- **Measured at 50,000 files (Phase 13.7, macOS arm64, Apple silicon, local SSD):** 45,000 media items plus 5,000 subtitles in 100 folders. Scan 0.26 s; replacing the repo's contents 7 ms; one `State()` library pass (copy + `Summarize`) 1.8 ms; watching them all set up in 2.0 s and a new file noticed in 1 ms. Nothing needs fixing at this size. Not covered: a network share, a spinning disk, a cold file cache, or an app launched from Finder (its file-handle limit may differ from the terminal's 1,048,576). Re-run with `EINCAST_SCALE_FILES=50000 go test ./internal/adapters/fsscan ./internal/adapters/fswatch -run TestScale -v`.
- **Statistics cost (11):** `State()` sums every library's items on each event; caching them per library is the seam if a library grows to hundreds of thousands of files.

- **No real TV yet (10b/10c):** discovery was checked with a second process over real multicast on this Mac, and the browse path with a test that plays a TV's part. TV firmwares differ (Samsung, LG, Sony, Android TV, Kodi, VLC). Before v0.5 is called finished: try at least one TV and one app (VLC's UPnP list is the easiest), and expect small changes in `didl.go` (`protocolInfo`, `DLNA.ORG_PN`, `dc:date`) or `handler.go`.
- **Subtitles to TVs (10b):** the listing has no caption `<res>`. Samsung wants `sec:CaptionInfoEx` and a `CaptionInfo.sec` header, others a second `<res>` with `text/srt`. Needs a per-vendor decision and a route that serves `.srt` (today `/subtitle/{id}/{n}` serves WebVTT).
- **Thumbnails in the DLNA listing (10b):** `upnp:albumArtURI` pointing at `/thumb/{id}` would show pictures on the TV.
- **Converted streams for TVs (10b):** a TV that cannot decode a file gets no HLS fallback over DLNA.
- **DLNA search and sort (10b):** `Search` is not implemented and `SortCriteria` is ignored; the capabilities say so.
- **UPnP eventing (10b):** subscriptions are accepted and never delivered, on purpose (ADR 0020); a TV learns the library changed from `SystemUpdateID` when it browses.
- **Applying the TV setting without a restart (10c):** announced at server start, like the port. Live toggling needs `streaming.Service` to start and stop one discovery by name.
- **mDNS name conflicts (10a):** two EinCasts on one network both claim `eincast.local`. Probing and renaming (`eincast-2.local`) is the seam.
- **Network changes (10a/10c):** the announced addresses are fixed at start, like the listeners (see "Network changes" above).
- **SSDP on IPv6 (10c):** IPv4 only; the server listens on IPv4 only.
- **Client isolation and firewalls (10a/10c), hint done in Phase 14.10:** Settings → Network lists the usual causes (same network, client isolation, UDP 1900/5353, the first-run firewall prompt) and the wizard repeats the firewall prompt per system. The app cannot detect any of them; they are advice.

- **HLS on Chrome/Firefox (9b), done in Phase 17.1 (ADR 0031):** `hls.js` 1.6.16 is vendored and loaded only by a browser that cannot play HLS itself; `canPlayHLS()` asks the browser. Firefox 158 on macOS and Chrome 156 (native, and with native HLS hidden) played an HEVC MKV with it.
- **HLS never run on a real Apple device or TV (9b):** the output is verified against the real ffmpeg and a handwritten player. Before relying on it: open an MKV from an iPhone and an iPad on the LAN, then through a tunnel (segments authenticate by cookie only), and from a smart TV browser if there is one. Check seeking, pausing for a minute, and subtitles on a converted stream.
- **Seeking ahead during an encode (9b):** a full encode is playable from the start but cannot be jumped past what is done. A per-segment on-demand encoder is the larger alternative (ADR 0018).
- **Resume during an encode (9b):** a still-converting stream has no known length, so no position is saved until it ends. If that matters, the page could save against the source's duration from `/api/library`.
- **Hardware encoding (9b), done in Phase 17.4 (ADR 0032):** VideoToolbox (macOS), NVENC then QuickSync (Windows), each behind the same `hlsArgs`, with the processor as the automatic last resort and a setting, on by default. Only VideoToolbox has run on real hardware.
- **Audio and subtitle track choice (9b), done in Phase 17.3 (ADR 0031):** the player page lists a video's audio tracks and picks one; each track is its own conversion under `/hls/{id}/audio/{n}/`. Seam: one conversion with every audio track as an HLS alternative rendition (`EXT-X-MEDIA`), switched without restarting; it makes every conversion pay for tracks nobody uses.
- **Embedded subtitles (9a), done in Phase 17.2 (ADR 0031):** text tracks inside an MKV/MP4 are extracted to WebVTT on first use and cached. **Image subtitles** (Blu-ray PGS, DVD) are listed as "can't be shown" and not served; reading them needs OCR, burning them in needs a conversion per choice.
- **Tracks on the desktop Library screen (17.2/17.3):** only the player page lists them. The desktop list does not show which videos have several languages.
- **First embedded subtitle is slow on a slow disk (17.2):** finding a track reads the whole file; a network share can take a minute (timeout three). It is cached after that.
- **Switching audio restarts the conversion (17.3):** a second ffmpeg process for the same video; not seekable past what is converted. With the picture copied or the chip encoding it takes seconds.
- **Subtitle sync and styling (9a):** none; the file is shown as written.
- **Resume across addresses (9a):** positions live in the browser's storage per origin, so the LAN address and a tunnel address do not share them.
- **Desktop Library list (9a):** does not show which videos have subtitles or are converted.
- **Conversion limits (9b), still open (17.6):** 2 at once, 2 minutes idle, 30 s before a busy slot is taken over; all constants in `transcode.DefaultOptions`, not yet settings.
- **Disk use of conversions (9b), done in Phase 17.5 (ADR 0032):** a conversion needs `min(source size, 8 GiB) + 1 GiB` free on the cache disk or it is refused with `507 disk_full` and a plain message on the page. The check is a guess about one conversion; two at once can still fill a small disk, and ffmpeg then fails cleanly.
- **DLNA plays original files only (17.7), not written up:** a TV that lists EinCast over DLNA cannot get a converted stream. The advice for a TV that cannot decode a file (open the player page in the TV's browser) is not in the app or the README yet.
- **Phase 17 on real screens:** checked with a throwaway server, a real Chrome 156 (DevTools protocol) and Firefox 158.0.1 (Marionette) on macOS: a 40-second HEVC MKV with two audio languages and two subtitle tracks, played natively in Chrome, through hls.js in Firefox and in Chrome with native HLS hidden; subtitle switching; audio switching mid-playback; the full-disk message; an MP4 still plays directly. **Not run:** Safari, iPhone, iPad, any smart-TV browser, Edge, Windows at all, NVENC, QuickSync, a film longer than a minute. Each has a row in [TESTING-DEVICES.md](TESTING-DEVICES.md).

- **Watcher on very large libraries (8b):** on macOS every watched file costs a file handle (kqueue). When the system refuses, nothing is watched and the Library screen says so; Rescan still works. If that is common, the seam is a polling `FolderWatcher` behind the same port (a stat-walk every few seconds) used as the fallback.
- **Watcher on network shares (8b):** some SMB/NFS mounts deliver no change notifications, and nothing tells EinCast. The folder shows "Watching" and never signals. A "check every N minutes" setting is the seam.
- **Watching can't be turned off (8b):** no setting yet; `library.WithWatcher` is the single place to make it optional.
- **ffmpeg location (8c), done in Phase 14.9:** Settings → ffmpeg says whether it was found and where, shows an install command (`brew install ffmpeg`, `winget install ffmpeg`), and has "Locate ffmpeg…". A chosen location that stops working is an error, never a silent switch to another ffmpeg. The `winget` package name is unchecked on a real Windows machine. ffmpeg is still not bundled (licence and size); revisit after a licence review.
- **Thumbnail warm-up (8c):** pictures are made when first looked at; a background pass over a freshly scanned library (low priority, one at a time) would make the first scroll instant. The service's `Get` is the seam.
- **Thumbnail queue (8c):** requests that wait behind slow files each hold their caller for up to the 30 s timeout. A bounded queue that answers "busy" would let a page give up sooner.
- **ffmpeg and exotic formats (8c):** HEIC/AVIF and unusual containers depend on the installed ffmpeg's build. Nothing was tested beyond what the development machine's ffmpeg 8 decodes (H.264/MP4, PNG, WAV, MP3 with cover art).
- **Thumbnails never seen in the running window (8c):** the row layout and the player page grid are covered by type-checks, logic tests and HTTP tests, not by looking at them. Give them a visual pass before v0.3 ships, with the Library list, the Appearance setting and the tinted forced themes.
- **Forced theme on macOS (8a):** a theme that disagrees with the OS is a tint over the OS vibrancy, because Wails cannot change the macOS backdrop at runtime (ADR 0016). Windows follows the choice natively.
- **Library list is flat (8a):** it shows each file with its folder as a caption. A folder tree or grouping by folder is the seam if libraries turn out to be deeply nested.
- **Search is a substring match (8a):** over the path inside the library, every word must match. Fuzzy matching or searching metadata (titles, tags) would need a `Metadata` port.

- **Secret redaction (7a, done):** `app.Secret` prints `[redacted]` through slog and fmt. Wrap tunnel tokens in it as providers arrive (7c, 7d); `logbus.Handler` still does not scrub text after the fact.
- **Log gap marker (optional):** `Seq` already reveals dropped lines; the viewer does not yet render a "N lines dropped" row.
- **Log level control (Phase 6):** the console handler is fixed at Info and the UI sees Debug and up. A setting can raise or lower the console level without touching `logbus`.

- **Rate limiting (7a, done):** per-address and global token buckets guard `POST /login` (ADR 0008). The LAN token is 192 bits and needs none.
- **Short-lived signed stream links (7a, done):** playlists served through a tunnel carry per-item `?exp=&sig=` links that expire after 6 hours. On the home network they still carry the LAN token.
- **Token expiry:** the cookie lasts 30 days and the token until replaced. A "sign out all devices" action is the existing token rotation.
- **Windows file attributes:** the scanner still skips by name only (dot-files and known system names), not the Windows "hidden" attribute.
- **Cross-platform tests:** `scripts/check-cross.sh` proves compilation for macOS and Windows from one machine; behaviour on Windows is checked only by `.github/workflows/ci.yml`, which has not run yet because nothing is pushed.
- **Network changes (Phase 14.10):** the bound addresses are fixed at start. Settings → Network has "Refresh addresses", which restarts the server (and so turns remote access off). A watcher that notices the change and offers it is the seam.
- **Tailscale and mDNS addresses (Phase 7 / v0.5):** `netinfo` deliberately skips the Tailscale interface (CGNAT range); the tailscale provider reports its own URL. `eincast.local` arrives with mDNS.
- **Player page on old TVs:** `app.js` uses `fetch`, `async/await`, template-free string concatenation and `replaceChildren`. A very old smart-TV browser may need a lighter fallback; test on real devices before v1.0.
- **Settings (Phase 6):** `StartServer()` uses `streaming.NewConfig()` defaults (port 8765, name "EinCast"). Port and name come from persisted settings later; `Config` and its options are already the seam.
- **Player page (Phase 5):** `GET /` serves a placeholder string; the real page goes in `adapters/httpapi/web` via `embed.FS`.
- **Forwarded headers (7a, done):** `clientIP` honours `CF-Connecting-IP` / the last `X-Forwarded-For` entry only when the TCP peer is loopback. Assumes one proxy hop; revisit if a provider chains proxies.
- **Tailscale CORS, listener, remote status in the UI (7b, done):** `isLANOrigin` accepts `100.64.0.0/10` and `*.ts.net`; `LocalServer.Bind` adds the tailnet listener while Tailscale is on (ADR 0012); the bridge exposes `GetRemoteStatus`, `ListRemoteProviders`, `EnableRemote`, `DisableRemote`, `SetRemotePIN`, `ClearRemotePIN` and the `remote:status` event.
- **Tailscale not testable here (7b):** the provider was written against Tailscale's documented `ipnstate.Status` fields and tested with fixtures and a stub CLI, not a live node (none was installed on the development machine). Before relying on it: run EinCast on a machine with Tailscale, turn it on, and open the address from a second tailnet device. If the field names ever change, `status.go` is the only place to fix.
- **ACL and firewall blind spot (7b):** a Tailscale ACL or the OS firewall can block the port while EinCast shows a working-looking address. If it proves common, add a hint on the Remote screen.
- **UI never rendered in a window (7b):** the Remote screen and Connect tab are covered by type-checks, logic tests and server-render smoke tests, not by looking at them in the running app. Give them a visual pass (spacing, the radio cards, the title-bar badge) before v0.3.
- **Playlist over a public address (7b):** a player cannot sign in with a PIN, so the Connect card shows no bare playlist link for public sources; the person signs in on the player page and downloads the playlist there (links valid six hours). A friendlier route (a one-time link generated in the app) can come later.
- **Tunnel death (7c, done):** `remote.Service` polls `Tunnel.Status()` once a second per run, follows address changes, and takes remote access down when a provider gives up (ADR 0007). A `Done()` channel on the port would make it instant; not needed yet.
- **Quick tunnel never run against the real cloudflared (7c):** none is installed on the development machine. The output parser is written to the documented command and tested against realistic banners, hostile lines and a stub script, and the whole path (supervisor, provider, service, HTTP) is tested end to end with a stub. Two things are therefore unverified: (1) the exact banner wording of the current cloudflared; (2) whether an existing `~/.cloudflared/config.yml` stops `cloudflared tunnel --url` from creating a quick tunnel (reports exist; the docs say nothing). If it does, the fix is in one place, the `Args` in `quick.go` (for example an explicit empty `--config`). Before v0.3: run it once for real.
- **"Locate cloudflared…" picker (7d, done):** an optional setting shared by both Cloudflare tunnels, with a native file dialog (`ChooseFile`). A saved location that stops working is an error, never silently replaced by another program.
- **Named tunnel and ngrok never run for real (7d):** like the quick tunnel, neither was run against its real service (no Cloudflare account, ngrok account or `cloudflared` available). What is verified: the named tunnel's "up" line, `Registered tunnel connection` (Info level, from Cloudflare's source), and that `cloudflared` reads `TUNNEL_TOKEN` from the environment (Cloudflare's own Docker examples); ngrok's API from its published v2.4.0 source. What is not: that a real token starts a real tunnel, that ngrok's `agent.Connect` returns only once signed in, and the exact wording of its authentication errors (the "did not accept the authtoken" hint matches on `authentication failed`, `ERR_NGROK_105` and the word authtoken). One real run of each before v0.3.
- **Named tunnel needs the port to match (7d):** the origin address lives in the Cloudflare dashboard, not in EinCast. If the server's port changes, the dashboard must change with it, and nothing detects a mismatch (the tunnel comes up and serves errors). The setup form says so. A check that requests through the tunnel actually reach this server is the seam.
- **Named tunnel is "up" on the first connection (7d):** `cloudflared` registers up to four connections; the first one flips the status to Running. If all drop but the process stays alive, the status stays Running.
- **Measured throughput (7c, not built):** the brief asks for the remote panel to show measured upload throughput. The Remote screen shows a standing note about upload speed instead. Measuring needs per-request byte counts from tunnelled requests, which the access log already records; summing them over a window is the seam.
- **Quick tunnel limits (7c):** Cloudflare documents 200 in-flight requests per quick tunnel (more get `429`), no Server-Sent Events, and no uptime guarantee. A player that opens many parallel ranges could meet the first. The terms-of-service notice is shown once per provider before it can be turned on.
- **Notice memory is per browser profile (7c):** "I understand" is remembered in the webview's local storage, not in Go settings, since it is a courtesy and not a security control. Clearing the app's web data shows it again.
- **Sign out one device / list sessions (later in v0.3):** the session registry already holds the client address and expiry per session; the service exposes only a count. A revoke-by-id method is the seam.
- **ngrok's free plan (7d):** a very small data allowance and a browser warning page. The player page sends `ngrok-skip-browser-warning` on its requests; the first visit to the address in a browser can still show ngrok's page, and the notice says the plan is for testing. ngrok's SDK logging is switched off, so its own diagnostics are not in the log; errors and connection changes are.
- **Dependencies (7d):** the ngrok SDK brought `muxado`, `protobuf` and a few others, and raised `x/net`, `x/sys`, `x/text` and `x/crypto`. All tests and the macOS and Windows builds pass with them.
- **Local browser while a tunnel is up:** the owner's own browser at `http://localhost:…` must enter the PIN while a public tunnel runs (ADR 0008, point 3). Revisit if that annoys in practice.

- `Transcoder` port (v0.4) — not added until transcoding starts.
- Windows title bar: Phase 1 keeps the native frame on Windows (Mica backdrop only). A frameless window needs custom min/max/close controls; decide in Phase 6 when the cross-platform build is checked.
- Multiple libraries (v1.0): `media.NewID` hashes only the relative path, so the same relative path in two roots collides. Add a library identifier to the hash input then (ADR 0005).
- Windows "hidden" file attribute: the scanner skips dot-prefixed and well-known system names only. Honouring the Windows hidden attribute needs a build-tagged helper; decide in Phase 6.
- Folder drag-and-drop onto the onboarding card (Wails `EnableFileDrop`): the button covers v0.1.
