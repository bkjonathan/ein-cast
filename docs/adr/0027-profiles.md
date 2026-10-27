# 0027 — Profiles (kids / adults) and continue watching

## Context
A household shares one library. Parents want a child's iPad to see the cartoons and
not the rest; everyone wants "Continue watching" to follow them from the phone to
the TV. Profiles are not accounts: there is no server to sign in to, nothing leaves
the home, and the people sharing a sofa trust each other with a remote. What they
need is a gate that stops a *casual* wrong click or a curious child, and that
cannot be bypassed by a URL.

Three things already exist and shape the design: the **access token** gets a
device onto the network (ADR 0011); the **tunnel PIN and its signed cookie** are the
precedent for a credential held in an HttpOnly cookie (ADR 0008); and every route
already takes an item **ID, never a path** (ADR 0005).

## Decision
**What a profile is** (`domain/profile`): a name, an avatar colour, an optional
4–6 digit PIN, the libraries it may see (all, or a list), the kinds it may see
(video / audio / image; none ticked means all), and folder names to hide (simple
`*`/`?` patterns matched against each folder in the item's path, ignoring case).
`Profile.Allows(item)` is the single rule, pure and table-tested.

**"Everyone" is the default.** A new or upgraded install has exactly one profile,
*Everyone*, with no restriction and no PIN. With one profile the whole feature is
invisible: no picker, no cookie, and the one profile is everyone's, so every check
passes. Nothing changes until the person adds a second. (If the only profile is
edited to hide something, that is enforced for everyone: there is nobody else to be.)

**How a device becomes a profile.** After the access token gets it in, the player
page shows a picker (only when there is more than one profile). Choosing a profile
with a PIN asks for it. The server then sets a **signed, HttpOnly cookie**
(`eincast_profile`) holding `id.version.HMAC(id|version)`; the secret is kept in
the OS credential store like the access token, so a device stays on its profile
across restarts. A PIN change or deleting the profile bumps its `version`, which
invalidates every credential issued for it. Players that cannot hold cookies (VLC,
Infuse) get the same credential in the query string of each playlist entry
(`?p=…`), exactly as the token already rides there.

**Enforced on the server, on every route, not in the picker.** The rule lives in
the `profiles` service and the HTTP layer asks it on each request:

| Route | Profile rule |
|---|---|
| `/api/library`, `/playlist.m3u`, `/api/resume` | need a profile; return only what it may see |
| `/api/library/{id}`, `/stream/{id}`, `/thumb/{id}`, `/hls/{id}/…`, `/subtitle/{id}/…` | the item must be allowed, else **404** (not 403), the same answer as an ID that does not exist, so a guessed or remembered ID reveals nothing |
| `/`, `/static/…`, `/api/health`, `/api/profiles`, `/api/profile` | no profile needed (the picker has to load) |

Unit and HTTP tests prove it route by route, including a guessed adult ID, the
playlist, HLS and a subtitle.

**No credential means "guest", never "Everyone".** With more than one profile, a
request that carries no valid profile credential is a guest. A guest sees no list
and no resume. It may reach an item by ID only through the one door that cannot
carry a credential: **Smart TVs** (below). A signed tunnel stream link is also
accepted for exactly the item it names, because it was only ever issued for an
allowed one. This closes the obvious hole: a child who knows the access token
cannot list the library by clearing a cookie.

**Smart TVs and DLNA cannot know the profile.** A TV sends no cookie. So DLNA, and
guest requests for IDs while Smart TVs are on, serve only libraries marked
**visible to TVs**. The mark is per library, on by default (an upgrade changes
nothing), set in Settings → Libraries. The existing rule stays: Smart TVs and the
access token are mutually exclusive (ADR 0011/0020), so turning on Smart TVs is
already the person saying "my network is open to TVs". Marking an adult library not
visible to TVs is how they keep it off the TV.

**A PIN is a household gate, not a vault.** It is stored salted and hashed
(PBKDF2-SHA-256, 100,000 rounds, from the standard library), never in clear, and
checking it is rate limited per client and profile (5 wrong tries lock that pair out
for 5 minutes). A 4–6 digit PIN can be guessed offline by anyone who can read
`profiles.json`, which is owner-only on disk; that is accepted. The UI says so.

**Stored in `profiles.json`, not `settings.json`** (a deviation from the plan, which
said settings.json). `settings.Service` owns that file and rewrites all of it on every
save; a second writer would race it, and PIN hashes do not belong beside the
settings people copy into bug reports ("Copy diagnostics" is planned). The new file
sits beside it, is written the same way (atomic, `0600`), and is read at startup.

**Continue watching moves to the server** (`resume.json`, per profile and item).
Positions were kept in each browser's storage, so the phone and the TV disagreed and
a tunnel address was a different "site" with its own memory. Now the page asks
`GET /api/resume` and saves with `PUT /api/resume/{id}`; the browser value stays as
the offline fallback and the newer of the two wins. Entries for items the profile
may not see are never returned. Writes are coalesced (at most one disk write every
2 seconds) and flushed on quit.

**Item IDs and old positions (16.6).** ADR 0021 made IDs include the library, which
orphaned positions saved by v0.x in browsers. A migration is cheap, so it is done:
the API also sends `legacyId` (the ID without the library), and the page moves a
position found under it to the new ID once.

## Consequences
- Profiles protect against honest mistakes and curious children, not against a
  determined household member who can read the disk, change the token's QR code, or
  use a phone that already has an adult profile selected. Parents are told to put a
  PIN on the adult profile (the desktop screen does).
- A device that was on "Everyone" before the second profile is added is a guest until
  it picks a profile. The page shows the picker at once.
- VLC and Infuse playlists are per profile; a playlist saved before a restriction was
  added still works only for the items the profile may see (every stream request is
  checked again).
- `/api/library` is the same shape for every profile, so the player page needs no
  special cases beyond the picker.
- Profile enforcement is invisible to the desktop app: its Library screen is the
  owner's view of everything.

## Alternatives rejected
- **Accounts with passwords.** A different product (the plan's "do not build" list).
- **Enforcing in the picker only.** A child could open `/stream/{id}` directly.
- **403 for forbidden items.** Confirms the item exists.
- **Treating no cookie as "Everyone".** Makes the cookie something to delete.
- **A cookie holding the profile ID unsigned.** Anyone could write another ID.
- **Keeping the hash in the keychain.** More private, but then profiles stop working
  when the keychain is locked, which the tunnel PIN already shows is annoying; the
  threat here is a child, not a thief.
- **Putting profiles into `settings.json`.** Two writers on one file (see above).
