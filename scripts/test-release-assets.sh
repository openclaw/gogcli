#!/usr/bin/env bash
# shellcheck disable=SC2016 # Literal source-text contract assertions.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p \
  "$tmp/bin" \
  "$tmp/assets" \
  "$tmp/api-assets" \
  "$tmp/stage-darwin-arm64" \
  "$tmp/stage-darwin-amd64" \
  "$tmp/stage-linux-arm64" \
  "$tmp/stage-linux-amd64" \
  "$tmp/stage-windows-arm64" \
  "$tmp/stage-windows-amd64"

fail() {
  echo "release asset test failed: $*" >&2
  exit 1
}

workflow="$root/.github/workflows/release-assets.yml"
release_check_workflow="$root/.github/workflows/release.yml"
grep -Fq "github.ref == format('refs/heads/{0}', github.event.repository.default_branch)" "$workflow" || fail "missing default-branch ref guard"
grep -Fq "endsWith(github.workflow_ref, format('@refs/heads/{0}', github.event.repository.default_branch))" "$workflow" || fail "missing default-branch workflow_ref guard"
grep -Fq "github.event_name == 'workflow_dispatch'" "$workflow" || fail "verifier is not dispatch-only"
if grep -Eq '^[[:space:]]+release:|github\.event\.release|release-event' "$workflow"; then
  fail "verifier retains an untrusted automatic release-event path"
fi
grep -Fq "TRUSTED_SHA: \${{ github.workflow_sha }}" "$workflow" || fail "verifier is not pinned to trusted workflow code"
grep -Fq "git -C trusted fetch --depth=1 --no-tags origin \"refs/heads/\$DEFAULT_BRANCH\"" "$workflow" || fail "verifier does not fetch the protected default branch"
grep -Fq 'test "$GITHUB_REPOSITORY" = openclaw/gogcli' "$workflow" || fail "workflow does not pin its repository identity"
grep -Fq "env -i PATH=\"\$go_dir:/usr/bin:/bin:/usr/sbin:/sbin\"" "$workflow" || fail "candidate verification does not use a minimal environment"
[[ "$(grep -Fc "GH_TOKEN: \${{ github.token }}" "$workflow")" == 1 ]] || fail "workflow token is not scoped to one step"
[[ "$(grep -Ec 'GH_TOKEN|GITHUB_TOKEN' "$workflow")" == 1 ]] || fail "workflow token is visible outside the download step"
grep -Fq 'contents: write' "$workflow" || fail "draft verifier lacks push-level draft visibility"
grep -Fq "assets at \${{ github.workflow_sha }}" "$workflow" || fail "run title is not bound to trusted verifier code"
grep -Fq "for \${{ inputs.tag_commit }}" "$workflow" || fail "run title is not bound to the exact tag commit"
grep -Fq "object \${{ inputs.tag_object }}" "$workflow" || fail "run title is not bound to the annotated tag object"
grep -Fq 'GOG_RELEASE_PROOF tag=%s object=%s commit=%s workflow=%s state=%s' "$workflow" || fail "workflow does not emit an exact signed-tag proof marker"
grep -Fq 'trusted/scripts/verify-release-tag.sh' "$workflow" || fail "protected verifier does not enforce the repository-pinned tag signer"
grep -Fq 'Git graft files are forbidden' "$root/scripts/verify-release-tag.sh" || fail "protected tag verifier does not reject graft ancestry overrides"
grep -Fq "expected_tag_object=\$(<trusted-tag-object.txt)" "$workflow" || fail "REST download is not bound to the anonymously verified Git tag object"
grep -Fq "expected_revision=\$(<artifacts/tag-commit.txt)" "$workflow" || fail "workflow does not bind build info to the signed tag commit"
grep -Fq 'trusted/scripts/rebuild-release-assets.sh' "$workflow" || fail "workflow does not reproducibly rebuild cross-platform assets"
grep -Fq 'trusted/scripts/freeze-release-inventory.sh' "$workflow" || fail "workflow does not freeze and recheck release bytes"
grep -Fq 'test -z "$(git -C trusted status --porcelain --untracked-files=all)"' "$workflow" || fail "workflow does not require a pristine verifier checkout before execution"
grep -Fq 'git -C trusted hash-object "$helper"' "$workflow" || fail "workflow does not rehash protected verifier helpers"
[[ "$(grep -F '      - name:' "$workflow" | tail -1)" == '      - name: Execute verified candidate last' ]] || fail "candidate execution is not the final workflow step"
static_line=$(grep -nF 'trusted/scripts/verify-release-assets.sh' "$workflow" | tail -1 | cut -d: -f1)
rebuild_line=$(grep -nF 'trusted/scripts/rebuild-release-assets.sh' "$workflow" | tail -1 | cut -d: -f1)
execute_line=$(grep -nF '"$version" execute' "$workflow" | tail -1 | cut -d: -f1)
[[ "$static_line" -lt "$rebuild_line" && "$rebuild_line" -lt "$execute_line" ]] || fail "candidate can execute before static rebuild proof completes"
grep -Fq 'efb87ff28af9a188d0536ef5d42e63dd52ba8263cd7344a993cc48dd11dedb6a' "$workflow" || fail "arm64 Go toolchain is not checksum-pinned"
grep -Fq '6231d8d3b8f5552ec6cbf6d685bdd5482e1e703214b120e89b3bf0d7bf1ef725' "$workflow" || fail "Intel Go toolchain is not checksum-pinned"
grep -Fq 'Accept: application/octet-stream' "$root/scripts/download-release-assets.sh" || fail "asset downloads do not require authenticated octet-stream responses"
grep -Fq "X-GitHub-Api-Version: 2026-03-10" "$root/scripts/release-local" || fail "verifier dispatch does not pin the response-bearing API version"
grep -Fq '.workflow_run_id' "$root/scripts/release-local" || fail "verifier dispatch does not consume the returned exact run ID"
grep -Fq 'validate-verifier-dispatch.sh' "$root/scripts/release-local" || fail "verifier dispatch response is not independently validated"
dispatch_function=$(sed -n '/^dispatch_verifier()/,/^}/p' "$root/scripts/release-local")
dispatch_poll=$(sed -n '/for _ in {1..15}/,/done/p' <<<"$dispatch_function")
grep -Fq 'checked_run=$("$root/scripts/validate-verifier-dispatch.sh"' <<<"$dispatch_poll" || \
  fail "verifier dispatch does not retry exact-record validation during GitHub materialization"
grep -Fq 'run_id=$checked_run' <<<"$dispatch_poll" || \
  fail "verifier dispatch does not retain the validated returned run ID"
validator_line=$(grep -nF 'checked_run=$("$root/scripts/validate-verifier-dispatch.sh"' <<<"$dispatch_poll" | cut -d: -f1)
break_line=$(grep -nF 'break' <<<"$dispatch_poll" | cut -d: -f1)
[[ "$validator_line" -lt "$break_line" ]] || \
  fail "verifier dispatch stops polling before the returned record validates"
