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

  export_path="$LIVE_TMP/slides-export-$TS.pdf"
  run_required "slides" "slides export" gog slides export "$slides_id" --format pdf --out "$export_path" >/dev/null

  copy_json=$(gog slides copy "$slides_id" "gogcli-smoke-slides-copy-$TS" --json)
  copy_id=$(extract_id "$copy_json")
  [ -n "$copy_id" ] || { echo "Failed to parse slides copy id" >&2; exit 1; }
  register_drive_cleanup "$copy_id"

  run_required "slides" "drive delete slides copy" gog drive delete "$copy_id" --force >/dev/null
  run_required "slides" "drive delete slides" gog drive delete "$slides_id" --force >/dev/null
}
