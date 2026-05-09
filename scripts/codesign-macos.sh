#!/usr/bin/env bash
set -euo pipefail

BIN="${1:-}"
if [[ -z "$BIN" ]]; then
  echo "usage: $0 <path-to-binary>" >&2
  exit 2
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  exit 0
fi

IDENTITY="${GOG_CODESIGN_IDENTITY:-${CODESIGN_IDENTITY:-}}"
if [[ -z "$IDENTITY" ]]; then
  echo "codesign: skipped (set GOG_CODESIGN_IDENTITY or CODESIGN_IDENTITY)" >&2
  exit 0
fi

ID="com.steipete.gogcli.gog"

codesign --force --sign "$IDENTITY" --timestamp --options runtime --identifier "$ID" "$BIN"
codesign --verify --deep --strict --verbose=2 "$BIN"
