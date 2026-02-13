#!/usr/bin/env bash

set -euo pipefail

PY="${PYTHON:-python3}"
if ! command -v "$PY" >/dev/null 2>&1; then
  PY="python"
fi

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

rata() {
  "$BIN" --account "$ACCOUNT" "$@"
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
  if [ "${ALLOW_NONTEST:-false}" = true ] || [ -n "${RATA_LIVE_ALLOW_NONTEST:-${GOG_LIVE_ALLOW_NONTEST:-}}" ]; then
    return 0
  fi
  if ! is_test_account "$ACCOUNT"; then
    echo "Refusing to run live tests against non-test account: $ACCOUNT" >&2
    echo "Pass --allow-nontest or set RATA_LIVE_ALLOW_NONTEST=1 to override." >&2
    exit 2
  fi
}
