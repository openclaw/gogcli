#!/usr/bin/env bash
# One-command upstreaming of a Fork-Patch: <slug> commit as a PR against upstream.
# Cherry-picks onto a fresh branch off upstream/<branch> and pushes to an upstream-facing fork.
set -euo pipefail

UPSTREAM_URL="https://github.com/steipete/gogcli.git"
UPSTREAM_BRANCH="main"

if [[ $# -ne 1 ]]; then
  echo "usage: $(basename "$0") <slug>" >&2
  exit 2
fi
slug="$1"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
cd "$repo_root"

# Extract owner/repo from upstream URL (handles https and git@).
upstream_slug="$(printf '%s' "$UPSTREAM_URL" \
  | sed -E 's#^(https://[^/]+/|git@[^:]+:)##' \
  | sed -E 's#\.git$##')"
upstream_owner="${upstream_slug%%/*}"
upstream_repo="${upstream_slug##*/}"

mapfile -t matches < <(git log --grep="^Fork-Patch: ${slug}\$" --format=%H main)
if [[ ${#matches[@]} -eq 0 ]]; then
  echo "error: no commit found with trailer 'Fork-Patch: $slug'" >&2
  exit 1
fi
if [[ ${#matches[@]} -gt 1 ]]; then
  echo "error: multiple commits found with trailer 'Fork-Patch: $slug':" >&2
  printf '  %s\n' "${matches[@]}" >&2
  exit 1
fi
commit="${matches[0]}"

if ! git remote get-url upstream >/dev/null 2>&1; then
  echo "error: 'upstream' remote not configured" >&2
  exit 1
fi
git fetch --quiet upstream "$UPSTREAM_BRANCH"

# Confirm or create the upstream-facing fork remote.
if ! git remote get-url upstream-fork >/dev/null 2>&1; then
  echo "upstream-patch.sh: 'upstream-fork' remote is not configured."
  echo "  Create a personal fork of steipete/gogcli first, then:"
  echo "    gh repo fork $UPSTREAM_URL --remote=false"
  echo "    git remote add upstream-fork git@github.com:<your-user>/gogcli.git"
  exit 1
fi

branch="upstream-contrib/${slug}"
if git show-ref --verify --quiet "refs/heads/$branch"; then
  echo "error: branch $branch already exists; delete or rename it first" >&2
  exit 1
fi
git checkout -b "$branch" "upstream/$UPSTREAM_BRANCH"

if ! git cherry-pick "$commit"; then
  echo
  echo "upstream-patch.sh: cherry-pick conflicted. Aborting."
  echo "  resolve manually: 'git cherry-pick --continue' when done, then rerun this script with --resume (not yet implemented)."
  git cherry-pick --abort || true
  git checkout -q -
  git branch -D "$branch"
  exit 1
fi

git push -u upstream-fork "$branch"

# Build PR body from the cherry-picked commit's message body (skip subject + trailers).
body="$(git log -1 --format=%b "$branch")"
gh_user="$(gh api user --jq .login)"

gh pr create \
  --repo "steipete/gogcli" \
  --head "$gh_user:$branch" \
  --base "$UPSTREAM_BRANCH" \
  --title "$(git log -1 --format=%s "$branch")" \
  --body "$body"
