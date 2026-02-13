#!/usr/bin/env bash

set -euo pipefail

run_drive_tests() {
  if skip "drive"; then
    echo "==> drive (skipped)"
    return 0
  fi

  run_required "drive" "drive ls" rata drive ls --json --max 1 >/dev/null
  run_optional "drive" "drive drives list" rata drive drives --json --max 1 >/dev/null

  local folder_a_json folder_b_json folder_a_id folder_b_id
  folder_a_json=$(rata drive mkdir "ratatosk-smoke-a-$TS" --json)
  folder_a_id=$(extract_id "$folder_a_json")
  [ -n "$folder_a_id" ] || { echo "Failed to parse folder A id" >&2; exit 1; }
  folder_b_json=$(rata drive mkdir "ratatosk-smoke-b-$TS" --json)
  folder_b_id=$(extract_id "$folder_b_json")
  [ -n "$folder_b_id" ] || { echo "Failed to parse folder B id" >&2; exit 1; }

  local upload_path upload_json file_id
  upload_path="$LIVE_TMP/drive-upload-$TS.txt"
  printf "drive upload %s\n" "$TS" >"$upload_path"
  upload_json=$(rata drive upload "$upload_path" --parent "$folder_a_id" --name "ratatosk-smoke-$TS.txt" --json)
  file_id=$(extract_id "$upload_json")
  [ -n "$file_id" ] || { echo "Failed to parse uploaded file id" >&2; exit 1; }

  run_required "drive" "drive get file" rata drive get "$file_id" --json >/dev/null
  run_required "drive" "drive rename" rata drive rename "$file_id" "ratatosk-smoke-renamed-$TS.txt" >/dev/null

  local copy_json copy_id
  copy_json=$(rata drive copy "$file_id" "ratatosk-smoke-copy-$TS.txt" --json)
  copy_id=$(extract_id "$copy_json")
  [ -n "$copy_id" ] || { echo "Failed to parse copy id" >&2; exit 1; }

  run_required "drive" "drive move" rata drive move "$file_id" --parent "$folder_b_id" --json >/dev/null
  run_required "drive" "drive search" rata drive search "name contains 'ratatosk-smoke'" --json --max 1 >/dev/null

  run_required "drive" "drive permissions" rata drive permissions "$file_id" --json >/dev/null

  local share_json perm_id perms_json
  share_json=$(rata drive share "$file_id" --email "$EMAIL_TEST" --role reader --json)
  perms_json=$(rata drive permissions "$file_id" --json --max 50)
  perm_id=$(extract_permission_id "$perms_json" "$EMAIL_TEST")
  if [ -z "$perm_id" ]; then
    perm_id=$(extract_field "$share_json" permissionId)
  fi
  [ -n "$perm_id" ] || { echo "Failed to parse permission id" >&2; exit 1; }
  run_required "drive" "drive unshare" rata drive unshare "$file_id" "$perm_id" --force >/dev/null

  run_required "drive" "drive url" rata drive url "$file_id" --json >/dev/null

  local comment_json comment_id
  comment_json=$(rata drive comments create "$file_id" "ratatosk comment $TS" --json)
  comment_id=$(extract_id "$comment_json")
  [ -n "$comment_id" ] || { echo "Failed to parse comment id" >&2; exit 1; }
  run_required "drive" "drive comments get" rata drive comments get "$file_id" "$comment_id" --json >/dev/null
  run_required "drive" "drive comments list" rata drive comments list "$file_id" --json >/dev/null
  run_required "drive" "drive comments update" rata drive comments update "$file_id" "$comment_id" "ratatosk comment updated $TS" --json >/dev/null
  run_required "drive" "drive comments reply" rata drive comments reply "$file_id" "$comment_id" "ratatosk reply $TS" --json >/dev/null
  run_required "drive" "drive comments delete" rata drive comments delete "$file_id" "$comment_id" --force >/dev/null

  local download_path
  download_path="$LIVE_TMP/drive-download-$TS.txt"
  run_required "drive" "drive download" rata drive download "$file_id" --out "$download_path" >/dev/null

  run_required "drive" "drive delete copy" rata drive delete "$copy_id" --force >/dev/null
  run_required "drive" "drive delete file" rata drive delete "$file_id" --force >/dev/null
  run_required "drive" "drive delete folder A" rata drive delete "$folder_a_id" --force >/dev/null
  run_required "drive" "drive delete folder B" rata drive delete "$folder_b_id" --force >/dev/null
}
