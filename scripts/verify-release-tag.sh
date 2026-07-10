#!/usr/bin/env bash
set -euo pipefail

unset \
  GIT_ALTERNATE_OBJECT_DIRECTORIES \
  GIT_ATTR_SOURCE \
  GIT_COMMON_DIR \
  GIT_CONFIG \
  GIT_CONFIG_PARAMETERS \
  GIT_CONFIG_SYSTEM \
  GIT_DIR \
  GIT_EXEC_PATH \
  GIT_INDEX_FILE \
  GIT_NAMESPACE \
  GIT_OBJECT_DIRECTORY \
  GIT_REPLACE_REF_BASE \
  GIT_SHALLOW_FILE \
  GIT_TEMPLATE_DIR \
  GIT_WORK_TREE
export GIT_CONFIG_COUNT=0
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_NO_REPLACE_OBJECTS=1

tag=${1:-}
expected_commit=${2:-}
root=$(cd "$(dirname "$0")/.." && pwd)
allowed_signers_file="$root/.github/release-allowed-signers"
expected_signer='Good "git" signature for steipete@gmail.com with ED25519 key SHA256:WmI9lVtd7F2c5XyRHbZVO3yYYJzwsSNzcZQMPT147HI' # gitleaks:allow -- public release-key fingerprint
expected_allowed_signer='steipete@gmail.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII9XsaCcr8TInPnHcuTVfvXXcsoUFrOE7menfbEIHFW9 steipete@gmail.com'

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ||
  ( -n "$expected_commit" && ! "$expected_commit" =~ ^[[:xdigit:]]{40}$ ) ]]; then
  echo "release tag: usage: $0 vX.Y.Z [expected-commit]" >&2
  exit 2
fi
for tool in git awk; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "release tag: missing required command: $tool" >&2
    exit 1
  }
done
[[ -x /usr/bin/ssh-keygen ]] || {
  echo "release tag: trusted SSH verifier is unavailable: /usr/bin/ssh-keygen" >&2
  exit 1
}

cd "$root"
graft_path=$(git rev-parse --git-path info/grafts) || {
  echo "release tag: cannot resolve Git graft path" >&2
  exit 1
}
[[ -n "$graft_path" && "$graft_path" != *$'\n'* ]] || {
  echo "release tag: invalid Git graft path" >&2
  exit 1
}
[[ "$graft_path" == /* ]] || graft_path="$root/$graft_path"
[[ ! -e "$graft_path" && ! -L "$graft_path" ]] || {
  echo "release tag: Git graft files are forbidden" >&2
  exit 1
}
[[ -f "$allowed_signers_file" && ! -L "$allowed_signers_file" && "$(<"$allowed_signers_file")" == "$expected_allowed_signer" ]] || {
  echo "release tag: repository signer policy is missing or unexpected" >&2
  exit 1
}
policy_from_head=$(git show 'HEAD:.github/release-allowed-signers') || {
  echo "release tag: checked-out verifier has no signer policy" >&2
  exit 1
}
[[ "$policy_from_head" == "$expected_allowed_signer" ]] || {
  echo "release tag: checked-out verifier has an unexpected signer policy" >&2
  exit 1
}

git fetch --quiet --force --no-tags origin "refs/tags/$tag"
[[ "$(git cat-file -t FETCH_HEAD)" == tag ]] || {
  echo "release tag: $tag must resolve to an annotated tag object" >&2
  exit 1
}
tag_object=$(git rev-parse FETCH_HEAD)
tag_commit=$(git rev-parse 'FETCH_HEAD^{}')
tag_payload=$(git cat-file tag FETCH_HEAD)
payload_object=$(awk '$1 == "object" { print $2; exit }' <<<"$tag_payload")
payload_type=$(awk '$1 == "type" { print $2; exit }' <<<"$tag_payload")
payload_tag=$(awk '$1 == "tag" { print $2; exit }' <<<"$tag_payload")
[[ "$tag_object" =~ ^[[:xdigit:]]{40}$ && "$tag_commit" =~ ^[[:xdigit:]]{40}$ &&
  "$payload_object" == "$tag_commit" && "$payload_type" == commit && "$payload_tag" == "$tag" ]] || {
  echo "release tag: $tag has invalid annotated-tag metadata" >&2
  exit 1
}
if ! signature_output=$(LC_ALL=C git \
  -c gpg.format=ssh \
  -c gpg.ssh.program=/usr/bin/ssh-keygen \
  -c "gpg.ssh.allowedSignersFile=$allowed_signers_file" \
  verify-tag --raw FETCH_HEAD 2>&1); then
  echo "release tag: $tag is not signed by the repository release signer" >&2
  exit 1
fi
[[ "$signature_output" == "$expected_signer" ]] || {
  echo "release tag: $tag has an unexpected signer principal or key" >&2
  exit 1
}

remote_tag_refs=$(git ls-remote --exit-code --tags origin "refs/tags/$tag" "refs/tags/$tag^{}") || {
  echo "release tag: origin no longer contains $tag" >&2
  exit 1
}
remote_tag_object=$(awk -v ref="refs/tags/$tag" '$2 == ref { print $1 }' <<<"$remote_tag_refs")
remote_tag_commit=$(awk -v ref="refs/tags/$tag^{}" '$2 == ref { print $1 }' <<<"$remote_tag_refs")
[[ "$remote_tag_object" == "$tag_object" && "$remote_tag_commit" == "$tag_commit" ]] || {
  echo "release tag: origin changed $tag during verification" >&2
  exit 1
}
[[ -z "$expected_commit" || "$tag_commit" == "$expected_commit" ]] || {
  echo "release tag: $tag does not match the expected commit" >&2
  exit 1
}

printf '%s %s\n' "$tag_object" "$tag_commit"
