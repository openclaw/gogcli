#!/usr/bin/env bash

set -euo pipefail

run_slides_tests() {
  if skip "slides"; then
    echo "==> slides (skipped)"
    return 0
  fi

  local slides_json slides_id copy_json copy_id export_path
  slides_json=$(gog slides create "gogcli-smoke-slides-$TS" --json)
  slides_id=$(extract_id "$slides_json")
  [ -n "$slides_id" ] || { echo "Failed to parse slides id" >&2; exit 1; }
  register_drive_cleanup "$slides_id"

  run_required "slides" "slides info" gog slides info "$slides_id" --json >/dev/null

  local add_json slide_id read_json
  add_json=$(gog slides add-slide "$slides_id" "$ROOT_DIR/docs/social-card.png" \
    --notes "gogcli live $TS" --json)
  slide_id=$(extract_field "$add_json" slideObjectId)
  [ -n "$slide_id" ] || { echo "Failed to parse slide object id" >&2; exit 1; }
  run_required "slides" "slides insert image" gog slides insert-image "$slides_id" \
    "$slide_id" "$ROOT_DIR/docs/assets/readme-banner.jpg" \
    --x 24 --y 24 --width 180 --json >/dev/null
  read_json=$(gog slides read-slide "$slides_id" "$slide_id" --json)
  "$PY" -c 'import json,sys; assert len(json.load(sys.stdin).get("images", [])) >= 2' <<<"$read_json"

  local native_json native_slide_id native_read native_after_delete table_read table_after_delete
  native_json=$(gog slides new-slide "$slides_id" --json)
  native_slide_id=$(extract_field "$native_json" slideObjectId)
  [ -n "$native_slide_id" ] || { echo "Failed to parse native slide object id" >&2; exit 1; }

  run_required "slides" "slides create table" gog slides table create \
    "$slides_id" "$native_slide_id" --rows 3 --cols 3 --object-id gogTableStruct >/dev/null
  run_required "slides" "slides insert table row" gog slides table row insert \
    "$slides_id" gogTableStruct --row 0 --below >/dev/null
  run_required "slides" "slides insert table column" gog slides table column insert \
    "$slides_id" gogTableStruct --col 0 --right >/dev/null
  run_required "slides" "slides merge table cells" gog slides table merge \
    "$slides_id" gogTableStruct --row 0 --col 0 --row-span 1 --col-span 2 >/dev/null
  table_read=$(gog slides read-slide "$slides_id" "$native_slide_id" --detail --json)
  "$PY" -c '
import json,sys
elements = {item["objectId"]: item for item in json.load(sys.stdin).get("elements", [])}
table = elements["gogTableStruct"]["table"]
assert (table["rows"], table["columns"]) == (4, 4)
assert any(cell["rowIndex"] == 0 and cell["columnIndex"] == 0 and cell["columnSpan"] == 2 for cell in table["cells"])
' <<<"$table_read"
  run_required "slides" "slides unmerge table cells" gog slides table unmerge \
    "$slides_id" gogTableStruct --row 0 --col 0 --row-span 1 --col-span 2 >/dev/null
  run_required "slides" "slides delete table row" gog slides table row delete \
    "$slides_id" gogTableStruct --row 3 --force >/dev/null
  run_required "slides" "slides delete table column" gog slides table column delete \
    "$slides_id" gogTableStruct --col 3 --force >/dev/null
  table_after_delete=$(gog slides read-slide "$slides_id" "$native_slide_id" --detail --json)
  "$PY" -c '
import json,sys
elements = {item["objectId"]: item for item in json.load(sys.stdin).get("elements", [])}
table = elements["gogTableStruct"]["table"]
assert (table["rows"], table["columns"]) == (3, 3)
assert all(cell["rowSpan"] == 1 and cell["columnSpan"] == 1 for cell in table["cells"])
' <<<"$table_after_delete"

  run_required "slides" "slides create shape A" gog slides element create-shape \
    "$slides_id" "$native_slide_id" --type ROUND_RECTANGLE --x 24 --y 24 --width 180 --height 80 \
    --object-id gogShapeA >/dev/null
  run_required "slides" "slides create shape B" gog slides element create-shape \
    "$slides_id" "$native_slide_id" --type ELLIPSE --x 240 --y 24 --width 100 --height 80 \
    --object-id gogShapeB >/dev/null
  run_required "slides" "slides create line" gog slides element create-line \
    "$slides_id" "$native_slide_id" --category STRAIGHT --x 50 --y 150 --width 240 --height 40 \
    --object-id gogLineA >/dev/null
  run_required "slides" "slides style shape" gog slides element style \
    "$slides_id" gogShapeA --fill-color '#3367d6' --outline-color '#ffffff' \
    --outline-weight 2 --outline-dash SOLID >/dev/null
  run_required "slides" "slides style line" gog slides element style \
    "$slides_id" gogLineA --kind line --outline-color '#ea4335' --outline-weight 3 \
    --outline-dash DASH >/dev/null
  run_required "slides" "slides transform element" gog slides element transform \
    "$slides_id" gogShapeA --translate-x 12 --translate-y 6 >/dev/null
  run_required "slides" "slides set alt text" gog slides element alt-text \
    "$slides_id" gogShapeA --title "gogcli live shape" --description "Slides element lifecycle proof" >/dev/null
  run_required "slides" "slides change z-order" gog slides element z-order \
    "$slides_id" gogShapeA gogShapeB --operation BRING_TO_FRONT >/dev/null
  run_required "slides" "slides group elements" gog slides element group \
    "$slides_id" gogShapeA gogShapeB --group-id gogGroupA >/dev/null

  native_read=$(gog slides read-slide "$slides_id" "$native_slide_id" --detail --json)
  "$PY" -c '
import json,sys
elements = {item["objectId"]: item for item in json.load(sys.stdin).get("elements", [])}
assert {"gogShapeA", "gogShapeB", "gogLineA", "gogGroupA"} <= elements.keys()
assert elements["gogShapeA"].get("parentObjectId") == "gogGroupA"
assert elements["gogShapeA"].get("title") == "gogcli live shape"
' <<<"$native_read"

  run_required "slides" "slides ungroup elements" gog slides element ungroup \
    "$slides_id" gogGroupA >/dev/null
  run_required "slides" "slides delete line" gog slides element delete \
    "$slides_id" gogLineA --force >/dev/null
  run_required "slides" "slides delete shape A" gog slides element delete \
    "$slides_id" gogShapeA --force >/dev/null
  run_required "slides" "slides delete shape B" gog slides element delete \
    "$slides_id" gogShapeB --force >/dev/null
  native_after_delete=$(gog slides read-slide "$slides_id" "$native_slide_id" --detail --json)
  "$PY" -c '
import json,sys
ids = {item["objectId"] for item in json.load(sys.stdin).get("elements", [])}
assert not ({"gogShapeA", "gogShapeB", "gogLineA", "gogGroupA"} & ids)
' <<<"$native_after_delete"

  export_path="$LIVE_TMP/slides-export-$TS.pdf"
  run_required "slides" "slides export" gog slides export "$slides_id" --format pdf --out "$export_path" >/dev/null

  copy_json=$(gog slides copy "$slides_id" "gogcli-smoke-slides-copy-$TS" --json)
  copy_id=$(extract_id "$copy_json")
  [ -n "$copy_id" ] || { echo "Failed to parse slides copy id" >&2; exit 1; }
  register_drive_cleanup "$copy_id"

  run_required "slides" "drive delete slides copy" gog drive delete "$copy_id" --force >/dev/null
  run_required "slides" "drive delete slides" gog drive delete "$slides_id" --force >/dev/null
}
