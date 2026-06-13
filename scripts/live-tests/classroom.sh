#!/usr/bin/env bash

set -euo pipefail

run_classroom_tests() {
  if skip "classroom"; then
    echo "==> classroom (skipped)"
    return 0
  fi

  run_optional "classroom" "classroom profile get" gog classroom profile get --json >/dev/null
  run_optional "classroom" "classroom courses list" gog classroom courses list --json --max 1 >/dev/null

  if [ -n "${GOG_LIVE_CLASSROOM_COURSE:-}" ]; then
    local course_id cw_json cw_id
    course_id="$GOG_LIVE_CLASSROOM_COURSE"
    run_optional "classroom" "classroom courses get" gog classroom courses get "$course_id" --json >/dev/null
    run_optional "classroom" "classroom courses url" gog classroom courses url "$course_id" --json >/dev/null
    run_optional "classroom" "classroom roster" gog classroom roster "$course_id" --students --teachers --max 1 --json >/dev/null
    run_optional "classroom" "classroom students list" gog classroom students "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom teachers list" gog classroom teachers "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom coursework list" gog classroom coursework "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom materials list" gog classroom materials "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom announcements list" gog classroom announcements "$course_id" --max 1 --json >/dev/null
    run_optional "classroom" "classroom topics list" gog classroom topics "$course_id" --max 1 --json >/dev/null

    cw_json=$(gog classroom coursework "$course_id" --max 1 --json 2>/dev/null || true)
    cw_id=$(extract_id "$cw_json")
    if [ -n "$cw_id" ]; then
      run_optional "classroom" "classroom submissions list" gog classroom submissions "$course_id" "$cw_id" --max 1 --json >/dev/null
    fi
  else
    if [ "${STRICT:-false}" = true ]; then
      echo "Missing GOG_LIVE_CLASSROOM_COURSE for classroom coverage." >&2
      return 1
    fi
    echo "==> classroom (optional; set GOG_LIVE_CLASSROOM_COURSE to expand)"
  fi

  # Disabled by default: consumer accounts cannot create ACTIVE courses through
  # the API, and PROVISIONED courses require browser acceptance before cleanup.
  if [ -n "${GOG_LIVE_CLASSROOM_CREATE:-}" ] && [ -n "${GOG_LIVE_CLASSROOM_ALLOW_STATE:-}" ]; then
    local course_json course_id topic_json topic_id announcement_json announcement_id material_json material_id coursework_json coursework_id

    echo "==> classroom courses create"
    if course_json=$(gog classroom courses create --name "gogcli-smoke-$TS" --section "gogcli" --state ACTIVE --json 2>/dev/null); then
      :
    else
      course_json=""
    fi
    course_id=$(extract_id "$course_json")
    if [ -z "$course_id" ]; then
      echo "Classroom ACTIVE course create failed; skipping create tests."
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
      return 0
    fi

    run_optional "classroom" "classroom courses update" gog classroom courses update "$course_id" --name "gogcli-smoke-updated-$TS" --json >/dev/null
    run_optional "classroom" "classroom courses archive" gog classroom courses archive "$course_id" --json >/dev/null
    run_optional "classroom" "classroom courses unarchive" gog classroom courses unarchive "$course_id" --json >/dev/null

    echo "==> classroom topics create"
    topic_json=$(gog classroom topics create "$course_id" --name "gogcli topic $TS" --json 2>/dev/null || true)
    topic_id=$(extract_id "$topic_json")

    echo "==> classroom announcements create"
    announcement_json=$(gog classroom announcements create "$course_id" --text "gogcli announcement $TS" --json 2>/dev/null || true)
    announcement_id=$(extract_id "$announcement_json")

    echo "==> classroom materials create"
    material_json=$(gog classroom materials create "$course_id" --title "gogcli material $TS" --json 2>/dev/null || true)
    material_id=$(extract_id "$material_json")

    echo "==> classroom coursework create"
    coursework_json=$(gog classroom coursework create "$course_id" --title "gogcli coursework $TS" --type ASSIGNMENT --max-points 10 --json 2>/dev/null || true)
    coursework_id=$(extract_id "$coursework_json")

    if [ -n "$announcement_id" ]; then
      run_optional "classroom" "classroom announcements update" gog classroom announcements update "$course_id" "$announcement_id" --text "gogcli announcement updated $TS" --json >/dev/null
      run_optional "classroom" "classroom announcements delete" gog --force classroom announcements delete "$course_id" "$announcement_id" --json >/dev/null
    fi
    if [ -n "$material_id" ]; then
      run_optional "classroom" "classroom materials update" gog classroom materials update "$course_id" "$material_id" --title "gogcli material updated $TS" --json >/dev/null
      run_optional "classroom" "classroom materials delete" gog --force classroom materials delete "$course_id" "$material_id" --json >/dev/null
    fi
    if [ -n "$coursework_id" ]; then
      run_optional "classroom" "classroom coursework update" gog classroom coursework update "$course_id" "$coursework_id" --title "gogcli coursework updated $TS" --json >/dev/null
      run_optional "classroom" "classroom coursework delete" gog --force classroom coursework delete "$course_id" "$coursework_id" --json >/dev/null
    fi
    if [ -n "$topic_id" ]; then
      run_optional "classroom" "classroom topics update" gog classroom topics update "$course_id" "$topic_id" --name "gogcli topic updated $TS" --json >/dev/null
      run_optional "classroom" "classroom topics delete" gog --force classroom topics delete "$course_id" "$topic_id" --json >/dev/null
    fi

    echo "==> classroom courses cleanup"
    if gog classroom courses archive "$course_id" --json >/dev/null &&
      gog --force classroom courses delete "$course_id" --json >/dev/null; then
      :
    else
      echo "Classroom course delete failed; manual cleanup needed: $course_id" >&2
      if [ "${STRICT:-false}" = true ]; then
        return 1
      fi
    fi
  elif [ -n "${GOG_LIVE_CLASSROOM_CREATE:-}" ]; then
    echo "==> classroom create (skipped; no account with course state permissions)"
  fi
}
