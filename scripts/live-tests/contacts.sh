#!/usr/bin/env bash

set -euo pipefail

run_contacts_other_tests() {
  if skip "contacts-other"; then
    echo "==> contacts other (skipped)"
    return 0
  fi

  local other_json other_query
  echo "==> contacts other list"
  other_json=$(gog contacts other list --json --max 1)
  other_query=$(extract_field "$other_json" email)
  if [ -z "$other_query" ]; then
    other_query="gogcli-smoke-$TS@example.com"
  fi
  run_required "contacts-other" "contacts other search" \
    gog contacts other search "$other_query" --json --max 1 >/dev/null
}

run_contacts_dedupe_apply_test() {
  local email first_json second_json first_id second_id dry_json dry_primary apply_json applied_primary merged_json

  email="gogcli-dedupe-$TS@example.com"
  first_json=$(gog contacts create \
    --given "gogcli" \
    --family "dedupe-$TS" \
    --email "$email" \
    --phone "+15550000001" \
    --json)
  first_id=$(extract_field "$first_json" resourceName)
  [ -n "$first_id" ] || { echo "Failed to parse first dedupe contact resourceName" >&2; exit 1; }
  register_contact_cleanup "$first_id"

  second_json=$(gog contacts create \
    --given "gogcli" \
    --family "dedupe-$TS" \
    --email "$email" \
    --phone "+15550000002" \
    --json)
  second_id=$(extract_field "$second_json" resourceName)
  [ -n "$second_id" ] || { echo "Failed to parse second dedupe contact resourceName" >&2; exit 1; }
  register_contact_cleanup "$second_id"

  dry_json=$(gog contacts dedupe \
    --resource "$first_id" \
    --resource "$second_id" \
    --match email \
    --apply \
    --dry-run \
    --json)
  dry_primary=$($PY -c 'import json,sys
obj=json.load(sys.stdin)
first,second=sys.argv[1:3]
request=obj.get("request") or {}
groups=request.get("groups") or []
if not obj.get("dry_run") or obj.get("op") != "contacts.dedupe.apply":
    raise SystemExit("unexpected dedupe dry-run envelope")
if request.get("groups_merged") != 1 or request.get("contacts_deleted") != 1 or len(groups) != 1:
    raise SystemExit("dedupe dry-run found unrelated duplicate groups; refusing live apply")
group=groups[0]
primary=(group.get("primary") or {}).get("resource")
deleted=[item.get("resource") for item in group.get("delete") or []]
if primary not in (first,second) or deleted != [second if primary == first else first]:
    raise SystemExit("dedupe dry-run did not target only disposable contacts")
print(primary)' "$first_id" "$second_id" <<<"$dry_json")

  apply_json=$(gog contacts dedupe \
    --resource "$first_id" \
    --resource "$second_id" \
    --match email \
    --apply \
    --force \
    --json)
  applied_primary=$($PY -c 'import json,sys
obj=json.load(sys.stdin)
want=sys.argv[1]
groups=obj.get("groups") or []
if not obj.get("applied") or obj.get("groups_merged") != 1 or obj.get("contacts_deleted") != 1 or len(groups) != 1:
    raise SystemExit("unexpected dedupe apply result")
primary=(groups[0].get("primary") or {}).get("resource")
if primary != want:
    raise SystemExit("dedupe apply primary changed after dry-run")
print(primary)' "$dry_primary" <<<"$apply_json")

  merged_json=$(gog contacts get "$applied_primary" --json)
  $PY -c 'import json,re,sys
obj=json.load(sys.stdin)
contact=obj.get("contact") or {}
emails={str(item.get("value","")).strip().lower() for item in contact.get("emailAddresses") or []}
phones={"".join(re.findall(r"\d", str(item.get("value","")))) for item in contact.get("phoneNumbers") or []}
if sys.argv[1].lower() not in emails:
    raise SystemExit("merged contact missing dedupe email")
if not {"15550000001","15550000002"}.issubset(phones):
    raise SystemExit("merged contact missing dedupe phone values")' "$email" <<<"$merged_json"

  run_required "contacts" "contacts dedupe cleanup" gog contacts delete "$applied_primary" --force >/dev/null
}

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
  register_contact_cleanup "$contact_id"

  run_required "contacts" "contacts get" gog contacts get "$contact_id" --json >/dev/null
  run_required "contacts" "contacts update" gog contacts update "$contact_id" --given "gogcli" --family "smoke-updated-$TS" --email "gogcli-smoke-$TS@example.com" --birthday "1990-05-12" --notes "gogcli smoke $TS" --json >/dev/null
  run_required "contacts" "contacts search" gog contacts search "gogcli-smoke-$TS@example.com" --json --max 1 >/dev/null
  local export_path="$LIVE_TMP/contacts-export-$TS.vcf"
  run_required "contacts" "contacts export" gog contacts export "$contact_id" --out "$export_path" >/dev/null
  grep -q "EMAIL:gogcli-smoke-$TS@example.com" "$export_path" || { echo "contacts export missing email" >&2; exit 1; }
  grep -q "BDAY:19900512" "$export_path" || { echo "contacts export missing birthday" >&2; exit 1; }
  run_required "contacts" "contacts delete" gog contacts delete "$contact_id" --force >/dev/null
  run_required "contacts" "contacts dedupe apply" run_contacts_dedupe_apply_test

  if is_consumer_account "$ACCOUNT"; then
    echo "==> contacts directory (skipped; Workspace only)"
  else
    run_optional "contacts-directory" "contacts directory list" gog contacts directory list --json --max 1 >/dev/null
    run_optional "contacts-directory" "contacts directory search" gog contacts directory search "gogcli" --json --max 1 >/dev/null
  fi

  run_contacts_other_tests
}
