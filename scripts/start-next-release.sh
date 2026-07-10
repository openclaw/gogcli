#!/usr/bin/env bash
set -euo pipefail

tag=${1:-}
root=$(cd "$(dirname "$0")/.." && pwd)

if [[ ! "$tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 2
fi

major=${BASH_REMATCH[1]}
minor=${BASH_REMATCH[2]}
patch=${BASH_REMATCH[3]}
version=${tag#v}
next_version="$major.$minor.$((patch + 1))"

cd "$root"
[[ "$(git branch --show-current)" == main ]] || {
  echo "next release: expected branch main" >&2
  exit 1
}
[[ -z "$(git status --porcelain --untracked-files=all)" ]] || {
  echo "next release: working tree must be clean" >&2
  exit 1
}
[[ "$(tr -d '[:space:]' < internal/cmd/VERSION)" == "$tag" ]] || {
  echo "next release: internal/cmd/VERSION must still contain $tag" >&2
  exit 1
}
[[ "$(grep -Ec "^## ${version} - [0-9]{4}-[0-9]{2}-[0-9]{2}$" CHANGELOG.md)" == 1 ]] || {
  echo "next release: released changelog heading is missing or ambiguous" >&2
  exit 1
}
if grep -Eq '^## [0-9]+\.[0-9]+\.[0-9]+ - Unreleased$' CHANGELOG.md; then
  echo "next release: an Unreleased section already exists" >&2
  exit 1
fi

tmp=$(mktemp "${TMPDIR:-/tmp}/gogcli-next-changelog.XXXXXX")
trap 'rm -f "$tmp"' EXIT
awk -v next_version="$next_version" '
  { print }
  /^# Changelog$/ && !inserted {
    print ""
    print "## " next_version " - Unreleased"
    inserted = 1
  }
  END { if (!inserted) exit 1 }
' CHANGELOG.md > "$tmp"
mv "$tmp" CHANGELOG.md
trap - EXIT
printf '%s\n' "${tag}-dev" > internal/cmd/VERSION

echo "next release: opened $next_version Unreleased and set internal/cmd/VERSION to ${tag}-dev"
echo "next release: review, commit, and push these two files only"
