package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestChatSearchLiveHarnessExercisesViewsPagingAndEmptyExit(t *testing.T) {
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
TS=20260903000000
GOG_LIVE_CHAT_SEARCH_QUERY="project one"
LIVE_TMP=$(mktemp -d)
TRACE_FILE="$LIVE_TMP/trace"
trap 'rm -rf "$LIVE_TMP"' EXIT
source "$ROOT_DIR/scripts/live-tests/chat-search.sh"
gog() {
  printf '%s\n' "$*" >>"$TRACE_FILE"
  case "$*" in
    "chat messages search project one --readonly --json --max 1 --order create_time desc --view basic")
      printf '{"results":[{"resource":"spaces/a/messages/one"}],"nextPageToken":"next"}\n'
      ;;
    "chat messages search project one --readonly --json --max 1 --order create_time desc --view full --markup markdown --wrap-untrusted")
      printf '{"results":[{"resource":"spaces/a/messages/one","read":false}],"nextPageToken":"next"}\n'
      ;;
    "chat messages search project one --readonly --json --max 1 --page next --order create_time desc --view full --markup markdown --wrap-untrusted")
      printf '{"results":[{"resource":"spaces/b/messages/two","read":true}],"nextPageToken":""}\n'
      ;;
    "chat messages search gogcli-live-no-match-20260903000000 --readonly --json --max 1 --fail-empty")
      printf '{"results":[],"nextPageToken":""}\n'
      return 3
      ;;
    *)
      echo "unexpected gog call: $*" >&2
      return 1
      ;;
  esac
}
run_chat_search_tests
cat "$TRACE_FILE"
`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root).CombinedOutput()
	if err != nil {
		t.Fatalf("run Chat search live-test path: %v\n%s", err, output)
	}

	text := string(output)
	for _, want := range []string{
		"chat messages search (basic)",
		"--view basic",
		"chat messages search (full Markdown options)",
		"--view full --markup markdown --wrap-untrusted",
		"chat messages search (explicit page)",
		"--page next",
		"chat messages search (fail-empty)",
		"--fail-empty",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestChatSearchLiveHarnessSkipsWithoutQuery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash live-test harness is not supported on Windows")
	}

	t.Setenv("GOG_LIVE_CHAT_SEARCH_QUERY", "")

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	script := `
set -euo pipefail
ROOT_DIR="$1"
PY=python3
TS=20260903000000
source "$ROOT_DIR/scripts/live-tests/chat-search.sh"
gog() {
  echo "unexpected API call" >&2
  return 1
}
run_chat_search_tests
`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root).CombinedOutput()
	if err != nil {
		t.Fatalf("run Chat search live-test skip path: %v\n%s", err, output)
	}

	text := string(output)
	if !strings.Contains(text, "chat messages search (skipped; set GOG_LIVE_CHAT_SEARCH_QUERY)") {
		t.Fatalf("output missing Chat search skip:\n%s", text)
	}

	if strings.Contains(text, "unexpected API call") {
		t.Fatalf("skip path invoked the API:\n%s", text)
	}
}

func TestChatSearchLiveHarnessRejectsBasicReadMetadata(t *testing.T) {
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
TS=20260903000000
GOG_LIVE_CHAT_SEARCH_QUERY=project
source "$ROOT_DIR/scripts/live-tests/chat-search.sh"
gog() {
  printf '{"results":[{"resource":"spaces/a/messages/one","read":false}],"nextPageToken":""}\n'
}

run_chat_search_tests
`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root).CombinedOutput()
	if err == nil {
		t.Fatalf("expected invalid basic read metadata to fail:\n%s", output)
	}

	if !strings.Contains(string(output), "basic chat search result unexpectedly includes read") {
		t.Fatalf("output missing validation failure:\n%s", output)
	}
}

func TestChatSearchLiveHarnessRejectsNonAdvancingPage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash live-test harness is not supported on Windows")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	script := `
set -euo pipefail
ROOT_DIR="$1"
PY=python3
TS=20260903000000
GOG_LIVE_CHAT_SEARCH_QUERY=project
source "$ROOT_DIR/scripts/live-tests/chat-search.sh"
gog() {
  printf '{"results":[{"resource":"spaces/a/messages/one"}],"nextPageToken":"next"}\n'
}
run_chat_search_tests
`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "explicit page repeated a previous result") {
		t.Fatalf("expected unchanged-page rejection, got %v:\n%s", err, output)
	}
}

func TestChatSearchLiveHarnessRejectsInvalidFullReadMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash live-test harness is not supported on Windows")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	script := `set -euo pipefail
PY=python3
source "$1/scripts/live-tests/chat-search.sh"
assert_chat_search_json "$2" full
`
	payload := `{"results":[{"resource":"spaces/a/messages/one","read":"yes"}]}`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root, payload).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "read metadata must be boolean when present") {
		t.Fatalf("expected invalid read metadata rejection, got %v:\n%s", err, output)
	}
}
