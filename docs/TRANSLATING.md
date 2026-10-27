# Translating EinCast

The desktop window's words live in two files, `frontend/src/shared/i18n/locales/en.json`
(the source) and `my.json` (Burmese). Design: [ADR 0028](adr/0028-typed-dictionary-for-translations.md).

## Add or fix a language
1. Copy `en.json` to `<code>.json` (a two-letter code, e.g. `th.json`) and translate the right-hand sides.
2. In `frontend/src/shared/i18n/translate.ts`: import it, add the code to `Locale`, `locales` and `dictionaries`.
3. Add its name, written in that language, as `language.<code>` in **every** file, and one option in
   `features/appearance/components/LanguageSettings.tsx`.
4. `npx tsc --noEmit` lists any key you missed; `npx vitest run` checks the `{placeholders}`.

Rules of the files:
- Keep every `{placeholder}` exactly as it is; you may move it in the sentence.
- `x.one` / `x.other` are the two forms of a plural. A language with one form writes the same text twice.
- Do not translate product names (EinCast, Tailscale, ngrok, VLC), the log level names, or file paths.
- Short labels (a button) should stay short: the window is laid out for them.

## The player page (phones and TVs)
The page that devices open has its own, smaller files: `internal/adapters/httpapi/web/locales/en.json` and
`my.json` (about forty keys). Design: [ADR 0033](adr/0033-player-page-words-come-from-go.md).

- To add a language, copy `en.json` to `<code>.json` in that folder and translate the right-hand sides. Nothing
  else needs changing: the server finds the file and offers it to any device whose browser language matches.
- Translate every key. `go test ./internal/adapters/httpapi/` fails if you leave one out, invent one, or drop a
  `{placeholder}`. (English fills any gap on the device as a safety net; a deliberately partial language means
  relaxing `TestTranslations_ShippedLanguagesAreComplete`.)
- A device's language is its browser's. To see another one on a device that cannot change it (a TV), open the
  address with `?lang=my` at the end.
- The "access token needed" page and the PIN page a tunnel visitor signs in with use the same files (`token.*`,
  `signin.*`). `signin.locked` is a plural.

## Burmese (`my.json`)
**Status: an unreviewed draft (both the window and the player page)** written by the assistant, not a person who speaks Burmese. Nothing in it
should be trusted for security wording (access token, PIN, public access) until it has been reviewed.

Terms used, so they stay consistent. Change them here and in the file together:

| English | Burmese used | Note |
|---|---|---|
| library | စာကြည့်တိုက် | the set of media folders |
| folder | ဖိုလ်ဒါ | |
| server | ဆာဗာ | |
| access token | ဝင်ရောက်ခွင့်တိုကင် | |
| PIN | PIN | left in Latin letters |
| profile | ပရိုဖိုင် | |
| dashboard | ဒက်ရှ်ဘုတ် | |
| settings | ဆက်တင်များ | |
| remote access | အဝေးမှ ဝင်ရောက်ခြင်း | |
| public / private | အများသုံး / ပုဂ္ဂလိက | |
| playlist | ပလေးလစ်စ် | |
| port | ပေါက် | |

The same review should cover `README.my.md`, the player page files and the strings on the pages listed above.

Reviewer's checklist: sentences read naturally, not word for word; the terms above are right and used the
same way everywhere; nothing is cut off in the window (long Burmese words in buttons); stacked marks are not
clipped; numbers and units look right.
