# 0013 — Provider settings live in the credential store, and secrets are write-only

## Context
Three providers need something from the person before they can run: the
Cloudflare named tunnel needs a tunnel token and the host name it serves; ngrok
needs an authtoken; both Cloudflare tunnels may need to be told where
`cloudflared` is installed. The token and authtoken are credentials: anyone who
has a tunnel token can run that tunnel. They must not end up in the settings file
(which people back up, sync and attach to bug reports), in the log, or in the UI's
memory longer than needed.

Each provider needing its own fields, its own storage and its own screen would
triple the work and the ways to leak something.

## Decision
- **A provider declares what it needs.** A provider that needs settings implements
  `app.Configurable.Fields()`: a list of `ConfigField` (key, label, help, kind,
  required, an optional `Shared` namespace, an optional `Clean` function). The
  registry passes the list through `ProviderInfo`, the bridge turns it into DTOs,
  and the UI draws a form from it. Adding a provider with settings adds no UI code.
- **Everything goes in the operating system's credential store**, secret or not,
  under `provider/<provider or shared namespace>/<key>`, through the existing
  `SecretStore` port (ADR 0010). The settings file has no tunnel-related field at
  all. Keeping a host name there would have meant extending the settings model
  for one string; one place to look is simpler.
- **Secrets are write-only.** `providerconfig.Store.States` and the bridge return a
  secret as "set" or "not set", never its value, and the bridge strips the value
  even if a bug upstream supplied one. The form shows "Saved in your keychain" with
  Replace and Remove. A field kind the UI does not recognise is treated as a
  secret.
- **Values are cleaned and checked on the way in, by the provider that knows the
  format.** A host name has to be a host name (a pasted `https://…/` is reduced to
  it; a port, an IP address or a user name is refused). A program location has to
  be an absolute path to an executable file. Every value is trimmed, capped in
  length (2 KiB for a secret, 512 bytes otherwise) and free of control characters;
  a secret may not contain whitespace, since a space in a pasted token means two
  things were pasted together.
- **The token reaches `cloudflared` in the environment**, as `TUNNEL_TOKEN`, not on
  its command line, where every user of the machine can read it. If the program
  echoes the token, the supervisor replaces it with `[redacted]` before the line
  reaches the log, the parser, or an error message. ngrok's SDK is given the
  authtoken in-process, its own logging is switched off, and its errors have the
  token removed.
- **Not set up is a state the service knows.** `Enable` checks required values
  first and answers with `NotConfiguredError`, which names what is missing; the
  status lists `NeedsSetup` so the UI can say so before the click. The list is
  cached (working it out reads the credential store) and refreshed on load and on
  every change.
- **Settings are frozen while the provider is on.** A running tunnel would not pick
  up the change, and the screen would show something not in effect. The shared
  `cloudflared` location is the one setting two providers use; it is stored once.
- **A saved program location is trusted or refused, never replaced.** If the path
  the person chose no longer works, starting fails and says so. Quietly using
  another `cloudflared` found on `PATH` would run a program they did not choose.

## Consequences
- Setting up the named tunnel is four steps in two places: create the tunnel and a
  public hostname in Cloudflare (pointing at `http://localhost:<port>`), then paste
  the token and hostname into EinCast. EinCast cannot read the hostname from the
  token and cannot configure Cloudflare for the person. The setup form says which
  port to give Cloudflare, and that changing the port in Settings means changing
  it in Cloudflare too.
- On a machine where the credential store is unavailable (a locked keychain, some
  Linux setups) nothing can be saved and providers that need settings read as not
  set up, with the reason logged. That is the safe failure.
- Reading settings on every start means a keychain read per field. Reads happen
  when a tunnel starts and when the screen opens, not per request.
- A secret that was saved cannot be shown again, even to its owner. To check it, the
  person re-pastes it. This is deliberate: the UI is the part of the app most
  likely to be screen-shared.

## Alternatives rejected
- **Putting the host name in `settings.json`**: it is not secret, but it would mean
  the settings model, DTO, form and migration all grow for tunnel details.
- **Per-provider bridge methods** (`SetCloudflareToken`, `SetNgrokToken`, …): every
  new provider would add bridge, DTO, store and form code.
- **Showing the token with a reveal button**, as the access token has: that token is
  ours to regenerate; these belong to accounts we cannot rotate for the person.
- **Passing the token as `--token`**: documented, and in Cloudflare's own examples,
  but visible in the process list to every user of the machine.
- **Falling back to `PATH` when the saved location is gone**: friendly, and wrong
  for the reason above.
