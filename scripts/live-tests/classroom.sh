#!/usr/bin/env bash

set -euo pipefail

run_classroom_tests() {
  if skip "classroom"; then
    echo "==> classroom (skipped)"
    return 0
  fi

  run_optional "classroom" "classroom profile get" rata classroom profile get --json >/dev/null
  run_optional "classroom" "classroom courses list" rata classroom courses list --json --max 1 >/dev/null

  if [ -n "${RATA_LIVE_CLASSROOM_COURSE:-${GOG_LIVE_CLASSROOM_COURSE:-}}" ]; then
    local course_id cw_json cw_id
    course_id="${RATA_LIVE_CLASSROOM_COURSE:-$GOG_LIVE_CLASSROOM_COURSE}"
    run_optional "classroom" "classroom courses get" rata classroom courses get "$course_id" --json >/dev/null
    run_optional "classroom" "classroom courses url" rata classroom courses url "$course_id" --json >/dev/null
    run_optional "classroom" "classroom roster" rata classroom roster "$course_id" --students --teachers --max 1 --json >/dev/null
    run_optional "classroom" "classroom students list" rata classroom students "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom teachers list" rata classroom teachers "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom coursework list" rata classroom coursework "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom materials list" rata classroom materials "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom announcements list" rata classroom announcements "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom topics list" rata classroom topics "$course_id" --max 1 --json >/dev/null

    cw_json=$(rata classroom coursework "$course_id" --max 1 --json 2>/dev/null || true)
    cw_id=$(extract_id "$cw_json")
    if [ -n "$cw_id" ]; then
      run_optional "classroom" "classroom submissions list" rata classroom submissions "$course_id" "$cw_id" --max 1 --json >/dev/null
    fi
  else
    if [ "${STRICT:-false}" = true ]; then
      echo "Missing RATA_LIVE_CLASSROOM_COURSE for classroom coverage." >&2
      return 1
    fi
    echo "==> classroom (optional; set RATA_LIVE_CLASSROOM_COURSE to expand)"
  fi

  # Disabled by default: creator account lacks course state permissions.
  if [ -n "${RATA_LIVE_CLASSROOM_CREATE:-${GOG_LIVE_CLASSROOM_CREATE:-}}" ] && [ -n "${RATA_LIVE_CLASSROOM_ALLOW_STATE:-${GOG_LIVE_CLASSROOM_ALLOW_STATE:-}}" ]; then
    local course_json course_id topic_json topic_id announcement_json announcement_id material_json material_id coursework_json coursework_id

    echo "==> classroom courses create"
    if course_json=$(rata classroom courses create --name "ratatosk-smoke-$TS" --section "ratatosk" --state ACTIVE --json 2>/dev/null); then
      :
    elif course_json=$(rata classroom courses create --name "ratatosk-smoke-$TS" --section "ratatosk" --state PROVISIONED --json 2>/dev/null); then
      :
    else
      course_json=""
    fi
    course_id=$(extract_id "$course_json")
    if [ -z "$course_id" ]; then
      echo "Classroom course create failed; skipping create tests."
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
      return 0
    fi

    run_optional "classroom" "classroom courses update" rata classroom courses update "$course_id" --name "ratatosk-smoke-updated-$TS" --json >/dev/null
    run_optional "classroom" "classroom courses archive" rata classroom courses archive "$course_id" --json >/dev/null
    run_optional "classroom" "classroom courses unarchive" rata classroom courses unarchive "$course_id" --json >/dev/null

    echo "==> classroom topics create"
    topic_json=$(rata classroom topics create "$course_id" --name "ratatosk topic $TS" --json 2>/dev/null || true)
    topic_id=$(extract_id "$topic_json")

    echo "==> classroom announcements create"
    announcement_json=$(rata classroom announcements create "$course_id" --text "ratatosk announcement $TS" --json 2>/dev/null || true)
    announcement_id=$(extract_id "$announcement_json")

    echo "==> classroom materials create"
    material_json=$(rata classroom materials create "$course_id" --title "ratatosk material $TS" --json 2>/dev/null || true)
    material_id=$(extract_id "$material_json")

    echo "==> classroom coursework create"
    coursework_json=$(rata classroom coursework create "$course_id" --title "ratatosk coursework $TS" --type ASSIGNMENT --max-points 10 --json 2>/dev/null || true)
    coursework_id=$(extract_id "$coursework_json")

    if [ -n "$announcement_id" ]; then
      run_optional "classroom" "classroom announcements update" rata classroom announcements update "$course_id" "$announcement_id" --text "ratatosk announcement updated $TS" --json >/dev/null
      run_optional "classroom" "classroom announcements delete" rata --force classroom announcements delete "$course_id" "$announcement_id" --json >/dev/null
    fi
    if [ -n "$material_id" ]; then
      run_optional "classroom" "classroom materials update" rata classroom materials update "$course_id" "$material_id" --title "ratatosk material updated $TS" --json >/dev/null
      run_optional "classroom" "classroom materials delete" rata --force classroom materials delete "$course_id" "$material_id" --json >/dev/null
    fi
    if [ -n "$coursework_id" ]; then
      run_optional "classroom" "classroom coursework update" rata classroom coursework update "$course_id" "$coursework_id" --title "ratatosk coursework updated $TS" --json >/dev/null
      run_optional "classroom" "classroom coursework delete" rata --force classroom coursework delete "$course_id" "$coursework_id" --json >/dev/null
    fi
    if [ -n "$topic_id" ]; then
      run_optional "classroom" "classroom topics update" rata classroom topics update "$course_id" "$topic_id" --name "ratatosk topic updated $TS" --json >/dev/null
      run_optional "classroom" "classroom topics delete" rata --force classroom topics delete "$course_id" "$topic_id" --json >/dev/null
    fi

    if rata --force classroom courses delete "$course_id" --json >/dev/null; then
      :
    else
      echo "Classroom course delete failed; manual cleanup needed: $course_id" >&2
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
    fi
  elif [ -n "${RATA_LIVE_CLASSROOM_CREATE:-${GOG_LIVE_CLASSROOM_CREATE:-}}" ]; then
    echo "==> classroom create (skipped; no account with course state permissions)"
  fi
}
