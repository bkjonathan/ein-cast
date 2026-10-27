# Security

EinCast runs a web server on your home network, and can optionally put it on the internet through a
tunnel. Security reports are taken seriously.

## Reporting a vulnerability

**Please do not open a public issue.** Use GitHub's private reporting instead:

1. Go to the repository's **Security** tab.
2. Choose **Report a vulnerability** and describe what you found.

Please include the EinCast version (Settings → Updates, or the window title), your system, what you
did, and what happened. A copy of **Copy diagnostics** helps, but read it first and remove anything you
would not share.

You can expect a first reply within about a week. This is a one-person project, so a fix may take longer;
you will be told what is happening. Reporters who want it are credited in the release notes.

## What is in scope

- Reaching a file outside a library folder (path traversal) or a file that a profile may not see.
- Getting past the access token, the profile PIN, or the tunnel sign-in (PIN, session cookie, signed
  links), or learning a secret from the app, its logs or its network traffic.
- Turning on public access without a PIN.
- Anything in the player page or the pages Go renders that lets one visitor act as another (injection,
  cross-site request forgery).
- A way for another computer to make EinCast end a process or write a file.

## What is not

- Anyone on your Wi-Fi opening the library when you have **turned the access token off**, or turned
  **Smart TVs** on. EinCast warns about both; it is the choice you made.
- A PIN or profile PIN being guessable by someone who can already read this computer's files.
- Problems in a tunnel provider (Cloudflare, ngrok, Tailscale) itself. Report those to them.
- Builds that are not code-signed warning on first launch. That is known (see the README).

## Supported versions

Only the latest release gets fixes.

## How EinCast protects you (summary)

The design is explained in [ADR 0005](docs/adr/0005-media-ids-instead-of-paths.md) (no file paths in URLs),
[ADR 0008](docs/adr/0008-public-access-requires-authentication.md) (public means authenticated),
[ADR 0011](docs/adr/0011-lan-access-control.md) and [ADR 0024](docs/adr/0024-access-token-on-by-default.md)
(the access token), and [docs/PRIVACY.md](docs/PRIVACY.md) (what leaves the computer).
