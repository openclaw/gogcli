#!/usr/bin/env bash

set -euo pipefail

PY="${PYTHON:-python3}"
if ! command -v "$PY" >/dev/null 2>&1; then
  PY="python"
fi

LIVE_DRIVE_CLEANUP_IDS=()
LIVE_BATCH_CLEANUP_IDS=()
LIVE_CALENDAR_CLEANUP_EVENTS=()
LIVE_GMAIL_CLEANUP_DRAFTS=()
LIVE_GMAIL_CLEANUP_THREADS=()
LIVE_PHOTOS_PICKER_CLEANUP_IDS=()

skip() {
  local key="$1"
  [ -n "${SKIP:-}" ] || return 1
  IFS=',' read -r -a items <<<"$SKIP"
  for item in "${items[@]}"; do
    if [ "$item" = "$key" ]; then
      return 0
    fi
  done
  return 1
}

run_required() {
  local key="$1"
  local label="$2"
  shift 2
  if skip "$key"; then
    echo "==> $label (skipped)"
    return 0
  fi
  echo "==> $label"
  "$@"
}

run_optional() {
  local key="$1"
  local label="$2"
  shift 2
  if skip "$key"; then
    echo "==> $label (skipped)"
    return 0
  fi
  echo "==> $label (optional)"
  if "$@"; then
    echo "ok"
    return 0
  fi
  echo "skipped/failed"
  if [ "${STRICT:-false}" = true ]; then
    return 1
  fi
  return 0
}

extract_id() {
  $PY -c 'import json,sys
obj=json.load(sys.stdin)

def find_id(x):
    if isinstance(x, dict):
        for key in ("id", "draftId", "spreadsheetId", "presentationId", "documentId", "topicId"):
            if isinstance(x.get(key), str):
                return x[key]
        for v in x.values():
            r=find_id(v)
            if r:
                return r
    if isinstance(x, list):
        for v in x:
            r=find_id(v)
            if r:
                return r
    return ""
print(find_id(obj))' <<<"$1"
}

extract_field() {
  local value="$1"
  local field="$2"
  $PY -c 'import json,sys
obj=json.load(sys.stdin)
key=sys.argv[1]

def find_field(x, k):
    if isinstance(x, dict):
        if k in x and isinstance(x[k], str):
            return x[k]
        for v in x.values():
            r=find_field(v, k)
            if r:
                return r
    if isinstance(x, list):
        for v in x:
            r=find_field(v, k)
            if r:
                return r
    return ""
print(find_field(obj, key))' "$field" <<<"$value"
}

extract_tasklist_id() {
  $PY -c 'import json,sys
obj=json.load(sys.stdin)
for key in ("tasklists","lists","items"):
    if isinstance(obj, dict) and obj.get(key):
        print(obj[key][0].get("id",""))
        sys.exit(0)
print("")' <<<"$1"
}

extract_task_ids() {
  $PY -c 'import json,sys
obj=json.load(sys.stdin)
ids=[]
if isinstance(obj, dict) and "tasks" in obj:
    ids=[t.get("id") for t in obj.get("tasks",[]) if t.get("id")]
elif isinstance(obj, dict) and "task" in obj:
    if obj["task"].get("id"):
        ids=[obj["task"]["id"]]
print("\n".join(ids))' <<<"$1"
}

extract_permission_id() {
  local value="$1"
  local email="$2"
  $PY -c 'import json,sys
obj=json.load(sys.stdin)
email=sys.argv[1].lower()
base=email
if "@" in email:
    local, domain = email.split("@", 1)
    if "+" in local:
        base = local.split("+", 1)[0] + "@" + domain
emails={email, base}

def find_permissions(x):
    if isinstance(x, dict):
        if isinstance(x.get("permissions"), list):
            return x["permissions"]
        for v in x.values():
            r = find_permissions(v)
            if r is not None:
                return r
    if isinstance(x, list):
        for v in x:
            r = find_permissions(v)
            if r is not None:
                return r
    return None

perms = find_permissions(obj) or []
for p in perms:
    if not isinstance(p, dict):
        continue
    addr = (p.get("emailAddress") or "").lower()
    if addr in emails:
        pid = p.get("id") or ""
        if pid:
            print(pid)
            sys.exit(0)
print("")' "$email" <<<"$value"
}

extract_keep_note_name() {
  $PY -c 'import json,sys,re
obj=json.load(sys.stdin)
def find(x):
    if isinstance(x, dict):
        name = x.get("name")
        if isinstance(name, str) and name.startswith("notes/"):
            return name
        for v in x.values():
            r = find(v)
            if r:
                return r
    if isinstance(x, list):
        for v in x:
            r = find(v)
            if r:
                return r
    return ""
print(find(obj))' <<<"$1"
}

