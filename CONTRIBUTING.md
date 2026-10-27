# Contributing to EinCast

Thank you for looking. EinCast is a small desktop app (Go + Wails + React) that serves a
folder of media to the screens in a home. Small, careful changes are the best kind.

You do not need to understand everything to help. A translation, a device report, a
file-name test case or a fix to these docs are all real contributions.

## Before you start

- **Look for an issue** labelled [good first issue](../../labels/good%20first%20issue), or open one
  to say what you would like to change. For anything bigger than a bug fix, talk first: it saves you
  from writing something we cannot merge.
- **Ask: does this keep EinCast simple?** The project deliberately has no accounts, no cloud, no
  analytics and no plugin system ([what we will not build](docs/PLAN-v1.1.md#4-what-not-to-build-in-this-release)).
  A feature that needs one of those will be declined, kindly.
- By contributing you agree your work is released under the [MIT licence](LICENSE).
- Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).

## Set up

You need Go 1.26+, Node 20+ and the [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation)
(`wails doctor` should report success).

```sh
git clone https://github.com/bkjonathan/ein-cast.git
cd ein-cast
(cd frontend && npm ci)
wails dev                 # live-reloading window
```

## Check your change

Run these before you open a pull request. CI runs the same on macOS and Windows.

```sh
go vet ./...
go test ./...
scripts/check-deps.sh      # the architecture rules below
scripts/check-network.sh   # EinCast only talks to the internet where docs/PRIVACY.md says
scripts/check-cross.sh     # compiles for macOS and Windows
cd frontend && npm run typecheck && npm test
```

## The two rules that matter most

EinCast is built as *ports and adapters* ([ADR 0001](docs/adr/0001-hexagonal-architecture.md)):

1. **`internal/domain` and `internal/app` never import Wails, `net/http` or `os`.** They talk to the
   outside through small interfaces declared where they are used. `scripts/check-deps.sh` fails if you
   break this.
2. **Only `internal/bridge` and `main.go` import Wails** ([ADR 0006](docs/adr/0006-bridge-only-wails-dependency.md)).
   On the frontend, only `frontend/src/shared/bridge` imports `wailsjs/*`.

A short tour of where things go: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Why they are that way:
[docs/adr/](docs/adr/). If your change makes a real decision (a new dependency, a new way of storing
something, a new network call), add an ADR; copy the format of a short existing one
(Context, Decision, Consequences, Alternatives rejected).

## Code style

- **Go:** idiomatic and boring. `context.Context` first on anything that blocks; wrap errors with
  `%w`; table-driven tests with hand-written fakes (no mocking framework); `log/slog` for logging;
  every exported name has a doc comment; comments explain *why*, not what. No globals.
- **Security:** never take a file path from a URL (clients ask for `/stream/{id}`), and never log a
  secret ([docs/adr/0005](docs/adr/0005-media-ids-instead-of-paths.md)).
- **Frontend:** TypeScript strict, no `any`, one Zustand store per feature, components never call Go
  directly. Every visible word goes through `t("key")` and exists in both `en.json` and `my.json`
  ([TRANSLATING.md](docs/TRANSLATING.md)).
- **Tests come with the change.** A bug fix starts with a test that fails.

## Commits and pull requests

- Commit messages: a short imperative summary, with a prefix when it helps:
  `feat: …`, `fix: …`, `docs: …`, `test: …`, `ci: …`, `chore: …` (a scope is welcome:
  `feat(i18n): …`).
- One pull request, one idea. A pull request with a green CI, a description of *why*, and a note on
  how you tried it is easy to review. Screenshots help for anything visible.
- Say what you could **not** test. "Not tried on Windows" is useful; a silent gap is not.

## Reporting bugs and device results

- **Bug:** Settings → Help and feedback → **Copy diagnostics**, then
  [open a bug report](../../issues/new?template=bug_report.yml) and paste it. Read it first; it hides folder
  names and secrets, but you are the last check.
- **Device report:** which TV, phone or player you tried and what happened is valuable even when it worked;
  see [docs/TESTING-DEVICES.md](docs/TESTING-DEVICES.md).
- **Security problem:** do not open a public issue. See [SECURITY.md](SECURITY.md).

## Translating

Add or fix a language without touching Go: [docs/TRANSLATING.md](docs/TRANSLATING.md). The Burmese text
is a draft by the maintainer's assistant and is waiting for a native reviewer, so corrections there are
especially welcome.
