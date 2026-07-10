#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/dist"
binary="$tmp/dist/gog"
log="$tmp/release.log"

cat > "$tmp/bin/uname" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' Darwin
EOF

cat > "$tmp/bin/codesign" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'codesign %s\n' "$*" >> "$RELEASE_TEST_LOG"
if [[ " $* " == *' --display '* ]]; then
  {
    printf 'Identifier=%s\n' "${MOCK_IDENTIFIER:-com.steipete.gogcli.gog}"
    printf 'CodeDirectory v=20500 size=100 flags=%s hashes=1+0 location=embedded\n' "${MOCK_CODE_FLAGS:-0x10000(runtime)}"
    printf 'Authority=%s\n' "${MOCK_AUTHORITY:-Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)}"
    printf 'Authority=Developer ID Certification Authority\n'
    printf 'Authority=Apple Root CA\n'
    printf 'Timestamp=%s\n' "${MOCK_TIMESTAMP:-09.07.2026 at 12:07:29}"
    printf 'TeamIdentifier=%s\n' "${MOCK_TEAM_ID:-FWJYW4S8P8}"
    printf 'Runtime Version=%s\n' "${MOCK_RUNTIME_VERSION:-12.0.0}"
  } >&2
  exit 0
fi
if [[ " $* " == *' -d '* && " $* " == *' -r- '* ]]; then
  echo 'Executable=/tmp/gog' >&2
  printf 'designated => %s\n' "${MOCK_EMBEDDED_REQUIREMENT:-identifier \"com.steipete.gogcli.gog\" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = FWJYW4S8P8}" >&2
  exit 0
fi
if [[ " $* " == *' --sign '* ]]; then
  for arg in "$@"; do target=$arg; done
  printf '\nsigned\n' >> "$target"
fi
if [[ " $* " == *' --check-notarization '* && "${MOCK_NOTARIZATION_REQUIREMENT_REJECT:-0}" == 1 ]]; then
  exit 3
fi
[[ "${MOCK_CODESIGN_VERIFY_REJECT:-0}" != 1 ]]
EOF

cat > "$tmp/bin/ditto" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
previous=
for arg in "$@"; do
  source=$previous
  previous=$arg
done
destination=$previous
cp "$source" "$destination"
printf 'ditto %s\n' "$*" >> "$RELEASE_TEST_LOG"
EOF

cat > "$tmp/bin/xcrun" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'xcrun %s\n' "$*" >> "$RELEASE_TEST_LOG"
[[ "${MOCK_NOTARY_REJECT:-0}" != 1 ]] || exit 1
printf '{"id":"12345678-1234-1234-1234-123456789abc","status":"%s"}\n' "${MOCK_NOTARY_STATUS:-Accepted}"
EOF

cat > "$tmp/bin/plutil" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${2:-}" in
  status) printf '%s\n' "${MOCK_NOTARY_STATUS:-Accepted}" ;;
  id) printf '%s\n' '12345678-1234-1234-1234-123456789abc' ;;
  *) exit 1 ;;
esac
EOF

chmod +x "$tmp/bin/"*
export PATH="$tmp/bin:$PATH"
export RELEASE_TEST_LOG="$log"

reset_binary() {
  printf 'unsigned-binary\n' > "$binary"
  original_hash=$(shasum -a 256 "$binary" | awk '{print $1}')
  : > "$log"
  unset MOCK_IDENTIFIER MOCK_AUTHORITY MOCK_TEAM_ID MOCK_TIMESTAMP
  unset MOCK_CODE_FLAGS MOCK_RUNTIME_VERSION MOCK_CODESIGN_VERIFY_REJECT
  unset MOCK_NOTARY_REJECT MOCK_NOTARY_STATUS MOCK_NOTARIZATION_REQUIREMENT_REJECT
  unset MOCK_EMBEDDED_REQUIREMENT
}

assert_unchanged() {
  [[ "$(shasum -a 256 "$binary" | awk '{print $1}')" == "$original_hash" ]] || {
    echo "codesign test: failed release mutated the GoReleaser artifact" >&2
    exit 1
  }
}

reset_binary
unset GOG_OFFICIAL_RELEASE CODESIGN_IDENTITY CODESIGN_KEYCHAIN
unset MAC_RELEASE_CODESIGN_KEYCHAIN NOTARYTOOL_KEYCHAIN_PROFILE
"$root/scripts/codesign-macos.sh" "$binary"
assert_unchanged
[[ ! -s "$log" ]] || {
  echo "codesign test: ordinary build used release credentials" >&2
  exit 1
}

