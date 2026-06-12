#!/usr/bin/env bash

set -euo pipefail

run_photos_picker_tests() {
  if skip "photos-picker"; then
    echo "==> photos picker (skipped)"
    return 0
  fi

  local auth_json
  auth_json=$("$BIN" auth list --json)
  if ! "$PY" -c 'import json,sys
account=sys.argv[1].lower()
for item in json.load(sys.stdin).get("accounts", []):
    if item.get("email", "").lower() == account:
        raise SystemExit(0 if "photospicker" in (item.get("services") or []) else 1)
raise SystemExit(1)' "$ACCOUNT" <<<"$auth_json"; then
    echo "==> photos picker (skipped; reauthorize with --services photospicker)"
    return 0
  fi

  local session_json session_id
  session_json=$(gog photos picker create --max-items 1 --json)
  session_id=$(extract_id "$session_json")
  [ -n "$session_id" ] || { echo "Failed to parse Photos Picker session id" >&2; exit 1; }
  register_photos_picker_cleanup "$session_id"
  run_required "photos-picker" "photos picker get" gog photos picker get "$session_id" --json >/dev/null
  run_required "photos-picker" "photos picker delete" gog photos picker delete \
    "$session_id" --force --json >/dev/null
}
