# 0010 — Settings in a JSON file, secrets in the OS credential store

## Context
EinCast needs to remember a few preferences (port, display name, the library
folder, whether the access token is on) and one secret (the access token). The
token grants access to the whole library and is placed in QR codes and links, so
where it is stored matters. A settings file in the user's config directory is
easy to read, back up, sync, attach to a bug report, or commit by accident.

## Decision
- **Preferences** go in `settings.json` under `os.UserConfigDir()/EinCast`
  (`~/Library/Application Support/EinCast` on macOS, `%AppData%\EinCast` on
  Windows), written by `adapters/settingsfile`:
  - atomic writes: temp file in the same directory, `fsync`, rename over the
    real file, so a crash leaves the old file or the new one, never half of one;
  - mode `0600` (directory `0700`);
  - a `version` field for future migrations; unknown fields are tolerated so an
    older build can open a newer file;
  - an unparseable file is moved aside to `settings.json.bad` (not overwritten
    by the next save) and the app starts with defaults.
- **The access token** goes in the OS credential store (macOS Keychain, Windows
  Credential Manager) through `adapters/secrets`, a thin wrapper over
  `go-keyring`, under the service name `com.bkthomas.eincast`. The settings file
  has no field that could hold a token, and a test asserts it.
- **`settings.Service` owns both** and is the only code that talks to them. At
  startup `Load` never fails: a missing file is a first run; an unusable file or
  invalid field falls back to defaults, field by field, with a warning.
- **A missing secret is not an unavailable store.** `ErrSecretNotFound` means
  "create one"; any other error (locked keychain) means "use a temporary token
  and do not touch what is stored", so a transient failure can never overwrite a
  good token. The UI shows when the token is temporary (`tokenPersisted: false`).
- Everything fails closed: if the token is required but cannot be generated or
  read, nobody gets in.

## Consequences
- Backing up or sharing the settings file never exposes the library.
- Tests use `go-keyring`'s in-memory mock; they never touch the developer's real
  Keychain.
- Without a working credential store (some Linux setups, a locked keychain) the
  token changes every launch. That is safer than writing it to disk, and visible
  in the UI.
- Library folder paths are stored in plain text. They are not secrets, and are
  needed to reopen the library at startup.

## Alternatives rejected
- **Token in `settings.json`**: simplest, and exactly the file people share.
- **Encrypting the token in the file with a key kept alongside it**: obscurity,
  not protection; the credential store does this properly.
- **A fresh token every launch, never stored**: safest, but every bookmarked
  link and every TV signed in by cookie breaks at each restart.
- **SQLite or a config library (viper)** for four fields: more dependency than
  the problem justifies.
