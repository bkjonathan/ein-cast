#!/usr/bin/env bash
# Cross-platform build check for the two supported platforms, macOS and Windows.
#
#   scripts/check-cross.sh          type-check (go vet, which includes the tests)
#                                   for every target, and link the Windows binary
#   scripts/check-cross.sh --full   also run the complete Wails build for Windows
#                                   (frontend included) and verify the output
#
# This proves the code COMPILES for the other platform. It cannot prove it
# behaves: tests run only on the host. The CI workflow (.github/workflows/ci.yml)
# runs them on real macOS and Windows machines.
set -euo pipefail
cd "$(dirname "$0")/.."

targets=(darwin/arm64 darwin/amd64 windows/amd64 windows/arm64)
fail=0

for t in "${targets[@]}"; do
  os=${t%/*} arch=${t#*/}
  if GOOS=$os GOARCH=$arch go vet ./... 2>/tmp/eincast-cross.err; then
    echo "ok    vet   $t"
  else
    echo "FAIL  vet   $t"
    sed 's/^/        /' /tmp/eincast-cross.err
    fail=1
  fi
done

# Linking catches problems vet cannot (unresolved symbols, bad build tags).
if GOOS=windows GOARCH=amd64 go build -o /dev/null . 2>/tmp/eincast-cross.err; then
  echo "ok    link  windows/amd64"
else
  echo "FAIL  link  windows/amd64"
  sed 's/^/        /' /tmp/eincast-cross.err
  fail=1
fi
rm -f /tmp/eincast-cross.err

if [ "${1:-}" = "--full" ] && [ "$fail" -eq 0 ]; then
  wails build -platform windows/amd64 -clean >/dev/null
  exe=build/bin/eincast.exe
  if [ -s "$exe" ] && file "$exe" | grep -q "PE32+ executable (GUI) x86-64"; then
    echo "ok    wails windows/amd64 → $exe ($(wc -c <"$exe" | tr -d ' ') bytes)"
  else
    echo "FAIL  wails windows/amd64 did not produce a Windows GUI executable"
    fail=1
  fi
fi

exit "$fail"
