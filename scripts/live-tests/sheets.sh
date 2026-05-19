#!/usr/bin/env bash

set -euo pipefail

run_sheets_tests() {
  if skip "sheets"; then
    echo "==> sheets (skipped)"
    return 0
  fi

  local sheet_json sheet_id copy_json copy_id export_path
  sheet_json=$(gog sheets create "gogcli-smoke-sheet-$TS" --json)
  sheet_id=$(extract_id "$sheet_json")
  [ -n "$sheet_id" ] || { echo "Failed to parse sheet id" >&2; exit 1; }

  run_required "sheets" "sheets metadata" gog sheets metadata "$sheet_id" --json >/dev/null
  run_required "sheets" "sheets update" gog sheets update "$sheet_id" "Sheet1!A1:B2" --values-json '[["A1","B1"],["A2","B2"]]' --json >/dev/null
  run_required "sheets" "sheets batch-update" gog sheets batch-update "$sheet_id" --data-json '[{"range":"Sheet1!C1:D1","values":[["C1","D1"]]},{"range":"Sheet1!C2:D2","values":[["C2","D2"]]}]' --json >/dev/null
  run_required "sheets" "sheets get" gog sheets get "$sheet_id" "Sheet1!A1:B2" --json >/dev/null
  run_required "sheets" "sheets append" gog sheets append "$sheet_id" "Sheet1!A3:B3" --values-json '[["A3","B3"]]' --json >/dev/null
  run_required "sheets" "sheets format" gog sheets format "$sheet_id" "Sheet1!A1:B1" --format-json '{"textFormat":{"bold":true}}' --format-fields textFormat.bold --json >/dev/null
  run_required "sheets" "sheets clear" gog sheets clear "$sheet_id" "Sheet1!A1:B3" --json >/dev/null

  export_path="$LIVE_TMP/sheets-export-$TS.xlsx"
  run_required "sheets" "sheets export" gog sheets export "$sheet_id" --format xlsx --out "$export_path" >/dev/null

  copy_json=$(gog sheets copy "$sheet_id" "gogcli-smoke-sheet-copy-$TS" --json)
  copy_id=$(extract_id "$copy_json")
  [ -n "$copy_id" ] || { echo "Failed to parse sheet copy id" >&2; exit 1; }

  run_required "sheets" "drive delete sheet copy" gog drive delete "$copy_id" --force >/dev/null
  run_required "sheets" "drive delete sheet" gog drive delete "$sheet_id" --force >/dev/null
}
