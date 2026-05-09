#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
if [[ -z "$version" ]]; then
  echo "usage: scripts/verify-release.sh X.Y.Z" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

changelog="CHANGELOG.md"
if ! rg -q "^## ${version} - " "$changelog"; then
  echo "missing changelog section for $version" >&2
  exit 2
fi
if rg -q "^## ${version} - Unreleased" "$changelog"; then
  echo "changelog section still Unreleased for $version" >&2
  exit 2
fi

notes_file="$(mktemp -t gogcli-release-notes)"
awk -v ver="$version" '
  $0 ~ "^## "ver" " {print "## "ver; in_section=1; next}
  in_section && /^## / {exit}
  in_section {print}
' "$changelog" | sed '/^$/d' > "$notes_file"

if [[ ! -s "$notes_file" ]]; then
  echo "release notes empty for $version" >&2
  exit 2
fi

release_body="$(gh release view "v$version" --json body -q .body)"
if [[ -z "$release_body" ]]; then
  echo "GitHub release notes empty for v$version" >&2
  exit 2
fi

assets_count="$(gh release view "v$version" --json assets -q '.assets | length')"
if [[ "$assets_count" -eq 0 ]]; then
  echo "no GitHub release assets for v$version" >&2
  exit 2
fi

release_run_id="$(gh api repos/steipete/gogcli/actions/runs --jq ".workflow_runs[] | select(.name==\"release\") | select(.head_branch==\"v$version\") | select(.conclusion==\"success\") | .id" | head -n1)"
if [[ -z "$release_run_id" ]]; then
  echo "release workflow not green for v$version" >&2
  exit 2
fi

ci_ok="$(gh api repos/steipete/gogcli/actions/runs --jq '.workflow_runs[] | select(.name=="ci") | select(.head_branch=="main") | .conclusion // ""' | head -n1)"
if [[ "$ci_ok" != "success" ]]; then
  echo "CI not green for main" >&2
  exit 2
fi

make ci

formula_path="../homebrew-tap/Formula/gogcli.rb"
if [[ ! -f "$formula_path" ]]; then
  echo "missing formula at $formula_path" >&2
  exit 2
fi

formula_version="$(awk -F '\"' '/^[[:space:]]*version /{print $2; exit}' "$formula_path" | xargs)"
if [[ "$formula_version" != "$version" ]]; then
  echo "formula version mismatch: $formula_version" >&2
  exit 2
fi

tmp_assets_dir="$(mktemp -d -t gogcli-release-assets)"
gh release download "v$version" -p checksums.txt -D "$tmp_assets_dir" >/dev/null
checksums_file="$tmp_assets_dir/checksums.txt"

sha_for_asset() {
  local name="$1"
  awk -v n="$name" '$2==n {print $1}' "$checksums_file"
}

formula_sha_for_url() {
  local url_substr="$1"
  awk -v s="$url_substr" '
    index($0, s) {found=1; next}
    found && $1=="sha256" {gsub(/"/, "", $2); print $2; exit}
  ' "$formula_path"
}

darwin_amd64_expected="$(sha_for_asset "gogcli_${version}_darwin_amd64.tar.gz")"
darwin_arm64_expected="$(sha_for_asset "gogcli_${version}_darwin_arm64.tar.gz")"
linux_amd64_expected="$(sha_for_asset "gogcli_${version}_linux_amd64.tar.gz")"
linux_arm64_expected="$(sha_for_asset "gogcli_${version}_linux_arm64.tar.gz")"

darwin_amd64_formula="$(formula_sha_for_url "gogcli_#{version}_darwin_amd64.tar.gz")"
darwin_arm64_formula="$(formula_sha_for_url "gogcli_#{version}_darwin_arm64.tar.gz")"
linux_amd64_formula="$(formula_sha_for_url "gogcli_#{version}_linux_amd64.tar.gz")"
linux_arm64_formula="$(formula_sha_for_url "gogcli_#{version}_linux_arm64.tar.gz")"

verify_darwin_asset_signature() {
  local asset="$1"
  if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "skipping macOS signature verification for $asset (not running on Darwin)" >&2
    return 0
  fi

  local work
  work="$(mktemp -d -t gogcli-signature-check)"
  tar -xzf "$tmp_assets_dir/$asset" -C "$work"

  local bin="$work/gog"
  if [[ ! -x "$bin" ]]; then
    echo "missing gog binary in $asset" >&2
    rm -rf "$work"
    exit 2
  fi

  codesign --verify --deep --strict --verbose=2 "$bin"

  local details
  details="$(codesign -dv --verbose=4 "$bin" 2>&1)"
  if grep -q "Signature=adhoc" <<<"$details"; then
    echo "darwin asset is ad-hoc signed: $asset" >&2
    rm -rf "$work"
    exit 2
  fi
  if grep -q "TeamIdentifier=not set" <<<"$details"; then
    echo "darwin asset has no TeamIdentifier: $asset" >&2
    rm -rf "$work"
    exit 2
  fi

  rm -rf "$work"
}

if [[ "$darwin_amd64_formula" != "$darwin_amd64_expected" ]]; then
  echo "formula sha mismatch (darwin_amd64): $darwin_amd64_formula (expected $darwin_amd64_expected)" >&2
  exit 2
fi
if [[ "$darwin_arm64_formula" != "$darwin_arm64_expected" ]]; then
  echo "formula sha mismatch (darwin_arm64): $darwin_arm64_formula (expected $darwin_arm64_expected)" >&2
  exit 2
fi
if [[ "$linux_amd64_formula" != "$linux_amd64_expected" ]]; then
  echo "formula sha mismatch (linux_amd64): $linux_amd64_formula (expected $linux_amd64_expected)" >&2
  exit 2
fi
if [[ "$linux_arm64_formula" != "$linux_arm64_expected" ]]; then
  echo "formula sha mismatch (linux_arm64): $linux_arm64_formula (expected $linux_arm64_expected)" >&2
  exit 2
fi

gh release download "v$version" \
  -p "gogcli_${version}_darwin_amd64.tar.gz" \
  -p "gogcli_${version}_darwin_arm64.tar.gz" \
  -D "$tmp_assets_dir" >/dev/null

verify_darwin_asset_signature "gogcli_${version}_darwin_amd64.tar.gz"
verify_darwin_asset_signature "gogcli_${version}_darwin_arm64.tar.gz"

brew update >/dev/null
brew upgrade gogcli || brew install steipete/tap/gogcli
brew test steipete/tap/gogcli
gog --version

rm -rf "$tmp_assets_dir"
rm -f "$notes_file"

echo "Release v$version verified (CI, GitHub release notes/assets, Homebrew install/test)."
