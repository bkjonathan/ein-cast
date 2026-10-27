# Plan: EinCast v1.1.1 — "Make it adoptable"

The goal of this release is not more features. It is that a stranger can download EinCast, trust it, play a video on their TV within two minutes, and want to tell someone.

**Positioning (one sentence, used in every task below):**
> The simplest way to play your laptop's videos on any TV. No server, no account.

**Progress note (Phase 14):** built 14.3–14.11. 14.1 and 14.2 (signing) stay open: they need accounts and certificates. ADR 0023 stays reserved for the signing decision; 0024 (token default) and 0025 (keep awake) took their planned numbers, and 14.7 is ADR 0030 because 0026–0029 belong to Phases 15, 16, 18 and 19. Windows tray, Run key and hide-on-close are compiled and unit-tested but never run on Windows (see [TESTING-DEVICES.md](TESTING-DEVICES.md)).

**How this plan follows the project rules** ([CLAUDE.md](../CLAUDE.md)): phases continue from 13, each phase ends with a build + tests + three understanding questions, every real decision gets an ADR (numbering continues at 0023), and anything not built goes into [ROADMAP.md](ROADMAP.md) "Seams", not into code.

Sizes: **S** = under a day, **M** = 1–3 days, **L** = a week or more. Items marked 🔒 need something outside the code (money, an account, a physical device).

---

## 1. Weakness → where it is fixed

| # | Weakness (from the review) | Fixed in |
|---|---|---|
| 1 | Jellyfin already exists; unclear why pick EinCast | Phase 20 (positioning, comparison), whole plan stays "simple" |
| 2 | No media metadata (posters, series grouping) | Phase 15 |
| 3 | Server runs only while the laptop is awake and the app is open | Phase 14 (keep awake, launch at login, auto-start) |
| 4 | macOS Gatekeeper / Windows SmartScreen warnings | Phase 14 (signing) |
| 5 | No auto-update | Phase 19 |
| 6 | Access token off by default | Phase 14 |
| 7 | No user profiles | Phase 16 |
| 8 | Features never verified on real devices | Phase 13 |
| 9 | Chrome/Firefox can't play converted video; no audio/subtitle tracks | Phase 17 |
| 10 | Burmese audience not served | Phase 18 |
| 11 | Missing LICENSE, CONTRIBUTING, screenshots, one author | Phase 20 |
| 12 | Cloudflare terms, upload-speed limits | Phase 14 (kept honest in UI), Phase 20 (docs) |

---

## 2. Order of work and why

```
13 Reality check ──► 14 First five minutes ──► 15 Library that looks like a library
                                                       │
                              16 Profiles ◄────────────┘
                                   │
                 17 Playback gaps ─┤
                 18 Burmese / i18n ┤   (17, 18, 19 can run in any order after 16)
                 19 Auto-update ───┘
                                   │
                          20 Community launch kit   (start the non-code parts at Phase 14)
```

Phase 13 comes first because the roadmap lists about ten things that were written but never run on real hardware. Do not market features you have not seen work.

---

## Phase 13 — Reality check (no new features)

Goal: every claim in the README is true on a real device.

- [x] **13.1 Device test matrix (S).** Create `docs/TESTING-DEVICES.md`: a table of device × feature × result × date × EinCast version. Rows: iPhone Safari, iPad Safari, Android Chrome, Windows Chrome/Edge, macOS Safari/Chrome, one Samsung or LG TV (browser + DLNA), Android TV / Chromecast with Google TV, VLC desktop (UPnP list), VLC mobile, Infuse. Columns: connect by QR, play MP4, play MKV (HLS), subtitles, seek, resume, playlist.m3u, DLNA browse.
- [ ] **13.2 Run the matrix 🔒 (M).** Needs the physical devices. Record failures as GitHub issues, not in your head.
- [ ] **13.3 Fix what fails (M–L).** Expect small changes in `didl.go` (`protocolInfo`, `DLNA.ORG_PN`), subtitle `<res>` for TVs, and the player page on old TV browsers.
- [ ] **13.4 Windows pass 🔒 (M).** Run on a real Windows 11 machine: install, start server, firewall prompt, Free a port (`netstat`/`taskkill` path has never run on Windows), Mica look, installer, uninstall.
- [ ] **13.5 Tunnel pass 🔒 (S each).** One real run each: Tailscale (second device), Cloudflare quick tunnel (check the banner parser and the `~/.cloudflared/config.yml` concern), Cloudflare named tunnel, ngrok.
- [ ] **13.6 Visual pass (S).** Open every screen in the running window: Library list, Remote screen, title-bar badge, Appearance light/dark/forced theme, thumbnails grid. Fix spacing and contrast.
- [x] **13.7 Large-library check (S).** Generate a folder with ~50,000 small files; measure scan time, `State()` cost, watcher behaviour on macOS (kqueue file handles). Fix only if it is bad; otherwise note numbers in ROADMAP.

