#!/usr/bin/env bash
# shellcheck disable=SC2016 # Literal source-text contract assertions.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
real_git=$(command -v git)
real_go=$(command -v go)
mkdir -p "$tmp/bin"
export RELEASE_TEST_LOG="$tmp/release.log"
export RELEASE_TEST_ROOT="$root"
export RELEASE_TEST_TAGGED_CHANGELOG="$tmp/tagged-CHANGELOG.md"
export RELEASE_TEST_TAGGED_VERSION="$tmp/tagged-VERSION"
export RELEASE_TEST_GRAFT_PATH="$tmp/active-grafts"
export RELEASE_TEST_MANIFEST="$tmp/.mac-release.env"
export MAC_RELEASE_MANIFEST="$RELEASE_TEST_MANIFEST"
: > "$RELEASE_TEST_MANIFEST"
cat > "$RELEASE_TEST_TAGGED_CHANGELOG" <<'EOF'
# Changelog

## 0.3.4 - 2026-07-09

### Changed

- Tagged release notes sentinel; mutable worktree notes must not be used.

## 0.3.3 - 2026-07-09
EOF
printf '%s\n' v0.3.4 > "$RELEASE_TEST_TAGGED_VERSION"

# Git can hide both untracked Go files and tracked skip-worktree changes from
# ordinary porcelain output. Prove the bypass and the fresh-clone isolation
# that the official producer relies on.
config_repo="$tmp/status-config-repo"
fresh_repo="$tmp/status-fresh-clone"
"$real_git" init --quiet "$config_repo"
"$real_git" -C "$config_repo" config user.name release-test
"$real_git" -C "$config_repo" config user.email release-test@example.com
printf '%s\n' 'module example.com/release-test' 'go 1.26.5' > "$config_repo/go.mod"
printf '%s\n' 'package main' 'const buildSource = "trusted-commit"' 'func main() {}' > "$config_repo/main.go"
"$real_git" -C "$config_repo" add go.mod main.go
"$real_git" -C "$config_repo" -c commit.gpgsign=false commit --quiet -m initial
cat > "$tmp/fake-ssh-keygen" <<'EOF'
#!/usr/bin/env bash
: > "${RELEASE_TEST_FAKE_SSH_SENTINEL:?}"
printf '%s\n' 'Good "git" signature for steipete@gmail.com with ED25519 key SHA256:WmI9lVtd7F2c5XyRHbZVO3yYYJzwsSNzcZQMPT147HI' >&2 # gitleaks:allow -- public release-key fingerprint
EOF
chmod +x "$tmp/fake-ssh-keygen"
"$real_git" -C "$config_repo" config gpg.ssh.program "$tmp/fake-ssh-keygen"
configured_ssh_program=$("$real_git" -C "$config_repo" config --get gpg.ssh.program)
RELEASE_TEST_FAKE_SSH_SENTINEL="$tmp/fake-ssh-keygen-ran" "$configured_ssh_program"
[[ -e "$tmp/fake-ssh-keygen-ran" ]] || {
  echo "repo-local fake SSH verifier regression did not reproduce" >&2
  exit 1
}
rm "$tmp/fake-ssh-keygen-ran"
[[ "$("$real_git" -C "$config_repo" \
  -c gpg.ssh.program=/usr/bin/ssh-keygen config --get gpg.ssh.program)" == /usr/bin/ssh-keygen ]] || {
  echo "command-line SSH verifier pin did not override repo-local config" >&2
  exit 1
}
"$real_git" -C "$config_repo" config status.showUntrackedFiles no
printf '%s\n' 'package main' 'const injected = true' > "$config_repo/injected.go"
[[ -z "$("$real_git" -C "$config_repo" status --porcelain)" ]] || {
  echo "Git untracked-file hiding regression did not reproduce" >&2
  exit 1
}
[[ "$("$real_git" -C "$config_repo" status --porcelain --untracked-files=all)" == '?? injected.go' ]] || {
  echo "explicit untracked-file inventory did not override local Git config" >&2
  exit 1
}
"$real_go" -C "$config_repo" list -f '{{join .GoFiles " "}}' | grep -Fq 'injected.go'
rm "$config_repo/injected.go"
"$real_git" -C "$config_repo" update-index --skip-worktree main.go
printf '%s\n' 'package main' 'const buildSource = "hidden-worktree"' 'func main() {}' > "$config_repo/main.go"
[[ -z "$("$real_git" -C "$config_repo" status --porcelain --untracked-files=all)" ]] || {
  echo "Git skip-worktree hiding regression did not reproduce" >&2
  exit 1
}
"$real_git" clone --quiet --no-checkout --no-local "$config_repo" "$fresh_repo"
"$real_git" -C "$fresh_repo" checkout --quiet --detach HEAD
grep -Fq 'trusted-commit' "$fresh_repo/main.go"
if grep -Fq 'hidden-worktree' "$fresh_repo/main.go"; then
  echo "fresh clone inherited hidden mutable working-tree bytes" >&2
  exit 1
fi
trusted_commit=$("$real_git" -C "$config_repo" rev-parse HEAD)
tree=$("$real_git" -C "$config_repo" write-tree)
unrelated_commit=$(printf '%s\n' 'unrelated root' | "$real_git" -C "$config_repo" commit-tree "$tree")
if "$real_git" -C "$config_repo" merge-base --is-ancestor "$trusted_commit" "$unrelated_commit"; then
  echo "synthetic commits unexpectedly had trusted ancestry before graft injection" >&2
  exit 1