extract_keep_attachment_name() {
  $PY -c 'import json,sys,re
obj=json.load(sys.stdin)
def find(x):
    if isinstance(x, dict):
        name = x.get("name")
        if isinstance(name, str) and "/attachments/" in name:
            return name
        for v in x.values():
            r = find(v)
            if r:
                return r
    if isinstance(x, list):
        for v in x:
            r = find(v)
            if r:
                return r
    return ""
print(find(obj))' <<<"$1"
}

gog() {
  local attempt max_attempts rc
  max_attempts="${GOG_LIVE_RETRIES:-3}"
  case "$max_attempts" in
    ''|*[!0-9]*|0)
      max_attempts=3
      ;;
  esac

  for ((attempt = 1; attempt <= max_attempts; attempt++)); do
    if "$BIN" --account "$ACCOUNT" "$@"; then
      return 0
    else
      rc=$?
    fi
    if [ "$rc" -ne 8 ] || [ "$attempt" -eq "$max_attempts" ]; then
      return "$rc"
    fi
    echo "retryable gog failure; retry $attempt/$max_attempts: $*" >&2
    sleep "$attempt"
  done
}

register_drive_cleanup() {
  [ -n "${1:-}" ] && LIVE_DRIVE_CLEANUP_IDS+=("$1")
}

register_batch_cleanup() {
  [ -n "${1:-}" ] && LIVE_BATCH_CLEANUP_IDS+=("$1")
}

register_calendar_cleanup() {
  [ -n "${1:-}" ] && [ -n "${2:-}" ] && LIVE_CALENDAR_CLEANUP_EVENTS+=("$1"$'\t'"$2")
}

register_gmail_draft_cleanup() {
  [ -n "${1:-}" ] && LIVE_GMAIL_CLEANUP_DRAFTS+=("$1")
}

register_gmail_thread_cleanup() {
  [ -n "${1:-}" ] && LIVE_GMAIL_CLEANUP_THREADS+=("$1")
}

register_photos_picker_cleanup() {
  [ -n "${1:-}" ] && LIVE_PHOTOS_PICKER_CLEANUP_IDS+=("$1")
}

cleanup_live_resources() {
  local entry calendar_id event_id id

  for id in "${LIVE_GMAIL_CLEANUP_DRAFTS[@]}"; do
    gog gmail drafts delete "$id" --force >/dev/null 2>&1 || true
  done
  for id in "${LIVE_BATCH_CLEANUP_IDS[@]}"; do
    "$BIN" batch abort "$id" >/dev/null 2>&1 || true
  done
  for entry in "${LIVE_CALENDAR_CLEANUP_EVENTS[@]}"; do
    IFS=$'\t' read -r calendar_id event_id <<<"$entry"
    gog calendar delete "$calendar_id" "$event_id" --force >/dev/null 2>&1 || true
  done
  for id in "${LIVE_GMAIL_CLEANUP_THREADS[@]}"; do
    gog gmail thread modify "$id" --add TRASH --json >/dev/null 2>&1 || true
  done
  for id in "${LIVE_PHOTOS_PICKER_CLEANUP_IDS[@]}"; do
    gog photos picker delete "$id" --force --json >/dev/null 2>&1 || true
  done
  for id in "${LIVE_DRIVE_CLEANUP_IDS[@]}"; do
    gog drive delete "$id" --force >/dev/null 2>&1 || true
  done
}

is_test_account() {
  local a
  a=$(echo "$1" | tr 'A-Z' 'a-z')
  case "$a" in
    *test*|*bot*|*sandbox*|*qa*|*staging*|*dev*|*@example.com)
      return 0
      ;;
  esac
  case "$a" in
    *+*)
      return 0
      ;;
  esac
  return 1
}

is_consumer_account() {
  local a domain
  a=$(echo "$1" | tr 'A-Z' 'a-z')
  domain="${a##*@}"
  case "$domain" in
    gmail.com|googlemail.com)
      return 0
      ;;
  esac
  return 1
}

ensure_test_account() {
  if [ "${ALLOW_NONTEST:-false}" = true ] || [ -n "${GOG_LIVE_ALLOW_NONTEST:-}" ]; then
    return 0
  fi
  if ! is_test_account "$ACCOUNT"; then
    echo "Refusing to run live tests against non-test account: $ACCOUNT" >&2
    echo "Pass --allow-nontest or set GOG_LIVE_ALLOW_NONTEST=1 to override." >&2
    exit 2
  fi
}
