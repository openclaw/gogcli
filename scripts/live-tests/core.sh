#!/usr/bin/env bash

set -euo pipefail

run_core_tests() {
  run_required "time" "time now" "$BIN" time now --json >/dev/null
  run_required "version" "version" "$BIN" version --json >/dev/null
  run_required "completion" "completion bash" "$BIN" completion bash >/dev/null
  if ! skip "schema"; then
    local schema_json
    echo "==> schema automation contract"
    schema_json=$("$BIN" schema --json)
    "$PY" -c 'import json,sys
obj=json.load(sys.stdin)
codes=obj.get("automation",{}).get("exit_codes",{})
assert codes.get("usage") == 2
assert codes.get("retryable") == 8
assert codes.get("orphaned") == 11' <<<"$schema_json"
  else
    echo "==> schema automation contract (skipped)"
  fi
  run_required "help" "git-style command help" "$BIN" help docs format >/dev/null

  if ! skip "output-precedence"; then
    local plain_output json_output
    echo "==> plain overrides JSON environment"
    plain_output=$(GOG_JSON=1 "$BIN" --plain time now)
    "$PY" -c 'import sys
text=sys.stdin.read()
assert text.startswith("timezone\t")' <<<"$plain_output"
    echo "==> JSON overrides plain environment"
    json_output=$(GOG_PLAIN=1 "$BIN" --json time now)
    "$PY" -c 'import json,sys; assert isinstance(json.load(sys.stdin), dict)' <<<"$json_output"
  else
    echo "==> output precedence (skipped)"
  fi

  local invalid_stdout invalid_stderr
  invalid_stdout="$LIVE_TMP/invalid-color.stdout"
  invalid_stderr="$LIVE_TMP/invalid-color.stderr"
  if "$BIN" --color invalid time now >"$invalid_stdout" 2>"$invalid_stderr"; then
    echo "Expected invalid color to fail, but it succeeded" >&2
    exit 1
  fi
  [ ! -s "$invalid_stdout" ] || { echo "Invalid color wrote to stdout" >&2; exit 1; }
  grep -q "invalid" "$invalid_stderr"

  if ! skip "auth-alias"; then
    local alias_name
    alias_name="smoke-$TS"
    run_required "auth-alias" "auth alias set" "$BIN" auth alias set "$alias_name" "$ACCOUNT" --json >/dev/null
    run_required "auth-alias" "auth alias list" "$BIN" auth alias list --json >/dev/null
    run_required "auth-alias" "auth alias unset" "$BIN" auth alias unset "$alias_name" --json >/dev/null
  fi

  run_required "auth" "auth list" "$BIN" auth list --json >/dev/null
  run_required "auth" "auth credentials list" "$BIN" auth credentials list --json >/dev/null
  run_required "auth" "auth services" "$BIN" auth services --json >/dev/null
  run_required "auth" "auth status" "$BIN" auth status --json >/dev/null
  run_required "auth" "auth tokens list" "$BIN" auth tokens list --json >/dev/null

  run_required "config" "config keys" "$BIN" config keys --json >/dev/null
  run_required "config" "config list" "$BIN" config list --json >/dev/null
  run_required "config" "config path" "$BIN" config path --json >/dev/null

  if ! skip "enable-commands"; then
    run_required "enable-commands" "enable-commands allow time" "$BIN" --enable-commands time time now --json >/dev/null
    if $BIN --enable-commands time gmail labels list >/dev/null 2>&1; then
      echo "Expected enable-commands to block gmail, but it succeeded" >&2
      exit 1
    else
      echo "enable-commands block OK"
    fi
    if $BIN --disable-commands gmail.labels gmail labels list >/dev/null 2>&1; then
      echo "Expected disable-commands to block gmail labels, but it succeeded" >&2
      exit 1
    else
      echo "disable-commands block OK"
    fi
    if $BIN --gmail-no-send gmail send --to nobody@example.com --subject Test --body Test >/dev/null 2>&1; then
      echo "Expected gmail-no-send to block send, but it succeeded" >&2
      exit 1
    else
      echo "gmail-no-send block OK"
    fi
    if $BIN --gmail-no-send gmail fwd msg-1 --to nobody@example.com >/dev/null 2>&1; then
      echo "Expected gmail-no-send to block forward alias, but it succeeded" >&2
      exit 1
    else
      echo "gmail-no-send forward alias block OK"
    fi
  fi
}
