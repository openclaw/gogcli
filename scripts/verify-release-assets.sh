#!/usr/bin/env bash
set -euo pipefail

tag=${1:-}
archive=${2:-}
checksums=${3:-}
expected_revision=${4:-}
candidate_output=${5:-}
script_dir=$(cd "$(dirname "$0")" && pwd)
expected_go_toolchain=go1.26.5
expected_main_package=github.com/steipete/gogcli/cmd/gog

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ || -z "$archive" || -z "$checksums" || ! "$expected_revision" =~ ^[[:xdigit:]]{40}$ ]]; then
  echo "usage: $0 vX.Y.Z gogcli_VERSION_darwin_ARCH.tar.gz checksums.txt expected-vcs-revision [verified-candidate-output]" >&2
  exit 2
fi
[[ -z "${GH_TOKEN+x}" && -z "${GITHUB_TOKEN+x}" ]] || {
  echo "release verify: GitHub tokens must be absent during artifact verification" >&2
  exit 1
}
[[ "$(uname -s)" == Darwin ]] || {
  echo "release verify: macOS artifacts require a native macOS verifier" >&2
  exit 1
}
[[ -f "$archive" && ! -L "$archive" && -f "$checksums" && ! -L "$checksums" ]] || {
  echo "release verify: missing archive or checksums" >&2
  exit 1
}

for tool in chmod cp dirname go mktemp mv shasum tar unzip; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "release verify: missing required command: $tool" >&2
    exit 1
  }
done
[[ -x "$script_dir/verify-macos-binary.sh" ]] || {
  echo "release verify: macOS candidate verifier is unavailable" >&2
  exit 1
}

version=${tag#v}
archive_dir=$(cd "$(dirname "$archive")" && pwd)
checksums_dir=$(cd "$(dirname "$checksums")" && pwd)
[[ "$archive_dir" == "$checksums_dir" && "$(basename "$checksums")" == checksums.txt ]] || {
  echo "release verify: archive and checksums must come from one exact inventory" >&2
  exit 1
}
selected_archive_name=$(basename "$archive")
case "$selected_archive_name" in
  "gogcli_${version}_darwin_arm64.tar.gz") expected_arch=arm64 ;;
  "gogcli_${version}_darwin_amd64.tar.gz") expected_arch=x86_64 ;;
  *)
    echo "release verify: unexpected archive name: $(basename "$archive")" >&2
    exit 1
    ;;
esac

expected_archives=(
  "gogcli_${version}_darwin_amd64.tar.gz"
  "gogcli_${version}_darwin_arm64.tar.gz"
  "gogcli_${version}_linux_amd64.tar.gz"
  "gogcli_${version}_linux_arm64.tar.gz"
  "gogcli_${version}_windows_amd64.zip"
  "gogcli_${version}_windows_arm64.zip"
)
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/gogcli-release-verify.XXXXXX")
candidate_tmp=
cleanup() {
  rm -rf "$work_dir"
  [[ -z "$candidate_tmp" ]] || rm -f "$candidate_tmp"
}
trap cleanup EXIT

verify_buildinfo() {
  local artifact_binary=$1 expected_os=$2 expected_goarch=$3
  local buildinfo toolchain_header main_count vcs_count revision_count modified_count goos_count goarch_count
  local actual_main actual_vcs actual_revision actual_modified actual_goos actual_goarch

  buildinfo=$(go version -m "$artifact_binary") || {
    echo "release verify: could not read Go build information" >&2
    exit 1
  }
  toolchain_header=${buildinfo%%$'\n'*}
  main_count=$(awk '$1 == "path" { count++ } END { print count + 0 }' <<<"$buildinfo")
  vcs_count=$(awk '$1 == "build" && $2 ~ /^vcs=/ { count++ } END { print count + 0 }' <<<"$buildinfo")
  revision_count=$(awk '$1 == "build" && $2 ~ /^vcs\.revision=/ { count++ } END { print count + 0 }' <<<"$buildinfo")
  modified_count=$(awk '$1 == "build" && $2 ~ /^vcs\.modified=/ { count++ } END { print count + 0 }' <<<"$buildinfo")
  goos_count=$(awk '$1 == "build" && $2 ~ /^GOOS=/ { count++ } END { print count + 0 }' <<<"$buildinfo")
  goarch_count=$(awk '$1 == "build" && $2 ~ /^GOARCH=/ { count++ } END { print count + 0 }' <<<"$buildinfo")
  actual_main=$(awk '$1 == "path" { print $2 }' <<<"$buildinfo")
  actual_vcs=$(awk '$1 == "build" && $2 ~ /^vcs=/ { sub(/^vcs=/, "", $2); print $2 }' <<<"$buildinfo")
  actual_revision=$(awk '$1 == "build" && $2 ~ /^vcs\.revision=/ { sub(/^vcs\.revision=/, "", $2); print $2 }' <<<"$buildinfo")
  actual_modified=$(awk '$1 == "build" && $2 ~ /^vcs\.modified=/ { sub(/^vcs\.modified=/, "", $2); print $2 }' <<<"$buildinfo")
  actual_goos=$(awk '$1 == "build" && $2 ~ /^GOOS=/ { sub(/^GOOS=/, "", $2); print $2 }' <<<"$buildinfo")
  actual_goarch=$(awk '$1 == "build" && $2 ~ /^GOARCH=/ { sub(/^GOARCH=/, "", $2); print $2 }' <<<"$buildinfo")
  [[ "$toolchain_header" == "$artifact_binary: $expected_go_toolchain" ]] || {
    echo "release verify: Go toolchain does not match $expected_go_toolchain" >&2
    exit 1
  }
  [[ "$main_count" == 1 && "$actual_main" == "$expected_main_package" ]] || {
    echo "release verify: Go main package does not match GogCLI" >&2
    exit 1
  }
  [[ "$vcs_count" == 1 && "$actual_vcs" == git ]] || {
    echo "release verify: Go build information must identify Git provenance" >&2
    exit 1
  }
  [[ "$revision_count" == 1 && "$actual_revision" == "$expected_revision" ]] || {
    echo "release verify: build vcs.revision does not match the trusted release commit" >&2
    exit 1
  }
  [[ "$modified_count" == 1 && "$actual_modified" == false ]] || {
    echo "release verify: build vcs.modified must be false" >&2
    exit 1
  }
  [[ "$goos_count" == 1 && "$goarch_count" == 1 &&
    "$actual_goos" == "$expected_os" && "$actual_goarch" == "$expected_goarch" ]] || {
    echo "release verify: Go build target does not match its archive" >&2
    exit 1
  }
}

