#!/usr/bin/env bash
set -euo pipefail

tag=${1:-}
goarch=${2:-}
expected_draft=${3:-}
expected_tag_commit=${4:-}
expected_tag_object=${5:-}
out_dir=${6:-}
repository=openclaw/gogcli

usage() {
  echo "usage: $0 vX.Y.Z arm64|amd64 true|false expected-tag-commit expected-tag-object output-directory" >&2
  exit 2
}

[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || usage
[[ "$goarch" == arm64 || "$goarch" == amd64 ]] || usage
[[ "$expected_draft" == true || "$expected_draft" == false ]] || usage
[[ "$expected_tag_commit" =~ ^[[:xdigit:]]{40}$ ]] || usage
[[ "$expected_tag_object" =~ ^[[:xdigit:]]{40}$ ]] || usage
[[ -n "$out_dir" ]] || usage
[[ -z "${GITHUB_REPOSITORY+x}" || "$GITHUB_REPOSITORY" == "$repository" ]] || {
  echo "release download: GITHUB_REPOSITORY must be $repository" >&2
  exit 1
}
[[ -n "${GH_TOKEN:-}" ]] || {
  echo "release download: GH_TOKEN is required" >&2
  exit 1
}
[[ ! -e "$out_dir" && ! -L "$out_dir" ]] || {
  echo "release download: refusing to replace destination: $out_dir" >&2
  exit 1
}

for tool in gh jq mktemp mv; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "release download: missing required command: $tool" >&2
    exit 1
  }
done

github_api() {
  command gh api --hostname github.com "$@"
}

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

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/gogcli-release-download.XXXXXX")
trap 'rm -rf "$work_dir"' EXIT
stage="$work_dir/artifacts"
mkdir -p "$stage"

github_api "repos/$repository/git/ref/tags/$tag" > "$work_dir/tag-ref.json"
[[ "$(jq -r '.ref // empty' "$work_dir/tag-ref.json")" == "refs/tags/$tag" ]] || {
  echo "release download: remote tag ref does not match $tag" >&2
  exit 1
}
tag_object=$(jq -r '.object.sha // empty' "$work_dir/tag-ref.json")
[[ "$(jq -r '.object.type // empty' "$work_dir/tag-ref.json")" == tag && "$tag_object" == "$expected_tag_object" ]] || {
  echo "release download: $tag must resolve to an annotated tag object" >&2
  exit 1
}

github_api "repos/$repository/git/tags/$tag_object" > "$work_dir/tag-object.json"
tag_commit=$(jq -r '.object.sha // empty' "$work_dir/tag-object.json")
[[ "$(jq -r '.tag // empty' "$work_dir/tag-object.json")" == "$tag" &&
  "$(jq -r '.object.type // empty' "$work_dir/tag-object.json")" == commit &&
  "$tag_commit" == "$expected_tag_commit" &&
  "$(jq -r '.verification.verified // false' "$work_dir/tag-object.json")" == true &&
  "$(jq -r '.verification.reason // empty' "$work_dir/tag-object.json")" == valid ]] || {
  echo "release download: $tag is not a valid signed annotated commit tag" >&2
  exit 1
}
printf '%s\n' "$tag_commit" > "$stage/tag-commit.txt"

github_api --paginate "repos/$repository/releases?per_page=100" > "$work_dir/release-pages.json"
release=$(
  jq -cs --arg tag "$tag" --argjson draft "$expected_draft" \
    '[.[][] | select(.tag_name == $tag and .draft == $draft)]' \
    "$work_dir/release-pages.json"
)
[[ "$(jq 'length' <<<"$release")" == 1 ]] || {
  echo "release download: expected exactly one release for $tag with draft=$expected_draft" >&2
  exit 1
}
release_id=$(jq -r '.[0].id' <<<"$release")
[[ "$release_id" =~ ^[0-9]+$ ]] || {
  echo "release download: release has an invalid API id" >&2
  exit 1
}

github_api --paginate "repos/$repository/releases/$release_id/assets?per_page=100" > "$work_dir/asset-pages.json"
assets=$(jq -cs '[.[][]]' "$work_dir/asset-pages.json")
[[ "$(jq 'length' <<<"$assets")" == "${#expected_names[@]}" ]] || {
  echo "release download: release must contain exactly seven assets" >&2
  exit 1
}

api_prefix="https://api.github.com/repos/$repository/releases/assets/"
for name in "${expected_names[@]}"; do
  matches=$(jq -c --arg name "$name" '[.[] | select(.name == $name)]' <<<"$assets")
  [[ "$(jq 'length' <<<"$matches")" == 1 ]] || {
    echo "release download: asset missing or duplicated: $name" >&2
    exit 1
  }
  api_url=$(jq -r '.[0].url' <<<"$matches")
  asset_id=${api_url#"$api_prefix"}
  [[ "$api_url" == "$api_prefix"* && "$asset_id" =~ ^[0-9]+$ ]] || {
    echo "release download: asset has an invalid API URL: $name" >&2
    exit 1
  }
done

for name in "${expected_names[@]}"; do
  api_url=$(jq -r --arg name "$name" '.[] | select(.name == $name) | .url' <<<"$assets")
  github_api "$api_url" -H "Accept: application/octet-stream" > "$stage/$name"
  [[ -s "$stage/$name" ]] || {
    echo "release download: downloaded asset is empty: $name" >&2
    exit 1
  }
done

mkdir -p "$(dirname "$out_dir")"
mv "$stage" "$out_dir"