export GOG_OFFICIAL_RELEASE=1
export CODESIGN_IDENTITY='Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)'
export CODESIGN_KEYCHAIN=/tmp/foundation-release.keychain-db
export MAC_RELEASE_CODESIGN_KEYCHAIN=$CODESIGN_KEYCHAIN

reset_binary
unset NOTARYTOOL_KEYCHAIN_PROFILE
if "$root/scripts/codesign-macos.sh" "$binary" >/dev/null 2>&1; then
  echo "codesign test: official release accepted a missing notary profile" >&2
  exit 1
fi
assert_unchanged

reset_binary
export NOTARYTOOL_KEYCHAIN_PROFILE=test-profile
export CODESIGN_IDENTITY='Developer ID Application: Wrong Identity (WRONGTEAM1)'
if "$root/scripts/codesign-macos.sh" "$binary" >/dev/null 2>&1; then
  echo "codesign test: official release accepted the wrong identity" >&2
  exit 1
fi
assert_unchanged

reset_binary
export CODESIGN_IDENTITY='Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)'
export MOCK_NOTARY_REJECT=1
if "$root/scripts/codesign-macos.sh" "$binary" >/dev/null 2>&1; then
  echo "codesign test: official release accepted notary rejection" >&2
  exit 1
fi
assert_unchanged

reset_binary
export MOCK_NOTARY_STATUS=Invalid
if "$root/scripts/codesign-macos.sh" "$binary" >/dev/null 2>&1; then
  echo "codesign test: official release accepted Invalid notarization status" >&2
  exit 1
fi
assert_unchanged

reset_binary
export MOCK_IDENTIFIER=org.example.wrong
if "$root/scripts/codesign-macos.sh" "$binary" >/dev/null 2>&1; then
  echo "codesign test: official release accepted the wrong identifier" >&2
  exit 1
fi
assert_unchanged

reset_binary
canonical_dr='identifier "com.steipete.gogcli.gog" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = FWJYW4S8P8'
for embedded_dr in \
  'identifier "org.example.wrong" and anchor apple generic' \
  "$canonical_dr and info[CFBundleVersion] = \"1\"" \
  'cdhash H"0123456789abcdef0123456789abcdef01234567"'; do
  export MOCK_EMBEDDED_REQUIREMENT=$embedded_dr
  if "$root/scripts/codesign-macos.sh" "$binary" >/dev/null 2>&1; then
    echo "codesign test: official release accepted a noncanonical embedded designated requirement" >&2
    exit 1
  fi
  assert_unchanged
  reset_binary
done

reset_binary
export MOCK_NOTARIZATION_REQUIREMENT_REJECT=1
if "$root/scripts/codesign-macos.sh" "$binary" >/dev/null 2>&1; then
  echo "codesign test: official release accepted a missing codesign notarization ticket" >&2
  exit 1
fi
assert_unchanged

reset_binary
"$root/scripts/codesign-macos.sh" "$binary"
[[ "$(shasum -a 256 "$binary" | awk '{print $1}')" != "$original_hash" ]] || {
  echo "codesign test: successful official release did not install the signed candidate" >&2
  exit 1
}
grep -Fq -- '--timestamp --options runtime --identifier com.steipete.gogcli.gog' "$log"
grep -Fq -- '--keychain /tmp/foundation-release.keychain-db --sign Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)' "$log"
grep -Fq -- 'notarytool submit' "$log"
grep -Fq -- '--keychain-profile test-profile --no-s3-acceleration --wait --output-format json' "$log"
grep -Fq -- '--verify --strict -R=identifier "com.steipete.gogcli.gog"' "$log"
grep -Fq -- 'codesign -d -r-' "$log"
grep -Fq -- '--verify --strict --check-notarization -R=notarized' "$log"
grep -Fq "codesign --verify --strict --check-notarization -R='notarized'" "$root/scripts/codesign-macos.sh"
# shellcheck disable=SC2016 # Literal source-text contract assertion.
grep -Fq -- '--keychain "$CODESIGN_KEYCHAIN"' "$root/scripts/codesign-macos.sh"
if find "$tmp/dist" -maxdepth 1 -name '.gog-notary.*' | grep -q .; then
  echo "codesign test: ephemeral notarization files were not removed" >&2
  exit 1
fi

echo "codesign/notarization tests passed"
