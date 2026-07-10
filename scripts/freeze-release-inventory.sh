#!/usr/bin/env bash
set -euo pipefail

tag=${1:-}
artifact_dir=${2:-}

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ||
  ! -d "$artifact_dir" ]]; then
  echo "usage: $0 vX.Y.Z artifact-directory" >&2
  exit 2
fi
[[ -z "${GH_TOKEN+x}" && -z "${GITHUB_TOKEN+x}" ]] || {
  echo "release inventory: GitHub tokens must be absent" >&2
  exit 1
}
command -v shasum >/dev/null 2>&1 || {
  echo "release inventory: shasum is required" >&2
  exit 1
}

version=${tag#v}
expected_files=(
  checksums.txt
  tag-commit.txt
  "gogcli_${version}_darwin_amd64.tar.gz"
  "gogcli_${version}_darwin_arm64.tar.gz"
  "gogcli_${version}_linux_amd64.tar.gz"
  "gogcli_${version}_linux_arm64.tar.gz"
  "gogcli_${version}_windows_amd64.zip"
  "gogcli_${version}_windows_arm64.zip"
)

[[ "$(find "$artifact_dir" -mindepth 1 -maxdepth 1 | wc -l | tr -d ' ')" == "${#expected_files[@]}" ]] || {
  echo "release inventory: artifact directory has an unexpected entry count" >&2
  exit 1
}
for name in "${expected_files[@]}"; do
  path="$artifact_dir/$name"
  [[ -f "$path" && ! -L "$path" ]] || {
    echo "release inventory: missing or unsafe file: $name" >&2
    exit 1
  }
  hash=$(shasum -a 256 "$path" | awk '{print $1}')
  [[ "$hash" =~ ^[[:xdigit:]]{64}$ ]] || {
    echo "release inventory: could not hash: $name" >&2
    exit 1
  }
  printf '%s  %s\n' "$hash" "$name"
done
