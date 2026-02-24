#!/usr/bin/env bash
set -euo pipefail

FAST=false
STRICT=false
ALLOW_NONTEST=false
ACCOUNT=""
SKIP=""
AUTH_SERVICES=""
CLIENT=""
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

usage() {
  cat <<'USAGE'
Usage: scripts/live-test.sh [options]

Options:
  --fast              Skip slower tests (docs/sheets/slides)
  --strict            Fail on optional tests (groups/keep/enterprise)
  --allow-nontest     Allow running against non-test accounts
  --account <email>   Account to use (defaults to GOG_IT_ACCOUNT or first auth)
  --skip <list>       Comma-separated skip list (e.g., gmail,drive,docs)
  --auth <services>   Re-auth before running (e.g., all,groups)
  --client <name>     OAuth client to use (passes GOG_CLIENT)
  -h, --help          Show this help

Skip keys (base):
  time, version, completion, auth, auth-alias, config, enable-commands,
  gmail, gmail-settings, gmail-delegates, gmail-batch-delete, gmail-history, gmail-url, gmail-labels,
  gmail-attachments, gmail-track, gmail-watch, drive, docs, sheets, slides,
  calendar, calendar-enterprise, calendar-respond, calendar-team, calendar-users,
  tasks, contacts, people, groups, keep, classroom

Env:
  GOG_LIVE_EMAIL_TEST=steipete+gogtest@gmail.com
  GOG_LIVE_GROUP_EMAIL=<group@domain>
  GOG_LIVE_CLASSROOM_COURSE=<courseId>
  GOG_LIVE_CLASSROOM_CREATE=1
  GOG_LIVE_CLASSROOM_ALLOW_STATE=1
  GOG_LIVE_TRACK=1
  GOG_LIVE_ALLOW_NONTEST=1
  GOG_LIVE_CALENDAR_RESPOND=1
  GOG_LIVE_CALENDAR_ATTENDEE=attendee@domain.com
  GOG_LIVE_GMAIL_BATCH_DELETE=1
  GOG_LIVE_GMAIL_FILTERS=1
  GOG_LIVE_CLIENT=work
  GOG_LIVE_GMAIL_WATCH_TOPIC=projects/.../topics/...
  GOG_LIVE_CALENDAR_RECURRENCE=1
  GOG_KEEP_SERVICE_ACCOUNT=/path/to/service-account.json
  GOG_KEEP_IMPERSONATE=user@workspace-domain
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --fast)
      FAST=true
      ;;
    --strict)
      STRICT=true
      ;;
    --allow-nontest)
      ALLOW_NONTEST=true
      ;;
    --account)
      ACCOUNT="$2"
      shift
      ;;
    --skip)
      SKIP="$2"
      shift
      ;;
    --auth)
      AUTH_SERVICES="$2"
      shift
      ;;
    --client)
      CLIENT="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown arg: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if [ -n "${GOG_LIVE_FAST:-}" ]; then
  FAST=true
fi
if [ -z "$AUTH_SERVICES" ] && [ -n "${GOG_LIVE_AUTH:-}" ]; then
  AUTH_SERVICES="$GOG_LIVE_AUTH"
fi
if [ -z "$CLIENT" ] && [ -n "${GOG_LIVE_CLIENT:-}" ]; then
  CLIENT="$GOG_LIVE_CLIENT"
fi

SKIP="${SKIP:-${GOG_LIVE_SKIP:-}}"
if [ "$FAST" = true ]; then
  if [ -n "$SKIP" ]; then
    SKIP="$SKIP,docs,sheets,slides"
  else
    SKIP="docs,sheets,slides"
  fi
fi

BIN="${GOG_BIN:-$ROOT_DIR/bin/gog}"
if [ ! -x "$BIN" ]; then
  make -C "$ROOT_DIR" build >/dev/null
fi

if [ -n "$CLIENT" ]; then
  export GOG_CLIENT="$CLIENT"
  echo "Using OAuth client: $CLIENT"
fi

PY="${PYTHON:-python3}"
if ! command -v "$PY" >/dev/null 2>&1; then
  PY="python"
fi

if [ -z "$ACCOUNT" ]; then
  ACCOUNT="${GOG_IT_ACCOUNT:-}"
fi
if [ -z "$ACCOUNT" ]; then
  acct_json=$($BIN auth list --json)
  ACCOUNT=$($PY -c 'import json,sys; obj=json.load(sys.stdin); print(obj.get("accounts", [{}])[0].get("email", ""))' <<<"$acct_json")
fi
if [ -z "$ACCOUNT" ]; then
  echo "No account available for live tests." >&2
  exit 1
fi

echo "Using account: $ACCOUNT"

EMAIL_TEST="${GOG_LIVE_EMAIL_TEST:-steipete+gogtest@gmail.com}"
TS=$(date +%Y%m%d%H%M%S)
LIVE_TMP=$(mktemp -d "${TMPDIR:-/tmp}/gog-live-$TS-XXXX")
trap 'rm -rf "$LIVE_TMP"' EXIT

source "$ROOT_DIR/scripts/live-tests/common.sh"
source "$ROOT_DIR/scripts/live-tests/core.sh"
source "$ROOT_DIR/scripts/live-tests/gmail.sh"
source "$ROOT_DIR/scripts/live-tests/drive.sh"
source "$ROOT_DIR/scripts/live-tests/docs.sh"
source "$ROOT_DIR/scripts/live-tests/sheets.sh"
source "$ROOT_DIR/scripts/live-tests/slides.sh"
source "$ROOT_DIR/scripts/live-tests/calendar.sh"
source "$ROOT_DIR/scripts/live-tests/tasks.sh"
source "$ROOT_DIR/scripts/live-tests/contacts.sh"
source "$ROOT_DIR/scripts/live-tests/people.sh"
source "$ROOT_DIR/scripts/live-tests/workspace.sh"
source "$ROOT_DIR/scripts/live-tests/classroom.sh"

ensure_test_account

if [ -n "$AUTH_SERVICES" ]; then
  $BIN auth add "$ACCOUNT" --services "$AUTH_SERVICES"
fi

read -r START END DAY1 DAY2 <<<"$($PY - <<'PY'
import datetime
now=datetime.datetime.now(datetime.timezone.utc).replace(minute=0, second=0, microsecond=0)
start=now + datetime.timedelta(hours=1)
end=start + datetime.timedelta(hours=1)
print(start.strftime('%Y-%m-%dT%H:%M:%SZ'), end.strftime('%Y-%m-%dT%H:%M:%SZ'), start.strftime('%Y-%m-%d'), (start+datetime.timedelta(days=1)).strftime('%Y-%m-%d'))
PY
)"

run_core_tests
run_gmail_tests
run_drive_tests
run_docs_tests
run_sheets_tests
run_slides_tests
run_calendar_tests
run_tasks_tests
run_contacts_tests
run_people_tests
run_workspace_tests
run_classroom_tests

echo "Live tests complete."
