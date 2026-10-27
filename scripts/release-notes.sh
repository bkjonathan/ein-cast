#!/usr/bin/env bash
# Writes the body of a GitHub release to stdout: What's new, Fixes, Known limits and
# how to check a download. The release workflow calls it; run it by hand to preview:
#
#   scripts/release-notes.sh v1.1.2        # a tag that exists
#   scripts/release-notes.sh HEAD          # the commits since the last tag
#   scripts/release-notes.sh HEAD 1.1.2    # the same, as the notes of 1.1.2 would read
#
# If CHANGELOG.md has a section "## [1.1.2] - date" for the version, that hand-written
# section is the "What's new" and "Fixes" part of the notes. Otherwise they are built
# from the commit subjects, and what goes where is decided by the subject (docs/RELEASING.md):
#   feat: ...            -> What's new
#   fix: ...             -> Fixes
#   a subject that is not "type: text" (older commits are written freely) -> What's new
#   chore, docs, test, ci, build, refactor, style, perf -> left out: nobody downloads for those
# The notes can be edited on the release page afterwards.
set -euo pipefail

cd "$(dirname "$0")/.."
rev=${1:?usage: scripts/release-notes.sh <tag-or-rev> [version]}
# The version whose CHANGELOG.md section to look for: the tag without its "v".
ver=${2:-${rev#refs/tags/v}}

# The text of "## [<ver>]" up to the next "## [", with its "###" headings promoted to
# "##" so they match the commit-built notes. Empty when there is no such section.
section=
if [ -f CHANGELOG.md ]; then
  section=$(awk -v v="$ver" '
    /^## \[/ { if (on) exit; on = (index($0, "## [" v "]") == 1); next }
    on { print }
  ' CHANGELOG.md | sed 's/^### /## /' | awk 'NF { started = 1 } started')
fi

# The commits since the previous release; with none, since the beginning.
prev=$(git describe --tags --abbrev=0 --match 'v*' "$rev^" 2>/dev/null || true)
range=${prev:+$prev..}$rev

new=() fixes=()
pattern='^([a-z]+)(\([^)]*\))?!?: (.+)$'
while IFS= read -r subject; do
  [ -n "$subject" ] || continue
  if [[ $subject =~ $pattern ]]; then
    type=${BASH_REMATCH[1]} text=${BASH_REMATCH[3]}
  else
    type=other text=$subject
  fi
  # A sentence starts with a capital; tr because macOS ships bash 3.2, which has no ${var^}.
  text="$(printf '%s' "${text:0:1}" | tr '[:lower:]' '[:upper:]')${text:1}"
  case $type in
    feat | other) new+=("$text") ;;
    fix) fixes+=("$text") ;;
    chore | docs | test | ci | build | refactor | style | perf) ;;
    *) new+=("$text") ;; # an unfamiliar type is more likely a feature than noise
  esac
done < <(git log --no-merges --format=%s "$range")

if [ -n "$section" ]; then
  printf '%s\n\n' "$section"
else
  if [ ${#new[@]} -eq 0 ] && [ ${#fixes[@]} -eq 0 ]; then
    printf '## What'\''s new\n\nA maintenance release with no change you will notice.\n\n'
  fi
  if [ ${#new[@]} -gt 0 ]; then
    printf '## What'\''s new\n\n'
    printf -- '- %s\n' "${new[@]}"
    printf '\n'
  fi
  if [ ${#fixes[@]} -gt 0 ]; then
    printf '## Fixes\n\n'
    printf -- '- %s\n' "${fixes[@]}"
    printf '\n'
  fi
fi

printf '## Known limits\n\n'
cat .github/release-known-limits.md
printf '\n## Checking your download\n\n'
printf 'Every file below has its SHA-256 in `SHA256SUMS.txt`. EinCast shows the same values when it tells you an update is available. To check a file, run `shasum -a 256 <file>` in Terminal (Mac) or `Get-FileHash <file>` in PowerShell (Windows) and compare.\n'

if [ -n "$prev" ] && [ -n "${GITHUB_REPOSITORY:-}" ]; then
  printf '\n**Full changelog:** https://github.com/%s/compare/%s...%s\n' "$GITHUB_REPOSITORY" "$prev" "${rev#refs/tags/}"
fi
