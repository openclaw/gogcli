package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestContactsLiveOtherContactsRunsForConsumerAccounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash live-test harness is not supported on Windows")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	tests := []struct {
		name      string
		otherJSON string
		wantQuery string
	}{
		{
			name:      "existing other contact",
			otherJSON: `{"contacts":[{"email":"friend@example.com"}]}`,
			wantQuery: "friend@example.com",
		},
		{
			name:      "empty other contacts",
			otherJSON: `{"contacts":[]}`,
			wantQuery: "gogcli-smoke-20260613000000@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := `
set -euo pipefail
ROOT_DIR="$1"
OTHER_JSON="$2"
PY=python3
SKIP=""
TS=20260613000000
LIVE_TMP=$(mktemp -d)
TRACE_FILE="$LIVE_TMP/trace"
trap 'rm -rf "$LIVE_TMP"' EXIT
source "$ROOT_DIR/scripts/live-tests/common.sh"
source "$ROOT_DIR/scripts/live-tests/contacts.sh"
gog() {
  printf '%s\n' "$*" >>"$TRACE_FILE"
  case "$*" in
    "contacts other list"*)
      printf '%s\n' "$OTHER_JSON"
      ;;
    "contacts other search"*)
      printf '{"contacts":[]}\n'
      ;;
    *)
      return 1
      ;;
  esac
}
run_contacts_other_tests
cat "$TRACE_FILE"
`

			output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root, tt.otherJSON).CombinedOutput()
			if err != nil {
				t.Fatalf("run contacts live-test path: %v\n%s", err, output)
			}

			text := string(output)
			if !strings.Contains(text, "contacts other list --json --max 1") {
				t.Fatalf("output missing other contacts list:\n%s", text)
			}

			if !strings.Contains(text, "contacts other search "+tt.wantQuery+" --json --max 1") {
				t.Fatalf("output missing expected other contacts search:\n%s", text)
			}
		})
	}
}

func TestContactsLiveOtherContactsSkipAvoidsAPI(t *testing.T) {
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
SKIP="contacts-other"
TS=20260613000000
source "$ROOT_DIR/scripts/live-tests/common.sh"
source "$ROOT_DIR/scripts/live-tests/contacts.sh"
gog() {
  echo "unexpected API call" >&2
  return 1
}
run_contacts_other_tests
`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root).CombinedOutput()
	if err != nil {
		t.Fatalf("run contacts live-test skip path: %v\n%s", err, output)
	}

	text := string(output)
	if !strings.Contains(text, "contacts other (skipped)") {
		t.Fatalf("output missing other contacts skip:\n%s", text)
	}

	if strings.Contains(text, "unexpected API call") {
		t.Fatalf("skip path invoked the API:\n%s", text)
	}
}