[[ "$(grep -Fc 'validate-verifier-dispatch.sh' <<<"$dispatch_function")" == 1 ]] || \
  fail "verifier dispatch validates outside the exact returned-record polling loop"
for github_script in \
  "$root/scripts/release-local" \
  "$root/scripts/check-release-verifier.sh" \
  "$root/scripts/download-release-assets.sh"; do
  api_count=$(grep -Ec '\bgh api\b' "$github_script" || true)
  pinned_api_count=$(grep -Fc 'command gh api --hostname github.com' "$github_script" || true)
  [[ "$api_count" == "$pinned_api_count" ]] || fail "relative GitHub API host remains in ${github_script##*/}"
done
[[ "$(grep -Ec '\bgh auth token\b' "$root/scripts/release-local")" == 1 &&
  "$(grep -Fc 'command gh auth token --hostname github.com' "$root/scripts/release-local")" == 1 ]] || \
  fail "GitHub token lookup is not pinned to github.com"
grep -Fq -- '--repo "github.com/$repository"' "$root/scripts/release-local" || \
  fail "GogCLI verifier run watch is not host-qualified"
[[ "$(grep -Fc -- '--repo github.com/openclaw/homebrew-tap' "$root/scripts/release-local")" == 2 ]] || \
  fail "Homebrew workflow operations are not host-qualified"
if grep -Fq 'gh workflow run release-assets.yml' "$root/scripts/release-local"; then
  fail "verifier dispatch uses the response-less CLI shortcut"
fi
grep -Fq '      - "none*"' "$root/.goreleaser.yaml" || fail "GoReleaser archives are not binary-only"
[[ "$(grep -Fc 'mod_timestamp: "{{ .CommitTimestamp }}"' "$root/.goreleaser.yaml")" == 2 ]] || \
  fail "GoReleaser builds do not pin both binary modification timestamps"
grep -A8 '^archives:' "$root/.goreleaser.yaml" | \
  grep -Fq 'mtime: "{{ .CommitDate }}"' || \
  fail "GoReleaser archives do not pin member modification timestamps"
grep -Fxq '.mac-release.env' "$root/.gitignore" || fail "runtime release manifest is not ignored"
grep -Fxq '.release-state/' "$root/.gitignore" || fail "non-secret release recovery state is not ignored"
if git -C "$root" ls-files --error-unmatch .mac-release.env >/dev/null 2>&1; then
  fail "runtime release manifest is tracked"
fi
if git -C "$root" ls-files '.release-state/**' | grep -q .; then
  fail "release recovery state is tracked"
fi
if grep -R -q 'NOTARYTOOL_KEYCHAIN_PROFILE' "$root/.github/workflows"; then
  fail "CI must not receive notarization profile routing"
fi
if git -C "$root" grep -n -E \
  'MACOS_SIGNING_CERT_BASE64|MACOS_SIGNING_CERT_PASSWORD|MACOS_CODESIGN_IDENTITY|HOMEBREW_TAP_TOKEN|security[[:space:]]+import|base64[[:space:]]+--decode|\.p12' \
  -- '.github/workflows' 'docs'; then
  fail "tracked workflows or docs retain the retired certificate import release path"
fi
[[ ! -e "$root/.github/workflows/post-release-bump.yml" ]] || \
  fail "automatic post-tag main mutation competes with serialized release closeout"
grep -Fq 'permissions:' "$release_check_workflow" || fail "release check has no explicit permissions"
grep -Fq '  contents: read' "$release_check_workflow" || fail "release check is not read-only"
if grep -Eq 'release --clean|gh release|git push|secrets\.' "$release_check_workflow"; then
  fail "tag workflow can still publish or receive release credentials"
fi
if grep -Eq 'gh release|git push|gh workflow run' "$root/scripts/release.sh" "$root/scripts/verify-release.sh"; then
  fail "legacy release entrypoints retain a competing publisher"
fi
if grep -Eq 'actions/checkout|upload-artifact|gh (release|workflow)|gh api --method|git push|curl .*-[Xx]' "$workflow"; then
  fail "verification workflow contains a credentialed checkout or write operation"
fi

version=0.3.4
tag=v0.3.4
tag_object=1111111111111111111111111111111111111111
tag_commit=2222222222222222222222222222222222222222
cat > "$tmp/stage-darwin-arm64/gog" <<'EOF'
#!/usr/bin/env bash
[[ "${1:-}" == --version ]] || exit 2
for name in GH_TOKEN GITHUB_TOKEN ACTIONS_RUNTIME_TOKEN NOTARYTOOL_KEYCHAIN_PROFILE; do
  [[ -z "${!name+x}" ]] || exit 3
done
echo 'v0.3.4 (22222222 2026-07-09T12:00:00Z)'
EOF
cp "$tmp/stage-darwin-arm64/gog" "$tmp/stage-darwin-amd64/gog"
printf 'linux-amd64\n' > "$tmp/stage-linux-amd64/gog"
printf 'linux-arm64\n' > "$tmp/stage-linux-arm64/gog"
printf 'windows-amd64\n' > "$tmp/stage-windows-amd64/gog.exe"
printf 'windows-arm64\n' > "$tmp/stage-windows-arm64/gog.exe"
chmod 0755 "$tmp/stage-darwin-arm64/gog" "$tmp/stage-darwin-amd64/gog"

(
  cd "$tmp/assets"
  tar -czf "gogcli_${version}_darwin_arm64.tar.gz" -C "$tmp/stage-darwin-arm64" gog
  tar -czf "gogcli_${version}_darwin_amd64.tar.gz" -C "$tmp/stage-darwin-amd64" gog
  tar -czf "gogcli_${version}_linux_arm64.tar.gz" -C "$tmp/stage-linux-arm64" gog
  tar -czf "gogcli_${version}_linux_amd64.tar.gz" -C "$tmp/stage-linux-amd64" gog
  zip -q -j "gogcli_${version}_windows_arm64.zip" "$tmp/stage-windows-arm64/gog.exe"
  zip -q -j "gogcli_${version}_windows_amd64.zip" "$tmp/stage-windows-amd64/gog.exe"
  shasum -a 256 \
    "gogcli_${version}_darwin_amd64.tar.gz" \
    "gogcli_${version}_darwin_arm64.tar.gz" \
    "gogcli_${version}_linux_amd64.tar.gz" \
    "gogcli_${version}_linux_arm64.tar.gz" \
    "gogcli_${version}_windows_amd64.zip" \
    "gogcli_${version}_windows_arm64.zip" > checksums.txt
  printf '%s\n' "$tag_commit" > tag-commit.txt
)

names=(
  checksums.txt
  "gogcli_${version}_darwin_amd64.tar.gz"
  "gogcli_${version}_darwin_arm64.tar.gz"
  "gogcli_${version}_linux_amd64.tar.gz"
  "gogcli_${version}_linux_arm64.tar.gz"
  "gogcli_${version}_windows_amd64.zip"
  "gogcli_${version}_windows_arm64.zip"
)
for index in 0 1 2 3 4 5 6; do
  cp "$tmp/assets/${names[$index]}" "$tmp/api-assets/$((index + 1))"
