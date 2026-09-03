#!/usr/bin/env bash

set -euo pipefail

# SearchMessages uses POST with a JSON request body.
# https://developers.google.com/workspace/chat/api/reference/rest/v1/spaces.messages/search
run_chat_search_tests() {
  local query basic_json full_json page_json page_token empty_query empty_rc
  query="${GOG_LIVE_CHAT_SEARCH_QUERY:-}"
  if [ -z "$query" ]; then
    echo "==> chat messages search (skipped; set GOG_LIVE_CHAT_SEARCH_QUERY)"
    return 0
  fi

  echo "==> chat messages search (basic)"
  basic_json=$(gog chat messages search "$query" --readonly --json --max 1 \
    --order "create_time desc" --view basic)
  assert_chat_search_json "$basic_json" basic

  echo "==> chat messages search (full Markdown)"
  full_json=$(gog chat messages search "$query" --readonly --json --max 1 \
    --order "create_time desc" --view full --markup markdown --wrap-untrusted)
  assert_chat_search_json "$full_json" full

  page_token=$(chat_search_next_page_token "$full_json")
  if [ -n "$page_token" ]; then
    echo "==> chat messages search (explicit page)"
    page_json=$(gog chat messages search "$query" --readonly --json --max 1 \
      --page "$page_token" --order "create_time desc" --view full \
      --markup markdown --wrap-untrusted)
    assert_chat_search_json "$page_json" full
  else
    echo "==> chat messages search explicit page (skipped; query has one result page)"
  fi

  echo "==> chat messages search (fail-empty)"
  empty_query="gogcli-live-no-match-$TS"
  if gog chat messages search "$empty_query" --readonly --json --max 1 \
    --fail-empty >/dev/null; then
    echo "chat search --fail-empty unexpectedly succeeded" >&2
    return 1
  else
    empty_rc=$?
  fi
  if [ "$empty_rc" -ne 3 ]; then
    echo "chat search --fail-empty returned $empty_rc, want 3" >&2
    return 1
  fi
}

chat_search_next_page_token() {
  $PY -c 'import json,sys
obj=json.load(sys.stdin)
value=obj.get("nextPageToken", "")
if not isinstance(value, str):
    raise SystemExit("chat search nextPageToken must be a string")
print(value)
' <<<"$1"
}

assert_chat_search_json() {
  local value="$1"
  local view="$2"
  $PY -c 'import json,sys
obj=json.load(sys.stdin)
view=sys.argv[1]
results=obj.get("results")
if not isinstance(results, list) or not results:
    raise SystemExit("chat search query must return at least one result")
if not isinstance(obj.get("nextPageToken", ""), str):
    raise SystemExit("chat search nextPageToken must be a string")
if view == "basic" and any("read" in item for item in results):
    raise SystemExit("basic chat search result unexpectedly includes read")
if any(not isinstance(item.get("resource"), str) or not item["resource"] for item in results):
    raise SystemExit("chat search result is missing resource")
' "$view" <<<"$value"
}
