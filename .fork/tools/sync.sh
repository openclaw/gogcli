#!/usr/bin/env bash
# Manual wrapper around the sync flow for local use (CI uses fork-upstream-sync.yml).
# Tags upstream, resets the upstream branch, rebases a sync branch, regenerates patches.
set -euo pipefail

UPSTREAM_BRANCH="main"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
cd "$repo_root"

sync_branch=""
started_from=""

cleanup() {
  local rc=$?
  if [[ $rc -ne 0 && -n "$sync_branch" ]]; then
    echo "sync.sh: interrupted; attempting to clean up $sync_branch" >&2
    git rebase --abort 2>/dev/null || true
    if [[ -n "$started_from" ]]; then
      git checkout -q "$started_from" 2>/dev/null || true
    fi
    git branch -D "$sync_branch" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

status_line="$("$script_dir/check-upstream.sh")"
echo "$status_line"
case "$status_line" in
  UP_TO_DATE*)
    exit 0
    ;;
  NEW_UPSTREAM*)
    ;;
  *)
    echo "sync.sh: unexpected output from check-upstream.sh: $status_line" >&2
    exit 1
    ;;
esac

started_from="$(git symbolic-ref --quiet --short HEAD || git rev-parse HEAD)"
new_sha="$(git rev-parse "upstream/$UPSTREAM_BRANCH")"
new_short="$(git rev-parse --short "$new_sha")"
date_slug="$(date -u +%Y-%m-%d)"
tag="upstream-sync/${date_slug}-${new_short}"
sync_branch="sync/${date_slug}-${new_short}"

if ! git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  git tag "$tag" "$new_sha"
  echo "sync.sh: tagged $tag"
fi

# Fast-forward the local 'upstream' tracking branch, or reset if it diverged.
if git show-ref --verify --quiet refs/heads/upstream; then
  if ! git branch -f upstream "$new_sha"; then
    echo "sync.sh: failed to update local upstream branch" >&2
    exit 1
  fi
else
  git branch upstream "$new_sha"
fi
git push --force-with-lease origin upstream || {
  echo "sync.sh: push of upstream branch failed (continuing locally)" >&2
}

git checkout -b "$sync_branch" main
echo "sync.sh: rebasing $sync_branch onto upstream/$UPSTREAM_BRANCH"
if ! git rebase "upstream/$UPSTREAM_BRANCH"; then
  conflicted="$(git diff --name-only --diff-filter=U | tr '\n' ' ')"
  echo
  echo "sync.sh: conflicts in: $conflicted"
  echo "sync.sh: run '.fork/tools/llm_resolve.py $conflicted' to try automatic resolution,"
  echo "         or resolve by hand and 'git rebase --continue'."
  trap - EXIT INT TERM
  exit 2
fi

"$script_dir/export-patches.sh"
if ! git diff --quiet -- .fork/patches/; then
  git add .fork/patches/
  git commit -m "fork: refresh patches for sync ${date_slug}-${new_short}"
fi

trap - EXIT INT TERM
echo
echo "sync.sh: success. Next steps:"
echo "  git push -u origin $sync_branch"
echo "  gh pr create --base main --head $sync_branch --title 'sync: upstream $new_short'"
