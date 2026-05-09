#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

usage() {
  cat >&2 <<'USAGE'
Usage: ./build-safe.sh <profile.yaml> [-o output]

Examples:
  ./build-safe.sh safety-profiles/readonly.yaml
  ./build-safe.sh safety-profiles/agent-safe.yaml -o /usr/local/bin/gog-safe
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -z "${1:-}" || "${1:-}" == -* ]]; then
  usage
  exit 2
fi

PROFILE="$1"
shift

OUTPUT="bin/gog-safe"
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output)
      if [[ -z "${2:-}" ]]; then
        echo "error: $1 requires a path" >&2
        exit 2
      fi
      OUTPUT="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown flag: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ ! -f "$PROFILE" ]]; then
  echo "error: profile not found: $PROFILE" >&2
  exit 1
fi

GEN_FILE="internal/cmd/safety_profile_baked_gen.go"
cleanup() {
  rm -f "$GEN_FILE"
}
trap cleanup EXIT

cleanup
go run ./cmd/bake-safety-profile "$PROFILE" "$GEN_FILE"

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT=$(git rev-parse --short=12 HEAD 2>/dev/null || echo "")
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-X github.com/steipete/gogcli/internal/cmd.version=${VERSION}-safe -X github.com/steipete/gogcli/internal/cmd.commit=${COMMIT} -X github.com/steipete/gogcli/internal/cmd.date=${DATE}"

mkdir -p "$(dirname "$OUTPUT")"
go build -tags safety_profile -ldflags "$LDFLAGS" -o "$OUTPUT" ./cmd/gog
RUN_OUTPUT="$OUTPUT"
if [[ "$RUN_OUTPUT" != */* ]]; then
  RUN_OUTPUT="./$RUN_OUTPUT"
fi
"$RUN_OUTPUT" --version

echo "built $OUTPUT with baked safety profile $PROFILE"
