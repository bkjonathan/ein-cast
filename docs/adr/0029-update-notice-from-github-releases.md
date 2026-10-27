# 0029 — Updates: tell the person, don't install for them

## Context
Phase 19 exists so that someone on 1.1.1 hears about 1.1.2 without hunting for it. A
real self-updater (download, verify, swap the running app, restart) is a different
size of problem. On macOS it means replacing a running, notarized `.app`; on Windows,
replacing a running `.exe` or driving the installer. Both depend on code signing
(Phase 14.1 and 14.2), which has not happened: an updater that installs unsigned code
fetched from the network is the thing a signing key exists to make safe.

EinCast also promises something (Phase 20.10): *no accounts, no analytics, no phoning
home*. A check for updates is a call home. It has to be small, visible, and
switchable, or the sentence stops being true.

## Decision
**Notice, not install.** The app asks GitHub which release is newest, compares it with
the running version, and, when it is newer, says so and offers a button that opens the
release page in the browser. The person downloads and installs it as they did the first time.

**What is sent.**
- One `GET https://api.github.com/repos/bkjonathan/ein-cast/releases/latest` and, if the
  release has a `SHA256SUMS.txt`, one `GET` of that file. Nothing else.
- No cookie, token, account, query string, machine identifier, or installed version. The
  `User-Agent` is the fixed word `EinCast` (GitHub requires one) so it is the same for
  everyone. GitHub sees the computer's IP address, as every web server does. A test asserts all of this.
- `/releases/latest` is GitHub's "newest release that is neither a draft nor a
  pre-release", so a release candidate is not offered to people on a stable version.

**When.** Fifteen seconds after start-up, then whenever the last good answer is a day
old. The service wakes hourly and asks the network only if a check is due, so turning
the setting on takes effect within the hour and a laptop that slept through the day
catches up. A failed check is retried at the next wake.

**The setting** `CheckForUpdates` is on by default (a settings file written before it
existed reads as on, like `KeepAwake`). Off means EinCast makes **no request by itself**.
The **Check now** button is the person asking and always works; the setting governs only
what EinCast does unprompted. A build from source (version `dev`) has nothing to compare
and never asks, button included.

**Shape.**
- `internal/domain/version`: parse and compare release numbers by semantic-versioning
  precedence (`1.10.0` is newer than `1.9.0`; `1.2.0-rc1` is older than `1.2.0`).
- `app.ReleaseSource` port (`Latest(ctx) (Release, error)`). The plan called it
  `Updater`; it is named for what it does, because it reads and never installs.
- `updates.Service`: owns the status, publishes `UpdateChanged`, runs the timer. The
  `githubrelease` adapter implements the port; the response is size-limited and the
  whole call has a 15 s deadline.
- The bridge exposes `GetUpdateStatus`, `CheckForUpdatesNow` and `OpenUpdatePage`. **The
  release address never crosses the bridge**: the UI asks Go to open "the page", and Go
  opens the https address it found. The UI cannot be made to open anything else.
- Release notes come from the network, so they are untrusted text: capped at 8,000
  characters and rendered as text, never as markup.

**Checksums.** The release workflow's last job writes `SHA256SUMS.txt`
(`<hash>  <file>`, readable by `shasum -a 256 -c`) once all three files are attached,
and refuses to publish it unless there are exactly three. The adapter reads that file
(https only, optional) and the update card shows each file's SHA-256 with a copy button.
**Release notes** are written by `scripts/release-notes.sh` from the commits since the
previous tag: What's new, Fixes, Known limits (`.github/release-known-limits.md`), and how to check a download.

## Consequences
- This is EinCast's only call to the internet that happens without the person enabling
  something (announcing itself on the local network, mDNS, is separate).
  The privacy statement (Phase 20.10) must keep saying so, and anyone adding another
  automatic call must change this ADR first.
- Updating stays manual: a person who ignores the notice stays on the old version, and
  nothing is ever replaced behind their back.
- **The checksums catch a corrupted or truncated download. They are not a signature.**
  They come from the same release as the files, so someone who could replace a file
  could replace its checksum. Real authenticity needs signing (14.1, 14.2); this ADR
  does not pretend otherwise, and the update card does not call them "verified".
- A person on `1.2.0-rc1` is told about `1.2.0` but never about `1.2.0-rc2`.
- GitHub limits anonymous API calls to 60 per hour per IP address. One call a day is far
  below that, but a shared network (an office, a campus) can use it up; the card then
  says GitHub is limiting requests and the next hourly wake tries again.
- The status is not saved to disk. After a restart the notice appears again about
  fifteen seconds in, which is the intended place to be reminded.
- Adding an asset to a release (a Linux build, say) means changing the "exactly three"
  guard in `release.yml`, and `assetsFor` in `updates/service.go` if the file name does
  not contain `macos` or `windows`.
- The release notes script puts commit subjects in public. A subject is written for the
  next developer, not for a customer; the notes can be edited on the release page.

## Alternatives rejected
- **A silent self-update.** Needs signing first, differs on both systems, and a bug in
  it can leave a person with no working app. Revisit after Phase 14.1/14.2.
- **An updater library (go-selfupdate, Sparkle, Squirrel).** Each pulls in a per-OS
  mechanism, a feed format and, for Sparkle, a key we would then have to guard. The
  value over "open the page" does not yet repay that.
- **A server of our own for update metadata.** Something to run, pay for and secure, and
  a second place that learns who uses EinCast. GitHub already has the files.
- **The release `digest` field on GitHub's API instead of `SHA256SUMS.txt`.** Newer
  releases carry one per asset, but the checksum file is something a person can verify
  with a tool they already have, and the workflow owns it rather than depending on a platform field.
- **Sending the installed version so the server can answer "is there an upgrade?".**
  Pointless (the client can compare) and the opposite of "nothing identifying leaves".
- **Nagging dialogs or a badge on the Dock icon.** A quiet line in the sidebar is enough;
  the person is not sitting in front of EinCast most of the time it runs.
- **Making Check now obey the setting.** A button that does nothing when pressed is a
  worse surprise than a request the person asked for.
