#!/usr/bin/env bash

set -euo pipefail

extract_history_id() {
  $PY -c 'import json,sys
obj=json.load(sys.stdin)
def find(x):
    if isinstance(x, dict):
        v = x.get("historyId")
        if isinstance(v, (str,int)):
            return str(v)
        for val in x.values():
            r = find(val)
            if r:
                return r
    if isinstance(x, list):
        for val in x:
            r = find(val)
            if r:
                return r
    return ""
print(find(obj))' <<<"$1"
}

extract_attachment_id() {
  $PY -c 'import json,sys
obj=json.load(sys.stdin)
def find(x):
    if isinstance(x, dict):
        v = x.get("attachmentId")
        if isinstance(v, str) and v:
            return v
        for val in x.values():
            r = find(val)
            if r:
                return r
    if isinstance(x, list):
        for val in x:
            r = find(val)
            if r:
                return r
    return ""
print(find(obj))' <<<"$1"
}

run_gmail_tests() {
  if skip "gmail"; then
    echo "==> gmail (skipped)"
    return 0
  fi

  run_required "gmail" "gmail labels list" gog gmail labels list --json >/dev/null
  run_required "gmail" "gmail labels get" gog gmail labels get INBOX --json >/dev/null

  if ! skip "gmail-settings"; then
    local sendas_json sendas_email
    echo "==> gmail settings sendas list"
    sendas_json=$(gog gmail settings sendas list --json)
    sendas_email=$(extract_field "$sendas_json" sendAsEmail)
    if [ -n "$sendas_email" ]; then
      run_required "gmail" "gmail settings sendas get" gog gmail settings sendas get "$sendas_email" --json >/dev/null
    else
      echo "==> gmail settings sendas get (skipped; no aliases)"
    fi
    run_required "gmail" "gmail settings vacation get" gog gmail settings vacation get --json >/dev/null
    run_required "gmail" "gmail settings filters list" gog gmail settings filters list --json >/dev/null
    if is_consumer_account "$ACCOUNT"; then
      echo "==> gmail delegates (skipped; Workspace/SA only)"
    else
      local delegates_json delegate_email
      echo "==> gmail settings delegates list (optional)"
      if delegates_json=$(gog gmail settings delegates list --json); then
        echo "ok"
      else
        echo "skipped/failed"
        if [ "${STRICT:-false}" = true ]; then
          return 1
        fi
        delegates_json=""
      fi
      if [ -n "$delegates_json" ]; then
        delegate_email=$(extract_field "$delegates_json" delegateEmail)
        if [ -n "$delegate_email" ]; then
          run_optional "gmail-delegates" "gmail settings delegates get" gog gmail settings delegates get "$delegate_email" --json >/dev/null
        else
          echo "==> gmail settings delegates get (skipped; no delegates)"
        fi
      fi
    fi
    local forwarding_json forwarding_email
    echo "==> gmail settings forwarding list"
    forwarding_json=$(gog gmail settings forwarding list --json)
    forwarding_email=$(extract_field "$forwarding_json" forwardingEmail)
    if [ -n "$forwarding_email" ]; then
      run_required "gmail" "gmail settings forwarding get" gog gmail settings forwarding get "$forwarding_email" --json >/dev/null
    else
      echo "==> gmail settings forwarding get (skipped; no forwarding)"
    fi
    run_required "gmail" "gmail settings autoforward get" gog gmail settings autoforward get --json >/dev/null
  fi

  if [ -n "${GOG_LIVE_GMAIL_FILTERS:-}" ]; then
    local filter_json filter_id
    filter_json=$(gog gmail filters create --from "gogcli-smoke-$TS@example.com" --star --json)
    filter_id=$(extract_id "$filter_json")
    if [ -n "$filter_id" ]; then
      run_required "gmail" "gmail filters get" gog gmail filters get "$filter_id" --json >/dev/null
      run_required "gmail" "gmail filters delete" gog --force gmail filters delete "$filter_id" --json >/dev/null
    fi
  else
    echo "==> gmail filters (skipped; set GOG_LIVE_GMAIL_FILTERS=1)"
  fi

  local draft_json draft_id sent_draft_json sent_draft_msg_id
  draft_json=$(gog gmail drafts create --to "$EMAIL_TEST" --subject "gogcli smoke draft $TS" --body "smoke draft" --json)
  draft_id=$(extract_field "$draft_json" draftId)
  [ -n "$draft_id" ] || { echo "Failed to parse draft id" >&2; exit 1; }
  run_required "gmail" "gmail drafts list" gog gmail drafts list --json --max 1 >/dev/null
  run_required "gmail" "gmail drafts get" gog gmail drafts get "$draft_id" --json >/dev/null
  run_required "gmail" "gmail drafts update" gog gmail drafts update "$draft_id" --subject "gogcli smoke draft updated $TS" --body "updated" --json >/dev/null
  sent_draft_json=$(gog gmail drafts send "$draft_id" --json)
  sent_draft_msg_id=$(extract_field "$sent_draft_json" messageId)
  [ -n "$sent_draft_msg_id" ] || { echo "Failed to parse sent draft message id" >&2; exit 1; }

  local delete_draft_json delete_draft_id
  delete_draft_json=$(gog gmail drafts create --to "$EMAIL_TEST" --subject "gogcli smoke draft delete $TS" --body "delete" --json)
  delete_draft_id=$(extract_field "$delete_draft_json" draftId)
  if [ -n "$delete_draft_id" ]; then
    run_required "gmail" "gmail drafts delete" gog --force gmail drafts delete "$delete_draft_id" --json >/dev/null
  fi

  local thread_seed_json recent_thread_id recent_attach_path recent_draft_json recent_draft_id recent_update_json recent_clear_json recent_thread_json
  recent_attach_path="$LIVE_TMP/gmail-recent-attach-$TS.txt"
  printf "recent attachment %s\n" "$TS" >"$recent_attach_path"
  thread_seed_json=$(gog gmail send --to "$ACCOUNT" --subject "gogcli recent thread $TS" --body "thread seed $TS" --json)
  recent_thread_id=$(extract_field "$thread_seed_json" threadId)
  [ -n "$recent_thread_id" ] || { echo "Failed to parse recent Gmail thread id" >&2; exit 1; }
  register_gmail_thread_cleanup "$recent_thread_id"
  recent_draft_json=$(gog gmail drafts create --to "$ACCOUNT" \
    --subject "Re: gogcli recent thread $TS" --body "draft reply" \
    --thread-id "$recent_thread_id" --attach "$recent_attach_path" --json)
  recent_draft_id=$(extract_field "$recent_draft_json" draftId)
  [ -n "$recent_draft_id" ] || { echo "Failed to parse threaded draft id" >&2; exit 1; }
  register_gmail_draft_cleanup "$recent_draft_id"
  "$PY" -c 'import json,sys
expected=sys.argv[1]
attachments=json.load(sys.stdin).get("attachments", [])
assert any(a.get("filename") == expected and a.get("size", 0) > 0 for a in attachments)' \
    "gmail-recent-attach-$TS.txt" <<<"$recent_draft_json"

  recent_update_json=$(gog gmail drafts update "$recent_draft_id" \
    --subject "Re: gogcli recent thread $TS" --body "updated reply" \
    --thread-id "$recent_thread_id" --json)
  "$PY" -c 'import json,sys
attachments=json.load(sys.stdin).get("attachments", [])
assert attachments and attachments[0].get("size", 0) > 0' <<<"$recent_update_json"
  recent_clear_json=$(gog gmail drafts update "$recent_draft_id" \
    --subject "Re: gogcli recent thread $TS" --body "cleared reply" \
    --thread-id "$recent_thread_id" --clear-attachments --json)
  "$PY" -c 'import json,sys; assert not json.load(sys.stdin).get("attachments")' <<<"$recent_clear_json"
  run_required "gmail" "gmail threaded draft delete" gog gmail drafts delete \
    "$recent_draft_id" --force >/dev/null
  run_required "gmail" "gmail archive thread" gog gmail archive \
    --thread "$recent_thread_id" --json >/dev/null
  recent_thread_json=$(gog gmail thread get "$recent_thread_id" --json)
  "$PY" -c 'import json,sys
obj=json.load(sys.stdin)
def walk(value):
    if isinstance(value, dict):
        if "labelIds" in value and "INBOX" in (value.get("labelIds") or []):
            return False
        return all(walk(v) for v in value.values())
    if isinstance(value, list):
        return all(walk(v) for v in value)
    return True
assert walk(obj)' <<<"$recent_thread_json"

  local body_file send_json send_msg_id send_thread_id
  body_file="$LIVE_TMP/gmail-body-$TS.txt"
  printf "hello from gogcli %s\n" "$TS" >"$body_file"
  send_json=$(gog gmail send --to "$EMAIL_TEST" --subject "gogcli smoke send $TS" --body-file "$body_file" --json)
  send_msg_id=$(extract_field "$send_json" messageId)
  send_thread_id=$(extract_field "$send_json" threadId)
  [ -n "$send_msg_id" ] || { echo "Failed to parse send message id" >&2; exit 1; }

  if skip "gmail-send-safety"; then
    echo "==> gmail send no-send block (skipped)"
  elif gog --gmail-no-send gmail send --to "$EMAIL_TEST" --subject "blocked $TS" --body "blocked" --json >/dev/null 2>&1; then
    echo "Expected gmail-no-send to block gmail send, but it succeeded" >&2
    exit 1
  else
    echo "gmail send no-send block OK"
  fi

  local message_json history_id
  echo "==> gmail get message"
  message_json=$(gog gmail get "$send_msg_id" --json)
  history_id=$(extract_history_id "$message_json")

  run_required "gmail-forward" "gmail forward dry-run" gog --dry-run gmail forward "$send_msg_id" --to "$EMAIL_TEST" --note "dry-run smoke $TS" --json >/dev/null
  if skip "gmail-forward"; then
    echo "==> gmail forward no-send block (skipped)"
  elif gog --gmail-no-send gmail forward "$send_msg_id" --to "$EMAIL_TEST" --note "blocked $TS" --json >/dev/null 2>&1; then
    echo "Expected gmail-no-send to block gmail forward, but it succeeded" >&2
    exit 1
  else
    echo "gmail forward no-send block OK"
  fi

  if [ -n "$history_id" ]; then
    run_optional "gmail-history" "gmail history" gog gmail history --since "$history_id" --json --max 5 >/dev/null
  else
    echo "==> gmail history (skipped; no historyId)"
  fi
  if [ -n "$send_thread_id" ]; then
    run_required "gmail" "gmail thread get" gog gmail thread get "$send_thread_id" --json >/dev/null
    run_required "gmail" "gmail thread modify add label" gog gmail thread modify "$send_thread_id" --add STARRED --json >/dev/null
    run_required "gmail" "gmail thread modify remove label" gog gmail thread modify "$send_thread_id" --remove STARRED --json >/dev/null
    run_required "gmail-url" "gmail url" gog gmail url "$send_thread_id" --json >/dev/null
  fi

  run_required "gmail" "gmail search" gog gmail search "subject:gogcli smoke send $TS" --json >/dev/null

  local messages_json
  echo "==> gmail messages search"
  messages_json=$(gog gmail messages search "subject:gogcli smoke send $TS" --json --max 5)
  $PY - "$TS" <<'PY' <<<"$messages_json"
import json,sys
ts=sys.argv[1]
obj=json.load(sys.stdin)
msgs=obj.get("messages") or []
if not msgs:
    raise SystemExit("no messages found for smoke search")
PY

  if skip "gmail-messages-body"; then
    echo "==> gmail messages search include body (skipped)"
  else
    local body_json
    echo "==> gmail messages search include body"
    body_json=$(gog gmail messages search "subject:gogcli smoke send $TS" --include-body --json --max 5)
    if ! $PY - "$TS" <<'PY' <<<"$body_json"; then
import json,sys
ts=sys.argv[1]
obj=json.load(sys.stdin)
msgs=obj.get("messages") or []
needle=f"hello from gogcli {ts}"
for m in msgs:
    body=m.get("body") or ""
    if needle in body:
        sys.exit(0)
raise SystemExit(f"missing body snippet: {needle}")
PY
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
    fi
  fi

  run_required "gmail" "gmail batch modify add" gog gmail batch modify "$send_msg_id" --add STARRED --json >/dev/null
  run_required "gmail" "gmail batch modify remove" gog gmail batch modify "$send_msg_id" --remove STARRED --json >/dev/null

  if ! skip "gmail-labels"; then
    local label_name label_ok
    label_name="gogcli-smoke"
    label_ok=false
    if gog gmail labels create "$label_name" --json >/dev/null 2>&1; then
      label_ok=true
    elif gog gmail labels get "$label_name" --json >/dev/null 2>&1; then
      label_ok=true
    fi
    if [ "$label_ok" = true ] && [ -n "$send_thread_id" ]; then
      run_required "gmail-labels" "gmail labels modify add" gog gmail labels modify "$send_thread_id" --add "$label_name" --json >/dev/null
      run_required "gmail-labels" "gmail labels modify remove" gog gmail labels modify "$send_thread_id" --remove "$label_name" --json >/dev/null
    else
      echo "==> gmail labels modify (skipped; label unavailable)"
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
    fi
  else
    echo "==> gmail labels modify (skipped)"
  fi

  if [ -z "${GOG_LIVE_GMAIL_BATCH_DELETE:-}" ] || skip "gmail-batch-delete"; then
    echo "==> gmail batch delete (skipped)"
  else
    echo "==> gmail batch delete"
    if gog --force gmail batch delete "$send_msg_id" "$sent_draft_msg_id" --json >/dev/null; then
      :
    else
      echo "gmail batch delete failed; falling back to trash" >&2
      gog gmail batch modify "$send_msg_id" "$sent_draft_msg_id" --add TRASH --json >/dev/null || true
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
    fi
  fi

  if skip "gmail-attachments"; then
    echo "==> gmail attachment (skipped)"
  else
    local attach_path attach_json attach_msg_id attach_msg_json attach_id attach_out
    attach_path="$LIVE_TMP/gmail-attach-$TS.txt"
    printf "attachment %s\n" "$TS" >"$attach_path"
    attach_json=$(gog gmail send --to "$EMAIL_TEST" --subject "gogcli smoke attach $TS" --body "attachment" --attach "$attach_path" --json)
    attach_msg_id=$(extract_field "$attach_json" messageId)
    if [ -n "$attach_msg_id" ]; then
      echo "==> gmail get attachment message"
      attach_msg_json=$(gog gmail get "$attach_msg_id" --json)
      attach_id=$(extract_attachment_id "$attach_msg_json")
      if [ -n "$attach_id" ]; then
        attach_out="$LIVE_TMP/gmail-attachment-$TS"
        run_required "gmail-attachments" "gmail attachment" gog gmail attachment "$attach_msg_id" "$attach_id" --out "$attach_out" >/dev/null
      else
        echo "No attachment id found" >&2
        if [ "${STRICT:-false}" = true ]; then
          return 1
        fi
      fi
    else
      echo "Failed to parse attachment message id" >&2
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
    fi
  fi

  if [ -n "${GOG_LIVE_TRACK:-}" ]; then
    run_optional "gmail-track" "gmail send --track" gog gmail send --to "$EMAIL_TEST" --subject "gogcli smoke track $TS" --body-html "<p>track $TS</p>" --track --json >/dev/null
    run_optional "gmail-track" "gmail track status" gog gmail track status --json >/dev/null
    run_optional "gmail-track" "gmail track opens" gog gmail track opens --json >/dev/null
  fi

  if [ -n "${GOG_LIVE_GMAIL_WATCH_TOPIC:-}" ]; then
    local watch_json
    if watch_json=$(gog gmail watch start --topic "$GOG_LIVE_GMAIL_WATCH_TOPIC" --json); then
      run_optional "gmail-watch" "gmail watch status" gog gmail watch status --json >/dev/null
      run_optional "gmail-watch" "gmail watch renew" gog gmail watch renew --json >/dev/null
      run_optional "gmail-watch" "gmail watch stop" gog --force gmail watch stop --json >/dev/null
    else
      echo "gmail watch start failed" >&2
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
    fi
  else
    echo "==> gmail watch (skipped; set GOG_LIVE_GMAIL_WATCH_TOPIC)"
  fi
}
