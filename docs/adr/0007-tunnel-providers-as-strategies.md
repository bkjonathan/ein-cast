# 0007 — Tunnel providers as strategies, with subprocess supervision

> Status: built (Phases 7a to 7d). The supervision half is described under
> "Subprocess supervision" below, as built in 7c.
>
> Amended in 7b: `Start` takes the running server (`app.LocalServer`), not a
> "host:port" string, because the first provider (Tailscale) needs the server to
> listen on one more address. See [ADR 0012](0012-tailscale-listens-on-the-tailnet-address.md).

## Context
EinCast can be reached from outside the home through four very different
mechanisms: Tailscale (a private network the user already runs), a Cloudflare
quick or named tunnel (a `cloudflared` child process), and ngrok (an embedded
SDK). They differ in how they start, how they report their address, whether
they need a secret, and above all whether the whole Internet or only the user's
own devices can reach the result. The rest of the app must not care which one is
running.

## Decision
- **`app.Tunnel` is a port; each provider is a strategy.** `remote.Service` asks
  an `app.TunnelRegistry` for a provider by `ProviderID` and calls `Start`,
  `Stop` and `Status`. It never switches on the provider. The registry
  (`adapters/tunnel`) is a map filled once in `main.go`; a duplicate or empty ID
  is a wiring error and `NewRegistry` returns it.
- **`Public() bool` is part of the port.** The sketch in the build brief had no
  such method, but the rule "public needs a PIN" ([ADR 0008](0008-public-access-requires-authentication.md))
  has to read it from somewhere. Putting it on the interface means a provider
  cannot be written without deciding, and the service never keeps a table of
  "which IDs are public" that could drift from the providers.
- **`Start(ctx, srv)` gets the running server.** Process-based providers forward to
  `app.LoopbackAddr(srv)`; a provider reached directly asks `srv.Bind` for an
  address. `streaming.Service.Local()` hands the server out only while it is
  Running, so "the server must be up" is enforced in the app layer, not by the
  caller remembering to check.
- **`Start`'s context is the tunnel's lifetime**, exactly as `WebServer.Start`'s
  is the server's. Cancelling it takes the tunnel down. `Start` returns once the
  public address is known; `remote.Service` bounds that wait with a timer that
  cancels the same context, and treats a late success as a failure.
- **The lifecycle reuses the server's state machine** (`domain/server.Machine`):
  Idle, Starting, Running, Stopping, Failed with the same transition table, so
  there is one tested definition of "what can follow what". `TunnelStatus` uses
  the same states, read as off / connecting / online.
- **Two views of state, on purpose.** The service's machine records what the user
  asked for (on, off, failed to start). `Tunnel.Status()` is the provider's live
  view (a supervised process that is reconnecting). `RemoteStatus` carries both.
- **The server withdraws public access first.** `streaming.Service` takes an
  optional `app.PublicAccess` and calls `Disable` before stopping the server and
  after it dies, so a tunnel never outlives what it points at. A stuck tunnel is
  logged and does not stop the server from stopping.
- **Subprocess supervision (`adapters/tunnel/procsup`).** A generic `Supervisor`
  owns one child process. It:
  - starts the process and reports a failure to spawn from `Start` itself;
  - splits stdout and stderr into lines, handing each to the provider's parser and
    to the log with `source=tunnel` (a level hook lets the provider mark errors);
  - restarts a process that dies, waiting 1 s, 2 s, 4 s, … up to 30 s, at most 5
    times in a row; a run that lasted 30 s counts as healthy and resets both;
  - on cancellation (the tunnel's lifetime context, or `Stop`) asks the process to
    exit with an interrupt, and kills it after a 5 s grace period (Windows has no
    interrupt, so it is killed at once);
  - never logs the arguments, since a command line can carry a token; secrets go in
    the environment, which is also kept out of the log.
- **One goroutine, not an errgroup.** The brief asked for an errgroup. The
  supervisor has exactly one goroutine of its own (`exec` runs the output copiers),
  so an errgroup of one would add a type and no safety. Ownership is explicit
  instead: the goroutine ends when the context is cancelled, `Stop` is called, or
  the process has failed too often, and `Done()` is closed only after the child
  has been waited for.
- **The service watches the provider.** A supervised process can restart (a quick
  tunnel then has a *new address*) or give up, and neither is announced. For each
  run `remote.Service` starts a watcher that asks `Tunnel.Status()` every second,
  ends with the run's context, and ignores itself if a newer run has begun. It:
  - adopts a new address (the Host check must accept it and refuse the old one) and
    publishes reconnecting/recovered as events;
  - when the provider reports Failed, tells it to stop to be sure, and takes remote
    access down. If that stop cannot be confirmed, the library stays "public" and
    the person is offered "turn off again", as with a failed manual stop.
  Polling rather than a channel on the port: the provider's status is already
  exposed, one second is fine for a UI, and no provider needs a second API.

- **The providers.** Tailscale (private, no process, ADR 0012); Cloudflare quick and
  named tunnels (a supervised `cloudflared`; they differ only in the command and in
  how "up" is recognised, so they share one engine); ngrok (the Go SDK in-process,
  which reconnects by itself, behind a small interface so its logic is tested
  without an account). Providers that need settings declare them
  ([ADR 0013](0013-provider-settings-in-the-credential-store.md)).

## Consequences
- Adding a provider is one package implementing five methods and one line in
  `main.go`. `remote.Service`, the HTTP layer and the UI do not change.
- A tunnel's death is noticed within a poll interval (one second), not instantly.
  Until then the service keeps treating tunnel traffic as public, which is the safe
  side.
- If EinCast itself is killed without cleaning up, a child `cloudflared` can keep
  running. Its traffic reaches loopback with forwarding headers, which the HTTP
  layer holds to the PIN rules whether or not a tunnel is believed to be up
  ([ADR 0008](0008-public-access-requires-authentication.md), point 3), and no
  sign-in can succeed without a public tunnel on, so a leftover process serves
  nothing. It should still be quit by the person; the next run does not adopt it.
- Reusing `domain/server` for tunnels means a package named for one thing models
  two. The alternative was a second identical machine; one tested table won.

## Alternatives rejected
- **A `switch` on provider ID inside the service**: every new provider edits the
  service, and the PIN rule would depend on remembering to add the new case.
- **A plugin system or provider discovery at runtime**: four providers known at
  compile time do not need it.
- **Deciding public versus private in the UI or a config table**: the safety rule
  would then depend on data that can disagree with the code that opens the tunnel.
- **Passing a timeout to `Start` instead of using the lifetime context**: the
  process must outlive `Start`, so the same context has to; a timeout context
  would kill the tunnel when the timer fired after success.
