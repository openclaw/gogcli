#!/usr/bin/env bash

set -euo pipefail

run_drive_tests() {
  if skip "drive"; then
    echo "==> drive (skipped)"
    return 0
  fi

  run_required "drive" "drive ls" gog drive ls --json --max 1 >/dev/null
  run_optional "drive" "drive drives list" gog drive drives --json --max 1 >/dev/null

  local folder_a_json folder_b_json folder_a_id folder_b_id
  folder_a_json=$(gog drive mkdir "gogcli-smoke-a-$TS" --json)
  folder_a_id=$(extract_id "$folder_a_json")
  [ -n "$folder_a_id" ] || { echo "Failed to parse folder A id" >&2; exit 1; }
  register_drive_cleanup "$folder_a_id"
  folder_b_json=$(gog drive mkdir "gogcli-smoke-b-$TS" --json)
  folder_b_id=$(extract_id "$folder_b_json")
  [ -n "$folder_b_id" ] || { echo "Failed to parse folder B id" >&2; exit 1; }
  register_drive_cleanup "$folder_b_id"

  local upload_path upload_json file_id
  upload_path="$LIVE_TMP/drive-upload-$TS.txt"
  printf "drive upload %s\n" "$TS" >"$upload_path"
  upload_json=$(gog drive upload "$upload_path" --parent "$folder_a_id" --name "gogcli-smoke-$TS.txt" --json)
  file_id=$(extract_id "$upload_json")
  [ -n "$file_id" ] || { echo "Failed to parse uploaded file id" >&2; exit 1; }
  register_drive_cleanup "$file_id"

  run_required "drive" "drive get file" gog drive get "$file_id" --json >/dev/null
  run_required "drive" "drive rename" gog drive rename "$file_id" "gogcli-smoke-renamed-$TS.txt" >/dev/null

  local copy_json copy_id
  copy_json=$(gog drive copy "$file_id" "gogcli-smoke-copy-$TS.txt" --json)
  copy_id=$(extract_id "$copy_json")
  [ -n "$copy_id" ] || { echo "Failed to parse copy id" >&2; exit 1; }
  register_drive_cleanup "$copy_id"

  run_required "drive" "drive move" gog drive move "$file_id" --parent "$folder_b_id" --json >/dev/null
  run_required "drive" "drive search" gog drive search "name contains 'gogcli-smoke'" --json --max 1 >/dev/null

  local shortcut_json shortcut_id shortcut_get_json shortcut_target_id shortcut_list_json shortcut_tree_json
  shortcut_json=$(gog drive shortcut create "$file_id" --parent "$folder_a_id" --name "gogcli-smoke-shortcut-$TS" --json)
  shortcut_id=$(extract_id "$shortcut_json")
  [ -n "$shortcut_id" ] || { echo "Failed to parse shortcut id" >&2; exit 1; }
  register_drive_cleanup "$shortcut_id"

  shortcut_get_json=$(gog drive get "$shortcut_id" --json)
  shortcut_target_id=$(extract_field "$shortcut_get_json" targetId)
  [ "$shortcut_target_id" = "$file_id" ] || { echo "Shortcut target mismatch from drive get" >&2; exit 1; }

  shortcut_list_json=$(gog drive ls --parent "$folder_a_id" --json --max 10)
  shortcut_target_id=$(extract_field "$shortcut_list_json" targetId)
  [ "$shortcut_target_id" = "$file_id" ] || { echo "Shortcut target missing from drive ls" >&2; exit 1; }

  shortcut_tree_json=$(gog drive tree --parent "$folder_a_id" --json --depth 2)
  shortcut_target_id=$(extract_field "$shortcut_tree_json" targetId)
  [ "$shortcut_target_id" = "$file_id" ] || { echo "Shortcut target missing from drive tree" >&2; exit 1; }

  run_required "drive" "drive rename shortcut" gog drive rename "$shortcut_id" "gogcli-smoke-shortcut-renamed-$TS" >/dev/null
  local target_after_shortcut_rename target_name
  target_after_shortcut_rename=$(gog drive get "$file_id" --json)
  target_name=$(extract_field "$target_after_shortcut_rename" name)
  [ "$target_name" = "gogcli-smoke-renamed-$TS.txt" ] || { echo "Renaming shortcut mutated target name" >&2; exit 1; }

  run_required "drive" "drive permissions" gog drive permissions "$file_id" --json >/dev/null

  local share_json perm_id perms_json
  if [ "$(echo "$EMAIL_TEST" | tr 'A-Z' 'a-z')" = "$(echo "$ACCOUNT" | tr 'A-Z' 'a-z')" ]; then
    echo "==> drive share/unshare (skipped; test recipient is authenticated account)"
  else
    share_json=$(gog drive share "$file_id" --email "$EMAIL_TEST" --role reader --notify --json)
    perms_json=$(gog drive permissions "$file_id" --json --max 50)
    perm_id=$(extract_permission_id "$perms_json" "$EMAIL_TEST")
    if [ -z "$perm_id" ]; then
      perm_id=$(extract_field "$share_json" permissionId)
    fi
    [ -n "$perm_id" ] || { echo "Failed to parse permission id" >&2; exit 1; }
    run_required "drive" "drive unshare" gog drive unshare "$file_id" "$perm_id" --force >/dev/null
  fi

  run_required "drive" "drive url" gog drive url "$file_id" --json >/dev/null

  printf "drive replacement %s\n" "$TS" >"$upload_path"
  run_required "drive" "drive upload replace" gog drive upload "$upload_path" --replace "$file_id" --json >/dev/null
  local revisions_json revision_id
  revisions_json=$(gog drive revisions list "$file_id" --all --json)
  revision_id=$(extract_id "$revisions_json")
  [ -n "$revision_id" ] || { echo "Failed to parse revision id" >&2; exit 1; }
  run_required "drive" "drive revisions get" gog drive revisions get "$file_id" "$revision_id" --json >/dev/null

  local changes_state
  changes_state="$LIVE_TMP/drive-changes-$TS.json"
  run_required "drive" "drive changes poll" gog drive changes poll \
    --state-file "$changes_state" --max-iterations 1 --interval 1ms --json >/dev/null
  [ -s "$changes_state" ] || { echo "Drive changes poll did not persist state" >&2; exit 1; }

  local comment_since comment_json comment_id
  comment_since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  comment_json=$(gog drive comments create "$file_id" "gogcli comment $TS" --json)
  comment_id=$(extract_id "$comment_json")
  [ -n "$comment_id" ] || { echo "Failed to parse comment id" >&2; exit 1; }
  run_required "drive" "drive comments get" gog drive comments get "$file_id" "$comment_id" --json >/dev/null
  run_required "drive" "drive comments list" gog drive comments list "$file_id" --json >/dev/null
  run_required "drive" "drive comments since" gog drive comments list "$file_id" \
    --since "$comment_since" --json >/dev/null
  run_required "drive" "drive comments update" gog drive comments update "$file_id" "$comment_id" "gogcli comment updated $TS" --json >/dev/null
  run_required "drive" "drive comments reply" gog drive comments reply "$file_id" "$comment_id" "gogcli reply $TS" --json >/dev/null
  run_required "drive" "drive comments delete" gog drive comments delete "$file_id" "$comment_id" --force >/dev/null

  local download_path
  download_path="$LIVE_TMP/drive-download-$TS.txt"
  run_required "drive" "drive download" gog drive download "$file_id" --out "$download_path" >/dev/null

  run_required "drive" "drive delete shortcut" gog drive delete "$shortcut_id" --force >/dev/null
  run_required "drive" "drive delete copy" gog drive delete "$copy_id" --force >/dev/null
  run_required "drive" "drive delete file" gog drive delete "$file_id" --force >/dev/null
  run_required "drive" "drive delete folder A" gog drive delete "$folder_a_id" --force >/dev/null
  run_required "drive" "drive delete folder B" gog drive delete "$folder_b_id" --force >/dev/null
}
