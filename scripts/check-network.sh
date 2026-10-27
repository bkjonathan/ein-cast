#!/usr/bin/env bash
# Keeps the privacy statement true (docs/PRIVACY.md, Phase 20.10):
#
#   "No accounts, no analytics, no phoning home. The only requests EinCast makes
#    by itself are the update check (which can be turned off) and what the person
#    turns on (tunnels)."
#
# The code that may open a connection to another computer is listed below. A
# change that adds a connection anywhere else fails here, and has to either live
# in one of these packages or update PRIVACY.md and this list together, in the
# same pull request. This is a tripwire for honest mistakes, not a security
# boundary: it reads the source, so it catches the usual ways of reaching out.
set -euo pipefail

cd "$(dirname "$0")/.."

# Packages allowed to make outbound connections, and why.
#   githubrelease      the update check (Settings → Updates; can be turned off)
#   tunnel/ngrok       the ngrok SDK, only while the person has turned ngrok on
# cloudflared and tailscale are separate programs started by procsup; they do
# the connecting, not our code.
allowed='^internal/adapters/(githubrelease|tunnel/ngrok)/'

# What opens a connection: an HTTP client request, a dial, or a network SDK.
pattern='http\.(Get|Post|PostForm|Head|DefaultClient|DefaultTransport|NewRequest)|http\.(Client|Transport)\{|net\.(Dial|DialTimeout|Dialer)|\.DialContext\(|websocket\.Dial|golang\.ngrok\.com'

fail=0
# -I skips binary files; tests are not shipped, so they may reach out to a fake.
hits=$(grep -rnE "$pattern" --include='*.go' main.go internal | grep -v '_test\.go:' || true)
while IFS= read -r line; do
  [ -z "$line" ] && continue
  file=${line%%:*}
  if [[ ! $file =~ $allowed ]]; then
    echo "FAIL: $line"
    fail=1
  fi
done <<< "$hits"

# Nothing the windows load may come from another site: no remote scripts, fonts,
# styles or images (the page would then tell that site who is watching).
remote=$(grep -rnE '(src|href)="https?://|url\(["'\'']?https?://|@import +["'\'']https?://' \
  frontend/index.html frontend/src internal/adapters/httpapi/web 2>/dev/null |
  grep -v '\.test\.' | grep -v 'hls\.min\.js' || true)
if [ -n "$remote" ]; then
  echo "FAIL: a page loads something from another site:"
  echo "$remote" | sed 's/^/    /'
  fail=1
fi

if [ "$fail" -eq 0 ]; then
  echo "OK: outbound connections are only where docs/PRIVACY.md says"
else
  echo
  echo "If this is intended, put the code in an allowed package, or update docs/PRIVACY.md and the"
  echo "list in scripts/check-network.sh together, and add an ADR."
  exit 1
fi
