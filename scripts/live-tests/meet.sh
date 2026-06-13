#!/usr/bin/env bash

set -euo pipefail

run_meet_tests() {
  if skip "meet"; then
    echo "==> meet (skipped)"
    return 0
  fi

  local space_json meeting_code
  echo "==> meet create (optional)"
  if ! space_json=$(gog meet create --json); then
    echo "skipped/failed"
    if [ "${STRICT:-false}" = true ]; then
      return 1
    fi
    return 0
  fi
  echo "ok"
  meeting_code=$(echo "$space_json" | "$PY" -c "import sys,json; print(json.load(sys.stdin)['meeting_code'])")
  [ -n "$meeting_code" ] || { echo "Failed to parse meeting code" >&2; exit 1; }

  run_required "meet" "meet get" gog meet get "$meeting_code" --json >/dev/null
  run_required "meet" "meet update" gog meet update "$meeting_code" --access open --json >/dev/null
  local history_json participants_json
  history_json=$(gog meet history "$meeting_code" --json --max 1)
  echo "$history_json" | "$PY" -c 'import json,sys; value=json.load(sys.stdin)["conferences"]; assert isinstance(value,list)'
  participants_json=$(gog meet participants "$meeting_code" --json --max 1)
  echo "$participants_json" | "$PY" -c 'import json,sys; value=json.load(sys.stdin)["participants"]; assert isinstance(value,list)'
}
