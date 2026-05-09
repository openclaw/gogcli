#!/usr/bin/env bash

set -euo pipefail

run_contacts_tests() {
  if skip "contacts"; then
    echo "==> contacts (skipped)"
    return 0
  fi

  run_required "contacts" "contacts list" gog contacts list --json --max 1 >/dev/null

  local contact_json contact_id
  contact_json=$(gog contacts create --given "gogcli" --family "smoke-$TS" --email "gogcli-smoke-$TS@example.com" --phone "+1555555$TS" --json)
  contact_id=$(extract_field "$contact_json" resourceName)
  [ -n "$contact_id" ] || { echo "Failed to parse contact resourceName" >&2; exit 1; }

  run_required "contacts" "contacts get" gog contacts get "$contact_id" --json >/dev/null
  run_required "contacts" "contacts update" gog contacts update "$contact_id" --given "gogcli" --family "smoke-updated-$TS" --email "gogcli-smoke-$TS@example.com" --birthday "1990-05-12" --notes "gogcli smoke $TS" --json >/dev/null
  run_required "contacts" "contacts search" gog contacts search "gogcli-smoke-$TS@example.com" --json --max 1 >/dev/null
  local export_path="$LIVE_TMP/contacts-export-$TS.vcf"
  run_required "contacts" "contacts export" gog contacts export "$contact_id" --out "$export_path" >/dev/null
  grep -q "EMAIL:gogcli-smoke-$TS@example.com" "$export_path" || { echo "contacts export missing email" >&2; exit 1; }
  grep -q "BDAY:19900512" "$export_path" || { echo "contacts export missing birthday" >&2; exit 1; }
  run_required "contacts" "contacts delete" gog contacts delete "$contact_id" --force >/dev/null

  if is_consumer_account "$ACCOUNT"; then
    echo "==> contacts directory (skipped; Workspace only)"
    echo "==> contacts other (skipped; Workspace only)"
  else
    run_optional "contacts-directory" "contacts directory list" gog contacts directory list --json --max 1 >/dev/null
    run_optional "contacts-directory" "contacts directory search" gog contacts directory search "gogcli" --json --max 1 >/dev/null
    run_optional "contacts-other" "contacts other list" gog contacts other list --json --max 1 >/dev/null
    run_optional "contacts-other" "contacts other search" gog contacts other search "gogcli" --json --max 1 >/dev/null
  fi
}
