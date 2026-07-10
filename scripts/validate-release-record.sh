#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: scripts/validate-release-record.sh <release-json> <tag> <draft> <tag-commit> <release-body-file> [expected-snapshot]" >&2
  exit 2
}

[[ "$#" -eq 5 || "$#" -eq 6 ]] || usage
release_file=$1
tag=$2
expected_draft=$3
tag_commit=$4
release_body_file=$5
expected_snapshot=${6:-}

[[ -f "$release_file" && -f "$release_body_file" ]] || usage
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || usage
[[ "$expected_draft" == true || "$expected_draft" == false ]] || usage
[[ "$tag_commit" =~ ^[[:xdigit:]]{40}$ ]] || usage
[[ -z "$expected_snapshot" || -f "$expected_snapshot" ]] || usage

version=${tag#v}
expected_assets=$(jq -cn \
  --arg checksums checksums.txt \
  --arg darwin_amd64 "gogcli_${version}_darwin_amd64.tar.gz" \
  --arg darwin_arm64 "gogcli_${version}_darwin_arm64.tar.gz" \
  --arg linux_amd64 "gogcli_${version}_linux_amd64.tar.gz" \
  --arg linux_arm64 "gogcli_${version}_linux_arm64.tar.gz" \
  --arg windows_amd64 "gogcli_${version}_windows_amd64.zip" \
  --arg windows_arm64 "gogcli_${version}_windows_arm64.zip" \
  '[$checksums, $darwin_amd64, $darwin_arm64, $linux_amd64, $linux_arm64, $windows_amd64, $windows_arm64] | sort')

jq -e \
  --arg tag "$tag" \
  --arg commit "$tag_commit" \
  --rawfile body "$release_body_file" \
  --argjson draft "$expected_draft" \
  --argjson names "$expected_assets" '
    type == "object" and
    (.id | type == "number" and . > 0 and floor == .) and
    .tag_name == $tag and
    .name == $tag and
    .target_commitish == $commit and
    .draft == $draft and
    .prerelease == false and
    .body == $body and
    (.assets | type == "array" and length == 7) and
    ([.assets[].name] | sort) == $names and
    ([.assets[].id] | unique | length) == 7 and
    (all(.assets[];
      (.id | type == "number" and . > 0 and floor == .) and
      (.name | type == "string" and length > 0) and
      (.size | type == "number" and . > 0 and floor == .) and
      (.digest | type == "string" and test("^sha256:[0-9a-f]{64}$")) and
      .state == "uploaded")) and
    (if $draft then true else
      (.published_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"))
    end)
  ' "$release_file" >/dev/null || {
  echo "release: API record failed exact metadata or asset validation" >&2
  exit 1
}

snapshot=$(jq -cS '
  {
    id,
    tag_name,
    name,
    target_commitish,
    prerelease,
    body,
    assets: ([.assets[] | {id, name, size, digest}] | sort_by(.name))
  }
' "$release_file")

if [[ -n "$expected_snapshot" ]]; then
  expected=$(jq -cS . "$expected_snapshot")
  [[ "$snapshot" == "$expected" ]] || {
    echo "release: API record differs from the verifier-accepted release snapshot" >&2
    exit 1
  }
fi

printf '%s\n' "$snapshot"
