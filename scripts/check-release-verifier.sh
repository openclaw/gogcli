#!/usr/bin/env bash
set -euo pipefail

tag=${1:-}
expected_draft=${2:-true}
expected_tag_commit=${3:-}
expected_tag_object=${4:-}
repository=openclaw/gogcli
root=$(cd "$(dirname "$0")/.." && pwd)

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ||
  ( "$expected_draft" != true && "$expected_draft" != false ) ||
  ! "$expected_tag_commit" =~ ^[[:xdigit:]]{40}$ ||
  ! "$expected_tag_object" =~ ^[[:xdigit:]]{40}$ ]]; then
  echo "usage: $0 vX.Y.Z true|false expected-tag-commit expected-tag-object" >&2
  exit 2
fi
[[ -z "${GITHUB_REPOSITORY+x}" || "$GITHUB_REPOSITORY" == "$repository" ]] || {
  echo "release verifier: GITHUB_REPOSITORY must be $repository" >&2
  exit 1
}
for tool in gh grep jq mktemp unzip; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "release verifier: missing required command: $tool" >&2
    exit 1
  }
done

github_api() {
  command gh api --hostname github.com "$@"
}

tag_metadata=$("$root/scripts/verify-release-tag.sh" "$tag" "$expected_tag_commit")
read -r pinned_tag_object pinned_tag_commit extra <<<"$tag_metadata"
[[ "$pinned_tag_object" == "$expected_tag_object" &&
  "$pinned_tag_commit" == "$expected_tag_commit" && -z "${extra:-}" ]] || {
  echo "release verifier: current tag does not match the repository-pinned signer proof" >&2
  exit 1
}

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/gogcli-verifier-check.XXXXXX")
trap 'rm -rf "$work_dir"' EXIT

version=${tag#v}
expected_names=(
  "checksums.txt"
  "gogcli_${version}_darwin_amd64.tar.gz"
  "gogcli_${version}_darwin_arm64.tar.gz"
  "gogcli_${version}_linux_amd64.tar.gz"
  "gogcli_${version}_linux_arm64.tar.gz"
  "gogcli_${version}_windows_amd64.zip"
  "gogcli_${version}_windows_arm64.zip"
)

github_api "repos/$repository/git/ref/tags/$tag" > "$work_dir/tag-ref.json"
tag_object=$(jq -r '.object.sha // empty' "$work_dir/tag-ref.json")
[[ "$(jq -r '.ref // empty' "$work_dir/tag-ref.json")" == "refs/tags/$tag" &&
  "$(jq -r '.object.type // empty' "$work_dir/tag-ref.json")" == tag &&
  "$tag_object" == "$expected_tag_object" ]] || {
  echo "release verifier: current remote tag is not annotated" >&2
  exit 1
}
github_api "repos/$repository/git/tags/$tag_object" > "$work_dir/tag-object.json"
current_tag_commit=$(jq -r '.object.sha // empty' "$work_dir/tag-object.json")
[[ "$(jq -r '.tag // empty' "$work_dir/tag-object.json")" == "$tag" &&
  "$(jq -r '.object.type // empty' "$work_dir/tag-object.json")" == commit &&
  "$current_tag_commit" == "$expected_tag_commit" &&
  "$(jq -r '.verification.verified // false' "$work_dir/tag-object.json")" == true &&
  "$(jq -r '.verification.reason // empty' "$work_dir/tag-object.json")" == valid ]] || {
  echo "release verifier: current signed tag commit does not match the trusted release commit" >&2
  exit 1
}

github_api --paginate "repos/$repository/releases?per_page=100" > "$work_dir/release-pages.json"
release=$(
  jq -cs --arg tag "$tag" --argjson draft "$expected_draft" \
    '[.[][] | select(.tag_name == $tag and .draft == $draft)]' \
    "$work_dir/release-pages.json"
)
[[ "$(jq 'length' <<<"$release")" == 1 ]] || {
  echo "release verifier: expected exactly one release for $tag with draft=$expected_draft" >&2
  exit 1
}
assets=$(jq -c '.[0].assets' <<<"$release")
[[ "$(jq 'length' <<<"$assets")" == "${#expected_names[@]}" ]] || {
  echo "release verifier: release must contain exactly seven assets" >&2
  exit 1
}
for name in "${expected_names[@]}"; do
  [[ "$(jq --arg name "$name" '[.[] | select(.name == $name)] | length' <<<"$assets")" == 1 ]] || {
    echo "release verifier: asset missing or duplicated: $name" >&2
    exit 1
  }
done
latest_asset_time=$(jq -r '[.[].updated_at] | max // empty' <<<"$assets")
[[ "$latest_asset_time" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T ]] || {
  echo "release verifier: release assets have invalid update times" >&2
  exit 1
}

repo_json=$(github_api "repos/$repository")
default_branch=$(jq -r '.default_branch // empty' <<<"$repo_json")
[[ "$default_branch" =~ ^[A-Za-z0-9._/-]+$ ]] || {
  echo "release verifier: repository has an invalid default branch" >&2
  exit 1
}
default_sha=$(github_api "repos/$repository/commits/$default_branch" --jq '.sha')
[[ "$default_sha" =~ ^[[:xdigit:]]{40}$ ]] || {
  echo "release verifier: default branch has an invalid commit" >&2
  exit 1
}

if [[ "$expected_draft" == true ]]; then
  expected_state=draft
  proof_time=$latest_asset_time
  published_floor=
else
  expected_state=published
  published_at=$(jq -r '.[0].published_at // empty' <<<"$release")
  [[ "$published_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T ]] || {
    echo "release verifier: published release has an invalid publication time" >&2
    exit 1
  }
  proof_time=$latest_asset_time
  published_floor=$published_at
fi

github_api --paginate \
  "repos/$repository/actions/workflows/release-assets.yml/runs?per_page=100" \
  > "$work_dir/run-pages.json"
dispatch_title="Verify $tag $expected_state assets at $default_sha for $expected_tag_commit object $expected_tag_object"
title_prefix="Verify $tag $expected_state assets at $default_sha for "
run=$(
  jq -cs \
    --arg title_prefix "$title_prefix" \
    --arg branch "$default_branch" \
    --arg sha "$default_sha" \
    --arg proof_time "$proof_time" \
    --arg published_floor "$published_floor" \
    '[.[].workflow_runs[] | select(
      .event == "workflow_dispatch" and
      .head_branch == $branch and
      .head_sha == $sha and
      (.display_title | startswith($title_prefix)) and
      .status == "completed" and
      .conclusion == "success" and
      .created_at > $proof_time and
      ($published_floor == "" or .created_at >= $published_floor)
    )] | sort_by([.created_at, .id]) | last' \
    "$work_dir/run-pages.json"
)
[[ "$run" != null ]] || {
  echo "release verifier: no successful current-default-branch native $expected_state verifier found for $tag" >&2
  exit 1
}
run_id=$(jq -r '.id' <<<"$run")
[[ "$run_id" =~ ^[0-9]+$ ]] || {
  echo "release verifier: workflow run has an invalid id" >&2
  exit 1
}
run_event=$(jq -r '.event // empty' <<<"$run")
run_title=$(jq -r '.display_title // empty' <<<"$run")
[[ "$run_event" == workflow_dispatch && "$run_title" == "$dispatch_title" &&
  "$(jq -r '.head_branch // empty' <<<"$run")" == "$default_branch" &&
  "$(jq -r '.head_sha // empty' <<<"$run")" == "$default_sha" ]] || {
    echo "release verifier: newest relevant run does not match the current signed tag identity" >&2
    exit 1
  }

