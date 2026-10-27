# Changelog

All notable changes to EinCast. Newest first.

The release workflow publishes the section for the tag it is building as the body of the GitHub release (`scripts/release-notes.sh`), so write each section for the person downloading the app, not for a developer. A tag with no section here falls back to the commit subjects. How to cut a release: [docs/RELEASING.md](docs/RELEASING.md).

## [1.1.1] - 2026-10-27

The theme of this release is *make it adoptable*: a stranger should be able to download EinCast, trust it, and have a video playing on their TV in a couple of minutes.

### What's new

**A better first five minutes**
- The access token is on by default for new installs, and the QR code already carries it. Existing installs keep their setting.
- A first-run wizard: pick a folder, start sharing and scan the QR code, then open it on a TV or phone. A live indicator turns green when the first device connects.
- The server can start by itself when EinCast opens (Settings → Startup).
- EinCast keeps the computer awake while it is sharing.
- Launch at login, plus menu-bar / tray mode: closing the window keeps sharing, and Quit really stops.
- When the port is already in use, a button takes you to Settings → Free a port with the port filled in.
- ffmpeg guidance: EinCast tells you whether ffmpeg is installed, what it unlocks (thumbnails, MKV/HEVC on iPhone) and gives a copyable install command.
- A "Refresh addresses" action for when the Wi-Fi changes, and hints for Wi-Fi client isolation and a blocking firewall.

**A library that looks like a library**
- Names are understood: `Movie.Name.2019.1080p.mkv` becomes a title and a year, and `Show.S02E05`, `Show - 2x05` and `Season 2/Episode 5` become episodes.
- Episodes are grouped under their show and season.
- Local artwork is picked up (`poster.jpg`, `folder.jpg`, `cover.jpg`, `<name>-poster.jpg`) and Kodi-style `.nfo` files supply the title, year and plot.
- The player page is now a poster grid with a "Continue watching" row and a detail view.
- Search covers title, show and year.

**Profiles and Continue watching**
- Profiles such as kids and adults, each with an optional PIN (stored hashed, never in plain text). A hidden title is simply not found for a profile that may not see it.
- A profile picker on the player page and a Settings screen to manage profiles.
- Resume positions and "Continue watching" are now kept on the server per profile, so they follow the person from device to device.

**Playback**
- Chrome, Firefox and Edge now play converted video (hls.js is bundled).
- Text subtitle tracks inside MKV files are extracted and shown like sidecar subtitles.
- An audio track picker for files with more than one language.
- Hardware video encoding where the computer has it (Apple VideoToolbox, NVIDIA NVENC, Intel Quick Sync), with an automatic fall back to the processor.
- Before converting a film EinCast checks there is room on the disk, and refuses with a clear message instead of filling it.

**Burmese**
- The app and the player page are available in Burmese, chosen automatically from the browser or profile.
- Myanmar Unicode file names scan, display and search correctly.
- A Burmese README draft: [README.my.md](README.my.md).

**Updates and support**
- EinCast tells you when a newer version is available (Settings → About, with the release notes), and shows each download's SHA-256. The check can be turned off, and nothing identifying is sent.
- Settings → Copy diagnostics copies the version, system, a redacted settings summary and the latest log lines, with tokens, addresses and folder names removed.
- Links to report a problem, suggest an idea and read the privacy page.
- Settings → Reset erases everything EinCast saved (settings, profiles, resume positions, cache, secrets in the Keychain / Credential Manager, the login item) and opens again as a first run.

**Project**
- LICENSE, CONTRIBUTING, Code of Conduct, Security policy, issue templates, a privacy statement (`docs/PRIVACY.md`) and a "Why not Jellyfin?" page.

### Fixes

- Windows builds ship the bundled hls.js byte for byte; line-ending conversion in a Windows checkout no longer alters it.
- Release builds now run the full macOS and Windows test suite first, so a release is never built from a red commit.