fi
graft_path=$("$real_git" -C "$config_repo" rev-parse --git-path info/grafts)
[[ "$graft_path" == /* ]] || graft_path="$config_repo/$graft_path"
mkdir -p "$(dirname "$graft_path")"
printf '%s %s\n' "$unrelated_commit" "$trusted_commit" > "$graft_path"
if ! env GIT_NO_REPLACE_OBJECTS=1 "$real_git" -C "$config_repo" \
  merge-base --is-ancestor "$trusted_commit" "$unrelated_commit" 2>/dev/null; then
  echo "legacy Git graft did not override ancestry with replacements disabled" >&2
  exit 1
fi

cat > "$tmp/bin/uname" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == -m ]]; then
  printf '%s\n' arm64
else
  printf '%s\n' Darwin
fi
EOF

cat > "$tmp/bin/go" <<'EOF'
#!/usr/bin/env bash
[[ "${1:-}" == version ]] || exit 2
printf '%s\n' 'go version go1.26.5 darwin/arm64'
EOF

cat > "$tmp/bin/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
tag_object=1111111111111111111111111111111111111111
head_commit=2222222222222222222222222222222222222222
mismatch=3333333333333333333333333333333333333333
tag_commit=$head_commit
[[ "${RELEASE_TEST_MODE:-exact}" != off-default-tag ]] || tag_commit=$mismatch
git_dir=
if [[ "${1:-}" == -C ]]; then
  git_dir=$2
  shift 2
fi
seen_format=false
seen_allowed_signers=false
seen_ssh_program=false
while [[ "${1:-}" == -c ]]; do
  case "${2:-}" in
    gpg.format=ssh) seen_format=true ;;
    gpg.ssh.program=/usr/bin/ssh-keygen) seen_ssh_program=true ;;
    gpg.ssh.allowedSignersFile="$RELEASE_TEST_ROOT/.github/release-allowed-signers") seen_allowed_signers=true ;;
  esac
  shift 2
done
command=$1
shift
case "$command" in
  status)
    if [[ -z "$git_dir" && "${RELEASE_TEST_MODE:-exact}" == hidden-untracked-config &&
      " ${*} " == *' --untracked-files=all '* ]]; then
      printf '%s\n' '?? injected.go'
    fi
    ;;
  branch) printf '%s\n' main ;;
  describe) printf '%s\n' v0.3.4 ;;
  clone)
    destination=${!#}
    mkdir -p "$destination"
    touch "$destination/.release-test-fresh-clone"
    printf 'git clone %s\n' "$*" >> "$RELEASE_TEST_LOG"
    ;;
  checkout) ;;
  for-each-ref)
    if [[ -z "$git_dir" && "${RELEASE_TEST_MODE:-exact}" == replacement-ref ]]; then
      printf '%s\n' refs/replace/2222222222222222222222222222222222222222
    fi
    ;;
  ls-files)
    if [[ -z "$git_dir" && "${RELEASE_TEST_MODE:-exact}" == hidden-index ]]; then
      printf '%s\n' 'S injected.go'
    fi
    ;;
  cat-file)
    case "${1:-} ${2:-}" in
      '-t FETCH_HEAD') printf '%s\n' tag ;;
      'tag FETCH_HEAD')
        printf 'object %s\ntype commit\ntag v0.3.4\n\nmock tag\n' "$tag_commit"
        ;;
      *) exit 1 ;;
    esac
    ;;
  verify-tag)
    [[ "$seen_format" == true && "$seen_ssh_program" == true &&
      "$seen_allowed_signers" == true ]] || exit 1
    if [[ "${RELEASE_TEST_MODE:-exact}" == untrusted-global-signer ]]; then
      printf '%s\n' 'Good "git" signature for attacker@example.com with ED25519 key SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' >&2
    else
      printf '%s\n' 'Good "git" signature for steipete@gmail.com with ED25519 key SHA256:WmI9lVtd7F2c5XyRHbZVO3yYYJzwsSNzcZQMPT147HI' >&2 # gitleaks:allow -- public release-key fingerprint
    fi
    ;;
  rev-parse)
    case "$1" in
      --git-path)
        [[ "${2:-}" == info/grafts ]] || exit 1
        printf '%s\n' "$RELEASE_TEST_GRAFT_PATH"
        ;;
      HEAD) printf '%s\n' "$head_commit" ;;
      'FETCH_HEAD^{}') printf '%s\n' "$tag_commit" ;;
      FETCH_HEAD) printf '%s\n' "$tag_object" ;;
      refs/tags/*'^{}') printf '%s\n' "$tag_commit" ;;
      refs/tags/*) printf '%s\n' "$tag_object" ;;
      refs/remotes/origin/main)
        if [[ "${RELEASE_TEST_MODE:-exact}" == off-default ]]; then
          printf '%s\n' "$mismatch"
        else
          printf '%s\n' "$head_commit"
        fi
        ;;
      *) exit 1 ;;
    esac
    ;;
  fetch) ;;
  show)
    case "${1:-}" in
      *:.github/release-allowed-signers) cat "$RELEASE_TEST_ROOT/.github/release-allowed-signers" ;;
      *:CHANGELOG.md) cat "$RELEASE_TEST_TAGGED_CHANGELOG" ;;
      *:internal/cmd/VERSION) cat "$RELEASE_TEST_TAGGED_VERSION" ;;
      *) exit 1 ;;
    esac
    ;;
  merge-base)
    [[ "${RELEASE_TEST_MODE:-exact}" != off-default-tag ]]
    ;;
  ls-remote)
    if [[ " ${*} " == *' --symref origin HEAD '* ]]; then
      printf 'ref: refs/heads/main\tHEAD\n'
      exit 0
    fi
    if [[ " ${*} " == *' origin refs/heads/main '* ]]; then
      if [[ "${RELEASE_TEST_MODE:-exact}" == off-default ]]; then
        printf '%s\t%s\n' "$mismatch" refs/heads/main
      else
        printf '%s\t%s\n' "$head_commit" refs/heads/main
      fi
      exit 0
    fi
    case "${RELEASE_TEST_MODE:-exact}" in
      missing) exit 2 ;;
      object-mismatch)
        printf '%s\t%s\n' "$mismatch" refs/tags/v0.3.4
        printf '%s\t%s\n' "$head_commit" 'refs/tags/v0.3.4^{}'
        ;;
      commit-mismatch)
        printf '%s\t%s\n' "$tag_object" refs/tags/v0.3.4
        printf '%s\t%s\n' "$mismatch" 'refs/tags/v0.3.4^{}'
        ;;
      exact|untrusted-global-signer|off-default|off-default-tag|unprotected-default|default-api-mismatch|tag-ci-failing|tag-release-pending)
        printf '%s\t%s\n' "$tag_object" refs/tags/v0.3.4
        printf '%s\t%s\n' "$tag_commit" 'refs/tags/v0.3.4^{}'
        ;;
    esac
    ;;
  *) exit 1 ;;
esac
EOF

cat > "$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-} ${2:-}" == 'auth token' ]]; then
  [[ "${3:-} ${4:-}" == '--hostname github.com' && "$#" -eq 4 ]] || exit 90
  printf '%s\n' test-token
  exit 0
fi
if [[ "${1:-}" == api ]]; then
  shift
  [[ "${1:-} ${2:-}" == '--hostname github.com' ]] || exit 90
  shift 2
  [[ "${1:-}" != --paginate ]] || shift
  if [[ "${1:-}" == 'repos/openclaw/gogcli' ]]; then
    jq -n '{default_branch:"main"}'
    exit 0
  fi
  if [[ "${1:-}" == 'repos/openclaw/gogcli/branches/main' ]]; then
    protected=true
    commit=2222222222222222222222222222222222222222
    [[ "${RELEASE_TEST_MODE:-exact}" != unprotected-default ]] || protected=false
    [[ "${RELEASE_TEST_MODE:-exact}" != default-api-mismatch ]] || \
      commit=3333333333333333333333333333333333333333
    jq -n --argjson protected "$protected" --arg commit "$commit" \
      '{protected:$protected,commit:{sha:$commit}}'
    exit 0
  fi
  if [[ "${1:-}" == 'repos/openclaw/gogcli/actions/workflows/ci.yml' ]]; then
    jq -n '{id:301,path:".github/workflows/ci.yml",state:"active"}'
    exit 0
  fi
  if [[ "${1:-}" == 'repos/openclaw/gogcli/actions/workflows/release.yml' ]]; then
    jq -n '{id:302,path:".github/workflows/release.yml",state:"active"}'
    exit 0
  fi
  if [[ "${1:-}" == 'repos/openclaw/gogcli/actions/workflows/301/runs?event=push&branch=v0.3.4&head_sha=2222222222222222222222222222222222222222&per_page=100' ]]; then
    if [[ "${RELEASE_TEST_MODE:-exact}" == tag-ci-failing ]]; then
      jq -n '{workflow_runs:[
        {id:300,workflow_id:301,event:"push",head_branch:"v0.3.4",head_sha:"2222222222222222222222222222222222222222",status:"completed",conclusion:"success",created_at:"2026-07-09T12:00:00Z",run_attempt:1},
        {id:301,workflow_id:301,event:"push",head_branch:"v0.3.4",head_sha:"2222222222222222222222222222222222222222",status:"completed",conclusion:"failure",created_at:"2026-07-09T12:01:00Z",run_attempt:1}
      ]}'
    else
      jq -n '{workflow_runs:[
        {id:301,workflow_id:301,event:"push",head_branch:"v0.3.4",head_sha:"2222222222222222222222222222222222222222",status:"completed",conclusion:"success",created_at:"2026-07-09T12:01:00Z",run_attempt:1}
      ]}'
    fi
    exit 0
  fi
  if [[ "${1:-}" == 'repos/openclaw/gogcli/actions/workflows/302/runs?event=push&branch=v0.3.4&head_sha=2222222222222222222222222222222222222222&per_page=100' ]]; then
    if [[ "${RELEASE_TEST_MODE:-exact}" == tag-release-pending ]]; then
      jq -n '{workflow_runs:[
        {id:400,workflow_id:302,event:"push",head_branch:"v0.3.4",head_sha:"2222222222222222222222222222222222222222",status:"completed",conclusion:"success",created_at:"2026-07-09T12:00:00Z",run_attempt:1},
        {id:401,workflow_id:302,event:"push",head_branch:"v0.3.4",head_sha:"2222222222222222222222222222222222222222",status:"in_progress",conclusion:null,created_at:"2026-07-09T12:01:00Z",run_attempt:1}
      ]}'
    else
      jq -n '{workflow_runs:[
        {id:401,workflow_id:302,event:"push",head_branch:"v0.3.4",head_sha:"2222222222222222222222222222222222222222",status:"completed",conclusion:"success",created_at:"2026-07-09T12:01:00Z",run_attempt:1}
      ]}'
    fi
    exit 0
  fi
  if [[ "${1:-}" == 'repos/openclaw/gogcli/releases?per_page=100' ]]; then
    jq -n --rawfile body "$RELEASE_TEST_LOG.body" '[{
      id: 42,
      tag_name: "v0.3.4",
      name: "v0.3.4",
      target_commitish: "2222222222222222222222222222222222222222",
      draft: true,
      prerelease: false,
      body: $body,
      assets: [
        {id:1,name:"checksums.txt",size:1,digest:("sha256:" + ("a" * 64)),state:"uploaded"},
        {id:2,name:"gogcli_0.3.4_darwin_amd64.tar.gz",size:2,digest:("sha256:" + ("b" * 64)),state:"uploaded"},
        {id:3,name:"gogcli_0.3.4_darwin_arm64.tar.gz",size:3,digest:("sha256:" + ("c" * 64)),state:"uploaded"},
        {id:4,name:"gogcli_0.3.4_linux_amd64.tar.gz",size:4,digest:("sha256:" + ("d" * 64)),state:"uploaded"},
        {id:5,name:"gogcli_0.3.4_linux_arm64.tar.gz",size:5,digest:("sha256:" + ("e" * 64)),state:"uploaded"},
        {id:6,name:"gogcli_0.3.4_windows_amd64.zip",size:6,digest:("sha256:" + ("f" * 64)),state:"uploaded"},
        {id:7,name:"gogcli_0.3.4_windows_arm64.zip",size:7,digest:("sha256:" + ("0" * 64)),state:"uploaded"}
      ]
    }]'
    exit 0
  fi
fi
printf 'gh %s\n' "$*" >> "$RELEASE_TEST_LOG"
exit 2
EOF

cat > "$tmp/bin/goreleaser" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$tmp/bin/release-helper" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
test_dir=${MAC_RELEASE_MANIFEST%/*}
test_log=$test_dir/release.log
[[ -f .release-test-fresh-clone ]] || {
  echo "release-helper did not run from a fresh source clone" >&2
  exit 1
}
[[ -f "${MAC_RELEASE_MANIFEST:-}" && ! -L "$MAC_RELEASE_MANIFEST" ]] || {
  echo "release-helper did not receive the operator release manifest" >&2
  exit 1
}
if declare -F codesign >/dev/null || [[ -n "${ATTACKER_SECRET+x}" || -n "${PYTHONPATH+x}" ]]; then
  echo "release-helper imported ambient executable state" >&2
  exit 1
fi
printf 'release-helper-cwd %s\n' "$PWD" >> "$test_log"
printf 'release-helper %s\n' "$*" >> "$test_log"
while [[ "$#" -gt 0 ]]; do
  if [[ "$1" == --release-notes ]]; then
    cp "$2" "$test_log.notes"
    {
      cat "$test_log.notes"
      printf '\n'
    } > "$test_log.body"
    break
  fi
  shift
done
EOF

chmod +x "$tmp/bin/"*
export PATH="$tmp/bin:$PATH"
export MAC_RELEASE_HELPER="$tmp/bin/release-helper"
export GH_HOST=attacker.example
export GH_CONFIG_DIR="$tmp/hostile-gh-config"
mkdir -p "$GH_CONFIG_DIR"

unset NOTARYTOOL_KEYCHAIN_PROFILE GITHUB_TOKEN
export NOTARYTOOL_KEYCHAIN_PROFILE=test-profile
if MAC_RELEASE_MANIFEST="$tmp/missing-release-manifest" \
  "$root/scripts/release-local" draft >/dev/null 2>&1; then
  echo "release-local accepted a missing release-mac-app manifest" >&2
  exit 1
fi
if MAC_RELEASE_HELPER=release-helper \
  "$root/scripts/release-local" draft >/dev/null 2>&1; then
  echo "release-local accepted a PATH-resolved credential-bound release helper" >&2
  exit 1
fi
mkdir -p "$tmp/overlay"
cat > "$tmp/overlay/main.go" <<'EOF'
//go:build darwin

package main

func main() {}
EOF
jq -n \
  --arg source "$root/cmd/gog/main.go" \
  --arg replacement "$tmp/overlay/main.go" \
  '{Replace:{($source):$replacement}}' > "$tmp/overlay.json"
cat > "$tmp/go.work" <<'EOF'
go 1.26.5

use .
EOF
cat > "$tmp/executable-go-hook" <<EOF
#!/usr/bin/env bash
touch "$tmp/executable-go-hook-ran"
EOF
chmod 0755 "$tmp/executable-go-hook"
for contaminated_mode in overlay workspace experiment goroot cacheprog auth cc cflags pkgconfig sdkroot; do
  : > "$RELEASE_TEST_LOG"
  case "$contaminated_mode" in
    overlay)
      contaminated_env=(GOFLAGS="-overlay=$tmp/overlay.json")
      ;;
    workspace)
      contaminated_env=(GOWORK="$tmp/go.work")
      ;;
    experiment)
      contaminated_env=(GOEXPERIMENT=fieldtrack)
      ;;
    goroot)
      contaminated_env=(GOROOT="$tmp/overlay")
      ;;
    cacheprog)
      contaminated_env=(GOCACHEPROG="$tmp/executable-go-hook")
      ;;
    auth)
      contaminated_env=(GOAUTH="$tmp/executable-go-hook")
      ;;
    cc)
      contaminated_env=(CC="$tmp/executable-go-hook")
      ;;
    cflags)
      contaminated_env=(CGO_CFLAGS="-include $tmp/overlay/main.go")
      ;;
    pkgconfig)
      contaminated_env=(PKG_CONFIG="$tmp/executable-go-hook")
      ;;
    sdkroot)
      contaminated_env=(SDKROOT="$tmp/overlay")
      ;;
  esac
  if env "${contaminated_env[@]}" "$root/scripts/release-local" draft >/dev/null 2>&1; then
    echo "release-local accepted ambient Go $contaminated_mode contamination" >&2
    exit 1
  fi
  if grep -Fq release-helper "$RELEASE_TEST_LOG"; then
    echo "release-local reached codesign-run with ambient Go $contaminated_mode contamination" >&2
    exit 1
  fi
  [[ ! -e "$tmp/executable-go-hook-ran" ]] || {
    echo "release-local executed ambient Go $contaminated_mode hook" >&2
    exit 1
  }
done

for release_mode in draft pilot; do
  release_args=()
  [[ "$release_mode" != pilot ]] || release_args=(v0.3.4)
  : > "$RELEASE_TEST_LOG"
  if RELEASE_TEST_MODE=off-default "$root/scripts/release-local" "$release_mode" "${release_args[@]}" >/dev/null 2>&1; then
    echo "release-local accepted an off-default signed release source: $release_mode" >&2
    exit 1
  fi
  if grep -Fq release-helper "$RELEASE_TEST_LOG"; then
    echo "release-local reached codesign-run from an off-default signed source: $release_mode" >&2
    exit 1
  fi
done

for mode in unprotected-default default-api-mismatch; do
  : > "$RELEASE_TEST_LOG"
  if RELEASE_TEST_MODE=$mode "$root/scripts/release-local" draft >/dev/null 2>&1; then
    echo "release-local accepted unsafe GitHub default branch mode: $mode" >&2
    exit 1
  fi
  if grep -Fq release-helper "$RELEASE_TEST_LOG"; then
    echo "release-local reached codesign-run with unsafe GitHub default branch mode: $mode" >&2
    exit 1
  fi
done

for mode in hidden-untracked-config hidden-index replacement-ref; do
  : > "$RELEASE_TEST_LOG"
  if RELEASE_TEST_MODE=$mode "$root/scripts/release-local" draft >/dev/null 2>&1; then
    echo "release-local accepted hidden local source mode: $mode" >&2
    exit 1
  fi
  if grep -Fq release-helper "$RELEASE_TEST_LOG"; then
    echo "release-local reached codesign-run with hidden local source mode: $mode" >&2
    exit 1
  fi
done

printf '%s\n' '3333333333333333333333333333333333333333 2222222222222222222222222222222222222222' \
  > "$RELEASE_TEST_GRAFT_PATH"
if RELEASE_TEST_MODE=graft-file "$root/scripts/release-local" draft >/dev/null 2>&1; then
  echo "release-local accepted a legacy Git graft ancestry override" >&2
  exit 1
fi
rm "$RELEASE_TEST_GRAFT_PATH"
if grep -Fq release-helper "$RELEASE_TEST_LOG"; then
  echo "release-local reached codesign-run with an active Git graft" >&2
  exit 1
fi

for mode in missing object-mismatch commit-mismatch off-default-tag untrusted-global-signer; do
  : > "$RELEASE_TEST_LOG"
  rm -f "$RELEASE_TEST_LOG.notes" "$RELEASE_TEST_LOG.body"
  if RELEASE_TEST_MODE=$mode "$root/scripts/release-local" draft >/dev/null 2>&1; then
    echo "release-local accepted remote tag mode: $mode" >&2
    exit 1
  fi
  if grep -Fq release-helper "$RELEASE_TEST_LOG"; then
    echo "release-local opened a draft after remote tag failure: $mode" >&2
    exit 1
  fi
done

for mode in tag-ci-failing tag-release-pending; do
  : > "$RELEASE_TEST_LOG"
  if RELEASE_TEST_MODE=$mode "$root/scripts/release-local" draft >/dev/null 2>&1; then
    echo "release-local accepted an unsuccessful exact-tag check: $mode" >&2
    exit 1
  fi
  if grep -Fq release-helper "$RELEASE_TEST_LOG"; then
    echo "release-local reached codesign-run before exact-tag checks passed: $mode" >&2
    exit 1
  fi
done

cp "$RELEASE_TEST_TAGGED_CHANGELOG" "$tmp/canonical-tagged-CHANGELOG.md"
for metadata_mode in unreleased duplicate stale-version; do
  : > "$RELEASE_TEST_LOG"
  cp "$tmp/canonical-tagged-CHANGELOG.md" "$RELEASE_TEST_TAGGED_CHANGELOG"
  printf '%s\n' v0.3.4 > "$RELEASE_TEST_TAGGED_VERSION"
  case "$metadata_mode" in
    unreleased)
      sed 's/## 0.3.4 - 2026-07-09/## 0.3.4 - Unreleased/' \
        "$tmp/canonical-tagged-CHANGELOG.md" > "$RELEASE_TEST_TAGGED_CHANGELOG"
      ;;
    duplicate)
      printf '\n## 0.3.4 - 2026-07-10\n\n- duplicate\n' >> "$RELEASE_TEST_TAGGED_CHANGELOG"
      ;;
    stale-version)
      printf '%s\n' v0.3.3 > "$RELEASE_TEST_TAGGED_VERSION"
      ;;
  esac
  if RELEASE_TEST_MODE=exact "$root/scripts/release-local" draft >/dev/null 2>&1; then
    echo "release-local accepted unfinalized signed-tag metadata: $metadata_mode" >&2
    exit 1
  fi
  if grep -Fq release-helper "$RELEASE_TEST_LOG"; then
    echo "release-local reached codesign-run with unfinalized signed-tag metadata: $metadata_mode" >&2
    exit 1
  fi
done
cp "$tmp/canonical-tagged-CHANGELOG.md" "$RELEASE_TEST_TAGGED_CHANGELOG"
printf '%s\n' v0.3.4 > "$RELEASE_TEST_TAGGED_VERSION"

: > "$RELEASE_TEST_LOG"
rm -f "$RELEASE_TEST_LOG.notes" "$RELEASE_TEST_LOG.body"
RELEASE_TEST_MODE=exact "$root/scripts/release-local" draft
grep -Fq 'git clone --quiet --no-checkout --no-local --no-tags' "$RELEASE_TEST_LOG"
grep -Fq 'https://github.com/openclaw/gogcli.git' "$RELEASE_TEST_LOG"
grep -Fq 'release-helper-cwd ' "$RELEASE_TEST_LOG"
grep -Fq 'release-helper codesign-run --with-package-secrets -- /usr/bin/python3 -I' "$RELEASE_TEST_LOG"
if grep -Fq 'codesign-run --with-package-secrets -- /usr/bin/env' "$RELEASE_TEST_LOG"; then
  echo "release-local crossed the credential boundary through an inherited environment" >&2
  exit 1
fi
grep -Fq 'GOTOOLCHAIN=local' "$RELEASE_TEST_LOG"
grep -Fq 'GOWORK=off' "$RELEASE_TEST_LOG"
grep -Fq 'LC_ALL=C' "$RELEASE_TEST_LOG"
grep -Fq 'GOG_OFFICIAL_RELEASE=1' "$RELEASE_TEST_LOG"
grep -Fq 'GOG_COMMIT=22222222' "$RELEASE_TEST_LOG"
grep -Fq '/goreleaser release --config ' "$RELEASE_TEST_LOG"
grep -Fq -- '--draft --clean --parallelism=2 --release-notes' "$RELEASE_TEST_LOG"
grep -Fq 'Tagged release notes sentinel; mutable worktree notes must not be used.' "$RELEASE_TEST_LOG.notes"
grep -Fq 'Full changelog: https://github.com/openclaw/gogcli/blob/v0.3.4/CHANGELOG.md' "$RELEASE_TEST_LOG.notes"
notes_size=$(wc -c < "$RELEASE_TEST_LOG.notes" | tr -d ' ')
body_size=$(wc -c < "$RELEASE_TEST_LOG.body" | tr -d ' ')
[[ "$body_size" == "$((notes_size + 1))" ]] || {
  echo "GoReleaser release body regression did not append its renderer newline" >&2
  exit 1
}
head -c "$notes_size" "$RELEASE_TEST_LOG.body" > "$tmp/release-body-prefix.md"
cmp -s "$RELEASE_TEST_LOG.notes" "$tmp/release-body-prefix.md" || {
  echo "GoReleaser release body regression changed tagged release notes" >&2
  exit 1
}
printf '\n' > "$tmp/expected-renderer-newline"
tail -c 1 "$RELEASE_TEST_LOG.body" | cmp -s - "$tmp/expected-renderer-newline" || {
  echo "GoReleaser release body regression appended a non-newline byte" >&2
  exit 1
}
if grep -Eq 'gh (workflow|release)|--method PATCH|homebrew-tap' "$RELEASE_TEST_LOG"; then
  echo "release-local draft crossed a later serialized gate" >&2
  exit 1
fi

"$tmp/bin/gh" api --hostname github.com 'repos/openclaw/gogcli/releases?per_page=100' |
  jq '.[0]' > "$tmp/draft-release.json"
"$root/scripts/validate-release-record.sh" \
  "$tmp/draft-release.json" v0.3.4 true \
  2222222222222222222222222222222222222222 \
  "$RELEASE_TEST_LOG.body" > "$tmp/release-snapshot.json"
jq '.draft = false | .published_at = "2026-07-09T12:34:56Z"' \
  "$tmp/draft-release.json" > "$tmp/published-release.json"
"$root/scripts/validate-release-record.sh" \
  "$tmp/published-release.json" v0.3.4 false \
  2222222222222222222222222222222222222222 \
  "$RELEASE_TEST_LOG.body" "$tmp/release-snapshot.json" >/dev/null

assert_release_record_rejected() {
  local label=$1 filter=$2
  jq "$filter" "$tmp/draft-release.json" > "$tmp/tampered-release.json"
  if "$root/scripts/validate-release-record.sh" \
    "$tmp/tampered-release.json" v0.3.4 true \
    2222222222222222222222222222222222222222 \
    "$RELEASE_TEST_LOG.body" "$tmp/release-snapshot.json" >/dev/null 2>&1; then
    echo "release record validator accepted tampered $label" >&2
    exit 1
  fi
}

assert_release_record_rejected title '.name = "tampered"'
assert_release_record_rejected body '.body += "tampered"'
assert_release_record_rejected prerelease '.prerelease = true'
assert_release_record_rejected tag '.tag_name = "v9.9.9"'
assert_release_record_rejected target '.target_commitish = "3333333333333333333333333333333333333333"'
assert_release_record_rejected zero-size '.assets[0].size = 0'
assert_release_record_rejected replaced-id '.assets[0].id = 99'
assert_release_record_rejected replaced-digest '.assets[0].digest = ("sha256:" + ("9" * 64))'
assert_release_record_rejected replaced-name '.assets[0].name = "replacement"'
assert_release_record_rejected release-id '.id = 43'

grep -A2 '^release:' "$root/.goreleaser.yaml" | grep -Fq 'draft: true'
grep -A2 '^changelog:' "$root/.goreleaser.yaml" | grep -Fq 'disable: false'
grep -Fq 'name_template: "{{ .Tag }}"' "$root/.goreleaser.yaml"
grep -Fq 'target_commitish: "{{ .Commit }}"' "$root/.goreleaser.yaml"
[[ "$(grep -Fc -- '- CGO_ENABLED=0' "$root/.goreleaser.yaml")" == 1 &&
  "$(grep -Fc -- '- CGO_ENABLED=1' "$root/.goreleaser.yaml")" == 1 ]] || {
  echo "GoReleaser does not isolate non-Darwin and Darwin cgo policy" >&2
  exit 1
}
for go_env in \
  '"GOCACHEPROG="' \
  '"GODEBUG="' \
  'GO111MODULE=on' \
  'GOAUTH=off' \
  'GOENV=off' \
  '"GOEXPERIMENT="' \
  '"GOFLAGS="' \
  'GOFIPS140=off' \
  'GOAMD64=v1' \
  'GOARM64=v8.0' \
  'GOPROXY=https://proxy.golang.org' \
  '"GOROOT="' \
  'GOSUMDB=sum.golang.org' \
  'GOTOOLCHAIN=local' \
  'GOWORK=off'; do
  [[ "$(grep -Fc -- "- $go_env" "$root/.goreleaser.yaml")" == 2 ]] || {
    echo "GoReleaser is missing sanitized Go environment entry: $go_env" >&2
    exit 1
  }
done
prepare_go_function=$(sed -n '/^prepare_official_go_environment()/,/^}/p' "$root/scripts/release-local")
official_bridge_function=$(sed -n '/^run_official_goreleaser()/,/^}/p' "$root/scripts/release-local")
download_tools_function=$(sed -n '/^download_pinned_release_archive()/,/^}/p' "$root/scripts/release-local")
pin_tools_function=$(sed -n '/^require_pinned_release_tools_unchanged()/,/^}/p' "$root/scripts/release-local")
grep -Fq 'ambient Go build control is forbidden' <<<"$prepare_go_function"
grep -Fq "https://go.dev/dl/\$go_archive_name" <<<"$prepare_go_function"
grep -Fq 'goreleaser/releases/download/v2.17.0/$goreleaser_archive_name' <<<"$prepare_go_function"
for pinned_archive_sha in \
  efb87ff28af9a188d0536ef5d42e63dd52ba8263cd7344a993cc48dd11dedb6a \
  6231d8d3b8f5552ec6cbf6d685bdd5482e1e703214b120e89b3bf0d7bf1ef725 \
  58912a80159199c0fd5c8484e4c868bf87414129655d6d87cd1cd84ee645736c \
  f37e89fb844ddfd23cffb97e30d91f972c42da68232a676bfba2beacea300543; do
  grep -Fq "$pinned_archive_sha" <<<"$prepare_go_function" || {
    echo "official producer is missing pinned archive checksum: $pinned_archive_sha" >&2
    exit 1
  }
done
grep -Fq "/usr/bin/env -i /usr/bin/curl" <<<"$download_tools_function"
grep -Fq '/usr/bin/shasum -a 256' <<<"$download_tools_function"
grep -Fq '/usr/bin/shasum -a 256' <<<"$pin_tools_function"
if grep -Eq 'command -v (go|goreleaser)' <<<"$prepare_go_function"; then
  echo "official producer still trusts ambient Go or GoReleaser" >&2
  exit 1
fi
printf '%s\n' "$pin_tools_function" > "$tmp/require-pinned-release-tools.sh"
mkdir -p "$tmp/pinned-tools-probe"
for pinned_file in go-archive goreleaser-archive go-bin goreleaser-bin credential-bridge; do
  printf '%s\n' "$pinned_file" > "$tmp/pinned-tools-probe/$pinned_file"
done
cat > "$tmp/pinned-tools-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
release_go_archive=$PIN_TOOLS_ROOT/go-archive
release_goreleaser_archive=$PIN_TOOLS_ROOT/goreleaser-archive
release_go_bin=$PIN_TOOLS_ROOT/go-bin
release_goreleaser_bin=$PIN_TOOLS_ROOT/goreleaser-bin
release_bridge_script=$PIN_TOOLS_ROOT/credential-bridge
release_go_archive_sha256=$(shasum -a 256 "$release_go_archive" | awk '{print $1}')
release_goreleaser_archive_sha256=$(shasum -a 256 "$release_goreleaser_archive" | awk '{print $1}')
release_go_binary_sha256=$(shasum -a 256 "$release_go_bin" | awk '{print $1}')
release_goreleaser_binary_sha256=$(shasum -a 256 "$release_goreleaser_bin" | awk '{print $1}')
release_bridge_sha256=$(shasum -a 256 "$release_bridge_script" | awk '{print $1}')
# shellcheck source=/dev/null
source "$PIN_TOOLS_FUNCTION"
require_pinned_release_tools_unchanged
[[ "${PIN_TOOLS_MODE:-exact}" != tampered ]] || printf '%s\n' changed >> "$release_goreleaser_bin"
[[ "${PIN_TOOLS_MODE:-exact}" != bridge-tampered ]] || printf '%s\n' changed >> "$release_bridge_script"
require_pinned_release_tools_unchanged
EOF
chmod +x "$tmp/pinned-tools-probe.sh"
PIN_TOOLS_ROOT="$tmp/pinned-tools-probe" \
  PIN_TOOLS_FUNCTION="$tmp/require-pinned-release-tools.sh" \
  "$tmp/pinned-tools-probe.sh"
for pin_mode in tampered bridge-tampered; do
  if PIN_TOOLS_MODE=$pin_mode \
    PIN_TOOLS_ROOT="$tmp/pinned-tools-probe" \
    PIN_TOOLS_FUNCTION="$tmp/require-pinned-release-tools.sh" \
    "$tmp/pinned-tools-probe.sh" >/dev/null 2>&1; then
    echo "official producer accepted a changed pinned artifact: $pin_mode" >&2
    exit 1
  fi
done

official_bridge="$root/scripts/run-official-goreleaser.py"
[[ -f "$official_bridge" && ! -L "$official_bridge" ]] || {
  echo "official credential bridge is missing or indirect" >&2
  exit 1
}
cat > "$tmp/bridge-target" <<'EOF'
#!/bin/bash
set -euo pipefail
if declare -F codesign >/dev/null; then
  codesign
  exit 1
fi
[[ -z "${ATTACKER_SECRET+x}" && -z "${PYTHONPATH+x}" &&
  -z "${MAC_RELEASE_OP_FIELDS+x}" ]]
[[ "${CODESIGN_IDENTITY:-}" == 'Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)' ]]
[[ "${CODESIGN_KEYCHAIN:-}" == /tmp/test-release.keychain-db ]]
[[ "${MAC_RELEASE_CODESIGN_KEYCHAIN:-}" == "$CODESIGN_KEYCHAIN" ]]
[[ "${NOTARYTOOL_KEYCHAIN_PROFILE:-}" == test-profile ]]
if [[ "${EXPECT_GITHUB_TOKEN:-}" == 1 ]]; then
  [[ "${GITHUB_TOKEN:-}" == test-token ]]
else
  [[ -z "${GITHUB_TOKEN+x}" ]]
fi
[[ "$#" == 2 && "$1" == release && "$2" == --draft ]]
printf '%s\n' passed > "${BRIDGE_RESULT:?}"
EOF
chmod 700 "$tmp/bridge-target"
bridge_credentials=(
  MAC_RELEASE_OP_FIELDS=NOTARYTOOL_KEYCHAIN_PROFILE
  'CODESIGN_IDENTITY=Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)'
  CODESIGN_KEYCHAIN=/tmp/test-release.keychain-db
  MAC_RELEASE_CODESIGN_KEYCHAIN=/tmp/test-release.keychain-db
  NOTARYTOOL_KEYCHAIN_PROFILE=test-profile
)
/usr/bin/env -i \
  "${bridge_credentials[@]}" \
  GITHUB_TOKEN=test-token \
  ATTACKER_SECRET=must-not-cross \
  PYTHONPATH="$tmp/attacker-python" \
  'BASH_FUNC_codesign%%=() { printf attacked > "$BRIDGE_ATTACK_LOG"; }' \
  /usr/bin/python3 -I "$official_bridge" goreleaser \
  --require-github-token \
  --env PATH=/usr/bin:/bin \
  --env HOME="$tmp/bridge-home" \
  --env EXPECT_GITHUB_TOKEN=1 \
  --env BRIDGE_RESULT="$tmp/bridge-result" \
  --env BRIDGE_ATTACK_LOG="$tmp/bridge-attack" \
  -- "$tmp/bridge-target" release --draft
[[ "$(<"$tmp/bridge-result")" == passed && ! -e "$tmp/bridge-attack" ]] || {
  echo "official credential bridge imported ambient executable state" >&2
  exit 1
}
rm -f "$tmp/bridge-result"
/usr/bin/env -i \
  "${bridge_credentials[@]}" \
  GITHUB_TOKEN=must-not-cross \
  /usr/bin/python3 -I "$official_bridge" goreleaser \
  --env PATH=/usr/bin:/bin \
  --env HOME="$tmp/bridge-home" \
  --env EXPECT_GITHUB_TOKEN=0 \
  --env BRIDGE_RESULT="$tmp/bridge-result" \
  --env BRIDGE_ATTACK_LOG="$tmp/bridge-attack" \
  -- "$tmp/bridge-target" release --draft
[[ "$(<"$tmp/bridge-result")" == passed ]] || {
  echo "pilot credential bridge did not remain token-free" >&2
  exit 1
}
if /usr/bin/env -i \
  "${bridge_credentials[@]}" \
  MAC_RELEASE_OP_FIELDS='NOTARYTOOL_KEYCHAIN_PROFILE EXTRA_SECRET' \
  GITHUB_TOKEN=test-token \
  /usr/bin/python3 -I "$official_bridge" goreleaser --require-github-token \
  --env PATH=/usr/bin:/bin -- "$tmp/bridge-target" release --draft \
  >/dev/null 2>&1; then
  echo "official credential bridge accepted an unlisted package secret" >&2
  exit 1
fi
if /usr/bin/env -i \
  "${bridge_credentials[@]}" \
  /usr/bin/python3 -I "$official_bridge" goreleaser --require-github-token \
  --env PATH=/usr/bin:/bin -- "$tmp/bridge-target" release --draft \
  >/dev/null 2>&1; then
  echo "official credential bridge accepted a missing draft token" >&2
  exit 1
fi

helper_manifest="$tmp/helper-bridge-manifest"
: > "$helper_manifest"
cat > "$tmp/helper-bridge-target" <<'EOF'
#!/bin/bash
set -euo pipefail
if declare -F security >/dev/null || [[ -n "${ATTACKER_SECRET+x}" || -n "${PYTHONPATH+x}" ]]; then
  printf '%s\n' attacked > "$MAC_RELEASE_MANIFEST.attack"
  exit 1
fi
[[ "$PATH" == /opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin ]]
[[ "$HOME" == /* && "$TMPDIR" == /tmp && "$LC_ALL" == C && "$TZ" == UTC ]]
if [[ "$1" == draft ]]; then
  [[ "${GITHUB_TOKEN:-}" == test-token ]]
else
  [[ -z "${GITHUB_TOKEN+x}" ]]
fi
printf '%s\n' passed > "$MAC_RELEASE_MANIFEST.result"
EOF
chmod 700 "$tmp/helper-bridge-target"
/usr/bin/env -i \
  GITHUB_TOKEN=test-token \
  ATTACKER_SECRET=must-not-cross \
  PYTHONPATH="$tmp/attacker-python" \
  'BASH_FUNC_security%%=() { printf attacked > "$HELPER_ATTACK_LOG"; }' \
  /usr/bin/python3 -I "$official_bridge" helper \
  --manifest "$helper_manifest" --require-github-token \
  -- "$tmp/helper-bridge-target" draft
[[ "$(<"$helper_manifest.result")" == passed && ! -e "$helper_manifest.attack" ]] || {
  echo "official helper bridge imported ambient executable state" >&2
  exit 1
}
rm -f "$helper_manifest.result"
/usr/bin/env -i \
  GITHUB_TOKEN=must-not-cross \
  /usr/bin/python3 -I "$official_bridge" helper \
  --manifest "$helper_manifest" \
  -- "$tmp/helper-bridge-target" pilot
[[ "$(<"$helper_manifest.result")" == passed ]] || {
  echo "pilot helper bridge did not remain token-free" >&2
  exit 1
}

for build_control in CC CXX CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_LDFLAGS PKG_CONFIG SDKROOT; do
  grep -Fxq "    $build_control" <<<"$prepare_go_function" || {
    echo "official build does not reject ambient $build_control" >&2
    exit 1
  }
done
for pinned_control in \
  '"AR=/usr/bin/ar"' \
  '"CC=/usr/bin/clang"' \
  '"CXX=/usr/bin/clang++"' \
  '"CGO_CFLAGS=-O2 -g"' \
  '"CGO_CPPFLAGS="' \
  '"CGO_CXXFLAGS=-O2 -g"' \
  '"CGO_LDFLAGS=-O2 -g"' \
  '"PKG_CONFIG=/usr/bin/false"'; do
  grep -Fq "$pinned_control" <<<"$prepare_go_function" || {
    echo "official build does not pin cgo control: $pinned_control" >&2
    exit 1
  }
done
workflow_release_tests=$(grep -R -F 'test-release-local.sh' "$root/.github/workflows")
[[ -n "$workflow_release_tests" ]]
if grep -Fv 'env -u GOTOOLCHAIN ./scripts/test-release-local.sh' <<<"$workflow_release_tests" | grep -q .; then
  echo "workflow invokes the local release test with ambient Go build controls" >&2
  exit 1
fi
grep -Fq 'go version go1.26.5 darwin/$expected_goarch' <<<"$prepare_go_function"
grep -Fq 'GoReleaser 2.17.0' <<<"$prepare_go_function"
grep -Fq 'GO111MODULE' <<<"$prepare_go_function"
grep -Fq 'GOAUTH' <<<"$prepare_go_function"
grep -Fq 'GOCACHEPROG' <<<"$prepare_go_function"
grep -Fq 'GOFIPS140' <<<"$prepare_go_function"
grep -Fq '"GOFLAGS="' <<<"$prepare_go_function"
grep -Fq 'GOROOT' <<<"$prepare_go_function"
grep -Fq '"GOWORK=off"' <<<"$prepare_go_function"
grep -Fq '"GOG_PILOT_VERSION=$version"' "$root/scripts/release-local"
grep -Fq '"GOG_COMMIT=${trusted_tag_commit:0:8}"' "$root/scripts/release-local"
grep -Fq '"GOG_COMMIT=${revision:0:8}"' "$root/scripts/release-local"
grep -Fq '"GORELEASER_CURRENT_TAG=$tag"' "$root/scripts/release-local"
grep -Fq 'goreleaser_bridge+=(-- "$release_goreleaser_bin" "$@")' <<<"$official_bridge_function"
grep -Fq '/usr/bin/python3' <<<"$official_bridge_function"
grep -Fq '"$release_bridge_script"' <<<"$official_bridge_function"
grep -Fq '/bin/cp "$root/scripts/run-official-goreleaser.py" "$release_bridge_script"' \
  <<<"$prepare_go_function"
grep -Fq 'helper_bridge+=(-- "$release_helper" codesign-run --with-package-secrets --)' \
  <<<"$official_bridge_function"
if grep -Fq '/usr/bin/env "${official_go_env[@]}"' "$root/scripts/release-local"; then
  echo "official producer still inherits the credential-bound environment" >&2
  exit 1
fi
grep -Fq -- '--snapshot --clean --skip=publish --parallelism=2' "$root/scripts/release-local"
grep -Fq "check-release-verifier.sh\" \"\$tag\" true \"\$trusted_tag_commit\"" "$root/scripts/release-local"
grep -Fq "dispatch_verifier \"\$tag\" false" "$root/scripts/release-local"
grep -Fq -- "-c \"gpg.ssh.allowedSignersFile=\$allowed_signers_file\"" "$root/scripts/verify-release-tag.sh"
grep -Fq -- '-c gpg.ssh.program=/usr/bin/ssh-keygen' "$root/scripts/verify-release-tag.sh"
grep -Fq '[[ -x /usr/bin/ssh-keygen ]]' "$root/scripts/verify-release-tag.sh"
grep -Fq "verify-release-tag.sh\" \"\$tag\"" "$root/scripts/release-local"
grep -Fq "git show \"\$trusted_tag_object:CHANGELOG.md\"" "$root/scripts/release-local"
grep -Fq "git show \"\$trusted_tag_object:internal/cmd/VERSION\"" "$root/scripts/release-local"
grep -Fq 'must have one dated changelog heading' "$root/scripts/release-local"
tag_checks_function=$(sed -n '/^require_successful_tag_checks()/,/^}/p' "$root/scripts/release-local")
grep -Fq 'ci.yml CI' <<<"$tag_checks_function"
grep -Fq 'release.yml release-check' <<<"$tag_checks_function"
for function_name in draft_release dispatch_verifier publish_release update_homebrew; do
  function_body=$(sed -n "/^${function_name}()/,/^}/p" "$root/scripts/release-local")
  grep -Fq "require_remote_signed_tag \"\$tag\"" <<<"$function_body" || {
    echo "release-local does not revalidate the remote signed tag in $function_name" >&2
    exit 1
  }
done
pilot_function=$(sed -n '/^pilot_release()/,/^}/p' "$root/scripts/release-local")
grep -Fq require_current_default_head <<<"$pilot_function"
grep -Fq 'prepare_default_source "$source_dir"' <<<"$pilot_function"
grep -Fq 'cd "$source_dir"' <<<"$pilot_function"
grep -Fq 'run_official_goreleaser "$release_helper" "$release_manifest" false' <<<"$pilot_function"
[[ "$(grep -Fc 'require_pinned_release_tools_unchanged' <<<"$pilot_function")" == 2 ]] || {
  echo "pilot does not pin producer tools immediately around the credential boundary" >&2
  exit 1
}
grep -Fq '"$source_dir/scripts/verify-release-assets.sh"' <<<"$pilot_function"
grep -Fq '"$source_dir/scripts/verify-macos-binary.sh"' <<<"$pilot_function"
default_head_function=$(sed -n '/^require_current_default_head()/,/^}/p' "$root/scripts/release-local")
grep -Fq "'.protected'" <<<"$default_head_function"
grep -Fq '"$api_default_commit" == "$trusted_default_commit"' <<<"$default_head_function"
grep -Fq 'status --porcelain --untracked-files=all' <<<"$default_head_function"
grep -Fq 'ls-files -v' <<<"$default_head_function"
grep -Fq 'refs/replace/' <<<"$default_head_function"
grep -Fq 'require_no_git_grafts "$root" "local release"' <<<"$default_head_function"
graft_function=$(sed -n '/^require_no_git_grafts()/,/^}/p' "$root/scripts/release-local")
grep -Fq 'rev-parse --git-path info/grafts' <<<"$graft_function"
grep -Fq '[[ ! -e "$graft_path" && ! -L "$graft_path" ]]' <<<"$graft_function"
graft_check_line=$(grep -nF 'require_no_git_grafts "$root" "local release"' <<<"$default_head_function" | cut -d: -f1)
default_ref_line=$(grep -nF 'default_ref=$(trusted_git ls-remote' <<<"$default_head_function" | cut -d: -f1)
[[ "$graft_check_line" -lt "$default_ref_line" ]] || {
  echo "release-local checks Git grafts after trusting default-branch metadata" >&2
  exit 1
}
grep -Fq 'rev-parse --git-path info/grafts' "$root/scripts/verify-release-tag.sh"
draft_function=$(sed -n '/^draft_release()/,/^}/p' "$root/scripts/release-local")
grep -Fq 'prepare_rebuild_source "$tag" "$source_dir"' <<<"$draft_function"
grep -Fq 'cd "$source_dir"' <<<"$draft_function"
grep -Fq 'run_official_goreleaser "$release_helper" "$release_manifest" true' <<<"$draft_function"
[[ "$(grep -Fc 'require_pinned_release_tools_unchanged' <<<"$draft_function")" == 2 ]] || {
  echo "draft does not pin producer tools immediately around the credential boundary" >&2
  exit 1
}
for producer_function in "$draft_function" "$pilot_function"; do
  source_line=$(grep -nF 'prepare_' <<<"$producer_function" | grep -E 'rebuild_source|default_source' | tail -1 | cut -d: -f1)
  tools_line=$(grep -nF 'prepare_official_go_environment' <<<"$producer_function" | cut -d: -f1)
  pin_line=$(grep -nF 'require_pinned_release_tools_unchanged' <<<"$producer_function" | head -1 | cut -d: -f1)
  credentials_line=$(grep -nF 'run_official_goreleaser ' <<<"$producer_function" | cut -d: -f1)
  [[ "$source_line" -lt "$tools_line" && "$tools_line" -lt "$pin_line" &&
    "$pin_line" -lt "$credentials_line" ]] || {
    echo "official producer pinning does not precede credential access" >&2
    exit 1
  }
done
remote_source_function=$(sed -n '/^prepare_remote_source()/,/^}/p' "$root/scripts/release-local")
grep -Fq 'clone --quiet --no-checkout --no-local --no-tags' <<<"$remote_source_function"
grep -Fq '"https://github.com/$repository.git"' <<<"$remote_source_function"
if grep -Fq '"$root" "$destination"' <<<"$remote_source_function"; then
  echo "release-local still clones official source from the mutable local repository" >&2
  exit 1
fi
publish_function=$(sed -n '/^publish_release()/,/^}/p' "$root/scripts/release-local")
[[ "$(grep -Fc 'require_successful_tag_checks "$tag"' <<<"$publish_function")" -ge 3 ]] || {
  echo "release-local does not recheck exact-tag CI immediately before publication" >&2
  exit 1
}
grep -Fq 'if [[ "$release_state" == published ]]' <<<"$publish_function"
grep -Fq 'was already published with the exact record and a fresh native verifier' <<<"$publish_function"
grep -Fq 'dispatch_verifier "$tag" false' <<<"$publish_function"
[[ "$(grep -Fc "verify_release_notes \"\$tag\"" <<<"$publish_function")" -ge 2 ]] || {
  echo "release-local does not verify notes before and after publication" >&2
  exit 1
}
grep -Fq 'publish_payload="$work_dir/publish-payload.json"' <<<"$publish_function"
grep -Fq -- '--input "$publish_payload"' <<<"$publish_function"
if grep -Fq -- '-F draft=false' <<<"$publish_function"; then
  echo "release-local still uses a partial publication PATCH that can detach the release tag" >&2
  exit 1
fi
for field in tag_name target_commitish name body draft prerelease; do
  grep -Fq "$field" <<<"$publish_function" || {
    echo "release-local publication payload omits $field" >&2
    exit 1
  }
done
[[ "$(grep -Fc 'github_api "repos/$repository/releases/$accepted_release_id"' <<<"$publish_function")" == 2 ]] || {
  echo "release-local does not GET the numeric release ID before and after publication" >&2
  exit 1
}
[[ "$(grep -Fc 'validate_release_record_file' <<<"$publish_function")" -ge 3 ]] || {
  echo "release-local does not fully validate numeric release records around publication" >&2
  exit 1
}
prepatch_line=$(grep -nF '> "$prepatch_record"' <<<"$publish_function" | cut -d: -f1)
patch_line=$(grep -nF -- '--method PATCH' <<<"$publish_function" | cut -d: -f1)
published_get_line=$(grep -nF '> "$published_record"' <<<"$publish_function" | cut -d: -f1)
[[ "$prepatch_line" -lt "$patch_line" && "$patch_line" -lt "$published_get_line" ]] || {
  echo "release-local publication revalidation order is unsafe" >&2
  exit 1
}
publication_path=$(sed -n '/^  release_record "$tag" true/,$p' <<<"$publish_function")
accepted_tag_line=$(grep -nF 'accepted_tag_object=$trusted_tag_object' <<<"$publication_path" | cut -d: -f1)
prepatch_tag_fetch_line=$(grep -nF 'require_remote_signed_tag "$tag" false' <<<"$publication_path" | head -2 | tail -1 | cut -d: -f1)
prepatch_tag_pin_line=$(grep -nF '"$trusted_tag_object" == "$accepted_tag_object"' <<<"$publication_path" | head -1 | cut -d: -f1)
[[ "$accepted_tag_line" -lt "$prepatch_tag_fetch_line" &&
  "$prepatch_tag_fetch_line" -lt "$prepatch_tag_pin_line" &&
  "$prepatch_tag_pin_line" -lt "$prepatch_line" ]] || {
  echo "release-local can publish after the verifier-accepted tag object changes" >&2
  exit 1
}
final_record_line=$(grep -nF 'release_record "$tag" false "$accepted_snapshot"' <<<"$publication_path" | tail -1 | cut -d: -f1)
final_tag_fetch_line=$(grep -nF 'require_remote_signed_tag "$tag" false' <<<"$publication_path" | tail -1 | cut -d: -f1)
final_tag_pin_line=$(grep -nF '"$trusted_tag_object" == "$accepted_tag_object"' <<<"$publication_path" | tail -1 | cut -d: -f1)
publish_success_line=$(grep -nF 'echo "release: published $tag' <<<"$publication_path" | cut -d: -f1)
[[ "$(grep -Fc '"$trusted_tag_object" == "$accepted_tag_object"' <<<"$publication_path")" == 2 &&
  "$final_record_line" -lt "$final_tag_fetch_line" &&
  "$final_tag_fetch_line" -lt "$final_tag_pin_line" &&
  "$final_tag_pin_line" -lt "$publish_success_line" ]] || {
  echo "release-local can report success after a tag move during publication" >&2
  exit 1
}

mkdir -p "$tmp/publish-probe-root/scripts" "$tmp/publish-probe-work"
printf '%s\n' "$publish_function" > "$tmp/publish-release-function.sh"
printf '%s\n' '{}' > "$tmp/publish-probe-work/release-snapshot.json"
cat > "$tmp/publish-probe-root/scripts/check-release-verifier.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' 42
EOF
chmod +x "$tmp/publish-probe-root/scripts/check-release-verifier.sh"
cat > "$tmp/publish-tag-move-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
root=$PUBLISH_PROBE_ROOT
work_dir=$PUBLISH_PROBE_WORK
repository=openclaw/gogcli
trusted_tag_object=1111111111111111111111111111111111111111
trusted_tag_commit=2222222222222222222222222222222222222222
release_id=42
release_snapshot_file=$work_dir/release-snapshot.json
release_json='[]'
notes_file=$work_dir/notes.md
release_body_file=$work_dir/release-body.md
tag_check_count=0
release_state_calls=0
printf '%s\n' 'Tagged publish probe notes' > "$notes_file"
{
  cat "$notes_file"
  printf '\n'
} > "$release_body_file"

validate_tag() { :; }
require_tools() { :; }
verify_release_notes() { :; }
require_successful_tag_checks() { :; }
extract_notes() {
  notes_file=$work_dir/notes.md
  release_body_file=$work_dir/release-body.md
}
validate_release_record_file() { :; }
release_state_for_tag() {
  release_state_calls=$((release_state_calls + 1))
  if [[ "$release_state_calls" -ge 3 ]]; then
    release_state=published
  else
    release_state=draft
  fi
}
release_record() {
  release_id=42
  release_snapshot_file=$work_dir/release-snapshot.json
}
require_remote_signed_tag() {
  tag_check_count=$((tag_check_count + 1))
  trusted_tag_object=1111111111111111111111111111111111111111
  trusted_tag_commit=2222222222222222222222222222222222222222
  if [[ "$tag_check_count" -eq 4 ]]; then
    trusted_tag_object=3333333333333333333333333333333333333333
  fi
}
gh() {
  printf 'gh %s\n' "$*" >> "$PUBLISH_PROBE_LOG"
  printf '%s\n' '{}'
}
github_api() {
  gh api --hostname github.com "$@"
}

# shellcheck source=/dev/null
source "$PUBLISH_PROBE_FUNCTION"
publish_release v0.3.4
EOF
chmod +x "$tmp/publish-tag-move-probe.sh"
: > "$tmp/publish-probe.log"
if PUBLISH_PROBE_ROOT="$tmp/publish-probe-root" \
  PUBLISH_PROBE_WORK="$tmp/publish-probe-work" \
  PUBLISH_PROBE_LOG="$tmp/publish-probe.log" \
  PUBLISH_PROBE_FUNCTION="$tmp/publish-release-function.sh" \
  "$tmp/publish-tag-move-probe.sh" > "$tmp/publish-probe-output.log" 2>&1; then
  echo "release-local reported publication success after a deterministic live tag move" >&2
  exit 1
fi
grep -Fq -- '--method PATCH' "$tmp/publish-probe.log" || {
  echo "publication tag-move regression did not reach the PATCH window" >&2
  exit 1
}
grep -Fq -- '--input' "$tmp/publish-probe.log" || {
  echo "publication tag-move regression did not use a JSON payload" >&2
  exit 1
}
jq -e \
  --arg tag v0.3.4 \
  --arg target_commitish 2222222222222222222222222222222222222222 \
  --rawfile body "$tmp/publish-probe-work/release-body.md" '
    keys == ["body", "draft", "name", "prerelease", "tag_name", "target_commitish"] and
    .tag_name == $tag and
    .target_commitish == $target_commitish and
    .name == $tag and
    .body == $body and
    .draft == false and
    .prerelease == false
  ' "$tmp/publish-probe-work/publish-payload.json" >/dev/null || {
  echo "publication regression did not preserve the canonical mutable release record" >&2
  exit 1
}
if grep -Fq 'release: published v0.3.4' "$tmp/publish-probe-output.log"; then
  echo "release-local emitted success after a deterministic live tag move" >&2
  exit 1
fi

cat > "$tmp/publish-recovery-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
root=$PUBLISH_PROBE_ROOT
work_dir=$PUBLISH_PROBE_WORK
repository=openclaw/gogcli
trusted_tag_object=1111111111111111111111111111111111111111
trusted_tag_commit=2222222222222222222222222222222222222222
release_snapshot_file=$work_dir/release-snapshot.json
release_record_calls=0
verifier_calls=0

validate_tag() { :; }
require_tools() { :; }
require_successful_tag_checks() { :; }
verify_release_notes() { :; }
release_state_for_tag() { release_state=published; }
require_remote_signed_tag() {
  trusted_tag_object=1111111111111111111111111111111111111111
  trusted_tag_commit=2222222222222222222222222222222222222222
}
release_record() {
  release_record_calls=$((release_record_calls + 1))
  [[ "$2" == false ]]
  if [[ "$release_record_calls" -eq 2 && "${PUBLISH_RECOVERY_MODE:-exact}" == replaced-before-verifier ]] ||
    [[ "$release_record_calls" -eq 3 && "${PUBLISH_RECOVERY_MODE:-exact}" == replaced-after-verifier ]]; then
    return 1
  fi
  release_snapshot_file=$work_dir/release-snapshot.json
}
dispatch_verifier() {
  [[ "$1" == v0.3.4 && "$2" == false ]]
  verifier_calls=$((verifier_calls + 1))
  printf 'dispatch_verifier %s %s\n' "$1" "$2" >> "$PUBLISH_PROBE_LOG"
}
github_api() {
  printf 'github_api %s\n' "$*" >> "$PUBLISH_PROBE_LOG"
  return 99
}

# shellcheck source=/dev/null
source "$PUBLISH_PROBE_FUNCTION"
publish_release v0.3.4
[[ "$verifier_calls" == 1 && "$release_record_calls" == 3 ]]
EOF
chmod +x "$tmp/publish-recovery-probe.sh"
: > "$tmp/publish-recovery.log"
PUBLISH_PROBE_ROOT="$tmp/publish-probe-root" \
  PUBLISH_PROBE_WORK="$tmp/publish-probe-work" \
  PUBLISH_PROBE_LOG="$tmp/publish-recovery.log" \
  PUBLISH_PROBE_FUNCTION="$tmp/publish-release-function.sh" \
  "$tmp/publish-recovery-probe.sh" > "$tmp/publish-recovery-output.log"
grep -Fq 'was already published with the exact record and a fresh native verifier' \
  "$tmp/publish-recovery-output.log"
[[ "$(<"$tmp/publish-recovery.log")" == 'dispatch_verifier v0.3.4 false' ]] || {
  echo "published recovery skipped or confused its fresh verifier dispatch" >&2
  exit 1
}
for recovery_mode in replaced-before-verifier replaced-after-verifier; do
  : > "$tmp/publish-recovery.log"
  if PUBLISH_RECOVERY_MODE=$recovery_mode \
    PUBLISH_PROBE_ROOT="$tmp/publish-probe-root" \
    PUBLISH_PROBE_WORK="$tmp/publish-probe-work" \
    PUBLISH_PROBE_LOG="$tmp/publish-recovery.log" \
    PUBLISH_PROBE_FUNCTION="$tmp/publish-release-function.sh" \
    "$tmp/publish-recovery-probe.sh" >/dev/null 2>&1; then
    echo "published recovery accepted a replaced release snapshot: $recovery_mode" >&2
    exit 1
  fi
done
homebrew_function=$(sed -n '/^update_homebrew()/,/^}/p' "$root/scripts/release-local")
homebrew_revalidate_function=$(sed -n '/^revalidate_homebrew_source()/,/^}/p' "$root/scripts/release-local")
handoff_revalidate_function=$(sed -n '/^revalidate_homebrew_handoff_snapshot()/,/^}/p' "$root/scripts/release-local")
snapshot_match_function=$(sed -n '/^require_release_snapshot_matches_directory()/,/^}/p' "$root/scripts/release-local")
grep -Fq "verify_release_notes \"\$tag\" published" <<<"$homebrew_function"
[[ "$(grep -Fc "download_and_verify_public_release \\" <<<"$homebrew_function")" == 2 ]] || {
  echo "release-local does not use two independently downloaded public inventories" >&2
  exit 1
}
grep -Fq 'public_dir="$work_dir/public-release-initial"' <<<"$homebrew_function"
grep -Fq 'handoff_dir="$work_dir/public-release-handoff"' <<<"$homebrew_function"
grep -Fq 'shasum -a 256 "$handoff_dir/gogcli_${version}_darwin_amd64.tar.gz"' <<<"$homebrew_function"
[[ "$(grep -Fc 'require_tap_default_at "$trusted_tap_branch" "$observed_tap_commit"' <<<"$homebrew_function")" -ge 3 ]] || {
  echo "release-local does not recheck the protected tap default before dispatch" >&2
  exit 1
}
grep -Fq 'resume_existing_homebrew_result' <<<"$homebrew_function"
grep -Fq 'if [[ "$resume_tap" == false ]]' <<<"$homebrew_function"
grep -Fq 'ensure_homebrew_install_state' <<<"$homebrew_function"
grep -Fq 'resuming the exact recorded Homebrew install proof' <<<"$homebrew_function"
grep -Fq -- '--ref "$trusted_tap_branch"' <<<"$homebrew_function"
grep -Fq "'.workflow_id'" <<<"$homebrew_function"
grep -Fq "'.path'" <<<"$homebrew_function"
grep -Fq 'contents/Formula/gogcli.rb?ref=$completed_tap_commit' <<<"$homebrew_function"
grep -Fq 'env -i "${homebrew_env[@]}" "$brew_bin" info --json=v2' <<<"$homebrew_function"
grep -Fq 'env -i "${homebrew_env[@]}" "$brew_bin" install --formula' <<<"$homebrew_function"
grep -Fq 'env -i "${homebrew_env[@]}" "$brew_bin" test gogcli' <<<"$homebrew_function"
grep -Fq 'cmp -s "$handoff_candidate" "$installed_binary"' <<<"$homebrew_function"
grep -Fq '"$installed_binary" "$native_arch" "$version" static' <<<"$homebrew_function"
grep -Fq 'assert_trusted_release_helpers_clean' <<<"$homebrew_function"
install_line=$(grep -nF '"$brew_bin" install --formula' <<<"$homebrew_function" | cut -d: -f1)
installed_cmp_line=$(grep -nF 'cmp -s "$handoff_candidate" "$installed_binary"' <<<"$homebrew_function" | cut -d: -f1)
installed_static_line=$(grep -nF '"$installed_binary" "$native_arch" "$version" static' <<<"$homebrew_function" | cut -d: -f1)
brew_test_line=$(grep -nF '"$brew_bin" test gogcli' <<<"$homebrew_function" | cut -d: -f1)
final_tap_line=$(grep -nF 'require_tap_default_at "$trusted_tap_branch" "$completed_tap_commit"' <<<"$homebrew_function" | cut -d: -f1)
homebrew_success_line=$(grep -nF 'echo "release: Homebrew formula and clean install passed' <<<"$homebrew_function" | cut -d: -f1)
[[ "$install_line" -lt "$installed_cmp_line" &&
  "$installed_cmp_line" -lt "$installed_static_line" &&
  "$installed_static_line" -lt "$brew_test_line" &&
  "$brew_test_line" -lt "$final_tap_line" &&
  "$final_tap_line" -lt "$homebrew_success_line" ]] || {
  echo "Homebrew candidate executes before installed-byte and signature verification" >&2
  exit 1
}
grep -Fq 'accepted_release_snapshot="$work_dir/homebrew-handoff-release-snapshot.json"' <<<"$homebrew_function"
grep -Fq 'cp "$release_snapshot_file" "$accepted_release_snapshot"' <<<"$homebrew_function"
grep -Fq 'verifier_release_snapshot="$work_dir/homebrew-pre-verifier-release-snapshot.json"' <<<"$homebrew_function"
pre_verifier_snapshot_line=$(grep -nF 'cp "$release_snapshot_file" "$verifier_release_snapshot"' <<<"$homebrew_function" | cut -d: -f1)
final_verifier_line=$(grep -nF 'check-release-verifier.sh' <<<"$homebrew_function" | tail -1 | cut -d: -f1)
post_verifier_record_line=$(grep -nF 'revalidate_homebrew_handoff_snapshot "$tag" "$verifier_release_snapshot"' <<<"$homebrew_function" | cut -d: -f1)
accepted_snapshot_line=$(grep -nF 'cp "$release_snapshot_file" "$accepted_release_snapshot"' <<<"$homebrew_function" | cut -d: -f1)
snapshot_bind_line=$(grep -nF 'require_release_snapshot_matches_directory' <<<"$homebrew_function" | cut -d: -f1)
handoff_hash_line=$(grep -nF 'darwin_amd64_sha256=$(shasum -a 256 "$handoff_dir/' <<<"$homebrew_function" | cut -d: -f1)
tap_dispatch_line=$(grep -nF 'gh workflow run update-formula.yml' <<<"$homebrew_function" | cut -d: -f1)
[[ "$pre_verifier_snapshot_line" -lt "$final_verifier_line" &&
  "$final_verifier_line" -lt "$post_verifier_record_line" &&
  "$post_verifier_record_line" -lt "$accepted_snapshot_line" &&
  "$accepted_snapshot_line" -lt "$snapshot_bind_line" &&
  "$snapshot_bind_line" -lt "$handoff_hash_line" &&
  "$handoff_hash_line" -lt "$tap_dispatch_line" ]] || {
  echo "Homebrew hashes are not derived from a post-verifier stable release snapshot" >&2
  exit 1
}
grep -Fq 'release_record "$tag" false "$expected_snapshot"' <<<"$handoff_revalidate_function"
grep -Fq 'verify_release_notes "$tag" published' <<<"$handoff_revalidate_function"
grep -Fq 'expected_title="Update gogcli for ${tag} (request-id=${request_id}; source-tag-object=${accepted_tag_object}; source-tag-commit=${accepted_tag_commit})"' <<<"$homebrew_function"
grep -Fq 'expected_tap_run_path=".github/workflows/update-formula.yml"' <<<"$homebrew_function"
grep -Fq '"$(jq -r '\''.path'\'' <<<"$tap_run_json")" == "$expected_tap_run_path"' <<<"$homebrew_function"
cat > "$tmp/live-tap-run.json" <<'EOF'
{"id":29010348667,"path":".github/workflows/update-formula.yml","head_branch":"main","head_sha":"ea346bb1b7b92cd3183b878c8c9c4d5a0f9acf92","workflow_id":220664022}
EOF
jq -e '
  .path == ".github/workflows/update-formula.yml" and
  .head_branch == "main" and
  .head_sha == "ea346bb1b7b92cd3183b878c8c9c4d5a0f9acf92" and
  .workflow_id == 220664022
' "$tmp/live-tap-run.json" >/dev/null || {
  echo "live Homebrew workflow-run record shape was rejected" >&2
  exit 1
}
jq '.path += "@main"' "$tmp/live-tap-run.json" > "$tmp/suffixed-tap-run.json"
if jq -e '.path == ".github/workflows/update-formula.yml"' \
  "$tmp/suffixed-tap-run.json" >/dev/null; then
  echo "documentation-example Homebrew workflow path suffix was accepted" >&2
  exit 1
fi
grep -Fq "'gogcli: update formula for %s\\n\\nSource-Repository: %s\\nSource-Tag-Object: %s\\nSource-Tag-Commit: %s\\nRequest-ID: %s'" <<<"$homebrew_function"
grep -Fq 'repos/openclaw/homebrew-tap/git/commits/$completed_tap_commit' <<<"$homebrew_function"
grep -Fq "'.parents[0].sha // empty'" <<<"$homebrew_function"

brew_test_line=$(grep -nF '"$brew_bin" test gogcli' <<<"$homebrew_function" | cut -d: -f1)
post_test_helper_line=$(grep -nF 'assert_trusted_release_helpers_clean' <<<"$homebrew_function" | tail -1 | cut -d: -f1)
post_test_release_line=$(grep -nF 'revalidate_homebrew_source' <<<"$homebrew_function" | tail -1 | cut -d: -f1)
final_tap_line=$(grep -nF 'require_tap_default_at "$trusted_tap_branch" "$completed_tap_commit"' <<<"$homebrew_function" | cut -d: -f1)
homebrew_success_line=$(grep -nF 'echo "release: Homebrew formula and clean install passed' <<<"$homebrew_function" | cut -d: -f1)
[[ "$brew_test_line" -lt "$post_test_helper_line" &&
  "$post_test_helper_line" -lt "$post_test_release_line" &&
  "$post_test_release_line" -lt "$final_tap_line" &&
  "$final_tap_line" -lt "$homebrew_success_line" ]] || {
  echo "Homebrew closeout does not revalidate exact source state after candidate execution" >&2
  exit 1
}
grep -Fq '"$trusted_tag_object" == "$expected_tag_object"' <<<"$homebrew_revalidate_function"
grep -Fq 'release_record "$tag" false "$expected_release_snapshot"' <<<"$homebrew_revalidate_function"
grep -Fq '"$tag" false "$expected_tag_commit" "$expected_tag_object"' <<<"$homebrew_revalidate_function"

mkdir -p "$tmp/homebrew-closeout-probe-root/scripts"
cat > "$tmp/homebrew-closeout-probe-root/scripts/check-release-verifier.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' 42
EOF
chmod +x "$tmp/homebrew-closeout-probe-root/scripts/check-release-verifier.sh"
printf '%s\n' "$homebrew_revalidate_function" > "$tmp/revalidate-homebrew-source.sh"
cat > "$tmp/homebrew-closeout-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
root=$HOMEBREW_CLOSEOUT_PROBE_ROOT
trusted_tag_object=
trusted_tag_commit=
verifier_run=
require_remote_signed_tag() {
  trusted_tag_object=1111111111111111111111111111111111111111
  trusted_tag_commit=2222222222222222222222222222222222222222
  [[ "${HOMEBREW_CLOSEOUT_TEST_MODE:-exact}" != tag-move ]] || \
    trusted_tag_object=3333333333333333333333333333333333333333
}
release_record() {
  [[ "${3:-}" == "$HOMEBREW_CLOSEOUT_EXPECTED_SNAPSHOT" ]]
  [[ "${HOMEBREW_CLOSEOUT_TEST_MODE:-exact}" != release-move ]]
}
verify_release_notes() { :; }
# shellcheck source=/dev/null
source "$HOMEBREW_CLOSEOUT_FUNCTION"
revalidate_homebrew_source \
  v0.3.4 \
  1111111111111111111111111111111111111111 \
  2222222222222222222222222222222222222222 \
  "$HOMEBREW_CLOSEOUT_EXPECTED_SNAPSHOT" \
  "$HOMEBREW_CLOSEOUT_EXPECTED_HASH"
EOF
chmod +x "$tmp/homebrew-closeout-probe.sh"
printf '%s\n' '{}' > "$tmp/homebrew-accepted-release-snapshot.json"
homebrew_snapshot_hash=$(shasum -a 256 "$tmp/homebrew-accepted-release-snapshot.json" | awk '{print $1}')
for closeout_mode in tag-move release-move; do
  if HOMEBREW_CLOSEOUT_PROBE_ROOT="$tmp/homebrew-closeout-probe-root" \
    HOMEBREW_CLOSEOUT_FUNCTION="$tmp/revalidate-homebrew-source.sh" \
    HOMEBREW_CLOSEOUT_EXPECTED_SNAPSHOT="$tmp/homebrew-accepted-release-snapshot.json" \
    HOMEBREW_CLOSEOUT_EXPECTED_HASH="$homebrew_snapshot_hash" \
    HOMEBREW_CLOSEOUT_TEST_MODE="$closeout_mode" \
    "$tmp/homebrew-closeout-probe.sh" >/dev/null 2>&1; then
    echo "Homebrew closeout accepted live source movement: $closeout_mode" >&2
    exit 1
  fi
done
HOMEBREW_CLOSEOUT_PROBE_ROOT="$tmp/homebrew-closeout-probe-root" \
  HOMEBREW_CLOSEOUT_FUNCTION="$tmp/revalidate-homebrew-source.sh" \
  HOMEBREW_CLOSEOUT_EXPECTED_SNAPSHOT="$tmp/homebrew-accepted-release-snapshot.json" \
  HOMEBREW_CLOSEOUT_EXPECTED_HASH="$homebrew_snapshot_hash" \
  "$tmp/homebrew-closeout-probe.sh"

mkdir -p "$tmp/homebrew-snapshot-a"
snapshot_assets="$tmp/homebrew-snapshot-assets.jsonl"
snapshot_asset_id=1000
: > "$snapshot_assets"
for name in \
  checksums.txt \
  gogcli_0.3.4_darwin_amd64.tar.gz \
  gogcli_0.3.4_darwin_arm64.tar.gz \
  gogcli_0.3.4_linux_amd64.tar.gz \
  gogcli_0.3.4_linux_arm64.tar.gz \
  gogcli_0.3.4_windows_amd64.zip \
  gogcli_0.3.4_windows_arm64.zip; do
  printf 'A:%s\n' "$name" > "$tmp/homebrew-snapshot-a/$name"
  size=$(wc -c < "$tmp/homebrew-snapshot-a/$name" | tr -d ' ')
  digest=$(shasum -a 256 "$tmp/homebrew-snapshot-a/$name" | awk '{print $1}')
  snapshot_asset_id=$((snapshot_asset_id + 1))
  jq -cn --argjson id "$snapshot_asset_id" --arg name "$name" \
    --argjson size "$size" --arg digest "sha256:$digest" \
    '{id:$id,name:$name,size:$size,digest:$digest}' >> "$snapshot_assets"
done
jq -s '{id:9001,tag_name:"v0.3.4",assets:.}' \
  "$snapshot_assets" > "$tmp/homebrew-snapshot-a.json"
printf '%s\n' "$snapshot_match_function" > "$tmp/require-release-snapshot-matches-directory.sh"
cat > "$tmp/homebrew-snapshot-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
gh() {
  printf 'gh %s\n' "$*" >> "$HOMEBREW_SNAPSHOT_DISPATCH_LOG"
}
# shellcheck source=/dev/null
source "$HOMEBREW_SNAPSHOT_FUNCTION"
require_release_snapshot_matches_directory \
  "$HOMEBREW_SNAPSHOT_FILE" "$HOMEBREW_SNAPSHOT_DIR"
gh workflow run update-formula.yml --repo github.com/openclaw/homebrew-tap
EOF
chmod +x "$tmp/homebrew-snapshot-probe.sh"
: > "$tmp/homebrew-snapshot-dispatch.log"
HOMEBREW_SNAPSHOT_FUNCTION="$tmp/require-release-snapshot-matches-directory.sh" \
  HOMEBREW_SNAPSHOT_FILE="$tmp/homebrew-snapshot-a.json" \
  HOMEBREW_SNAPSHOT_DIR="$tmp/homebrew-snapshot-a" \
  HOMEBREW_SNAPSHOT_DISPATCH_LOG="$tmp/homebrew-snapshot-dispatch.log" \
  "$tmp/homebrew-snapshot-probe.sh"
[[ "$(<"$tmp/homebrew-snapshot-dispatch.log")" == \
  'gh workflow run update-formula.yml --repo github.com/openclaw/homebrew-tap' ]] || {
  echo "matching Homebrew handoff snapshot did not reach dispatch" >&2
  exit 1
}
for snapshot_move in digest size; do
  : > "$tmp/homebrew-snapshot-dispatch.log"
  if [[ "$snapshot_move" == digest ]]; then
    jq '(.assets[] | select(.name == "gogcli_0.3.4_darwin_arm64.tar.gz")) |=
      (.id = 2001 | .digest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")' \
      "$tmp/homebrew-snapshot-a.json" > "$tmp/homebrew-snapshot-b.json"
  else
    jq '(.assets[] | select(.name == "gogcli_0.3.4_darwin_amd64.tar.gz")) |=
      (.id = 2002 | .size += 1)' \
      "$tmp/homebrew-snapshot-a.json" > "$tmp/homebrew-snapshot-b.json"
  fi
  if HOMEBREW_SNAPSHOT_FUNCTION="$tmp/require-release-snapshot-matches-directory.sh" \
    HOMEBREW_SNAPSHOT_FILE="$tmp/homebrew-snapshot-b.json" \
    HOMEBREW_SNAPSHOT_DIR="$tmp/homebrew-snapshot-a" \
    HOMEBREW_SNAPSHOT_DISPATCH_LOG="$tmp/homebrew-snapshot-dispatch.log" \
    "$tmp/homebrew-snapshot-probe.sh" >/dev/null 2>&1; then
    echo "Homebrew handoff A was accepted against replacement snapshot B: $snapshot_move" >&2
    exit 1
  fi
  [[ ! -s "$tmp/homebrew-snapshot-dispatch.log" ]] || {
    echo "Homebrew workflow dispatched after A-to-B asset replacement: $snapshot_move" >&2
    exit 1
  }
done

printf '%s\n' "$handoff_revalidate_function" > "$tmp/revalidate-homebrew-handoff-snapshot.sh"
cat > "$tmp/homebrew-final-check-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
release_snapshot_file="$HOMEBREW_FINAL_CHECK_RELEASE_SNAPSHOT"
live_release_snapshot="$HOMEBREW_FINAL_CHECK_PRE_SNAPSHOT"
release_record() {
  local tag=$1 draft=$2 expected_snapshot=$3
  [[ "$tag" == v0.3.4 && "$draft" == false ]]
  cp "$live_release_snapshot" "$release_snapshot_file"
  cmp "$expected_snapshot" "$release_snapshot_file"
}
verify_release_notes() { :; }
check_release_verifier() {
  [[ "$HOMEBREW_FINAL_CHECK_MODE" == stable ]] || \
    live_release_snapshot="$HOMEBREW_FINAL_CHECK_POST_SNAPSHOT"
}
gh() {
  printf 'gh %s\n' "$*" >> "$HOMEBREW_FINAL_CHECK_DISPATCH_LOG"
}
# shellcheck source=/dev/null
source "$HOMEBREW_FINAL_CHECK_REVALIDATE_FUNCTION"
# shellcheck source=/dev/null
source "$HOMEBREW_SNAPSHOT_FUNCTION"
check_release_verifier
revalidate_homebrew_handoff_snapshot v0.3.4 "$HOMEBREW_FINAL_CHECK_PRE_SNAPSHOT"
cp "$release_snapshot_file" "$HOMEBREW_FINAL_CHECK_ACCEPTED_SNAPSHOT"
require_release_snapshot_matches_directory \
  "$HOMEBREW_FINAL_CHECK_ACCEPTED_SNAPSHOT" "$HOMEBREW_SNAPSHOT_DIR"
gh workflow run update-formula.yml --repo github.com/openclaw/homebrew-tap
EOF
chmod +x "$tmp/homebrew-final-check-probe.sh"
for final_check_mode in stable replaced; do
  : > "$tmp/homebrew-final-check-dispatch.log"
  if HOMEBREW_FINAL_CHECK_MODE="$final_check_mode" \
    HOMEBREW_FINAL_CHECK_PRE_SNAPSHOT="$tmp/homebrew-snapshot-a.json" \
    HOMEBREW_FINAL_CHECK_POST_SNAPSHOT="$tmp/homebrew-snapshot-b.json" \
    HOMEBREW_FINAL_CHECK_RELEASE_SNAPSHOT="$tmp/homebrew-final-check-release.json" \
    HOMEBREW_FINAL_CHECK_ACCEPTED_SNAPSHOT="$tmp/homebrew-final-check-accepted.json" \
    HOMEBREW_FINAL_CHECK_REVALIDATE_FUNCTION="$tmp/revalidate-homebrew-handoff-snapshot.sh" \
    HOMEBREW_SNAPSHOT_FUNCTION="$tmp/require-release-snapshot-matches-directory.sh" \
    HOMEBREW_SNAPSHOT_DIR="$tmp/homebrew-snapshot-a" \
    HOMEBREW_FINAL_CHECK_DISPATCH_LOG="$tmp/homebrew-final-check-dispatch.log" \
    "$tmp/homebrew-final-check-probe.sh" >/dev/null 2>&1; then
    [[ "$final_check_mode" == stable ]] || {
      echo "Homebrew final checker accepted A-to-B release movement" >&2
      exit 1
    }
  elif [[ "$final_check_mode" == stable ]]; then
    echo "stable Homebrew final checker snapshot did not reach dispatch" >&2
    exit 1
  fi
  if [[ "$final_check_mode" == stable ]]; then
    grep -Fxq 'gh workflow run update-formula.yml --repo github.com/openclaw/homebrew-tap' \
      "$tmp/homebrew-final-check-dispatch.log"
  elif [[ -s "$tmp/homebrew-final-check-dispatch.log" ]]; then
    echo "Homebrew workflow dispatched after movement during the final verifier check" >&2
    exit 1
  fi
done

tap_default_function=$(sed -n '/^load_tap_default()/,/^}/p' "$root/scripts/release-local")
grep -Fq "'.protected'" <<<"$tap_default_function"
grep -Fq "'.default_branch // empty'" <<<"$tap_default_function"
tap_contract_function=$(sed -n '/^require_tap_hash_contract_at()/,/^}/p' "$root/scripts/release-local")
grep -Fq '# verified-hashes-v1' <<<"$tap_contract_function"
grep -Fq '45b93a0b3de27e46b636a0cef819fb1ecef25bcd' "$root/scripts/release-local"
grep -Fq '.github/scripts/update_formula.py' <<<"$tap_contract_function"
grep -Fq 'cmp -s "$base_file" "$current_file"' <<<"$tap_contract_function"
grep -Fq '.github/workflows/update-formula.yml' <<<"$tap_contract_function"
grep -Fq 'trusted_tap_workflow_id' <<<"$tap_contract_function"
grep -Fq 'request_id' <<<"$tap_contract_function"
grep -Fq 'compare/$contract_commit...$expected_head' <<<"$tap_contract_function"
download_function=$(sed -n '/^download_and_verify_public_release()/,/^}/p' "$root/scripts/release-local")
download_runner=$(sed -n '/^run_release_asset_download()/,/^}/p' "$root/scripts/release-local")
grep -Fq 'export GH_TOKEN="$token"' <<<"$download_runner"
grep -Fq 'unset GITHUB_TOKEN' <<<"$download_runner"
grep -Fq '"$root/scripts/download-release-assets.sh" "$@"' <<<"$download_runner"
[[ "$(grep -Fc 'unset download_token' <<<"$download_function")" -ge 2 ]] || {
  echo "release-local does not immediately clear the parent download token" >&2
  exit 1
}
if grep -Eq 'download_token_env|env .*GH_TOKEN=|printf .*GH_TOKEN' <<<"$download_function$download_runner"; then
  echo "release-local exposes the download token through a child argv assignment" >&2
  exit 1
fi
grep -Fq 'verify-release-assets.sh' <<<"$download_function"
grep -Fq 'rebuild-release-assets.sh' <<<"$download_function"
grep -Fq 'freeze-release-inventory.sh' <<<"$download_function"
grep -Fq 'assert_trusted_release_helpers_clean' <<<"$download_function"
if grep -Fq ' execute' <<<"$download_function"; then
  echo "public release candidate executes before the caller finishes trust decisions" >&2
  exit 1
fi

mkdir -p "$tmp/token-probe-root/scripts"
cat > "$tmp/token-probe-root/scripts/download-release-assets.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "${GH_TOKEN:-}" == "$RELEASE_TEST_EXPECTED_TOKEN" ]]
[[ -z "${GITHUB_TOKEN+x}" ]]
[[ "${GITHUB_REPOSITORY:-}" == openclaw/gogcli ]]
printf '%s\n' "$@" > "$RELEASE_TEST_ARGV_LOG"
EOF
chmod +x "$tmp/token-probe-root/scripts/download-release-assets.sh"
printf '%s\n' "$download_runner" > "$tmp/run-release-asset-download.sh"
cat > "$tmp/token-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
root=$RELEASE_TEST_PROBE_ROOT
repository=openclaw/gogcli
(
  # shellcheck source=/dev/null
  source "$RELEASE_TEST_RUNNER_FUNCTION"
  run_release_asset_download "$RELEASE_TEST_EXPECTED_TOKEN" alpha 'two words'
)
EOF
chmod +x "$tmp/token-probe.sh"
RELEASE_TEST_PROBE_ROOT="$tmp/token-probe-root" \
  RELEASE_TEST_RUNNER_FUNCTION="$tmp/run-release-asset-download.sh" \
  RELEASE_TEST_EXPECTED_TOKEN='argv7d02' \
  RELEASE_TEST_ARGV_LOG="$tmp/download-argv.log" \
  GITHUB_TOKEN=must-be-removed \
  "$tmp/token-probe.sh"
grep -Fxq alpha "$tmp/download-argv.log"
grep -Fxq 'two words' "$tmp/download-argv.log"
if grep -Fq 'argv7d02' "$tmp/download-argv.log"; then
  echo "release asset download token appeared in the downloader argv" >&2
  exit 1
fi

[[ "$(grep -Fc '      - -trimpath' "$root/.goreleaser.yaml")" == 2 ]] || {
  echo "GoReleaser does not trim paths in every build ID" >&2
  exit 1
}
grep -Fq -- '      -trimpath' "$root/scripts/rebuild-release-assets.sh"
grep -Fq 'short_commit=${expected_commit:0:8}' "$root/scripts/rebuild-release-assets.sh"
if grep -Fq -- '--format=%h' "$root/scripts/rebuild-release-assets.sh"; then
  echo "rebuild helper uses clone-dependent Git abbreviation" >&2
  exit 1
fi
[[ "$(grep -Fo 'index .Env "GOG_COMMIT"' "$root/.goreleaser.yaml" | wc -l | tr -d ' ')" == 4 ]] || {
  echo "GoReleaser does not use the fixed official commit field in every build" >&2
  exit 1
}
for go_env in '"GOCACHEPROG="' '"GO111MODULE=on"' '"GOAUTH=off"' '"GOENV=off"' '"GOEXPERIMENT="' '"GOFLAGS="' \
  '"GOFIPS140=off"' '"GOROOT="' '"GOTOOLCHAIN=local"' '"GOWORK=off"'; do
  grep -Fq "$go_env" "$root/scripts/rebuild-release-assets.sh" || {
    echo "rebuild helper is missing sanitized Go environment entry: $go_env" >&2
    exit 1
  }
done

darwin_amd64_hash=$(printf 'a%.0s' {1..64})
darwin_arm64_hash=$(printf 'b%.0s' {1..64})
linux_amd64_hash=$(printf 'c%.0s' {1..64})
linux_arm64_hash=$(printf 'd%.0s' {1..64})
cat > "$tmp/gogcli.rb" <<EOF
class Gogcli < Formula
  version "0.3.4"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/openclaw/gogcli/releases/download/v0.3.4/gogcli_0.3.4_darwin_arm64.tar.gz"
      sha256 "$darwin_arm64_hash"
    else
      url "https://github.com/openclaw/gogcli/releases/download/v0.3.4/gogcli_0.3.4_darwin_amd64.tar.gz"
      sha256 "$darwin_amd64_hash"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/openclaw/gogcli/releases/download/v0.3.4/gogcli_0.3.4_linux_arm64.tar.gz"
      sha256 "$linux_arm64_hash"
    else
      url "https://github.com/openclaw/gogcli/releases/download/v0.3.4/gogcli_0.3.4_linux_amd64.tar.gz"
      sha256 "$linux_amd64_hash"
    end
  end

  def install
    bin.install "gog"
  end

  test do
    assert_match "Google CLI", shell_output("#{bin}/gog --help")
  end
end
EOF
formula_verifier_function=$(sed -n '/^verify_homebrew_formula()/,/^}/p' "$root/scripts/release-local")
printf '%s\n' "$formula_verifier_function" > "$tmp/verify-homebrew-formula.sh"
(
  # shellcheck source=/dev/null
  source "$tmp/verify-homebrew-formula.sh"
  verify_homebrew_formula "$tmp/gogcli.rb" 0.3.4 \
    "$darwin_amd64_hash" "$darwin_arm64_hash" "$linux_amd64_hash" "$linux_arm64_hash"
)
sed 's|v0.3.4/gogcli_0.3.4_darwin_arm64|v0.3.5/gogcli_0.3.4_darwin_arm64|' \
  "$tmp/gogcli.rb" > "$tmp/gogcli-bad-url.rb"
if (
  # shellcheck source=/dev/null
  source "$tmp/verify-homebrew-formula.sh"
  verify_homebrew_formula "$tmp/gogcli-bad-url.rb" 0.3.4 \
    "$darwin_amd64_hash" "$darwin_arm64_hash" "$linux_amd64_hash" "$linux_arm64_hash"
) >/dev/null 2>&1; then
  echo "Homebrew verifier accepted a mismatched literal-version URL" >&2
  exit 1
fi
if (
  # shellcheck source=/dev/null
  source "$tmp/verify-homebrew-formula.sh"
  verify_homebrew_formula "$tmp/gogcli.rb" 0.3.4 \
    "$linux_amd64_hash" "$darwin_arm64_hash" "$linux_amd64_hash" "$linux_arm64_hash"
) >/dev/null 2>&1; then
  echo "Homebrew verifier accepted a mismatched Darwin amd64 hash" >&2
  exit 1
fi

resume_homebrew_function=$(sed -n '/^resume_existing_homebrew_result()/,/^}/p' "$root/scripts/release-local")
printf '%s\n' "$resume_homebrew_function" > "$tmp/resume-existing-homebrew-result.sh"
cat > "$tmp/homebrew-resume-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
work_dir=$HOMEBREW_RESUME_WORK
repository=openclaw/gogcli
trusted_tap_branch=main
accepted_tag_object=1111111111111111111111111111111111111111
accepted_tag_commit=2222222222222222222222222222222222222222
fixture_result=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
fixture_parent=ffffffffffffffffffffffffffffffffffffffff
request_id=
tap_source_commit=
completed_tap_commit=
mkdir -p "$work_dir"

github_api_without_tokens() {
  local endpoint=${!#} message_commit=$accepted_tag_commit file=Formula/gogcli.rb
  case "$endpoint" in
    *contents/Formula/gogcli.rb*)
      cat "$HOMEBREW_RESUME_FORMULA"
      ;;
    *git/commits/*)
      [[ "${HOMEBREW_RESUME_MODE:-exact}" != bad-provenance ]] || \
        message_commit=3333333333333333333333333333333333333333
      jq -n \
        --arg result "$fixture_result" \
        --arg parent "$fixture_parent" \
        --arg tag_commit "$message_commit" \
        '{
          sha: $result,
          message: ("gogcli: update formula for v0.3.4\n\n" +
            "Source-Repository: openclaw/gogcli\n" +
            "Source-Tag-Object: 1111111111111111111111111111111111111111\n" +
            "Source-Tag-Commit: " + $tag_commit + "\n" +
            "Request-ID: gogcli-v0.3.4-111111111111-1750000000"),
          parents: [{sha: $parent}]
        }'
      ;;
    *compare/*)
      [[ "${HOMEBREW_RESUME_MODE:-exact}" != bad-delta ]] || file=README.md
      jq -n \
        --arg result "$fixture_result" \
        --arg parent "$fixture_parent" \
        --arg file "$file" \
        '{
          status: "ahead",
          base_commit: {sha: $parent},
          merge_base_commit: {sha: $parent},
          ahead_by: 1,
          behind_by: 0,
          total_commits: 1,
          commits: [{sha: $result}],
          files: [{filename: $file, status: "modified"}]
        }'
      ;;
    *) exit 90 ;;
  esac
}
require_tap_hash_contract_at() {
  [[ "$1" == main && "$2" == "$fixture_parent" && "$3" == "$fixture_result" ]]
}

# shellcheck source=/dev/null
source "$HOMEBREW_FORMULA_FUNCTION"
# shellcheck source=/dev/null
source "$HOMEBREW_RESUME_FUNCTION"
resume_existing_homebrew_result v0.3.4 "$fixture_result" 0.3.4 \
  "$HOMEBREW_DARWIN_AMD64" "$HOMEBREW_DARWIN_ARM64" \
  "$HOMEBREW_LINUX_AMD64" "$HOMEBREW_LINUX_ARM64"
printf '%s\n' "$request_id" "$tap_source_commit" "$completed_tap_commit"
EOF
chmod +x "$tmp/homebrew-resume-probe.sh"
HOMEBREW_RESUME_WORK="$tmp/homebrew-resume-work" \
  HOMEBREW_RESUME_FORMULA="$tmp/gogcli.rb" \
  HOMEBREW_FORMULA_FUNCTION="$tmp/verify-homebrew-formula.sh" \
  HOMEBREW_RESUME_FUNCTION="$tmp/resume-existing-homebrew-result.sh" \
  HOMEBREW_DARWIN_AMD64="$darwin_amd64_hash" \
  HOMEBREW_DARWIN_ARM64="$darwin_arm64_hash" \
  HOMEBREW_LINUX_AMD64="$linux_amd64_hash" \
  HOMEBREW_LINUX_ARM64="$linux_arm64_hash" \
  "$tmp/homebrew-resume-probe.sh" > "$tmp/homebrew-resume-output.log"
cat > "$tmp/homebrew-resume-expected.log" <<'EOF'
gogcli-v0.3.4-111111111111-1750000000
ffffffffffffffffffffffffffffffffffffffff
eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
EOF
cmp "$tmp/homebrew-resume-expected.log" "$tmp/homebrew-resume-output.log"
for resume_mode in bad-provenance bad-delta; do
  if HOMEBREW_RESUME_MODE=$resume_mode \
    HOMEBREW_RESUME_WORK="$tmp/homebrew-resume-work-$resume_mode" \
    HOMEBREW_RESUME_FORMULA="$tmp/gogcli.rb" \
    HOMEBREW_FORMULA_FUNCTION="$tmp/verify-homebrew-formula.sh" \
    HOMEBREW_RESUME_FUNCTION="$tmp/resume-existing-homebrew-result.sh" \
    HOMEBREW_DARWIN_AMD64="$darwin_amd64_hash" \
    HOMEBREW_DARWIN_ARM64="$darwin_arm64_hash" \
    HOMEBREW_LINUX_AMD64="$linux_amd64_hash" \
    HOMEBREW_LINUX_ARM64="$linux_arm64_hash" \
    "$tmp/homebrew-resume-probe.sh" >/dev/null 2>&1; then
    echo "Homebrew recovery accepted a tampered updater result: $resume_mode" >&2
    exit 1
  fi
done

install_state_shape_function=$(sed -n '/^require_homebrew_install_state_shape()/,/^}/p' "$root/scripts/release-local")
install_state_ensure_function=$(sed -n '/^ensure_homebrew_install_state()/,/^}/p' "$root/scripts/release-local")
printf '%s\n' "$install_state_shape_function" > "$tmp/homebrew-install-state-shape.sh"
printf '%s\n' "$install_state_ensure_function" > "$tmp/homebrew-install-state-ensure.sh"
cat > "$tmp/homebrew-install-state-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
work_dir=$HOMEBREW_INSTALL_STATE_WORK
repository=openclaw/gogcli
state_file=$HOMEBREW_INSTALL_STATE_FILE
mkdir -p "$work_dir"
# shellcheck source=/dev/null
source "$HOMEBREW_INSTALL_STATE_SHAPE_FUNCTION"
# shellcheck source=/dev/null
source "$HOMEBREW_INSTALL_STATE_ENSURE_FUNCTION"
ensure_homebrew_install_state \
  "$state_file" v0.3.4 \
  1111111111111111111111111111111111111111 \
  2222222222222222222222222222222222222222 \
  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee \
  bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
ensure_homebrew_install_state \
  "$state_file" v0.3.4 \
  1111111111111111111111111111111111111111 \
  2222222222222222222222222222222222222222 \
  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee \
  bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
EOF
chmod +x "$tmp/homebrew-install-state-probe.sh"
install_state_file="$tmp/release-state/homebrew-v0.3.4.json"
HOMEBREW_INSTALL_STATE_WORK="$tmp/homebrew-install-state-work" \
  HOMEBREW_INSTALL_STATE_FILE="$install_state_file" \
  HOMEBREW_INSTALL_STATE_SHAPE_FUNCTION="$tmp/homebrew-install-state-shape.sh" \
  HOMEBREW_INSTALL_STATE_ENSURE_FUNCTION="$tmp/homebrew-install-state-ensure.sh" \
  "$tmp/homebrew-install-state-probe.sh"
require_state_mode=$(stat -f '%Lp' "$(dirname "$install_state_file")")
[[ "$require_state_mode" == 700 ]]
if grep -Eiq 'keychain|notary|credential|identity|profile' "$install_state_file"; then
  echo "Homebrew recovery state contains credential-bound metadata" >&2
  exit 1
fi
sed 's/^  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/  cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc/' \
  "$tmp/homebrew-install-state-probe.sh" > "$tmp/homebrew-install-state-tampered-probe.sh"
chmod +x "$tmp/homebrew-install-state-tampered-probe.sh"
if HOMEBREW_INSTALL_STATE_WORK="$tmp/homebrew-install-state-work-tampered" \
  HOMEBREW_INSTALL_STATE_FILE="$install_state_file" \
  HOMEBREW_INSTALL_STATE_SHAPE_FUNCTION="$tmp/homebrew-install-state-shape.sh" \
  HOMEBREW_INSTALL_STATE_ENSURE_FUNCTION="$tmp/homebrew-install-state-ensure.sh" \
  "$tmp/homebrew-install-state-tampered-probe.sh" >/dev/null 2>&1; then
  echo "Homebrew install recovery accepted mismatched handoff state" >&2
  exit 1
fi

echo "local release gate tests passed"
