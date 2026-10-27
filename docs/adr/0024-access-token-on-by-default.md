# 0024 — The access token is on by default for new installs

## Context
Until now the access token was off. A new install served the whole library to
anyone on the same Wi-Fi who knew the address (`http://192.168.x.x:8765`), and the
README had to say so. For a tool aimed at people who are not security-minded, that
default decides how most installs end up: open. A shared flat, a dorm, a café
network or a guest on the home Wi-Fi can all reach it.

The cost of the safer default is small. The QR code and the "Open on this
device" links on the Dashboard already carry the token, so a phone that scans the
code is signed in without typing anything. What it breaks is the clients that
cannot carry a token: smart TVs that find the server over DLNA/SSDP (ADR 0011,
ADR 0020).

## Decision
- **A new install starts with the token on.** `settings.Service.Load` treats
  "no settings file" as a first run and uses `firstRun()` instead of `Default()`:
  token on, `AutoStart` on, `KeepAwake` on. The token is generated and stored in the
  OS credential store at that moment, like any other.
- **An existing install changes nothing.** The decision is made only on
  `ErrNoSettings`. A file written by an older version has no `autoStart`,
  `openAtLogin` or `hideOnClose` and reads as off, and the token setting is whatever
  it was. The one new field that is on for them is `KeepAwake` (ADR 0025), stored as
  a pointer so that "absent" and "off" can be told apart.
- **A corrupt file falls back to `Default()`, not `firstRun()`.** The person may
  have an install with a token; losing the file must not silently also switch their
  protection to a different state than the one they chose, and a repair that turns
  things on is a surprise too. The warning in the log says what happened.
- **Smart TVs stay exclusive with the token** (unchanged rule, still enforced in the
  service). What changes is how the person meets it. Turning Smart TVs on while the
  token is on no longer shows a dead switch: it shows a plain warning ("EinCast
  would list your library to everyone on this network, without a password") and two
  buttons, "Turn off the token and enable Smart TVs" and "Keep the token". The
  first sends one save with both changes, so the service never sees an illegal
  state in between.
- **The first-run wizard shows the code with the token in it** and says so, so the
  first connection works without explanation.

## Consequences
- A person who first installs and then tries a TV that only knows DLNA has to make
  the choice above. That is the intended friction: the choice is theirs, with the
  risk stated.
- Existing users who never turned the token on remain open, and the Access panel
  still recommends turning it on. This release does not change their behaviour
  behind their back.
- `Default()` and `firstRun()` now differ, and tests pin both: a first run has a
  saved token; an old file keeps its choices; a bad file gets the old defaults.
- The README line "the access token is off by default" is replaced by the opposite
  and a pointer to this ADR.

## Alternatives rejected
- **Turn it on for everyone on upgrade.** Safer for the open installs, but it would
  lock out a TV that works today with no explanation. The person did not ask.
- **Ask in the wizard whether to turn the token on.** One more question on the way
  to the first video, and the honest answer for most people is "yes, I guess".
  Defaults are for that case.
- **Keep it off and show a warning.** What we had. Warnings that are not acted on
  leave the library open.
