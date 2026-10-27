# 0033 — The player page's words are served by Go, in the device's language

## Context
Phase 18.2 makes the page that phones, iPads and TVs open available in Burmese. That page
is plain JavaScript with no build step (ADR 0006's neighbour: the page is embedded, and the
CSP forbids inline script), so it cannot use the desktop window's typed dictionary
(ADR 0028). It has about forty strings: buttons, section titles, playback hints, the
profile picker.

Whose language? The desktop window's language is a setting of the person at the computer.
The person holding the phone is somebody else, and a TV's browser often cannot change its
language at all.

## Decision
- **Own files, same format.** `internal/adapters/httpapi/web/locales/<code>.json`: flat
  keys, `{placeholder}` values, plurals as `x.one` / `x.other`, exactly as in ADR 0028.
  They are separate from the desktop files because Go can only embed files inside its own
  package, and because the desktop's 300 keys are none of a phone's business.
- **Go picks the language and serves the words.** `GET /api/strings` answers
  `{ "lang", "messages" }`. The language is `?lang=` if given, else the first entry of
  `Accept-Language` that we have (best quality first, region ignored), else English.
  English is laid under every other language as a safety net, so a gap can never show a
  key name to a viewer.
- **The page fills itself in.** `app.js` fetches the strings before anything else, then
  sets `<html lang>` and fills the text written in `index.html` (`data-i18n`,
  `data-i18n-placeholder`, `data-i18n-label` attributes). Strings it builds go through
  `t(key, params)` and `tn(base, count, params)`, which use `Intl.PluralRules`.
- **Tests stand in for `tsc`.** Go tests fail if a language lacks a key English has, has
  a key English lacks, drops a `{placeholder}`, or if `app.js` / `index.html` ask for a key English does not have (or
  English has a key nobody asks for).
- **No profile language.** Profiles have no language setting, so the browser decides. That
  is also what the desktop's own help text promises ("follows each device's own language").

## Consequences
- Adding a language to the player page is one JSON file; no Go or JavaScript change.
  `docs/TRANSLATING.md` says so.
- Until the strings arrive, the page shows its English text, so a Burmese phone can show
  English for a moment on a slow link. Hiding the page until then would trade that for a blank screen.
- If `/api/strings` fails, the page keeps its English static text and dynamic messages show
  their key names. The server is then not answering and the next request says so.
- The key lookup in `app.js` is by string, checked only by the Go test. A key built at
  run time (`t('a.' + x)`) would escape the test, so call sites name their keys in full.
- Two sets of locale files to keep in step by hand; nothing ties a desktop key to a
  player key with the same meaning ("Start over").
- The two pages Go renders itself, the access-token-needed page and the tunnel PIN page,
  use the same files and the same negotiation (`translations.text`, `.plural`), so a phone
  that arrives through a tunnel is greeted in its own language. They have no script, so they
  cannot fetch `/api/strings`; Go fills them in and escapes every word it prints.
- Go's plural rule is "one when the count is 1, else other". That is right for English and
  for Burmese (both forms written alike). A language that needs more forms needs a rule in
  `translations.plural`; `app.js` already uses `Intl.PluralRules` and would not.
- Every server-rendered key also travels to the player page in `/api/strings`, unused there:
  about seven short strings.
- The Burmese draft has had no native review (18.3), same as the desktop's.

## Alternatives rejected
- **Share the desktop `en.json` / `my.json`.** One source of truth sounds right, but
  `go:embed` cannot reach `frontend/`, copying them in at build time adds a step that can
  be forgotten, and every phone would download keys for settings screens it never shows.
- **Browser picks, Go serves raw files** (`/static/my.json`). Works, but the fallback to
  English and the list of languages then live in JavaScript, and each language needs a
  registration line there.
- **Render the page on the server (`html/template`).** No flash of English, but it turns a
  static embedded page into templates, and the strings built in JavaScript would still need
  a second route to the same words.
- **`golang.org/x/text/language` for negotiation.** Handles more of the standard, but is a
  new direct dependency for one lookup among two or three languages.
