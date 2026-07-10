#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)

if [[ "${1:-}" == --check && "$#" -eq 1 ]]; then
  exec "$root/scripts/release-local" --check
fi

cat >&2 <<'EOF'
scripts/release.sh no longer tags or publishes.
Use the serialized gates documented in docs/RELEASING.md:
  scripts/release-local pilot vX.Y.Z
  scripts/release-local draft
  scripts/release-local verify-draft vX.Y.Z
  scripts/release-local publish vX.Y.Z
  scripts/release-local homebrew vX.Y.Z
EOF
exit 2
