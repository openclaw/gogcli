package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCalendarLiveConflictsHandlesCalendarCount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash live-test harness is not supported on Windows")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	for _, calendarCount := range []string{"1", "2"} {
		t.Run(calendarCount+" calendars", func(t *testing.T) {
			script := `
set -euo pipefail
ROOT_DIR="$1"
CALENDAR_COUNT="$2"
SKIP=""
LIVE_TMP=$(mktemp -d)
TRACE_FILE="$LIVE_TMP/trace"
trap 'rm -rf "$LIVE_TMP"' EXIT
source "$ROOT_DIR/scripts/live-tests/common.sh"
source "$ROOT_DIR/scripts/live-tests/calendar.sh"
gog() {
  echo invoked >>"$TRACE_FILE"
  if [ "$CALENDAR_COUNT" -lt 2 ]; then
    echo "calendar conflicts requires at least two calendars" >&2
    return 2
  fi
  printf '{"conflicts":[],"count":0}\n'
}
run_calendar_conflicts_test "$CALENDAR_COUNT" \
  2026-06-13T00:00:00Z 2026-06-14T00:00:00Z
cat "$TRACE_FILE"
`

			output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root, calendarCount).CombinedOutput()
			if err != nil {
				t.Fatalf("run calendar conflict live-test path: %v\n%s", err, output)
			}

			text := string(output)
			if calendarCount == "1" && !strings.Contains(text, "single-calendar validation") {
				t.Fatalf("output missing single-calendar validation:\n%s", text)
			}

			if !strings.Contains(text, "invoked") {
				t.Fatalf("output missing CLI invocation trace:\n%s", text)
			}
		})
	}
}
