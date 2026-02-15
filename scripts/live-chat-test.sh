#!/usr/bin/env bash
set -euo pipefail

ACCOUNT=""
ALLOW_NONTEST=false
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

usage() {
  cat <<'USAGE'
Usage: scripts/live-chat-test.sh [options]

Options:
  --account <email>   Account to use (defaults to GOG_IT_ACCOUNT or first auth)
  --allow-nontest     Allow running against non-test accounts
  -h, --help          Show this help

Env:
  GOG_LIVE_CHAT_SPACE=spaces/<id>        Existing space to use for list/send
  GOG_LIVE_CHAT_THREAD=<id|resource>    Thread id or resource for sends
  GOG_LIVE_CHAT_DM=user@domain          DM target (workspace user)
  GOG_LIVE_CHAT_DM_THREAD=<id|resource> Thread id for DM send
  GOG_LIVE_CHAT_CREATE=1                Create a new space (no cleanup)
  GOG_LIVE_CHAT_MEMBER=user@domain      Member to add when creating a space
  GOG_LIVE_ALLOW_NONTEST=1              Allow non-test accounts
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

BIN="${GOG_BIN:-$ROOT_DIR/bin/gog}"
if [ ! -x "$BIN" ]; then
  make -C "$ROOT_DIR" build >/dev/null
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

if [ "${ALLOW_NONTEST:-false}" = false ] && [ -z "${GOG_LIVE_ALLOW_NONTEST:-}" ]; then
  if ! is_test_account "$ACCOUNT"; then
    echo "Refusing to run live tests against non-test account: $ACCOUNT" >&2
    echo "Pass --allow-nontest or set GOG_LIVE_ALLOW_NONTEST=1 to override." >&2
    exit 2
  fi
fi

if is_consumer_account "$ACCOUNT"; then
  echo "==> chat (skipped; Workspace only)"
  exit 0
fi

gog() {
  "$BIN" --account "$ACCOUNT" "$@"
}

TS=$(date +%Y%m%d%H%M%S)

echo "Using account: $ACCOUNT"
echo "==> chat spaces list"
gog chat spaces list --json --max 1 >/dev/null

if [ -n "${GOG_LIVE_CHAT_SPACE:-}" ]; then
  echo "==> chat messages list"
  gog chat messages list "$GOG_LIVE_CHAT_SPACE" --json --max 1 >/dev/null
  echo "==> chat threads list"
  gog chat threads list "$GOG_LIVE_CHAT_SPACE" --json --max 1 >/dev/null
  echo "==> chat messages send"
  if [ -n "${GOG_LIVE_CHAT_THREAD:-}" ]; then
    gog chat messages send "$GOG_LIVE_CHAT_SPACE" --text "gogcli smoke $TS" --thread "$GOG_LIVE_CHAT_THREAD" --json >/dev/null
  else
    gog chat messages send "$GOG_LIVE_CHAT_SPACE" --text "gogcli smoke $TS" --json >/dev/null
  fi
else
  echo "==> chat messages/threads (skipped; set GOG_LIVE_CHAT_SPACE)"
fi

if [ -n "${GOG_LIVE_CHAT_CREATE:-}" ]; then
  if [ -z "${GOG_LIVE_CHAT_MEMBER:-}" ]; then
    echo "==> chat spaces create (skipped; set GOG_LIVE_CHAT_MEMBER)"
  else
    echo "==> chat spaces create"
    gog chat spaces create "gogcli-smoke-$TS" --member "$GOG_LIVE_CHAT_MEMBER" --json >/dev/null
  fi
fi

if [ -n "${GOG_LIVE_CHAT_DM:-}" ]; then
  echo "==> chat dm space"
  gog chat dm space "$GOG_LIVE_CHAT_DM" --json >/dev/null
  echo "==> chat dm send"
  if [ -n "${GOG_LIVE_CHAT_DM_THREAD:-}" ]; then
    gog chat dm send "$GOG_LIVE_CHAT_DM" --text "gogcli dm $TS" --thread "$GOG_LIVE_CHAT_DM_THREAD" --json >/dev/null
  else
    gog chat dm send "$GOG_LIVE_CHAT_DM" --text "gogcli dm $TS" --json >/dev/null
  fi
else
  echo "==> chat dm (skipped; set GOG_LIVE_CHAT_DM)"
fi

echo "Chat live tests complete."