[[ "$(wc -l < "$checksums" | tr -d ' ')" == "${#expected_archives[@]}" ]] || {
  echo "release verify: checksums.txt must contain exactly six records" >&2
  exit 1
}

for expected_name in "${expected_archives[@]}"; do
  records=$(awk -v name="$expected_name" '$2 == name { print }' "$checksums")
  record_count=$(awk -v name="$expected_name" '$2 == name { count++ } END { print count + 0 }' "$checksums")
  [[ "$record_count" == 1 ]] || {
    echo "release verify: checksum missing or duplicated: $expected_name" >&2
    exit 1
  }
  read -r hash recorded_name extra <<<"$records"
  [[ "$hash" =~ ^[[:xdigit:]]{64}$ && "$recorded_name" == "$expected_name" && -z "${extra:-}" ]] || {
    echo "release verify: invalid checksum record: $expected_name" >&2
    exit 1
  }
  inventory_archive="$archive_dir/$expected_name"
  [[ -f "$inventory_archive" && ! -L "$inventory_archive" ]] || {
    echo "release verify: inventory archive is missing: $expected_name" >&2
    exit 1
  }
  actual_hash=$(shasum -a 256 "$inventory_archive" | awk '{print $1}')
  [[ "$actual_hash" == "$hash" ]] || {
    echo "release verify: checksum mismatch: $expected_name" >&2
    exit 1
  }

  case "$expected_name" in
    "gogcli_${version}_darwin_amd64.tar.gz") artifact_os=darwin; artifact_goarch=amd64; member=gog ;;
    "gogcli_${version}_darwin_arm64.tar.gz") artifact_os=darwin; artifact_goarch=arm64; member=gog ;;
    "gogcli_${version}_linux_amd64.tar.gz") artifact_os=linux; artifact_goarch=amd64; member=gog ;;
    "gogcli_${version}_linux_arm64.tar.gz") artifact_os=linux; artifact_goarch=arm64; member=gog ;;
    "gogcli_${version}_windows_amd64.zip") artifact_os=windows; artifact_goarch=amd64; member=gog.exe ;;
    "gogcli_${version}_windows_arm64.zip") artifact_os=windows; artifact_goarch=arm64; member=gog.exe ;;
    *) exit 1 ;;
  esac
  artifact_dir="$work_dir/$artifact_os-$artifact_goarch"
  artifact_binary="$artifact_dir/$member"
  mkdir -p "$artifact_dir"
  case "$expected_name" in
    *.tar.gz)
      [[ "$(tar -tzf "$inventory_archive")" == "$member" ]] || {
        echo "release verify: archive must contain only $member: $expected_name" >&2
        exit 1
      }
      tar -xOf "$inventory_archive" "$member" > "$artifact_binary"
      ;;
    *.zip)
      [[ "$(unzip -Z1 "$inventory_archive")" == "$member" ]] || {
        echo "release verify: archive must contain only $member: $expected_name" >&2
        exit 1
      }
      unzip -p "$inventory_archive" "$member" > "$artifact_binary"
      ;;
  esac
  [[ -s "$artifact_binary" ]] || {
    echo "release verify: archive contains an empty binary: $expected_name" >&2
    exit 1
  }
  verify_buildinfo "$artifact_binary" "$artifact_os" "$artifact_goarch"
  if [[ "$expected_name" == "$selected_archive_name" ]]; then
    binary=$artifact_binary
  fi
done
[[ -n "${binary:-}" ]] || {
  echo "release verify: selected archive is missing from the exact inventory" >&2
  exit 1
}
chmod 0755 "$binary"
"$script_dir/verify-macos-binary.sh" "$binary" "$expected_arch" "$version" static

if [[ -n "$candidate_output" ]]; then
  candidate_parent=$(dirname "$candidate_output")
  [[ -d "$candidate_parent" && ! -e "$candidate_output" && ! -L "$candidate_output" ]] || {
    echo "release verify: candidate output must be a new path in an existing directory" >&2
    exit 1
  }
  candidate_parent=$(cd "$candidate_parent" && pwd)
  candidate_output="$candidate_parent/$(basename "$candidate_output")"
  candidate_tmp=$(mktemp "$candidate_parent/.gogcli-candidate.XXXXXX")
  cp "$binary" "$candidate_tmp"
  chmod 0755 "$candidate_tmp"
  mv "$candidate_tmp" "$candidate_output"
  candidate_tmp=
fi