github_api --paginate "repos/$repository/actions/runs/$run_id/jobs?per_page=100" > "$work_dir/job-pages.json"
jobs=$(jq -cs '[.[].jobs[]]' "$work_dir/job-pages.json")
[[ "$(jq 'length' <<<"$jobs")" == 2 ]] || {
  echo "release verifier: expected exactly two native verifier jobs" >&2
  exit 1
}
for arch in arm64 x86_64; do
  name="Verify notarized macOS archive ($arch)"
  [[ "$(jq --arg name "$name" '[.[] | select(.name == $name and .status == "completed" and .conclusion == "success")] | length' <<<"$jobs")" == 1 ]] || {
    echo "release verifier: missing successful native job: $name" >&2
    exit 1
  }
done

github_api "repos/$repository/actions/runs/$run_id/logs" > "$work_dir/run-logs.zip"
unzip -Z1 "$work_dir/run-logs.zip" > "$work_dir/run-log-entries.txt"
proof_marker="GOG_RELEASE_PROOF tag=$tag object=$expected_tag_object commit=$expected_tag_commit workflow=$default_sha state=$expected_state"
for arch in arm64 x86_64; do
  step_log="Verify notarized macOS archive ($arch)/6_Execute verified candidate last.txt"
  [[ "$(grep -Fxc -- "$step_log" "$work_dir/run-log-entries.txt")" == 1 ]] || {
    echo "release verifier: missing exact final-step log for $arch" >&2
    exit 1
  }
  unzip -p "$work_dir/run-logs.zip" "$step_log" > "$work_dir/run-log-$arch.txt"
  marker_count=$(awk -v marker="$proof_marker" '
    {
      separator = index($0, " ")
      if (separator > 0 && substr($0, separator + 1) == marker) count++
    }
    END { print count + 0 }
  ' "$work_dir/run-log-$arch.txt")
  [[ "$marker_count" == 1 ]] || {
    echo "release verifier: $arch job is not bound to the exact signed tag object" >&2
    exit 1
  }
done

printf '%s\n' "$run_id"
