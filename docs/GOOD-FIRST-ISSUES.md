# Good first issues and labels (Phase 20.4, 20.5)

Drafts to post as GitHub issues once the repository is public. Each one is small, says where to look,
and says how to know it is done. Post them with the `good first issue` label and the area label; keep
the text, change the numbers.

## Labels

| Label | Colour | Meaning |
|---|---|---|
| `good first issue` | `7057ff` | Small, self-contained, explained |
| `help wanted` | `008672` | The maintainer would love a hand |
| `bug` | `d73a4a` | Something does not work |
| `enhancement` | `a2eeef` | A new idea (used by the idea template) |
| `translation` | `0075ca` | Words, not code |
| `device report` | `fbca04` | A result from a real TV, phone or player |
| `docs` | `0075ca` | Documentation |
| `windows` / `macos` | `ededed` | Only on that system |
| `needs reproduction` | `ededed` | Cannot be fixed until someone can make it happen again |

Create them with the GitHub CLI (it publishes to the repository, so run it deliberately):

```sh
gh label create "good first issue" --color 7057ff --description "Small and self-contained" --force
gh label create "help wanted"      --color 008672 --description "Extra hands welcome" --force
gh label create "translation"      --color 0075ca --description "Words, not code" --force
gh label create "device report"    --color fbca04 --description "A result from a real device" --force
gh label create "docs"             --color 0075ca --description "Documentation" --force
gh label create "windows"          --color ededed --force
gh label create "macos"            --color ededed --force
gh label create "needs reproduction" --color ededed --force
```

## The ten issues

### 1. Translate EinCast into your language  `good first issue` `translation`
Add a language to the desktop window and the player page without touching Go.
Follow [docs/TRANSLATING.md](TRANSLATING.md): copy `en.json` to `<code>.json`, translate the right-hand
sides, register it. **Done when** `npx tsc --noEmit` and `npx vitest run` pass and the language shows in
Settings → Language. Comment first so two people do not do the same one.

### 2. Review the Burmese text  `good first issue` `translation` `help wanted`
`my.json` (window) and the player page's `my.json` were drafted by the maintainer's assistant, not a
native speaker. Read it in the running app, fix awkward or wrong wording, and especially the security words
(access token, PIN, public access). **Done when** the terms table in [TRANSLATING.md](TRANSLATING.md) matches
what you settled on. No coding needed: you can edit the file on GitHub.

### 3. Add file-name test cases for the title parser  `good first issue`
EinCast reads titles, years and series from names such as `Movie.Name.2019.1080p.mkv` and
`Show.S02E05.mkv` (`internal/domain/media/meta.go`, tests in `meta_test.go`). Add five real names from your
own library that it gets wrong or right, as table rows. If one is wrong, fix the parser too, or say so in the
issue. **Done when** the new rows pass (or fail with a clear note). Names with Burmese letters are especially
useful.

### 4. Try EinCast on your TV or player and report  `good first issue` `device report`
Pick a row in [docs/TESTING-DEVICES.md](TESTING-DEVICES.md) that is empty and fill it in: ✅, ⚠️ or ❌, and
what happened. A failure is a useful result. **Done when** a pull request fills in at least one row (or an issue
reports it with Copy diagnostics).

### 5. Skip Windows "hidden" files when scanning  `help wanted` `windows`
The scanner skips dot-files and a few system names, but not files with the Windows hidden attribute.
Needs a build-tagged helper in `internal/adapters/fsscan` (see the ROADMAP seam "Windows file attributes").
**Done when** a hidden file on Windows is not listed, a test covers the helper, and `scripts/check-cross.sh`
still passes. You need a Windows machine.

### 6. Two EinCasts on one network: rename the second  `help wanted`
Both claim `eincast.local` (ROADMAP: "mDNS name conflicts"). Probe for the name and, if it is taken,
announce `eincast-2.local` instead (`internal/adapters/mdns`). **Done when** a test with a fake responder shows
the second name being chosen, and the Dashboard shows the name that was announced.

### 7. Use titles in the M3U playlist  `good first issue`
`/playlist.m3u` writes the file name after `#EXTINF:-1,`; the parsed title (and year, or "Show S01E02") reads
better in VLC and Infuse (`internal/adapters/httpapi/handlers_playlist.go`). **Done when** the playlist shows
the title where there is one, falls back to the name, and a test covers both.

### 8. Show posters on a TV: `upnp:albumArtURI`  `help wanted`
The DLNA listing has no picture; `/thumb/{id}` already exists. Add `upnp:albumArtURI` to the DIDL
(`internal/adapters/dlna/didl.go`). **Done when** a unit test shows the element, with a URL a TV can reach
(and the token rule from [ADR 0020](adr/0020-dlna-for-smart-tvs.md) still holds). A real TV to check on would be a bonus.

### 9. A "N lines dropped" row in the Logs view  `good first issue`
The log bus numbers every line (`Seq`), so a gap means lines were dropped under load, but the viewer shows
nothing. Render a quiet row where the gap is (`frontend/src/features/logs`). **Done when** a Vitest test builds
a list with a gap and sees the marker, and the word goes through `t()` in both languages.

### 10. Improve a doc you got stuck on  `good first issue` `docs`
Did you stumble reading the README, the guide or the install steps? Fix the sentence that confused you, or add the
missing one. **Done when** the change is merged. This one is open-ended on purpose: your confusion is the evidence.
