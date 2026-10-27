# Privacy

**EinCast has no accounts, no analytics and no phoning home.** It does not send your library, your
file names, your settings, your logs or anything about your computer to anyone. The only requests it
makes by itself are the update check (which you can turn off) and the things you turn on yourself
(a tunnel for remote access).

This page says exactly what that means, so you can check it.

## What EinCast sends over the internet

| When | To whom | What is sent | Can you stop it? |
|---|---|---|---|
| About once a day, and when you press **Check now** (Settings → Updates) | GitHub (`api.github.com`, and the release's checksum file, which GitHub hosts) | An ordinary read of this project's public release list. The request carries your IP address (as any website request does) and the name `EinCast` as the program. **Not** the version you run, your system, or anything about your library. | Yes: turn off **Check for updates automatically**. Then nothing is sent until you press **Check now**. |
| Only while you have turned a tunnel on (Remote tab) | The provider you chose: Cloudflare or ngrok | Everything your remote viewers watch passes through that provider's servers, so they can see it. EinCast runs their program or SDK; their privacy terms apply. | Yes: turn the tunnel off. It also stops when you stop the server or quit. |
| Only while you have turned on Tailscale | Tailscale | EinCast only asks the Tailscale program on your computer for its address. The traffic is Tailscale's, between your own devices. | Yes. |

That is the whole list. EinCast does not load fonts, scripts, images or maps from any website: the
window and the player page that your devices open come with the app. Nothing here is hidden behind a
setting that is on by default, except the update check above, which is on because an install that
never hears about a fix keeps its bugs ([ADR 0029](adr/0029-update-notice-from-github-releases.md)).

If a later version adds an optional lookup (for example movie posters from the internet), it will be
**off until you turn it on**, and it will be added to the table above in the same change.

## What stays on your computer

- **Your media** is read from the folders you chose and streamed to devices on your network. EinCast
  never uploads or copies it anywhere. Converted video is a temporary file, deleted when you stop
  watching and when EinCast quits.
- **Settings, profiles and "continue watching"** are small files in your user folder
  (`~/Library/Application Support/EinCast` on macOS, `%AppData%\EinCast` on Windows).
- **The access token and PINs** are in the macOS Keychain or the Windows Credential Manager, never in
  those files and never in the logs.
- **The log** (the Logs screen) is kept in memory while EinCast runs and is gone when it closes. It is
  not written to disk and not sent.
- **Thumbnails and subtitles** you have looked at are cached in your user cache folder, and are safe to
  delete.
- **On a viewing device**, the player page remembers where you stopped in that browser's own storage, and also tells your EinCast server, which keeps it per profile (`resume.json`) so "continue watching" follows a person from phone to TV.

**Reset EinCast** (Settings) erases all of the above except your media files.

## Who can see what on your network

Devices on your Wi-Fi can find EinCast by name (`eincast.local`) and, if you turn on **Smart TVs**, by
the TV protocol. With the **access token** on (the default for a new install), a device needs the link or
QR code to open the library. With it off, or with Smart TVs on, anyone on your network can. Settings says
so next to each switch. See [SECURITY.md](../SECURITY.md) for reporting a problem.

## Bug reports

**Copy diagnostics** (Settings → Help and feedback) puts a report on your clipboard: the version, your
system, which settings are on, and the last 200 log lines, with folder names and secrets hidden.
It goes nowhere until you paste it. Read it first.

## How this stays true

`scripts/check-network.sh` runs in CI. It fails if any code outside the update check and the ngrok
adapter opens a connection to another computer, or if a page loads something from another site. If
you find a request that this page does not describe, that is a bug: please
[tell us](../SECURITY.md).
