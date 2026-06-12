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
  register_drive_cleanup "$sheet_id"

  run_required "sheets" "sheets metadata" gog sheets metadata "$sheet_id" --json >/dev/null
  run_required "sheets" "sheets update" gog sheets update "$sheet_id" "Sheet1!A1:B2" --values-json '[["A1","B1"],["A2","B2"]]' --json >/dev/null
  run_required "sheets" "sheets batch-update" gog sheets batch-update "$sheet_id" --data-json '[{"range":"Sheet1!C1:D1","values":[["C1","D1"]]},{"range":"Sheet1!C2:D2","values":[["C2","D2"]]}]' --json >/dev/null
  run_required "sheets" "sheets get" gog sheets get "$sheet_id" "Sheet1!A1:B2" --json >/dev/null
  run_required "sheets" "sheets append" gog sheets append "$sheet_id" "Sheet1!A3:B3" --values-json '[["A3","B3"]]' --json >/dev/null
  run_required "sheets" "sheets format" gog sheets format "$sheet_id" "Sheet1!A1:B1" --format-json '{"textFormat":{"bold":true}}' --format-fields textFormat.bold --json >/dev/null

  run_required "sheets" "sheets links set" gog sheets links set "$sheet_id" \
    "Sheet1!C1" https://example.com/a "Link A" --json >/dev/null
  run_required "sheets" "sheets rich links set" gog sheets links set "$sheet_id" \
    "Sheet1!D1" --runs-json '[{"text":"One","uri":"https://example.com/one"},{"text":" + "},{"text":"Two","uri":"https://example.com/two"}]' --json >/dev/null
  run_required "sheets" "sheets batch links set" gog sheets links set "$sheet_id" \
    --cells-json '[{"cell":"Sheet1!E1","url":"mailto:test@example.com","text":"Mail"}]' --json >/dev/null
  local links_json
  links_json=$(gog sheets links get "$sheet_id" "Sheet1!C1:E1" --json)
  "$PY" -c 'import json,sys
links={item["link"] for item in json.load(sys.stdin).get("links", [])}
assert {"https://example.com/a","https://example.com/one","https://example.com/two","mailto:test@example.com"} <= links' <<<"$links_json"

  run_required "sheets" "sheets validation set" gog sheets validation set "$sheet_id" \
    "Sheet1!A2:A3" --type ONE_OF_LIST --value Open --value Done --strict --json >/dev/null
  local validation_json
  validation_json=$(gog sheets validation get "$sheet_id" "Sheet1!A2:A3" --json)
  "$PY" -c 'import json,sys
rules=json.load(sys.stdin).get("validations", [])
assert rules and all(v.get("rule",{}).get("condition",{}).get("type") == "ONE_OF_LIST" for v in rules)' <<<"$validation_json"
  run_required "sheets" "sheets validation clear" gog sheets validation clear "$sheet_id" \
    "Sheet1!A2:A3" --json >/dev/null

  run_required "sheets" "sheets table seed" gog sheets update "$sheet_id" \
    "Sheet1!A5:C9" --values-json '[["Task","Amount","Done"],["one",1,false],["two",2,false],["three",3,true],["four",4,false]]' --json >/dev/null
  run_required "sheets" "sheets table create" gog sheets table create "$sheet_id" \
    "Sheet1!A5:C9" --name SmokeTable \
    --columns-json '[{"columnName":"Task","columnType":"TEXT"},{"columnName":"Amount","columnType":"DOUBLE"},{"columnName":"Done","columnType":"BOOLEAN"}]' --json >/dev/null
  local table_delete_rc
  if gog sheets table delete "$sheet_id" SmokeTable --force --json \
    >/dev/null 2>"$LIVE_TMP/sheets-table-delete-$TS.err"; then
    echo "Expected table delete without --discard-data to fail" >&2
    exit 1
  else
    table_delete_rc=$?
  fi
  [ "$table_delete_rc" -eq 2 ] || { echo "Unexpected table delete guard exit: $table_delete_rc" >&2; exit 1; }
  run_required "sheets" "sheets table-aware row delete" gog sheets delete-dimension "$sheet_id" \
    "Sheet1!7:7" --dimension ROWS --force --json >/dev/null
  local table_json
  table_json=$(gog sheets table get "$sheet_id" SmokeTable --json)
  "$PY" -c 'import json,sys; assert json.load(sys.stdin)["table"]["a1"] == "Sheet1!A5:C8"' <<<"$table_json"

  run_required "sheets" "sheets clear" gog sheets clear "$sheet_id" "Sheet1!A1:B3" --json >/dev/null

  export_path="$LIVE_TMP/sheets-export-$TS.xlsx"
  run_required "sheets" "sheets export" gog sheets export "$sheet_id" --format xlsx --out "$export_path" >/dev/null

  copy_json=$(gog sheets copy "$sheet_id" "gogcli-smoke-sheet-copy-$TS" --json)
  copy_id=$(extract_id "$copy_json")
  [ -n "$copy_id" ] || { echo "Failed to parse sheet copy id" >&2; exit 1; }
  register_drive_cleanup "$copy_id"

  run_required "sheets" "drive delete sheet copy" gog drive delete "$copy_id" --force >/dev/null
  run_required "sheets" "drive delete sheet" gog drive delete "$sheet_id" --force >/dev/null
}
