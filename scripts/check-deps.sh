#!/usr/bin/env bash
# Verifies the hexagonal dependency rules (see docs/adr/0001-hexagonal-architecture.md):
#
#   domain  -> nothing (no Wails, net/http, os, nor any other EinCast layer)
#   app     -> domain only (no Wails, net/http, os, adapters, bridge)
#   Wails   -> only internal/bridge and the main package
#
# Only *direct* imports are inspected. `go list -deps` would be useless here:
# fmt itself imports os, so everything transitively "uses" the filesystem.
set -euo pipefail

cd "$(dirname "$0")/.."
module=$(go list -m)
fail=0

# direct_imports prints the unique direct imports of a package, tests included,
# so a test that reaches for net/http in the app layer is caught too.
direct_imports() {
  go list -f '{{join .Imports "\n"}}{{"\n"}}{{join .TestImports "\n"}}{{"\n"}}{{join .XTestImports "\n"}}' "$1" |
    sort -u | grep -v '^$' || true
}

# check_layer <label> <package-pattern> <forbidden-import-regex> [exempt-package-regex]
check_layer() {
  local label=$1 pattern=$2 forbidden=$3 exempt=${4:-}
  local pkg bad
  for pkg in $(go list "$pattern"); do
    if [ -n "$exempt" ] && [[ $pkg =~ $exempt ]]; then continue; fi
    bad=$(direct_imports "$pkg" | grep -E "$forbidden" || true)
    if [ -n "$bad" ]; then
      echo "FAIL [$label] $pkg imports forbidden packages:"
      echo "$bad" | sed 's/^/    /'
      fail=1
    fi
  done
}

stdlib_io='net/http($|/)|os($|/)'
wails='github\.com/wailsapp/'
inner="$module/internal/(adapters|bridge)"

check_layer domain "./internal/domain/..." "^($wails|$stdlib_io|$module/internal/(app|adapters|bridge))"
check_layer app "./internal/app/..." "^($wails|$stdlib_io|$inner)"
# main (the composition root, listed as "$module") and bridge may import Wails.
check_layer wails-confinement "./..." "^$wails" "^($module|$module/internal/bridge)\$"

if [ "$fail" -eq 0 ]; then
  echo "OK: dependency rules hold"
else
  exit 1
fi
