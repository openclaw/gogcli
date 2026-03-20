#!/usr/bin/env bash

set -euo pipefail

run_meet_tests() {
  if skip "meet"; then
    echo "==> meet (skipped)"
    return 0
  fi

  local space_json meeting_code
  echo "==> meet create"
  space_json=$(gog meet create --json)
  meeting_code=$(echo "$space_json" | "$PY" -c "import sys,json; print(json.load(sys.stdin)['meeting_code'])")
  [ -n "$meeting_code" ] || { echo "Failed to parse meeting code" >&2; exit 1; }

  run_required "meet" "meet get" gog meet get "$meeting_code" --json >/dev/null
  run_required "meet" "meet update" gog meet update "$meeting_code" --access open --json >/dev/null
  run_required "meet" "meet history" gog meet history "$meeting_code" --json --max 1 >/dev/null
}
