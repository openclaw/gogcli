#!/usr/bin/env bash
set -euo pipefail

ACCOUNT=""
ALLOW_NONTEST=false

usage() {
  cat <<'USAGE'
Usage: scripts/live-chat-test.sh [options]

Options:
  --account <email>   Account to use (defaults to RATA_IT_ACCOUNT or first auth)
  --allow-nontest     Allow running against non-test accounts
  -h, --help          Show this help

Env:
  RATA_LIVE_CHAT_SPACE=spaces/<id>        Existing space to use for list/send
  RATA_LIVE_CHAT_THREAD=<id|resource>    Thread id or resource for sends
  RATA_LIVE_CHAT_DM=user@domain          DM target (workspace user)
  RATA_LIVE_CHAT_DM_THREAD=<id|resource> Thread id for DM send
  RATA_LIVE_CHAT_CREATE=1                Create a new space (no cleanup)
  RATA_LIVE_CHAT_MEMBER=user@domain      Member to add when creating a space
  RATA_LIVE_ALLOW_NONTEST=1              Allow non-test accounts
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --account)
      ACCOUNT="$2"
      shift
      ;;
    --allow-nontest)
      ALLOW_NONTEST=true
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

BIN="${RATA_BIN:-${GOG_BIN:-./bin/rata}}"
if [ ! -x "$BIN" ]; then
  make build >/dev/null
fi

PY="${PYTHON:-python3}"
if ! command -v "$PY" >/dev/null 2>&1; then
  PY="python"
fi

if [ -z "$ACCOUNT" ]; then
  ACCOUNT="${RATA_IT_ACCOUNT:-${GOG_IT_ACCOUNT:-}}"
fi
if [ -z "$ACCOUNT" ]; then
  acct_json=$($BIN auth list --json)
  ACCOUNT=$($PY -c 'import json,sys; obj=json.load(sys.stdin); print(obj.get("accounts", [{}])[0].get("email", ""))' <<<"$acct_json")
fi
if [ -z "$ACCOUNT" ]; then
  echo "No account available for live tests." >&2
  exit 1
fi

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

if [ "${ALLOW_NONTEST:-false}" = false ] && [ -z "${RATA_LIVE_ALLOW_NONTEST:-${GOG_LIVE_ALLOW_NONTEST:-}}" ]; then
  if ! is_test_account "$ACCOUNT"; then
    echo "Refusing to run live tests against non-test account: $ACCOUNT" >&2
    echo "Pass --allow-nontest or set RATA_LIVE_ALLOW_NONTEST=1 to override." >&2
    exit 2
  fi
fi

if is_consumer_account "$ACCOUNT"; then
  echo "==> chat (skipped; Workspace only)"
  exit 0
fi

rata() {
  "$BIN" --account "$ACCOUNT" "$@"
}

TS=$(date +%Y%m%d%H%M%S)

echo "Using account: $ACCOUNT"
echo "==> chat spaces list"
rata chat spaces list --json --max 1 >/dev/null

if [ -n "${RATA_LIVE_CHAT_SPACE:-${GOG_LIVE_CHAT_SPACE:-}}" ]; then
  CHAT_SPACE="${RATA_LIVE_CHAT_SPACE:-${GOG_LIVE_CHAT_SPACE:-}}"
  echo "==> chat messages list"
  rata chat messages list "$CHAT_SPACE" --json --max 1 >/dev/null
  echo "==> chat threads list"
  rata chat threads list "$CHAT_SPACE" --json --max 1 >/dev/null
  echo "==> chat messages send"
  if [ -n "${RATA_LIVE_CHAT_THREAD:-${GOG_LIVE_CHAT_THREAD:-}}" ]; then
    CHAT_THREAD="${RATA_LIVE_CHAT_THREAD:-${GOG_LIVE_CHAT_THREAD:-}}"
    rata chat messages send "$CHAT_SPACE" --text "ratatosk smoke $TS" --thread "$CHAT_THREAD" --json >/dev/null
  else
    rata chat messages send "$CHAT_SPACE" --text "ratatosk smoke $TS" --json >/dev/null
  fi
else
  echo "==> chat messages/threads (skipped; set RATA_LIVE_CHAT_SPACE)"
fi

if [ -n "${RATA_LIVE_CHAT_CREATE:-${GOG_LIVE_CHAT_CREATE:-}}" ]; then
  if [ -z "${RATA_LIVE_CHAT_MEMBER:-${GOG_LIVE_CHAT_MEMBER:-}}" ]; then
    echo "==> chat spaces create (skipped; set RATA_LIVE_CHAT_MEMBER)"
  else
    CHAT_MEMBER="${RATA_LIVE_CHAT_MEMBER:-${GOG_LIVE_CHAT_MEMBER:-}}"
    echo "==> chat spaces create"
    rata chat spaces create "ratatosk-smoke-$TS" --member "$CHAT_MEMBER" --json >/dev/null
  fi
fi

if [ -n "${RATA_LIVE_CHAT_DM:-${GOG_LIVE_CHAT_DM:-}}" ]; then
  CHAT_DM="${RATA_LIVE_CHAT_DM:-${GOG_LIVE_CHAT_DM:-}}"
  echo "==> chat dm space"
  rata chat dm space "$CHAT_DM" --json >/dev/null
  echo "==> chat dm send"
  if [ -n "${RATA_LIVE_CHAT_DM_THREAD:-${GOG_LIVE_CHAT_DM_THREAD:-}}" ]; then
    CHAT_DM_THREAD="${RATA_LIVE_CHAT_DM_THREAD:-${GOG_LIVE_CHAT_DM_THREAD:-}}"
    rata chat dm send "$CHAT_DM" --text "ratatosk dm $TS" --thread "$CHAT_DM_THREAD" --json >/dev/null
  else
    rata chat dm send "$CHAT_DM" --text "ratatosk dm $TS" --json >/dev/null
  fi
else
  echo "==> chat dm (skipped; set RATA_LIVE_CHAT_DM)"
fi

echo "Chat live tests complete."
