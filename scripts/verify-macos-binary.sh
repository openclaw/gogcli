#!/usr/bin/env bash
set -euo pipefail

binary=${1:-}
expected_arch=${2:-}
expected_version=${3:-}
mode=${4:-}
identifier=com.steipete.gogcli.gog
expected_authority='Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)'
expected_team_id=FWJYW4S8P8
requirement="identifier \"$identifier\" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = \"$expected_team_id\""
# codesign renders this simple alphanumeric OU value unquoted; compare its
# stable emitted form byte-for-byte.
canonical_designated_requirement="identifier \"$identifier\" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = $expected_team_id"

if [[ -z "$binary" || ! "$expected_arch" =~ ^(arm64|x86_64)$ ||
  ! "$expected_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ||
  ! "$mode" =~ ^(static|execute)$ ]]; then
  echo "macOS verify: usage: $0 binary arm64|x86_64 version static|execute" >&2
  exit 2
fi
[[ -z "${GH_TOKEN+x}" && -z "${GITHUB_TOKEN+x}" ]] || {
  echo "macOS verify: GitHub tokens must be absent" >&2
  exit 1
}
[[ "$(uname -s)" == Darwin ]] || {
  echo "macOS verify: a native macOS host is required" >&2
  exit 1
}
[[ -f "$binary" && ! -L "$binary" ]] || {
  echo "macOS verify: candidate is missing or is a symbolic link" >&2
  exit 1
}
if [[ "$mode" == execute ]]; then
  command -v env >/dev/null 2>&1 || {
    echo "macOS verify: missing required command: env" >&2
    exit 1
  }
  version_output=$(env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin "$binary" --version)
  version_pattern="^v${expected_version//./\\.} \\([[:xdigit:]]{7,40} [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z\\)$"
  [[ "$version_output" =~ $version_pattern ]] || {
    echo "macOS verify: wrong embedded version" >&2
    exit 1
  }
  exit 0
fi

for tool in codesign lipo; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "macOS verify: missing required command: $tool" >&2
    exit 1
  }
done

details=$(codesign --display --verbose=4 "$binary" 2>&1)
actual_identifier=$(awk -F= '$1 == "Identifier" { print $2; exit }' <<<"$details")
actual_team_id=$(awk -F= '$1 == "TeamIdentifier" { print $2; exit }' <<<"$details")
actual_authority=$(awk -F= '$1 == "Authority" { sub(/^[^=]*=/, ""); print; exit }' <<<"$details")
runtime_version=$(awk -F= '$1 == "Runtime Version" { print $2; exit }' <<<"$details")
timestamp=$(awk -F= '$1 == "Timestamp" { sub(/^[^=]*=/, ""); print; exit }' <<<"$details")
[[ "$actual_identifier" == "$identifier" ]] || {
  echo "macOS verify: wrong identifier: ${actual_identifier:-none}" >&2
  exit 1
}
[[ "$actual_team_id" == "$expected_team_id" ]] || {
  echo "macOS verify: wrong Team ID: ${actual_team_id:-none}" >&2
  exit 1
}
[[ "$actual_authority" == "$expected_authority" ]] || {
  echo "macOS verify: wrong signing authority: ${actual_authority:-none}" >&2
  exit 1
}
grep -Eq '^CodeDirectory .*flags=.*\([^)]*runtime[^)]*\)' <<<"$details" || {
  echo "macOS verify: hardened runtime flag is missing" >&2
  exit 1
}
[[ "$runtime_version" =~ ^[0-9]+(\.[0-9]+)+$ ]] || {
  echo "macOS verify: hardened runtime metadata is missing" >&2
  exit 1
}
[[ -n "$timestamp" && "$timestamp" != none ]] || {
  echo "macOS verify: trusted timestamp is missing" >&2
  exit 1
}

requirement_details=$(codesign -d -r- "$binary" 2>&1)
[[ "$(grep -c '^designated => ' <<<"$requirement_details")" == 1 ]] || {
  echo "macOS verify: embedded designated requirement is missing or ambiguous" >&2
  exit 1
}
actual_designated_requirement=$(awk '/^designated => / { sub(/^designated => /, ""); print }' <<<"$requirement_details")
[[ "$actual_designated_requirement" == "$canonical_designated_requirement" ]] || {
  echo "macOS verify: embedded designated requirement is not canonical" >&2
  exit 1
}

codesign --verify --strict -R="$requirement" --verbose=2 "$binary"
codesign --verify --strict --check-notarization -R='notarized' "$binary"
actual_arch=$(lipo -archs "$binary" | tr -d '[:space:]')
[[ "$actual_arch" == "$expected_arch" ]] || {
  echo "macOS verify: wrong architecture" >&2
  exit 1
}
