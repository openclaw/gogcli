#!/usr/bin/env bash
set -euo pipefail

tag=${1:-}
source_repo=${2:-}
artifact_dir=${3:-}
expected_commit=${4:-}
expected_tag_object=${5:-}
go_bin=${GOG_RELEASE_GO:-}

usage() {
  echo "usage: GOG_RELEASE_GO=/absolute/go $0 vX.Y.Z source-repo artifact-directory expected-commit expected-tag-object" >&2
  exit 2
}

[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || usage
[[ -d "$source_repo/.git" && -d "$artifact_dir" ]] || usage
[[ "$expected_commit" =~ ^[[:xdigit:]]{40}$ && "$expected_tag_object" =~ ^[[:xdigit:]]{40}$ ]] || usage
[[ "$go_bin" == /* && -x "$go_bin" ]] || usage
[[ -z "${GH_TOKEN+x}" && -z "${GITHUB_TOKEN+x}" ]] || {
  echo "release rebuild: GitHub tokens must be absent" >&2
  exit 1
}

for tool in cmp date dirname env git mkdir mktemp tar unzip; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "release rebuild: missing required command: $tool" >&2
    exit 1
  }
done

go_version=$("$go_bin" version)
[[ "$go_version" =~ ^go\ version\ go1\.26\.5\ darwin/(amd64|arm64)$ ]] || {
  echo "release rebuild: expected official Go 1.26.5 Darwin toolchain" >&2
  exit 1
}
[[ "$(git -C "$source_repo" rev-parse HEAD)" == "$expected_commit" &&
  "$(git -C "$source_repo" rev-parse "refs/tags/$tag")" == "$expected_tag_object" &&
  "$(git -C "$source_repo" rev-parse "refs/tags/$tag^{}")" == "$expected_commit" ]] || {
  echo "release rebuild: source checkout does not match the exact signed tag object" >&2
  exit 1
}
[[ -z "$(git -C "$source_repo" status --porcelain --untracked-files=all)" ]] || {
  echo "release rebuild: source checkout must be clean" >&2
  exit 1
}

version=${tag#v}
go_dir=$(dirname "$go_bin")
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/gogcli-release-rebuild.XXXXXX")
cleanup() {
  chmod -R u+w "$work_dir" 2>/dev/null || true
  rm -rf "$work_dir" 2>/dev/null || true
}
trap cleanup EXIT
mkdir -p "$work_dir/home" "$work_dir/gocache" "$work_dir/gomodcache" "$work_dir/tmp"
short_commit=${expected_commit:0:8}
commit_epoch=$(git -C "$source_repo" show --format=%ct --no-patch HEAD)
commit_date=$(date -u -r "$commit_epoch" '+%Y-%m-%dT%H:%M:%SZ')
[[ "$short_commit" =~ ^[[:xdigit:]]{8}$ && "$commit_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T ]] || {
  echo "release rebuild: invalid deterministic Git metadata" >&2
  exit 1
}
ldflags="-s -w -X github.com/steipete/gogcli/internal/cmd.version=$tag -X github.com/steipete/gogcli/internal/cmd.commit=$short_commit -X github.com/steipete/gogcli/internal/cmd.date=$commit_date"

targets=(linux_amd64 linux_arm64 windows_amd64 windows_arm64)
for target in "${targets[@]}"; do
  goos=${target%_*}
  goarch=${target#*_}
  member=gog
  suffix=tar.gz
  [[ "$goos" != windows ]] || {
    member=gog.exe
    suffix=zip
  }
  archive="$artifact_dir/gogcli_${version}_${target}.${suffix}"
  [[ -f "$archive" && ! -L "$archive" ]] || {
    echo "release rebuild: missing archive: $archive" >&2
    exit 1
  }

  extracted="$work_dir/extracted-$target"
  if [[ "$suffix" == tar.gz ]]; then
    [[ "$(tar -tzf "$archive")" == "$member" ]] || {
      echo "release rebuild: unexpected archive inventory: $archive" >&2
      exit 1
    }
    tar -xOf "$archive" "$member" > "$extracted"
  else
    [[ "$(unzip -Z1 "$archive")" == "$member" ]] || {
      echo "release rebuild: unexpected archive inventory: $archive" >&2
      exit 1
    }
    unzip -p "$archive" "$member" > "$extracted"
  fi
  [[ -s "$extracted" ]] || {
    echo "release rebuild: empty binary in $archive" >&2
    exit 1
  }

  rebuilt="$work_dir/rebuilt-$target"
  build_env=(
    "CGO_ENABLED=0"
    "GODEBUG="
    "GIT_CONFIG_GLOBAL=/dev/null"
    "GIT_CONFIG_NOSYSTEM=1"
    "GOCACHE=$work_dir/gocache"
    "GOCACHEPROG="
    "GO111MODULE=on"
    "GOAUTH=off"
    "GOENV=off"
    "GOEXPERIMENT="
    "GOFLAGS="
    "GOFIPS140=off"
    "GOINSECURE="
    "GOAMD64=v1"
    "GOARM64=v8.0"
    "GOOS=$goos"
    "GOARCH=$goarch"
    "GOMODCACHE=$work_dir/gomodcache"
    "GONOPROXY="
    "GONOSUMDB="
    "GOPRIVATE="
    "GOPROXY=https://proxy.golang.org"
    "GOROOT="
    "GOSUMDB=sum.golang.org"
    "GOTOOLCHAIN=local"
    "GOWORK=off"
    "HOME=$work_dir/home"
    "LC_ALL=C"
    "PATH=$go_dir:/usr/bin:/bin:/usr/sbin:/sbin"
    "TMPDIR=$work_dir/tmp"
    "TZ=UTC"
  )
  (
    cd "$source_repo"
    env -i "${build_env[@]}" "$go_bin" build \
      -trimpath \
      -ldflags "$ldflags" \
      -o "$rebuilt" \
      ./cmd/gog
  )
  cmp -s "$rebuilt" "$extracted" || {
    echo "release rebuild: byte mismatch for $target" >&2
    exit 1
  }
  echo "release rebuild: byte-identical $target"
done

[[ -z "$(git -C "$source_repo" status --porcelain --untracked-files=all)" ]] || {
  echo "release rebuild: build mutated the signed source checkout" >&2
  exit 1
}
