#!/usr/bin/env bash
set -euo pipefail

cat >&2 <<'EOF'
scripts/verify-release.sh is retired because it did not bind proof to the exact
signed tag object and protected verifier code. Use docs/RELEASING.md and the
serialized scripts/release-local verify-draft, publish, and homebrew gates.
EOF
exit 2
