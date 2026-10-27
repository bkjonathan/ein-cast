## What and why

<!-- One idea per pull request. Say why, not only what. Link the issue: "Closes #123". -->

## How I tried it

<!-- What you ran, on which system. Screenshots help for anything visible. -->

## Not tried

<!-- Be honest: "not on Windows", "no real TV". A gap you name is useful; a silent one is not. -->

## Checklist

- [ ] `go vet ./... && go test ./...` pass
- [ ] `scripts/check-deps.sh` and `scripts/check-network.sh` pass
- [ ] `cd frontend && npm run typecheck && npm test` pass
- [ ] New visible words are in both `en.json` and `my.json`
- [ ] A bug fix has a test that failed before
- [ ] It keeps EinCast simple (no account, cloud, analytics or plugin system)
- [ ] I added an ADR if this is a real decision (new dependency, storage, or network call)