done

assets_json="$tmp/assets.json"
cat > "$assets_json" <<EOF
[
  {"name":"${names[0]}","url":"https://api.github.com/repos/openclaw/gogcli/releases/assets/1","updated_at":"2026-07-09T12:00:00Z"},
  {"name":"${names[1]}","url":"https://api.github.com/repos/openclaw/gogcli/releases/assets/2","updated_at":"2026-07-09T12:00:00Z"},
  {"name":"${names[2]}","url":"https://api.github.com/repos/openclaw/gogcli/releases/assets/3","updated_at":"2026-07-09T12:00:00Z"},
  {"name":"${names[3]}","url":"https://api.github.com/repos/openclaw/gogcli/releases/assets/4","updated_at":"2026-07-09T12:00:00Z"},
  {"name":"${names[4]}","url":"https://api.github.com/repos/openclaw/gogcli/releases/assets/5","updated_at":"2026-07-09T12:00:00Z"},
  {"name":"${names[5]}","url":"https://api.github.com/repos/openclaw/gogcli/releases/assets/6","updated_at":"2026-07-09T12:00:00Z"},
  {"name":"${names[6]}","url":"https://api.github.com/repos/openclaw/gogcli/releases/assets/7","updated_at":"2026-07-09T12:00:00Z"}
]
EOF
jq -n --argjson assets "$(<"$assets_json")" '[{"id":42,"tag_name":"v0.3.4","draft":true,"assets":$assets}]' > "$tmp/releases.json"

cat > "$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == api ]] || exit 2
shift
[[ "${1:-} ${2:-}" == '--hostname github.com' ]] || exit 90
shift 2
if [[ "${1:-}" == --paginate ]]; then shift; fi
endpoint=${1:-}
case "$endpoint" in
  repos/openclaw/gogcli/git/ref/tags/v0.3.4)
    jq -n --arg object "${MOCK_TAG_OBJECT:-1111111111111111111111111111111111111111}" \
      '{ref:"refs/tags/v0.3.4", object:{type:"tag", sha:$object}}'
    ;;
  repos/openclaw/gogcli/git/tags/*)
    [[ "${endpoint##*/}" == "${MOCK_TAG_OBJECT:-1111111111111111111111111111111111111111}" ]] || exit 2
    verified=${MOCK_TAG_VERIFIED:-true}
    reason=valid
    [[ "$verified" == true ]] || reason=unknown_key
    jq -n \
      --arg tag v0.3.4 \
      --arg commit 2222222222222222222222222222222222222222 \
      --argjson verified "$verified" \
      --arg reason "$reason" \
      '{tag:$tag, object:{type:"commit", sha:$commit}, verification:{verified:$verified, reason:$reason}}'
    ;;
  repos/openclaw/gogcli/releases\?per_page=100)
    cat "$MOCK_GH_RELEASES_JSON"
    ;;
  repos/openclaw/gogcli/releases/42/assets\?per_page=100)
    cat "$MOCK_GH_ASSETS_JSON"
    ;;
  https://api.github.com/repos/openclaw/gogcli/releases/assets/*)
    cat "$MOCK_GH_ASSET_DIR/${endpoint##*/}"
    ;;
  repos/openclaw/gogcli)
    printf '%s\n' '{"default_branch":"main"}'
    ;;
  repos/openclaw/gogcli/commits/main)
    printf '%s\n' "${MOCK_DEFAULT_SHA:-0123456789abcdef0123456789abcdef01234567}"
    ;;
  repos/openclaw/gogcli/actions/workflows/release-assets.yml/runs*)
    cat "$MOCK_GH_RUNS_JSON"
    ;;
  repos/openclaw/gogcli/actions/runs/42/jobs*)
    cat "$MOCK_GH_JOBS_JSON"
    ;;
  repos/openclaw/gogcli/actions/runs/42/logs)
    cat "$MOCK_GH_LOGS_ZIP"
    ;;
  *) exit 2 ;;
esac
EOF

cat > "$tmp/bin/uname" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' Darwin
EOF

cat > "$tmp/bin/codesign" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'codesign %s\n' "$*" >> "$RELEASE_ASSET_TEST_LOG"
if [[ " $* " == *' --display '* ]]; then
  {
    echo 'Identifier=com.steipete.gogcli.gog'
    echo 'CodeDirectory v=20500 size=100 flags=0x10000(runtime) hashes=1+0 location=embedded'
    printf 'Authority=%s\n' "${MOCK_AUTHORITY:-Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)}"
    echo 'Authority=Developer ID Certification Authority'
    echo 'Authority=Apple Root CA'
    echo 'Timestamp=09.07.2026 at 12:07:29'
    echo 'TeamIdentifier=FWJYW4S8P8'
    echo 'Runtime Version=12.0.0'
  } >&2
fi
if [[ " $* " == *' -d '* && " $* " == *' -r- '* ]]; then
  echo 'Executable=/tmp/gog' >&2
  printf 'designated => %s\n' "${MOCK_EMBEDDED_REQUIREMENT:-identifier \"com.steipete.gogcli.gog\" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = FWJYW4S8P8}" >&2
  exit 0
fi
if [[ " $* " == *' --check-notarization '* && "${MOCK_NOTARIZATION_REQUIREMENT_REJECT:-0}" == 1 ]]; then
  exit 3
fi
EOF

cat > "$tmp/bin/lipo" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${MOCK_LIPO_ARCH:-arm64}"
EOF

cat > "$tmp/bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
test_root=$(cd "$(dirname "$0")/.." && pwd)
if [[ "${1:-}" == version && "${2:-}" != -m ]]; then
  printf '%s\n' 'go version go1.26.5 darwin/arm64'
  exit 0
fi
if [[ "${1:-}" == build ]]; then
  [[ " $* " == *'internal/cmd.commit=22222222'* ]] || exit 2
  output=
  previous=
  for arg in "$@"; do
    if [[ "$previous" == -o ]]; then output=$arg; fi
    previous=$arg
  done
  [[ -n "$output" && -n "${GOOS:-}" && -n "${GOARCH:-}" ]] || exit 2
  member=gog
  [[ "$GOOS" != windows ]] || member=gog.exe
  cp "$test_root/stage-$GOOS-$GOARCH/$member" "$output"
  if [[ -f "$test_root/rebuild-mismatch-$GOOS-$GOARCH" ]]; then
    printf 'mismatch\n' >> "$output"
  fi
  exit 0
