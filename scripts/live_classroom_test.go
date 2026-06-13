package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClassroomLiveCreateFailureSkipsCleanly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash live-test harness is not supported on Windows")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	script := `
set -euo pipefail
ROOT_DIR="$1"
PY=python3
SKIP=""
STRICT=false
TS=20260613000000
GOG_LIVE_CLASSROOM_CREATE=1
GOG_LIVE_CLASSROOM_ALLOW_STATE=1
source "$ROOT_DIR/scripts/live-tests/common.sh"
source "$ROOT_DIR/scripts/live-tests/classroom.sh"
gog() { return 1; }
run_classroom_tests
`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root).CombinedOutput()
	if err != nil {
		t.Fatalf("run classroom live-test failure path: %v\n%s", err, output)
	}

	text := string(output)
	if !strings.Contains(text, "Classroom ACTIVE course create failed; skipping create tests.") {
		t.Fatalf("output missing clean skip message:\n%s", text)
	}

	if strings.Contains(text, "Traceback") || strings.Contains(text, "JSONDecodeError") {
		t.Fatalf("unexpected parser traceback:\n%s", text)
	}
}
