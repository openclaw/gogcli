#!/usr/bin/env bash
# Stable build entry point invoked by CI and local users.
# Wraps the project-specific build command so the invocation surface stays fixed.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
cd "$repo_root"

# TODO: adapt for go — the detected command is baked in below.
exec go build ./...
