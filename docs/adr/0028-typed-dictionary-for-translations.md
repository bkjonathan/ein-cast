# 0028 — Translations are a typed dictionary, not a library

## Context
Phase 18 makes the desktop app available in Burmese and gives other communities a way
to add their language. The window has about sixty components and a dozen helper
modules with text in them. The usual choice is `react-i18next`; the alternative is a
few dozen lines of our own.

What the app needs from translation is small: look up a string, fill `{name}` values,
pick a plural form, follow the computer's language or a saved choice. It does not need
namespaces, lazy-loaded bundles, a backend for translations, or language detection
beyond `navigator.languages`.

## Decision
- **One flat JSON file per language** in `frontend/src/shared/i18n/locales/`, keys like
  `"server.state.idle"`. `en.json` is the source of truth.
- **The keys are the type.** `MessageKey = keyof typeof en`, and a language is a
  `Record<MessageKey, string>`. A key missing from `my.json`, or a key used in code that
  English lacks, fails `tsc`, not a screenshot. A test also compares placeholders, so a
  `{port}` cannot be dropped from a translation.
- **`translate(locale, key, params)`** is a pure function, so helper modules and tests
  can use it without React. Components use `useT()` for a `{ t, tn }` bound to the
  window's language, which re-renders them when it changes.
- **Plurals** are two keys, `x.one` and `x.other`, chosen with `Intl.PluralRules` for the
  language. Burmese has one form, and a missing form falls back to `.other`.
- **The language is a preference of the window**, kept in the browser's storage like the
  theme (system / English / မြန်မာ), and sets `<html lang>`. A language is always named in
  itself in its own picker.
- **Helpers return keys, not text** (`describeState` returns `labelKey`), so the logic
  stays testable in English and the component translates.
- English wording was moved into `en.json` unchanged, so the existing tests, which assert
  English text, kept their meaning.

## Consequences
- No new dependency, and an added language is one file plus one line in `translate.ts`.
- We maintain plural and placeholder handling ourselves. It is about sixty lines and
  covers what the app uses; a language with several plural forms works through
  `Intl.PluralRules` but has not been tried.
- Placeholders are not typed per key (`t("server.serving", { port })` is checked by the
  test, not the compiler).
- Text assembled from fragments (a link in the middle of a sentence) must become one
  string with the markup outside it; the first such case, "Serving your library on port
  {port}", lost the monospace styling on the port number.
- Messages that Go produces (a failed start, a validation error, the connection log) are shown as Go
  wrote them, in English. Translating those means error codes from Go, which is a separate change.
- Helpers that build a sentence take the translator (`describeAccess(p, libs, tr)`) or return a key;
  none of them reads the window's language on its own.
- Burmese drafts were written without a native review. They are marked as drafts in
  `docs/ROADMAP.md` until a Burmese-speaking reviewer signs off (18.3).

## Alternatives rejected
- **`react-i18next`.** The standard answer and a good one, but two dependencies and a
  separate typing setup for what is a lookup table here.
- **Machine translation at build time.** The plan forbids it for the UI: wrong words in a
  security setting ("access token") are worse than English.
- **Strings in the Go side.** The player page (18.2) will be served from Go; the desktop
  window's text lives with the window.
