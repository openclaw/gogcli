#!/usr/bin/env bash

set -euo pipefail

run_core_tests() {
  run_required "time" "time now" "$BIN" time now --json >/dev/null
  run_required "version" "version" "$BIN" version --json >/dev/null
  run_required "completion" "completion bash" "$BIN" completion bash >/dev/null

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
  fi
}
