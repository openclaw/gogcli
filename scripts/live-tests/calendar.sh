#!/usr/bin/env bash

set -euo pipefail

run_calendar_tests() {
  if skip "calendar"; then
    echo "==> calendar (skipped)"
    return 0
  fi

  read -r START END DAY1 DAY2 <<<"$($PY - <<'PY'
import datetime
now=datetime.datetime.now(datetime.timezone.utc).replace(minute=0, second=0, microsecond=0)
start=now + datetime.timedelta(hours=1)
end=start + datetime.timedelta(hours=1)
print(start.strftime('%Y-%m-%dT%H:%M:%SZ'), end.strftime('%Y-%m-%dT%H:%M:%SZ'), start.strftime('%Y-%m-%d'), (start+datetime.timedelta(days=1)).strftime('%Y-%m-%d'))
PY
)"

  run_required "calendar" "calendar list" rata calendar calendars --json --max 1 >/dev/null
  run_required "calendar" "calendar acl" rata calendar acl primary --json --max 1 >/dev/null
  run_required "calendar" "calendar colors" rata calendar colors --json >/dev/null
  run_required "calendar" "calendar time" rata calendar time --json >/dev/null

  local ev_json ev_id
  ev_json=$(rata calendar create primary --summary "ratatosk-smoke-$TS" --from "$START" --to "$END" --location "Test" --send-updates none --json)
  ev_id=$(extract_id "$ev_json")
  [ -n "$ev_id" ] || { echo "Failed to parse calendar event id" >&2; exit 1; }

  run_required "calendar" "calendar event get" rata calendar event primary "$ev_id" --json >/dev/null
  run_required "calendar" "calendar propose-time" rata calendar propose-time primary "$ev_id" --json >/dev/null
  run_required "calendar" "calendar update" rata calendar update primary "$ev_id" --summary "ratatosk-smoke-updated-$TS" --json >/dev/null
  run_required "calendar" "calendar events list" rata calendar events primary --from "$START" --to "$END" --json --max 5 >/dev/null
  run_required "calendar" "calendar search" rata calendar search "ratatosk-smoke" --from "$START" --to "$END" --json --max 5 >/dev/null
  run_required "calendar" "calendar freebusy" rata calendar freebusy primary --from "$START" --to "$END" --json >/dev/null
  run_required "calendar" "calendar conflicts" rata calendar conflicts --from "$START" --to "$END" --json >/dev/null

  if [ -n "${RATA_LIVE_CALENDAR_RESPOND:-${GOG_LIVE_CALENDAR_RESPOND:-}}" ]; then
    run_optional "calendar-respond" "calendar respond" rata calendar respond primary "$ev_id" --status accepted --json >/dev/null
  else
    echo "==> calendar respond (skipped; needs invite from another account)"
  fi

  run_required "calendar" "calendar delete event" rata calendar delete primary "$ev_id" --force >/dev/null

  if ! skip "calendar-enterprise"; then
    local focus_json focus_id ooo_json ooo_id wl_json wl_id
    focus_json=$(rata calendar create primary --event-type focus-time --from "$START" --to "$END" --json 2>/dev/null || true)
    if [ -n "$focus_json" ]; then
      focus_id=$(extract_id "$focus_json")
    else
      focus_id=""
    fi
    if [ -n "$focus_id" ]; then
      run_optional "calendar-enterprise" "calendar delete focus-time" rata calendar delete primary "$focus_id" --force >/dev/null
    else
      echo "==> calendar focus-time (skipped/failed)"
    fi

    ooo_json=$(rata calendar create primary --event-type out-of-office --from "$DAY1" --to "$DAY2" --all-day --json 2>/dev/null || true)
    if [ -n "$ooo_json" ]; then
      ooo_id=$(extract_id "$ooo_json")
    else
      ooo_id=""
    fi
    if [ -n "$ooo_id" ]; then
      run_optional "calendar-enterprise" "calendar delete out-of-office" rata calendar delete primary "$ooo_id" --force >/dev/null
    else
      echo "==> calendar out-of-office (skipped/failed)"
    fi

    wl_json=$(rata calendar create primary --event-type working-location --working-location-type office --working-office-label "HQ" --from "$DAY1" --to "$DAY2" --json 2>/dev/null || true)
    if [ -n "$wl_json" ]; then
      wl_id=$(extract_id "$wl_json")
    else
      wl_id=""
    fi
    if [ -n "$wl_id" ]; then
      run_optional "calendar-enterprise" "calendar delete working-location" rata calendar delete primary "$wl_id" --force >/dev/null
    else
      echo "==> calendar working-location (skipped/failed)"
    fi
  fi

  if [ -n "${RATA_LIVE_CALENDAR_RECURRENCE:-${GOG_LIVE_CALENDAR_RECURRENCE:-}}" ]; then
    local rec_json rec_id
    rec_json=$(rata calendar create primary --summary "ratatosk-recurring-$TS" --from "$START" --to "$END" --rrule "RRULE:FREQ=DAILY;COUNT=2" --reminder "popup:30m" --json)
    rec_id=$(extract_id "$rec_json")
    if [ -n "$rec_id" ]; then
      run_required "calendar" "calendar delete recurring" rata calendar delete primary "$rec_id" --force >/dev/null
    fi
  else
    echo "==> calendar recurrence/reminders (skipped; set RATA_LIVE_CALENDAR_RECURRENCE=1)"
  fi

  if [ -n "${RATA_LIVE_GROUP_EMAIL:-${GOG_LIVE_GROUP_EMAIL:-}}" ] && ! is_consumer_account "$ACCOUNT"; then
    run_optional "calendar-team" "calendar team" rata calendar team "${RATA_LIVE_GROUP_EMAIL:-$GOG_LIVE_GROUP_EMAIL}" --json --max 5 >/dev/null
  fi

  if is_consumer_account "$ACCOUNT"; then
    echo "==> calendar users (skipped; Workspace only)"
  else
    run_optional "calendar-users" "calendar users list" rata calendar users --json --max 1 >/dev/null
  fi
}
