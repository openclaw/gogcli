#!/usr/bin/env bash
set -euo pipefail

version=${1:-}
if [[ "$#" -ne 1 || ! "$version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: scripts/release.sh <version>" >&2
  exit 2
fi

exec gh workflow run release-unified.yml \
  --repo openclaw/gogcli \
  --ref main \
  -f "version=${version#v}"