fi
[[ "${1:-} ${2:-}" == 'version -m' ]] || exit 2
binary=${3:-gog}
printf 'go version -m %s\n' "$binary" >> "$RELEASE_ASSET_TEST_LOG"
case "$binary" in
  */darwin-amd64/*) goos=darwin; goarch=amd64 ;;
  */darwin-arm64/*) goos=darwin; goarch=arm64 ;;
  */linux-amd64/*) goos=linux; goarch=amd64 ;;
  */linux-arm64/*) goos=linux; goarch=arm64 ;;
  */windows-amd64/*) goos=windows; goarch=amd64 ;;
  */windows-arm64/*) goos=windows; goarch=arm64 ;;
  *) exit 2 ;;
esac
revision=${MOCK_VCS_REVISION:-2222222222222222222222222222222222222222}
if [[ "${MOCK_BAD_PLATFORM:-}" == "$goos-$goarch" ]]; then
  revision=3333333333333333333333333333333333333333
fi
printf '%s: %s\n' "$binary" "${MOCK_GO_TOOLCHAIN:-go1.26.5}"
printf '\tpath\t%s\n' "${MOCK_MAIN_PACKAGE:-github.com/steipete/gogcli/cmd/gog}"
printf '\tbuild\tvcs=%s\n' "${MOCK_VCS:-git}"
printf '\tbuild\tGOARCH=%s\n' "${MOCK_GOARCH:-$goarch}"
printf '\tbuild\tGOOS=%s\n' "${MOCK_GOOS:-$goos}"
printf '\tbuild\tvcs.revision=%s\n' "$revision"
printf '\tbuild\tvcs.modified=%s\n' "${MOCK_VCS_MODIFIED:-false}"
EOF

cat > "$tmp/bin/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
repo=
if [[ "${1:-}" == -C ]]; then
  repo=$2
  shift 2
fi
seen_format=false
seen_allowed_signers=false
seen_ssh_program=false
while [[ "${1:-}" == -c ]]; do
  case "${2:-}" in
    gpg.format=ssh) seen_format=true ;;
    gpg.ssh.program=/usr/bin/ssh-keygen) seen_ssh_program=true ;;
    gpg.ssh.allowedSignersFile="$RELEASE_ASSET_TEST_ROOT/.github/release-allowed-signers") seen_allowed_signers=true ;;
  esac
  shift 2
done
command=${1:-}
shift || true
case "$command" in
  show)
    case "$*" in
      *--format=%h*) printf '%s\n' 2222222 ;;
      *--format=%ct*) printf '%s\n' 1783598400 ;;
      *) cat "$RELEASE_ASSET_TEST_ROOT/.github/release-allowed-signers" ;;
    esac
    ;;
  fetch) ;;
  cat-file)
    if [[ "${1:-} ${2:-}" == '-t FETCH_HEAD' ]]; then
      printf '%s\n' tag
    else
      printf '%s\n' 'object 2222222222222222222222222222222222222222' 'type commit' 'tag v0.3.4' '' 'mock tag'
    fi
    ;;
  rev-parse)
    if [[ "${1:-} ${2:-}" == '--git-path info/grafts' ]]; then
      printf '%s\n' "$RELEASE_ASSET_TEST_GRAFT_PATH"
      exit 0
    fi
    if [[ -n "$repo" ]]; then
      case "${1:-}" in
        HEAD|'refs/tags/v0.3.4^{}') printf '%s\n' 2222222222222222222222222222222222222222 ;;
        refs/tags/v0.3.4) printf '%s\n' 1111111111111111111111111111111111111111 ;;
        *) exit 1 ;;
      esac
      exit 0
    fi
    case "${1:-}" in
      FETCH_HEAD) printf '%s\n' 1111111111111111111111111111111111111111 ;;
      'FETCH_HEAD^{}') printf '%s\n' 2222222222222222222222222222222222222222 ;;
      *) exit 1 ;;
    esac
    ;;
  status)
    [[ -n "$repo" ]] || exit 1
    ;;
  verify-tag)
    [[ "$seen_format" == true && "$seen_ssh_program" == true &&
      "$seen_allowed_signers" == true ]] || exit 1
    if [[ "${MOCK_PINNED_SIGNER:-expected}" == attacker ]]; then
      printf '%s\n' 'Good "git" signature for attacker@example.com with ED25519 key SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' >&2
    else
      printf '%s\n' 'Good "git" signature for steipete@gmail.com with ED25519 key SHA256:WmI9lVtd7F2c5XyRHbZVO3yYYJzwsSNzcZQMPT147HI' >&2 # gitleaks:allow -- public release-key fingerprint
    fi
    ;;
  ls-remote)
    printf '%s\t%s\n' 1111111111111111111111111111111111111111 refs/tags/v0.3.4
    printf '%s\t%s\n' 2222222222222222222222222222222222222222 'refs/tags/v0.3.4^{}'
    ;;
  *) exit 1 ;;
esac
EOF

chmod +x "$tmp/bin/"*
export PATH="$tmp/bin:$PATH"
export MOCK_GH_RELEASES_JSON="$tmp/releases.json"
export MOCK_GH_ASSETS_JSON="$assets_json"
export MOCK_GH_ASSET_DIR="$tmp/api-assets"
export RELEASE_ASSET_TEST_LOG="$tmp/release-assets.log"
export RELEASE_ASSET_TEST_ROOT="$root"
export RELEASE_ASSET_TEST_GRAFT_PATH="$tmp/active-grafts"
export GH_HOST=attacker.example
export GH_CONFIG_DIR="$tmp/hostile-gh-config"
mkdir -p "$GH_CONFIG_DIR"
: > "$RELEASE_ASSET_TEST_LOG"

download="$tmp/download"
GITHUB_REPOSITORY=openclaw/gogcli GH_TOKEN=test \
  "$root/scripts/download-release-assets.sh" "$tag" arm64 true "$tag_commit" "$tag_object" "$download"
if GITHUB_REPOSITORY=attacker/gogcli GH_TOKEN=test \
  "$root/scripts/download-release-assets.sh" "$tag" arm64 true \
  "$tag_commit" "$tag_object" "$tmp/attacker-repository" >/dev/null 2>&1; then
  fail "downloader accepted an attacker-controlled repository override"
fi
[[ ! -e "$tmp/attacker-repository" ]] || fail "attacker repository override created the destination"
for name in "${names[@]}"; do
  cmp "$tmp/assets/$name" "$download/$name"
done
[[ "$(<"$download/tag-commit.txt")" == "$tag_commit" ]] || fail "downloader did not record the signed tag commit"
[[ "$(find "$download" -type f | wc -l | tr -d ' ')" == 8 ]] || fail "downloader did not fetch the exact full inventory"

[[ "$("$root/scripts/verify-release-tag.sh" "$tag" "$tag_commit")" == "$tag_object $tag_commit" ]] || \
  fail "protected tag verifier rejected the repository-pinned signer"
if MOCK_PINNED_SIGNER=attacker "$root/scripts/verify-release-tag.sh" "$tag" "$tag_commit" >/dev/null 2>&1; then
  fail "protected tag verifier accepted a different GitHub-verified signer"
fi
printf '%s %s\n' "$tag_commit" 3333333333333333333333333333333333333333 \
  > "$RELEASE_ASSET_TEST_GRAFT_PATH"
if "$root/scripts/verify-release-tag.sh" "$tag" "$tag_commit" >/dev/null 2>&1; then
  fail "protected tag verifier accepted an active Git graft"
fi
rm "$RELEASE_ASSET_TEST_GRAFT_PATH"

mkdir "$tmp/existing"
printf 'sentinel\n' > "$tmp/existing/keep"
if GITHUB_REPOSITORY=openclaw/gogcli GH_TOKEN=test \
  "$root/scripts/download-release-assets.sh" "$tag" arm64 true "$tag_commit" "$tag_object" "$tmp/existing" >/dev/null 2>&1; then
  fail "downloader replaced an existing destination"
fi
[[ "$(<"$tmp/existing/keep")" == sentinel ]] || fail "existing destination was mutated"

jq '. + [{"name":"unexpected","url":"https://api.github.com/repos/openclaw/gogcli/releases/assets/8"}]' \
  "$assets_json" > "$tmp/bad-assets.json"
if MOCK_GH_ASSETS_JSON="$tmp/bad-assets.json" GITHUB_REPOSITORY=openclaw/gogcli GH_TOKEN=test \
  "$root/scripts/download-release-assets.sh" "$tag" arm64 true "$tag_commit" "$tag_object" "$tmp/bad-inventory" >/dev/null 2>&1; then
  fail "downloader accepted the wrong release inventory"
fi
[[ ! -e "$tmp/bad-inventory" ]] || fail "bad inventory created the destination"

if GITHUB_REPOSITORY=openclaw/gogcli GH_TOKEN=test \
  "$root/scripts/download-release-assets.sh" "$tag" arm64 false "$tag_commit" "$tag_object" "$tmp/wrong-draft" >/dev/null 2>&1; then
  fail "draft release matched a published-release lookup"
fi
[[ ! -e "$tmp/wrong-draft" ]] || fail "wrong draft state created the destination"

if MOCK_TAG_VERIFIED=false GITHUB_REPOSITORY=openclaw/gogcli GH_TOKEN=test \
  "$root/scripts/download-release-assets.sh" "$tag" arm64 true "$tag_commit" "$tag_object" "$tmp/unverified-tag" >/dev/null 2>&1; then
  fail "downloader accepted an unverified tag object"
fi
[[ ! -e "$tmp/unverified-tag" ]] || fail "unverified tag created the destination"

if GITHUB_REPOSITORY=openclaw/gogcli GH_TOKEN=test \
  "$root/scripts/download-release-assets.sh" "$tag" arm64 true \
  3333333333333333333333333333333333333333 "$tag_object" "$tmp/wrong-tag-commit" >/dev/null 2>&1; then
  fail "downloader accepted a REST tag commit different from the run identity"
fi
[[ ! -e "$tmp/wrong-tag-commit" ]] || fail "wrong tag commit created the destination"

archive="$tmp/assets/gogcli_${version}_darwin_arm64.tar.gz"
checksums="$tmp/assets/checksums.txt"
archive_hash=$(shasum -a 256 "$archive" | awk '{print $1}')
checksums_hash=$(shasum -a 256 "$checksums" | awk '{print $1}')
inventory_hashes=$(cd "$tmp/assets" && shasum -a 256 "${names[@]}")
verified_candidate="$tmp/verified-candidate"
"$root/scripts/freeze-release-inventory.sh" "$tag" "$tmp/assets" > "$tmp/inventory-before.sha256"
unset GH_TOKEN GITHUB_TOKEN MOCK_AUTHORITY MOCK_EMBEDDED_REQUIREMENT MOCK_NOTARIZATION_REQUIREMENT_REJECT
unset MOCK_VCS MOCK_VCS_REVISION MOCK_VCS_MODIFIED
ACTIONS_RUNTIME_TOKEN=runtime-secret NOTARYTOOL_KEYCHAIN_PROFILE=notary-secret MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" \
  "$tag" "$archive" "$checksums" "$tag_commit" "$verified_candidate"
for target in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64; do
  grep -Fq "/$target/" "$RELEASE_ASSET_TEST_LOG" || fail "verifier skipped build info for $target"
done
[[ "$(shasum -a 256 "$archive" | awk '{print $1}')" == "$archive_hash" ]] || fail "verifier mutated the archive"
[[ "$(shasum -a 256 "$checksums" | awk '{print $1}')" == "$checksums_hash" ]] || fail "verifier mutated checksums.txt"
[[ "$(cd "$tmp/assets" && shasum -a 256 "${names[@]}")" == "$inventory_hashes" ]] || fail "verifier mutated the release inventory"

mkdir -p "$tmp/source/.git"
GOG_RELEASE_GO="$tmp/bin/go" \
  "$root/scripts/rebuild-release-assets.sh" \
  "$tag" "$tmp/source" "$tmp/assets" "$tag_commit" "$tag_object" >/dev/null
"$root/scripts/freeze-release-inventory.sh" "$tag" "$tmp/assets" > "$tmp/inventory-after.sha256"
cmp "$tmp/inventory-before.sha256" "$tmp/inventory-after.sha256"
"$root/scripts/verify-macos-binary.sh" \
  "$verified_candidate" arm64 "$version" execute

mkdir "$tmp/hostile-assets" "$tmp/hostile-stage"
cp "$tmp/assets"/* "$tmp/hostile-assets/"
cat > "$tmp/hostile-stage/gog" <<EOF
#!/usr/bin/env bash
rm -f '$tmp/rebuild-mismatch-linux-arm64'
touch '$tmp/hostile-executed'
printf '%s\\n' 'v0.3.4 (22222222 2026-07-09T12:00:00Z)'
EOF
chmod 0755 "$tmp/hostile-stage/gog"
tar -czf "$tmp/hostile-assets/gogcli_${version}_darwin_arm64.tar.gz" \
  -C "$tmp/hostile-stage" gog
hostile_hash=$(shasum -a 256 "$tmp/hostile-assets/gogcli_${version}_darwin_arm64.tar.gz" | awk '{print $1}')
awk -v name="gogcli_${version}_darwin_arm64.tar.gz" -v hash="$hostile_hash" \
  '$2 == name { $1 = hash } { print $1 "  " $2 }' \
  "$tmp/hostile-assets/checksums.txt" > "$tmp/hostile-assets/checksums.new"
mv "$tmp/hostile-assets/checksums.new" "$tmp/hostile-assets/checksums.txt"
touch "$tmp/rebuild-mismatch-linux-arm64"
MOCK_LIPO_ARCH=arm64 "$root/scripts/verify-release-assets.sh" \
  "$tag" "$tmp/hostile-assets/gogcli_${version}_darwin_arm64.tar.gz" \
  "$tmp/hostile-assets/checksums.txt" "$tag_commit" "$tmp/hostile-candidate"
[[ -f "$tmp/rebuild-mismatch-linux-arm64" && ! -e "$tmp/hostile-executed" ]] || \
  fail "static verification executed the hostile candidate"
if GOG_RELEASE_GO="$tmp/bin/go" \
  "$root/scripts/rebuild-release-assets.sh" \
  "$tag" "$tmp/source" "$tmp/hostile-assets" "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "hostile candidate influenced rebuild proof before execution"
fi
[[ -f "$tmp/rebuild-mismatch-linux-arm64" && ! -e "$tmp/hostile-executed" ]] || \
  fail "hostile candidate mutated verifier state before rebuild rejection"
rm "$tmp/rebuild-mismatch-linux-arm64"
touch "$tmp/rebuild-mismatch-linux-arm64"
if GOG_RELEASE_GO="$tmp/bin/go" \
  "$root/scripts/rebuild-release-assets.sh" \
  "$tag" "$tmp/source" "$tmp/assets" "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "reproducible verifier accepted different Linux bytes"
fi
rm "$tmp/rebuild-mismatch-linux-arm64"
if GOG_RELEASE_GO="$tmp/bin/go" \
  "$root/scripts/rebuild-release-assets.sh" \
  "$tag" "$tmp/source" "$tmp/assets" "$tag_commit" \
  3333333333333333333333333333333333333333 >/dev/null 2>&1; then
  fail "reproducible verifier accepted the wrong annotated tag object"
fi

if GH_TOKEN=test MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier executed an artifact with a GitHub token"
fi
if MOCK_AUTHORITY='Developer ID Application: Wrong Identity (WRONGTEAM1)' MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted the wrong signing authority"
fi
canonical_dr='identifier "com.steipete.gogcli.gog" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = FWJYW4S8P8'
for embedded_dr in \
  'identifier "org.example.wrong" and anchor apple generic' \
  "$canonical_dr and info[CFBundleVersion] = \"1\"" \
  'cdhash H"0123456789abcdef0123456789abcdef01234567"'; do
  if MOCK_EMBEDDED_REQUIREMENT="$embedded_dr" MOCK_LIPO_ARCH=arm64 \
    "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
    fail "verifier accepted a noncanonical embedded designated requirement"
  fi
done
if MOCK_NOTARIZATION_REQUIREMENT_REJECT=1 MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted a binary without the codesign notarization requirement"
fi
if MOCK_VCS_REVISION=3333333333333333333333333333333333333333 MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted build info from the wrong commit"
fi
if MOCK_VCS=hg MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted non-Git build provenance"
fi
if MOCK_VCS_MODIFIED=true MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted a modified Go build"
fi
if MOCK_MAIN_PACKAGE=github.com/example/not-gogcli/cmd/gog MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted same-commit build info for a different main package"
fi
if MOCK_GO_TOOLCHAIN=go1.26.4 MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted a binary built with the wrong Go toolchain"
fi
if MOCK_GOOS=linux MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" "$archive" "$checksums" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted a binary whose Go target did not match its archive"
fi

cp -R "$tmp/assets" "$tmp/extra-entry-assets"
printf 'unexpected\n' > "$tmp/README.md"
tar -czf "$tmp/extra-entry-assets/gogcli_${version}_darwin_arm64.tar.gz" \
  -C "$tmp/stage-darwin-arm64" gog -C "$tmp" README.md
extra_hash=$(shasum -a 256 "$tmp/extra-entry-assets/gogcli_${version}_darwin_arm64.tar.gz" | awk '{print $1}')
awk -v name="gogcli_${version}_darwin_arm64.tar.gz" -v hash="$extra_hash" \
  '$2 == name { $1 = hash } { print $1 "  " $2 }' \
  "$tmp/extra-entry-assets/checksums.txt" > "$tmp/extra-entry-assets/checksums.new"
mv "$tmp/extra-entry-assets/checksums.new" "$tmp/extra-entry-assets/checksums.txt"
if MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" \
  "$tmp/extra-entry-assets/gogcli_${version}_darwin_arm64.tar.gz" \
  "$tmp/extra-entry-assets/checksums.txt" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted an archive with GoReleaser's default extra files"
fi

cp -R "$tmp/assets" "$tmp/replaced-nonnative-assets"
printf 'replacement-linux-arm64\n' > "$tmp/stage-linux-arm64/gog"
tar -czf "$tmp/replaced-nonnative-assets/gogcli_${version}_linux_arm64.tar.gz" \
  -C "$tmp/stage-linux-arm64" gog
replacement_hash=$(shasum -a 256 "$tmp/replaced-nonnative-assets/gogcli_${version}_linux_arm64.tar.gz" | awk '{print $1}')
awk -v name="gogcli_${version}_linux_arm64.tar.gz" -v hash="$replacement_hash" \
  '$2 == name { $1 = hash } { print $1 "  " $2 }' \
  "$tmp/replaced-nonnative-assets/checksums.txt" > "$tmp/replaced-nonnative-assets/checksums.new"
mv "$tmp/replaced-nonnative-assets/checksums.new" "$tmp/replaced-nonnative-assets/checksums.txt"
if MOCK_BAD_PLATFORM=linux-arm64 MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" \
  "$tmp/replaced-nonnative-assets/gogcli_${version}_darwin_arm64.tar.gz" \
  "$tmp/replaced-nonnative-assets/checksums.txt" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier accepted a replaced non-native archive and matching mutable checksum"
fi

cp -R "$tmp/assets" "$tmp/tampered-assets"
printf 'replacement\n' >> "$tmp/tampered-assets/gogcli_${version}_linux_arm64.tar.gz"
if MOCK_LIPO_ARCH=arm64 \
  "$root/scripts/verify-release-assets.sh" "$tag" \
  "$tmp/tampered-assets/gogcli_${version}_darwin_arm64.tar.gz" \
  "$tmp/tampered-assets/checksums.txt" "$tag_commit" >/dev/null 2>&1; then
  fail "verifier ignored a checksum mismatch in a non-native platform archive"
fi
[[ "$(shasum -a 256 "$archive" | awk '{print $1}')" == "$archive_hash" ]] || fail "failed verification mutated the archive"
grep -Fq -- '--verify --strict --check-notarization -R=notarized' "$RELEASE_ASSET_TEST_LOG" || fail "extracted archive skipped the codesign notarization requirement"
grep -Fq -- 'codesign -d -r-' "$RELEASE_ASSET_TEST_LOG" || fail "extracted archive skipped embedded designated-requirement inspection"
grep -Fq 'verify-macos-binary.sh" "$binary" "$expected_arch" "$version" static' "$root/scripts/verify-release-assets.sh" || fail "archive verifier does not delegate static extracted-candidate verification"
grep -Fq "codesign --verify --strict --check-notarization -R='notarized'" "$root/scripts/verify-macos-binary.sh" || fail "archive verifier does not use the exact notarized requirement"

forbidden_policy_tools=$(printf '%s|%s|%s' 'sp''ctl' 'sys''policy' 'sta''pler')
if grep -Eiq "$forbidden_policy_tools" \
  "$root/scripts/codesign-macos.sh" \
  "$root/scripts/freeze-release-inventory.sh" \
  "$root/scripts/verify-macos-binary.sh" \
  "$root/scripts/verify-release-assets.sh" \
  "$root/scripts/rebuild-release-assets.sh" \
  "$root/scripts/release-local" \
  "$root/scripts/validate-release-record.sh" \
  "$root/scripts/test-codesign-macos.sh" \
  "$root/scripts/test-release-assets.sh" \
  "$root/scripts/test-release-local.sh" \
  "$root/.github/workflows/ci.yml" \
  "$root/.github/workflows/release.yml" \
  "$root/.github/workflows/release-assets.yml" \
  "$root/README.md" \
  "$root/docs/releasing.md"; then
  fail "raw CLI application-policy tooling must not be a producer or verifier gate"
fi

# Values observed from an API 2026-03-10 workflow-run record; URLs remain
# GogCLI-specific because the validator deliberately pins this repository.
live_release_run_id=29009699237
live_release_workflow_id=309911276
live_release_head_sha=10725cf8aa5daba9f8714a6774b2f7ddcaabc1e6
dispatch_title="Verify $tag draft assets at $live_release_head_sha for $tag_commit object $tag_object"
cat > "$tmp/dispatch-response.json" <<'EOF'
{"workflow_run_id":29009699237,"run_url":"https://api.github.com/repos/openclaw/gogcli/actions/runs/29009699237","html_url":"https://github.com/openclaw/gogcli/actions/runs/29009699237"}
EOF
printf '%s\n' '[42]' > "$tmp/preexisting-run-ids.json"
jq -n \
  --arg title "$dispatch_title" \
  --arg sha "$live_release_head_sha" \
  --argjson id "$live_release_run_id" \
  --argjson workflow_id "$live_release_workflow_id" \
  '{id:$id,workflow_id:$workflow_id,path:".github/workflows/release-assets.yml",event:"workflow_dispatch",display_title:$title,head_branch:"main",head_sha:$sha,created_at:"2026-07-09T12:01:00Z"}' \
  > "$tmp/dispatched-run.json"
[[ "$("$root/scripts/validate-verifier-dispatch.sh" \
  "$tmp/dispatch-response.json" "$tmp/preexisting-run-ids.json" \
  "$tmp/dispatched-run.json" "$live_release_workflow_id" "$dispatch_title" main \
  "$live_release_head_sha" 2026-07-09T12:00:00Z)" == "$live_release_run_id" ]] || \
  fail "exact live-shape dispatch response was not accepted"

printf '[42,%s]\n' "$live_release_run_id" > "$tmp/preexisting-returned-run-id.json"
if "$root/scripts/validate-verifier-dispatch.sh" \
  "$tmp/dispatch-response.json" "$tmp/preexisting-returned-run-id.json" \
  "$tmp/dispatched-run.json" "$live_release_workflow_id" "$dispatch_title" main \
  "$live_release_head_sha" \
  2026-07-09T12:00:00Z >/dev/null 2>&1; then
  fail "preexisting matching workflow run was accepted as this dispatch"
fi

jq '.id = 43' "$tmp/dispatched-run.json" > "$tmp/concurrent-dispatched-run.json"
if "$root/scripts/validate-verifier-dispatch.sh" \
  "$tmp/dispatch-response.json" "$tmp/preexisting-run-ids.json" \
  "$tmp/concurrent-dispatched-run.json" "$live_release_workflow_id" "$dispatch_title" main \
  "$live_release_head_sha" \
  2026-07-09T12:00:00Z >/dev/null 2>&1; then
  fail "concurrent matching dispatch substituted its run for the returned exact run ID"
fi

jq '.path += "@main"' "$tmp/dispatched-run.json" > "$tmp/suffixed-dispatched-run.json"
if "$root/scripts/validate-verifier-dispatch.sh" \
  "$tmp/dispatch-response.json" "$tmp/preexisting-run-ids.json" \
  "$tmp/suffixed-dispatched-run.json" "$live_release_workflow_id" "$dispatch_title" main \
  "$live_release_head_sha" \
  2026-07-09T12:00:00Z >/dev/null 2>&1; then
  fail "documentation-example workflow path suffix was accepted over the live canonical path"
fi

cat > "$tmp/runs.json" <<'EOF'
{"workflow_runs":[{"id":42,"display_title":"Verify v0.3.4 draft assets at 0123456789abcdef0123456789abcdef01234567 for 2222222222222222222222222222222222222222 object 1111111111111111111111111111111111111111","event":"workflow_dispatch","head_branch":"main","head_sha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"success","created_at":"2026-07-09T12:01:00Z"}]}
EOF
cat > "$tmp/jobs.json" <<'EOF'
{"total_count":2,"jobs":[
  {"name":"Verify notarized macOS archive (arm64)","status":"completed","conclusion":"success"},
  {"name":"Verify notarized macOS archive (x86_64)","status":"completed","conclusion":"success"}
]}
EOF
export MOCK_GH_RUNS_JSON="$tmp/runs.json"
export MOCK_GH_JOBS_JSON="$tmp/jobs.json"
mock_log_timestamp=2026-07-09T12:01:00.0000000Z
write_run_logs() {
  local state=$1 object=${2:-$tag_object} workflow=${3:-0123456789abcdef0123456789abcdef01234567}
  local arch index proof step_log
  proof="GOG_RELEASE_PROOF tag=$tag object=$object commit=$tag_commit workflow=$workflow state=$state"
  rm -rf "$tmp/run-logs" "$tmp/run-logs.zip"
  mkdir -p "$tmp/run-logs"
  index=0
  for arch in arm64 x86_64; do
    mkdir -p "$tmp/run-logs/Verify notarized macOS archive ($arch)"
    step_log="$tmp/run-logs/Verify notarized macOS archive ($arch)/6_Execute verified candidate last.txt"
    printf '%s %s\n' "$mock_log_timestamp" "$proof" > "$step_log"
    printf '%s %s\n' "$mock_log_timestamp" "$proof" > "$tmp/run-logs/${index}_Verify notarized macOS archive ($arch).txt"
    index=$((index + 1))
  done
  (cd "$tmp/run-logs" && zip -qr "$tmp/run-logs.zip" .)
  export MOCK_GH_LOGS_ZIP="$tmp/run-logs.zip"
}
write_run_logs draft
[[ "$(GITHUB_REPOSITORY=openclaw/gogcli "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object")" == 42 ]] || fail "native verifier proof was not recognized"
rm "$tmp/run-logs/Verify notarized macOS archive (arm64)/6_Execute verified candidate last.txt"
rm "$tmp/run-logs.zip"
(cd "$tmp/run-logs" && zip -qr "$tmp/run-logs.zip" .)
if GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "combined job log substituted for the exact native final-step proof"
fi
write_run_logs draft
duplicate_proof="GOG_RELEASE_PROOF tag=$tag object=$tag_object commit=$tag_commit workflow=0123456789abcdef0123456789abcdef01234567 state=draft"
printf '%s %s%s\n' "$mock_log_timestamp" "$duplicate_proof" "$duplicate_proof" \
  > "$tmp/run-logs/Verify notarized macOS archive (arm64)/6_Execute verified candidate last.txt"
rm "$tmp/run-logs.zip"
(cd "$tmp/run-logs" && zip -qr "$tmp/run-logs.zip" .)
if GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "same-line duplicate native proof marker was accepted"
fi
write_run_logs draft
if GITHUB_REPOSITORY=attacker/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "verifier checker accepted an attacker-controlled repository override"
fi
if MOCK_PINNED_SIGNER=attacker GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "release proof checker accepted a signer trusted outside the repository policy"
fi

jq '.workflow_runs += [(.workflow_runs[0] |
  .id = 43 |
  .created_at = "2026-07-09T12:02:00Z" |
  .display_title |= sub(
    "for 2222222222222222222222222222222222222222 object 1111111111111111111111111111111111111111$";
    "for 4444444444444444444444444444444444444444 object 3333333333333333333333333333333333333333"
  )
)]' "$tmp/runs.json" > "$tmp/a-b-a-runs.json"
if MOCK_GH_RUNS_JSON="$tmp/a-b-a-runs.json" GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "newer B verifier proof was hidden by stale A proof after an A-to-B-to-A tag move"
fi

jq '.workflow_runs[0].display_title |= sub("object 1111111111111111111111111111111111111111$"; "object 3333333333333333333333333333333333333333")' \
  "$tmp/runs.json" > "$tmp/moved-tag-runs.json"
if MOCK_GH_RUNS_JSON="$tmp/moved-tag-runs.json" GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "A-to-B verifier proof survived the signed tag moving back to A"
fi

grep -Fq 'sort_by([.created_at, .id]) | last' "$root/scripts/check-release-verifier.sh" || \
  fail "verifier does not select the newest otherwise-relevant run before identity validation"

jq '.workflow_runs[0].created_at = "2026-07-09T12:00:00Z"' "$tmp/runs.json" > "$tmp/same-second-runs.json"
if MOCK_GH_RUNS_JSON="$tmp/same-second-runs.json" GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "same-second verifier proof satisfied a replaced asset"
fi

jq '.workflow_runs[0].head_sha = "ffffffffffffffffffffffffffffffffffffffff"' "$tmp/runs.json" > "$tmp/wrong-ref-runs.json"
if MOCK_GH_RUNS_JSON="$tmp/wrong-ref-runs.json" GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" true "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "verifier proof accepted the wrong default-branch ref"
fi

jq '.[0].draft = false | .[0].published_at = "2026-07-09T12:02:00Z"' \
  "$tmp/releases.json" > "$tmp/published-releases.json"
if MOCK_GH_RELEASES_JSON="$tmp/published-releases.json" MOCK_GH_RUNS_JSON="$tmp/runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "stale draft proof satisfied published closeout"
fi

cat > "$tmp/old-published-runs.json" <<'EOF'
{"workflow_runs":[{"id":42,"display_title":"Verify v0.3.4 published assets at ffffffffffffffffffffffffffffffffffffffff for 2222222222222222222222222222222222222222 object release-event","event":"release","head_branch":"v0.3.4","head_sha":"2222222222222222222222222222222222222222","status":"completed","conclusion":"success","created_at":"2026-07-09T12:03:00Z"}]}
EOF
if MOCK_GH_RELEASES_JSON="$tmp/published-releases.json" MOCK_GH_RUNS_JSON="$tmp/old-published-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "published closeout accepted stale protected-default-branch proof"
fi

cat > "$tmp/published-runs.json" <<'EOF'
{"workflow_runs":[{"id":42,"display_title":"Verify v0.3.4 published assets at 0123456789abcdef0123456789abcdef01234567 for 2222222222222222222222222222222222222222 object 1111111111111111111111111111111111111111","event":"workflow_dispatch","head_branch":"main","head_sha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"success","created_at":"2026-07-09T12:03:00Z"}]}
EOF
write_run_logs published
[[ "$(MOCK_GH_RELEASES_JSON="$tmp/published-releases.json" MOCK_GH_RUNS_JSON="$tmp/published-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object")" == 42 ]] || \
  fail "current protected-default-branch published verifier proof was not recognized"

if MOCK_DEFAULT_SHA=ffffffffffffffffffffffffffffffffffffffff \
  MOCK_GH_RELEASES_JSON="$tmp/published-releases.json" MOCK_GH_RUNS_JSON="$tmp/published-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "one-time release proof survived default-branch advancement"
fi
cat > "$tmp/advanced-published-runs.json" <<'EOF'
{"workflow_runs":[{"id":42,"display_title":"Verify v0.3.4 published assets at ffffffffffffffffffffffffffffffffffffffff for 2222222222222222222222222222222222222222 object 1111111111111111111111111111111111111111","event":"workflow_dispatch","head_branch":"main","head_sha":"ffffffffffffffffffffffffffffffffffffffff","status":"completed","conclusion":"success","created_at":"2026-07-09T12:03:00Z"}]}
EOF
write_run_logs published "$tag_object" ffffffffffffffffffffffffffffffffffffffff
[[ "$(MOCK_DEFAULT_SHA=ffffffffffffffffffffffffffffffffffffffff \
  MOCK_GH_RELEASES_JSON="$tmp/published-releases.json" MOCK_GH_RUNS_JSON="$tmp/advanced-published-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object")" == 42 ]] || \
  fail "fresh manual published proof did not recover after default-branch advancement"
write_run_logs published

write_run_logs published 3333333333333333333333333333333333333333
if MOCK_GH_RELEASES_JSON="$tmp/published-releases.json" MOCK_GH_RUNS_JSON="$tmp/published-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "same-commit different-tag-object proof survived an A-to-B-to-A tag move"
fi
write_run_logs published

jq '.workflow_runs[0].created_at = "2026-07-09T12:02:00Z"' \
  "$tmp/published-runs.json" > "$tmp/equal-publication-time-runs.json"
[[ "$(MOCK_GH_RELEASES_JSON="$tmp/published-releases.json" MOCK_GH_RUNS_JSON="$tmp/equal-publication-time-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object")" == 42 ]] || \
  fail "dispatched proof equal to second-resolution published_at was rejected"

jq '.[0].assets[0].updated_at = "2026-07-09T12:04:00Z"' \
  "$tmp/published-releases.json" > "$tmp/mutated-published-releases.json"
if MOCK_GH_RELEASES_JSON="$tmp/mutated-published-releases.json" MOCK_GH_RUNS_JSON="$tmp/published-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "published closeout accepted proof predating an asset mutation"
fi

jq '.workflow_runs[0].created_at = "2026-07-09T12:04:00Z"' \
  "$tmp/published-runs.json" > "$tmp/equal-asset-time-runs.json"
if MOCK_GH_RELEASES_JSON="$tmp/mutated-published-releases.json" MOCK_GH_RUNS_JSON="$tmp/equal-asset-time-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli \
  "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object" >/dev/null 2>&1; then
  fail "same-second published proof satisfied a replaced asset"
fi

jq '.workflow_runs[0].created_at = "2026-07-09T12:05:00Z"' \
  "$tmp/published-runs.json" > "$tmp/fresh-published-runs.json"
[[ "$(MOCK_GH_RELEASES_JSON="$tmp/mutated-published-releases.json" MOCK_GH_RUNS_JSON="$tmp/fresh-published-runs.json" \
  GITHUB_REPOSITORY=openclaw/gogcli "$root/scripts/check-release-verifier.sh" "$tag" false "$tag_commit" "$tag_object")" == 42 ]] || \
  fail "fresh published verifier proof was not recognized after an asset mutation"

echo "release asset tests passed"
