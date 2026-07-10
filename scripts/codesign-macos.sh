#!/usr/bin/env bash
set -euo pipefail

binary=${1:-}
identifier=com.steipete.gogcli.gog
expected_authority='Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)'
expected_team_id=FWJYW4S8P8
requirement="identifier \"$identifier\" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = \"$expected_team_id\""
# codesign renders this simple alphanumeric OU value unquoted; compare its
# stable emitted form byte-for-byte.
canonical_designated_requirement="identifier \"$identifier\" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = $expected_team_id"

if [[ -z "$binary" ]]; then
  echo "usage: $0 <path-to-binary>" >&2
  exit 2
fi

case "${GOG_OFFICIAL_RELEASE:-0}" in
  0)
    echo "codesign: ordinary build; leaving $binary unchanged" >&2
    exit 0
    ;;
  1) ;;
  *)
    echo "codesign: GOG_OFFICIAL_RELEASE must be 0 or 1" >&2
    exit 1
    ;;
esac

[[ "$(uname -s)" == Darwin ]] || {
  echo "codesign: official Gog releases require macOS" >&2
  exit 1
}
[[ -f "$binary" ]] || {
  echo "codesign: binary not found: $binary" >&2
  exit 1
}

identity=${CODESIGN_IDENTITY:-}
[[ "$identity" == "$expected_authority" ]] || {
  echo "codesign: official releases require $expected_authority" >&2
  exit 1
}
[[ -n "${CODESIGN_KEYCHAIN:-}" && -n "${MAC_RELEASE_CODESIGN_KEYCHAIN:-}" && "$CODESIGN_KEYCHAIN" == "$MAC_RELEASE_CODESIGN_KEYCHAIN" ]] || {
  echo "codesign: official releases must run through release-mac-app codesign-run" >&2
  exit 1
}
[[ -n "${NOTARYTOOL_KEYCHAIN_PROFILE:-}" ]] || {
  echo "codesign: NOTARYTOOL_KEYCHAIN_PROFILE is required at runtime" >&2
  exit 1
}

for tool in codesign ditto mktemp mv plutil xcrun; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "codesign: missing required command: $tool" >&2
    exit 1
  }
done

binary_dir=$(cd "$(dirname "$binary")" && pwd)
binary_name=$(basename "$binary")
work_dir=$(mktemp -d "$binary_dir/.gog-notary.XXXXXX")
candidate="$work_dir/$binary_name"
submission="$work_dir/$binary_name.zip"
trap 'rm -rf "$work_dir"' EXIT

# Keep the GoReleaser output immutable until signing, notarization, and every
# assessment pass. A rejected submission never leaves a half-valid artifact.
cp -p "$binary" "$candidate"
codesign \
  --force \
  --keychain "$CODESIGN_KEYCHAIN" \
  --sign "$identity" \
  --timestamp \
  --options runtime \
  --identifier "$identifier" \
  "$candidate"

ditto -c -k --sequesterRsrc --keepParent "$candidate" "$submission"
notary_result=$(xcrun notarytool submit "$submission" \
  --keychain-profile "$NOTARYTOOL_KEYCHAIN_PROFILE" \
  --no-s3-acceleration \
  --wait \
  --output-format json)
notary_status=$(plutil -extract status raw -o - - <<<"$notary_result")
notary_id=$(plutil -extract id raw -o - - <<<"$notary_result")
[[ "$notary_status" == Accepted ]] || {
  echo "codesign: notarization status is ${notary_status:-missing}, expected Accepted" >&2
  exit 1
}
[[ "$notary_id" =~ ^[[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}$ ]] || {
  echo "codesign: notarization response has an invalid submission id" >&2
  exit 1
}

details=$(codesign --display --verbose=4 "$candidate" 2>&1)
actual_identifier=$(awk -F= '$1 == "Identifier" { print $2; exit }' <<<"$details")
actual_team_id=$(awk -F= '$1 == "TeamIdentifier" { print $2; exit }' <<<"$details")
actual_authority=$(awk -F= '$1 == "Authority" { sub(/^[^=]*=/, ""); print; exit }' <<<"$details")
runtime_version=$(awk -F= '$1 == "Runtime Version" { print $2; exit }' <<<"$details")
timestamp=$(awk -F= '$1 == "Timestamp" { sub(/^[^=]*=/, ""); print; exit }' <<<"$details")

[[ "$actual_identifier" == "$identifier" ]] || {
  echo "codesign: expected identifier $identifier, got ${actual_identifier:-none}" >&2
  exit 1
}
[[ "$actual_team_id" == "$expected_team_id" ]] || {
  echo "codesign: expected Team ID $expected_team_id, got ${actual_team_id:-none}" >&2
  exit 1
}
[[ "$actual_authority" == "$expected_authority" ]] || {
  echo "codesign: expected authority $expected_authority, got ${actual_authority:-none}" >&2
  exit 1
}
grep -Eq '^CodeDirectory .*flags=.*\([^)]*runtime[^)]*\)' <<<"$details" || {
  echo "codesign: hardened runtime flag is missing" >&2
  exit 1
}
[[ "$runtime_version" =~ ^[0-9]+(\.[0-9]+)+$ ]] || {
  echo "codesign: hardened runtime metadata is missing" >&2
  exit 1
}
[[ -n "$timestamp" && "$timestamp" != none ]] || {
  echo "codesign: trusted timestamp is missing" >&2
  exit 1
}

requirement_details=$(codesign -d -r- "$candidate" 2>&1)
[[ "$(grep -c '^designated => ' <<<"$requirement_details")" == 1 ]] || {
  echo "codesign: embedded designated requirement is missing or ambiguous" >&2
  exit 1
}
actual_designated_requirement=$(awk '/^designated => / { sub(/^designated => /, ""); print }' <<<"$requirement_details")
[[ "$actual_designated_requirement" == "$canonical_designated_requirement" ]] || {
  echo "codesign: embedded designated requirement is not canonical" >&2
  exit 1
}

codesign --verify --strict -R="$requirement" --verbose=2 "$candidate"
codesign --verify --strict --check-notarization -R='notarized' "$candidate"

mv -f "$candidate" "$binary"
