# Why not Jellyfin (or Plex, or Infuse)?

Often you should use one of them. This page is here to help you choose, not to sell EinCast.

## The short answer

- **EinCast** is for the household that wants *"play this folder on that TV, now"*, with nothing to
  set up, no accounts and nothing to maintain.
- **Jellyfin** and **Plex** are *media servers*: they build a catalogue with posters and descriptions
  from the internet, keep a database, have apps for many TVs, and run all day on a NAS or a spare
  computer. They do more, and they take more setting up.
- **Infuse** is a *player* for Apple devices. It plays from many sources, and can play from EinCast.

## Side by side

| | EinCast | Jellyfin | Plex |
|---|---|---|---|
| What it is | A desktop app for macOS and Windows | A server (Windows, macOS, Linux, Docker, NAS) | A server plus a company's cloud |
| Setup | Pick a folder, scan a QR code | Install a server, create users, set up libraries | Install a server, make an account, set up libraries |
| Needs an account | No | Local accounts on your server; no company account | Yes, a Plex account |
| Cost | Free, MIT licence | Free, open source | Free to use, some features need a paid pass |
| Posters and descriptions | From file names and files beside the video (`.nfo`, `poster.jpg`); nothing is looked up online | Downloaded from online databases | Downloaded from online databases |
| On the TV | The TV's web browser or its media-server list (DLNA), VLC, Infuse, any M3U player | Apps for many TV platforms, plus browser and DLNA | Apps for nearly every TV platform |
| Runs when | EinCast is open on your computer (it can open at login) | Always on, if the server is | Always on, if the server is |
| Several people | Profiles with optional PIN, limits by library, kind and folder | Full user accounts and permissions | Users, and managed accounts |
| Live TV and recording | No | Yes, with a tuner | Yes, with a tuner and a pass |
| Watching away from home | Tailscale (private), or a Cloudflare/ngrok tunnel behind a PIN | Possible, set up by you | Built in, with limits |
| Plugins | None, on purpose | Yes | Limited |
| Sends data to a company | Only the update check, which can be turned off ([privacy](PRIVACY.md)) | Depends on the plugins and metadata sources you turn on | Yes, account and usage data |
| Maturity | Young. Tried on a handful of devices ([list](TESTING-DEVICES.md)) | Years, large community | Years, large company |

Check each project's own site for what is true today; this table is a sketch, not a promise about
other people's software.

## Use EinCast when

- You want to **watch something now**: a folder on your laptop or an external drive, on the living-room
  TV or a tablet, without installing an app on the TV.
- You share with **family who should not have to learn anything**: a QR code, a browser, done.
- You do not want an account, a cloud, a database or a server to look after.
- Your files are named in an ordinary way, and you do not mind that posters come from your own files.

## Use something else when

- You want a **catalogue** with posters, cast lists, collections and search across thousands of titles
  that appear by themselves → **Jellyfin** or **Plex**.
- You want the library **always on**, whether or not a computer is open → a NAS or spare machine running
  **Jellyfin** or **Plex** (EinCast has no headless mode yet).
- You need a **native app on a TV** (Roku, Fire TV, Apple TV, Android TV) with remote-control navigation
  and a good player → **Jellyfin**, **Plex**, or **Infuse** on Apple TV.
- You need **live TV or recording**, many separate users, or plugins.
- You run **Linux** (EinCast is macOS and Windows only for now).

## Using them together

Nothing stops you running EinCast beside a server. And **Infuse** can open an EinCast library: take
`http://<address>:8765/playlist.m3u` from the Connect card (with the token if it is on), and open it in
VLC or Infuse. The playlist works in most players.
