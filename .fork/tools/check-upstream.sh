#!/usr/bin/env bash
# Check whether upstream has advanced past the SHA pinned in .fork/revision.txt.
# Prints "NEW_UPSTREAM <old> -> <new>" or "UP_TO_DATE <sha>" and exits 0.
set -euo pipefail

UPSTREAM_BRANCH="main"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
revision_file="$repo_root/.fork/revision.txt"

if [[ ! -f "$revision_file" ]]; then
  echo "error: $revision_file not found" >&2
  exit 2
fi

# revision.txt format (from template):
#   upstream = <sha> (<tag>)
#   synced   = <iso>
#   tag      = upstream-sync/<date>-<sha>
pinned_sha="$(grep -E '^upstream[[:space:]]*=' "$revision_file" \
  | head -n1 \
  | sed -E 's/^upstream[[:space:]]*=[[:space:]]*([0-9a-f]+).*/\1/')"

if [[ -z "$pinned_sha" ]]; then
  echo "error: could not parse pinned upstream sha from $revision_file" >&2
  exit 2
fi

if ! git -C "$repo_root" remote get-url upstream >/dev/null 2>&1; then
  echo "error: 'upstream' remote is not configured" >&2
  exit 2
fi

git -C "$repo_root" fetch --quiet upstream "$UPSTREAM_BRANCH"

new_sha="$(git -C "$repo_root" rev-parse "upstream/$UPSTREAM_BRANCH")"
old_short="$(git -C "$repo_root" rev-parse --short "$pinned_sha" 2>/dev/null || echo "$pinned_sha")"
new_short="$(git -C "$repo_root" rev-parse --short "$new_sha")"

if [[ "$(git -C "$repo_root" rev-parse "$pinned_sha" 2>/dev/null || echo "$pinned_sha")" == "$new_sha" ]]; then
  echo "UP_TO_DATE $new_short"
  exit 0
fi

echo "NEW_UPSTREAM $old_short -> $new_short"
exit 0