**Done when:** `TESTING-DEVICES.md` has no empty cells for the "must work" rows (iPhone, iPad, Windows Chrome, macOS Safari, VLC), and every failure is either fixed or listed in the README under "Known limits".

---

## Phase 14 — First five minutes (trust and friction)

Goal: download → first video playing on a TV in under two minutes, with no scary warnings and no open library.

- [ ] **14.1 macOS signing + notarization 🔒 (M).** Apple Developer Program membership (paid, yearly). Add the certificate and notarization credentials as GitHub Actions secrets; extend `release.yml` to sign, notarize and staple. Remove the `xattr` instruction from release notes. ADR 0023.
- [ ] **14.2 Windows code signing 🔒 (M).** Options: a paid certificate, Azure Trusted Signing, or a free program for open-source projects (check current eligibility, e.g. SignPath Foundation). Sign the installer and portable exe in CI. SmartScreen reputation still builds slowly; say so in the README.
- [x] **14.3 Access token on by default (M).** New installs generate a token on first run and the QR code already contains it. Existing installs keep their setting. The Smart TV option stays exclusive with the token (ADR 0011/0020): when the person turns on Smart TVs, show a plain warning and let them choose. Update the README line that says the token is off by default. ADR 0024.
- [x] **14.4 First-run wizard (M).** Replace the single "Choose Media Folder" screen with 3 steps: (1) pick folder, (2) start server and show QR, (3) "Open this on your TV or phone" with a live indicator that turns green when the first device connects (the access log already knows). Skippable.
- [x] **14.5 Start server automatically after choosing a folder (S).** A setting "Start the server when EinCast opens" (default on after first run).
- [x] **14.6 Keep the computer awake while serving (M).** macOS: IOKit power assertion (`IOPMAssertionCreateWithName`) or spawn `caffeinate -i`. Windows: `SetThreadExecutionState`. Put it behind a `PowerGuard` port so the app layer stays pure; build-tagged adapters. Setting "Keep my computer awake while sharing" (default on), released when the server stops. ADR 0025.
- [x] **14.7 Launch at login + menu-bar/tray mode (M).** Closing the window keeps sharing from the tray/menu bar; "Quit" really stops. This fixes "server only runs while the app is open" for most people. Wails v2 has no built-in tray, so check the options (a small tray library behind a `Tray` port, or hide-window-on-close only) and record the choice in an ADR. Setting "Open EinCast at login".
- [x] **14.8 Port-in-use link (S).** From ROADMAP seam: when Start fails with "address already in use", the Dashboard offers a button into Settings → Free a port with the port filled in.
- [x] **14.9 ffmpeg guidance (M).** First-run and Settings detect ffmpeg. If missing, show what it unlocks (thumbnails, MKV/HEVC on iPhone) with a copyable install command (`brew install ffmpeg`, `winget install ffmpeg`) and a **Locate ffmpeg…** picker (ROADMAP seam, same as cloudflared's). Do **not** bundle ffmpeg in 1.1 (licensing and size); note it in ROADMAP.
- [x] **14.10 Network-change and firewall hints (S).** "Refresh addresses" action when the Wi-Fi changes (seam from ROADMAP). A Settings hint for Wi-Fi client isolation and blocked UDP 1900/5353 when a TV can't see the server. First-run macOS/Windows firewall prompt explained in the wizard.
- [x] **14.11 Honest warnings stay (S).** Keep the Cloudflare terms notice and ngrok warning. Recommend Tailscale first in the Remote screen order.

**Done when:** a person who has never seen EinCast, on a clean Mac and a clean Windows PC, gets a video playing on their phone in under two minutes with no manual terminal commands. Time it with a stopwatch on three people.

---

## Phase 15 — A library that looks like a library (local metadata first)

Goal: movies and shows look recognisable, with no accounts and no internet needed. This is the biggest gap against Plex/Jellyfin, so do the cheap 80%.

- [x] **15.1 Filename parser (M).** Domain function (pure, table-driven tests): `Movie.Name.2019.1080p.mkv` → title + year; `Show.S02E05.mkv`, `Show - 2x05`, `Season 2/Episode 5` → show, season, episode. Strip release junk (`1080p`, `BluRay`, `x265`). Keep the original file name always available. ADR 0026.
- [x] **15.2 Series grouping (M).** Library items gain optional `Show`, `Season`, `Episode`, `Year` fields (DTO too). Desktop list and player page group episodes under a show and seasons under that, sorted by episode number.
- [x] **15.3 Local artwork (M).** Pick up `poster.jpg`, `folder.jpg`, `cover.jpg`, `<name>-poster.jpg` next to the media (same approach as sidecar subtitles). Served through the existing thumbnail route with the same path guard. Fall back to the ffmpeg frame grab.
- [x] **15.4 Local `.nfo` reading (S–M).** Read title, year and plot from Kodi-style `.nfo` if present. Many users already have them. Skip if it grows; it is optional.
- [x] **15.5 Player page: poster grid + detail view (L).** Rework `web/` into a grid of posters, a "Continue watching" row, a detail view (title, year, plot, play button, subtitle picker). Keep it dependency-free and fast on old TV browsers (existing note).
- [x] **15.6 Search over metadata (S).** Search covers title, show, year. Keep the substring match; do not add fuzzy matching yet.
- [ ] **15.7 Optional online metadata (stretch, M), decided: 1.2 (ADR 0026).** A `Metadata` port with a TMDB adapter, **off by default**, using the person's own API key stored in the keychain (like tunnel tokens). Needs a privacy line in the UI: "turning this on sends titles to themoviedb.org". Decide in an ADR whether this ships in 1.1 or 1.2. Recommendation: **1.2**, after 15.1–15.6 prove useful.

**Done when:** a folder of 20 episodes plus 10 movies shows as 10 posters + 1 show with seasons, on the TV browser and the desktop Library screen, with zero network calls.

---

## Phase 16 — Profiles (kids / adults) and continue watching

Goal: the household use case. Needs a design decision first (ROADMAP already says so); decide before coding.

- [x] **16.1 Design + ADR 0027 (S).** Recommended design to review:
  - A **profile** = name + avatar colour + optional 4–6 digit PIN + allowed libraries + allowed kinds (+ optional "hide folders matching…").
  - The **access token** still gets a device onto the network; a **profile picker** appears on the player page after that.
  - Profile choice lives in a signed, HttpOnly cookie (reuse the session machinery from remote auth). The server checks the profile on `/api/library`, `/stream`, `/thumb`, `/subtitle`, HLS and the playlist — **enforced in the service layer, not only the UI** (same rule as ADR 0008).
  - DLNA and the Smart-TV mode cannot know the profile, so: DLNA serves only libraries marked "visible to TVs". The exclusive-with-token rule stays.
- [x] **16.2 Domain + settings (M).** `Profile` type, persisted in `settings.json`; PINs hashed, never stored plain. Default profile "Everyone" so existing installs behave the same.
- [x] **16.3 Server enforcement (M).** Middleware resolves the profile; library queries are filtered by it. Tests: a kids profile gets 404 (not 403) for an adults-only `/stream/{id}`, including with a guessed ID; playlist and HLS respect it too.
- [x] **16.4 Profile picker on the player page + Settings screen to manage profiles (M).**
- [x] **16.5 Server-side resume and "Continue watching" (M).** Store position per profile on the server (small JSON file), not only in browser storage. This also fixes the "resume differs between LAN and tunnel addresses" seam. Keep the browser value as an offline fallback.
- [x] **16.6 Item ID stability (S).** Check ADR 0021's renumbering note; if a migration is cheap, keep old resume positions alive.

**Done when:** a child's profile on an iPad cannot see, search, stream or get a playlist entry for adult content, proven by tests, and "Continue watching" follows the person between phone and TV browser.

---

## Phase 17 — Playback gaps

Goal: the "it just plays" promise holds for the common failures.

- [ ] **17.1 hls.js for Chrome/Firefox/Edge (M).** Vendor `hls.js` into `web/` (CSP line, version pinned in the ADR). Change `canPlayHLS()` so desktop browsers get converted streams too. Biggest win per hour in this phase.
- [ ] **17.2 Embedded subtitle extraction (M).** ffmpeg extracts text tracks (SRT/ASS) from MKV into the cache, served as WebVTT like sidecars. Image subtitles (PGS) stay unsupported and the UI says so.
- [ ] **17.3 Audio track picker (M).** Expose the probe's stream list per item; let the player choose audio. HLS session gets an audio-stream argument.
- [ ] **17.4 Hardware encoding (M).** macOS VideoToolbox first (`h264_videotoolbox`), then NVENC/QuickSync on Windows behind the existing `hlsArgs` plan branches, with automatic fallback to `libx264` if the encoder fails. Setting "Use hardware acceleration" (default on).
- [ ] **17.5 Disk safety for conversions (S).** Free-space check before starting an encode; refuse with a clear message instead of filling the disk.
- [ ] **17.6 Conversion limits as settings (S).** Expose `DefaultOptions` (concurrent conversions, idle timeout).
- [ ] **17.7 Fallback for TVs over DLNA (S, optional).** Document that DLNA plays original files only; point TVs that can't decode a file to the TV's browser/the player page.

**Done when:** an MKV with HEVC plays with the right audio language and subtitles in Chrome on Windows, Safari on iPhone, and the TV browser, with matrix results recorded.

---

## Phase 18 — Burmese and i18n

Goal: first-class Burmese, and a structure other communities can add languages to.

- [ ] **18.1 i18n foundation (M).** `react-i18next` (or a tiny typed dictionary if you want fewer dependencies; decide in ADR 0028). Move every user-visible string out of components into `locales/en.json`. Language setting: System / English / မြန်မာ. Strict typing for keys so a missing key fails `tsc`.
- [ ] **18.2 Player page i18n (S).** The player page is plain JS; give it a small `t()` with the same JSON, served by the Go side, language chosen by browser or profile.
- [ ] **18.3 Burmese translation (M).** `locales/my.json`. Have a native speaker review it; don't rely on machine translation for the UI or release notes. Keep terms consistent (e.g. "library", "profile", "token").
- [ ] **18.4 Font and rendering check (S).** Verify Myanmar Unicode renders correctly in the Wails webview on macOS and Windows and on iPhone/TV browsers; bundle a Myanmar-capable font (e.g. Noto Sans Myanmar, OFL licence) if system fonts are weak. Check line-height so stacked marks aren't clipped by tight glass UI.
- [ ] **18.5 Unicode vs Zawgyi (S).** Many older Burmese files and phones used the Zawgyi encoding. Make sure file names in Myanmar Unicode scan, display and search correctly, normalise Unicode (NFC) before searching, and document that Zawgyi file names may look wrong and how to rename. Don't auto-convert in 1.1.
- [ ] **18.6 Burmese README + docs (S).** `README.my.md` with install, first run and troubleshooting, linked from the top of the English README.
- [ ] **18.7 Locale-aware formatting (S).** Dates, sizes and numbers through `Intl`. Optional Burmese numerals off by default.

**Done when:** the whole desktop app and player page can be switched to Burmese with no English leftovers on the main paths, and a Burmese-speaking tester signs off.

---

## Phase 19 — Auto-update 🔒

Goal: people on 1.1.1 get 1.1.2 without hunting for it. Depends on Phase 14 signing.

- [ ] **19.1 Decide the mechanism (S).** ADR 0029. Simplest honest option: check the GitHub Releases API on launch (and daily), compare versions, show "Version X is available" with a button that opens the download page. Full silent self-update is much harder with Wails and signing; leave it for later.
- [ ] **19.2 `Updater` port + GitHub adapter (M).** Port in `app/ports.go`; adapter calls the Releases API with a timeout; no telemetry, no identifiers sent.
- [ ] **19.3 UI + setting (S).** "Check for updates" (default on) with an off switch, the notice in Settings → About, and the release notes shown.
- [ ] **19.4 Verify downloads (S).** Publish SHA-256 checksums with each release (CI step). Show them in the update dialog.
- [ ] **19.5 Release notes template (S).** `release.yml` generates "What's new / Fixes / Known limits"; remove the "not signed" paragraph once 14.1/14.2 are done.

**Done when:** a 1.1.1 build, pointed at a test release 1.1.2, shows the update notice, and turning the setting off stops the network call (test with a fake).

---

## Phase 20 — Community launch kit (start early, finish last)

Goal: when the release lands, a visitor understands it in 30 seconds and a contributor can start in 30 minutes. Most of this is not code, so begin during Phase 14.

**Repository basics**
- [ ] **20.1 LICENSE (S).** There is none today, which legally means nobody may use or copy the code. Pick one: MIT (simplest), Apache-2.0 (patent grant), GPLv3 (keeps forks open). Check that dependency licences allow it. Add SPDX note in README.
- [ ] **20.2 CONTRIBUTING.md (S).** How to run, test, the architecture rules (link ADR 0001/0006), commit style, what a good PR looks like, the dependency check script.
- [ ] **20.3 CODE_OF_CONDUCT.md + SECURITY.md (S).** SECURITY.md matters because the app exposes a server: how to report a vulnerability privately.
- [ ] **20.4 Issue and PR templates (S).** Bug report asks for OS, EinCast version, device, and the log excerpt ("Copy diagnostics" below). Feature request template says "does this keep EinCast simple?".
- [ ] **20.5 Labels + 10 "good first issue"s (S).** Candidates: a language translation, a filename-parser test case, a device report for the matrix, a docs fix, a Windows hidden-attribute scanner (ROADMAP seam), mDNS name-conflict rename.

**Product-facing**
- [ ] **20.6 README rewrite (M).** Order: one-sentence pitch → 20-second demo GIF → screenshots (light, dark, Burmese, player on phone and TV) → download buttons → "Why not Jellyfin?" honest table → quick start → privacy & security → known limits. Move the long status paragraph into ROADMAP.
- [ ] **20.7 Demo video, 60–90 s (M).** Screen capture + a phone/TV shot: pick folder → QR → playing. A Burmese-narrated or subtitled version too.
- [ ] **20.8 Landing page (M).** Static page on GitHub Pages: pitch, download for macOS/Windows (links to latest release), screenshots, FAQ, Burmese/English switch.
- [ ] **20.9 "Why not Jellyfin?" doc (S).** One page: when to use EinCast, when to use Jellyfin/Plex/Infuse. Honest; it builds trust.
- [ ] **20.10 Privacy statement (S).** "No accounts, no analytics, no phoning home. The only network calls: the update check (can be turned off) and anything you enable (tunnels, optional metadata)." Make sure that sentence stays true.
- [ ] **20.11 Copy diagnostics button (S).** Settings → "Copy diagnostics": version, OS, redacted settings summary, last 200 log lines (secrets already redacted by `app.Secret`). Bug reports become useful, with no telemetry.
- [ ] **20.12 In-app "Send feedback" / "Report a problem" links (S).**

**Distribution**
- [ ] **20.13 Package managers (M).** Homebrew cask, winget manifest, Scoop bucket. Each is a small PR to a third-party repo; start after signing is done.
- [ ] **20.14 Checksums + release hygiene (S).** Publish SHA-256 sums; keep `RELEASING.md` current.

**Community**
- [ ] **20.15 Where the people are (S).** Create a GitHub Discussions space. For Burmese users, a Facebook group or page and a Telegram group are probably where people already are (verify with a few real users first; don't assume). Pin a short "start here" post in each, in both languages.
- [ ] **20.16 Five-user test before launch 🔒 (M).** Sit with 5 real people (family, friends, 2 from the Burmese community). Watch silently; note every hesitation. Fix the top 3 issues before announcing. This matters more than any feature.
- [ ] **20.17 Launch posts (S).** One factual post per channel: what it is, what it is not, the screenshot, the download, the issue link. Offer to help install. Be present for the first week answering.
- [ ] **20.18 Roadmap in public (S).** Pin a GitHub Project/Discussion with "next up" so people see direction and can vote.

**Done when:** the README passes the 30-second test with someone who has never heard of the project, and a first-time contributor can open a small PR following only CONTRIBUTING.md.

---

## 3. Stretch / v1.2 candidates (decide after launch feedback)

- **Headless mode (`eincastd`)** — a second composition root (`cmd/eincastd`) running the same server with no window, for a NAS, Raspberry Pi or Docker. The hexagonal layout (only `bridge` imports Wails) makes this cheap and answers "always-on" for power users. Needs a config file and a tiny admin page. Only build if people ask.
- **TMDB metadata** (Phase 15.7) with the person's own key.
- **Per-segment on-demand encoder** so seeking works during a conversion (ADR 0018 alternative).
- **Chromecast / AirPlay** from the player page (already in "Later ideas").
- **More languages** contributed by the community once i18n exists.
- **Bundled ffmpeg** after a licence review.
- **Library reorder, per-library views on DLNA** (ROADMAP seams).

## 4. What not to build in this release

- User accounts with passwords, a cloud service, sync, ads or analytics.
- A plugin system, a transcoding farm, live TV/DVR, a mobile app.
- More tunnel providers. Four is enough.
- A database. The in-memory repo plus small JSON files stay until a real library size forces the issue.

Each of these makes EinCast look more like Jellyfin and less like itself.

---

## 5. Release gate (all must be true before tagging v1.1.1)

1. CI green on macOS and Windows, including `go test -race`, `scripts/check-deps.sh`, frontend typecheck and tests.
2. Device matrix filled in for the "must work" rows (Phase 13).
3. Signed and notarized macOS build; signed Windows installer (Phase 14). If signing slips, ship as a pre-release tag (`v1.1.1-rc1`) and say so.
4. Fresh-install stopwatch test: under two minutes, three people, both OSes.
5. Access token on by default for new installs; Smart-TV warning present.
6. Profiles enforced server-side with tests (Phase 16).
7. Burmese reviewed by a native speaker (Phase 18).
8. LICENSE, README with screenshots, CONTRIBUTING, SECURITY, templates are in `main`.
9. ROADMAP updated: finished items marked done, new seams recorded.
10. `README.md` and `wails.json` version bumped; `RELEASING.md` checklist followed.

## 6. How to know it worked (first 60 days)

Pick numbers before you launch so you can't fool yourself:
- 5 of 5 test users reach first playback unaided.
- Install-to-first-play time under 2 minutes (median).
- At least 50 stars or 200 downloads from the first announcement. Treat anything less as a message about positioning, not about code quality.
- At least 5 issues from people you don't know, and 1 outside pull request (a translation counts).
- The top 3 complaints become the next release's plan.

## 7. Decisions needed from you

1. **Licence:** MIT, Apache-2.0 or GPLv3?
2. **Signing budget:** are you willing to pay for an Apple Developer account and a Windows signing option? This decides whether v1.1.1 is a full release or a pre-release.
3. **Tray/menu-bar:** hide-to-tray on close (recommended) or keep "window closed = app quits"?
4. **Token default:** on for new installs (recommended), with Smart TVs as an explicit opt-in that turns it off?
5. **Online metadata:** ship TMDB in 1.1 or wait for 1.2 (recommended)?
6. **Burmese reviewer:** who will check the translation?
7. **Real devices:** which TV(s) and phones can you test on?
