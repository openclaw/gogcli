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

func TestContactsLiveDedupeApplyWorkflow(t *testing.T) {
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
TS=20260613000000
LIVE_TMP=$(mktemp -d)
TRACE_FILE="$LIVE_TMP/trace"
COUNT_FILE="$LIVE_TMP/create-count"
printf '0\n' >"$COUNT_FILE"
trap 'rm -rf "$LIVE_TMP"' EXIT
source "$ROOT_DIR/scripts/live-tests/common.sh"
source "$ROOT_DIR/scripts/live-tests/contacts.sh"
gog() {
  printf '%s\n' "$*" >>"$TRACE_FILE"
  case "$*" in
    "contacts create"*)
      count=$(cat "$COUNT_FILE")
      count=$((count + 1))
      printf '%d\n' "$count" >"$COUNT_FILE"
      printf '{"resourceName":"people/%d"}\n' "$count"
      ;;
    "contacts dedupe --resource people/1 --resource people/2 --match email --apply --dry-run --json")
      printf '{"dry_run":true,"op":"contacts.dedupe.apply","request":{"groups_merged":1,"contacts_deleted":1,"groups":[{"primary":{"resource":"people/1"},"delete":[{"resource":"people/2"}]}]}}\n'
      ;;
    "contacts dedupe --resource people/1 --resource people/2 --match email --apply --force --json")
      printf '{"applied":true,"groups_merged":1,"contacts_deleted":1,"groups":[{"primary":{"resource":"people/1"}}]}\n'
      ;;
    "contacts get people/1 --json")
      printf '{"contact":{"emailAddresses":[{"value":"gogcli-dedupe-20260613000000@example.com"}],"phoneNumbers":[{"value":"+1 555 000 0001"},{"value":"+1 555 000 0002"}]}}\n'
      ;;
    "contacts delete people/1 --force")
      printf '{}\n'
      ;;
    *)
      echo "unexpected gog call: $*" >&2
      return 1
      ;;
  esac
}
run_contacts_dedupe_apply_test
printf 'cleanup_ids:%s\n' "${LIVE_CONTACT_CLEANUP_IDS[*]}"
cat "$TRACE_FILE"
`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root).CombinedOutput()
	if err != nil {
		t.Fatalf("run contacts dedupe live-test path: %v\n%s", err, output)
	}

	text := string(output)
	for _, want := range []string{
		"cleanup_ids:people/1 people/2",
		"contacts dedupe --resource people/1 --resource people/2 --match email --apply --dry-run --json",
		"contacts dedupe --resource people/1 --resource people/2 --match email --apply --force --json",
		"contacts get people/1 --json",
		"contacts delete people/1 --force",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestContactsLiveDedupeRefusesUnrelatedGroups(t *testing.T) {
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
TS=20260613000000
LIVE_TMP=$(mktemp -d)
COUNT_FILE="$LIVE_TMP/create-count"
printf '0\n' >"$COUNT_FILE"
trap 'rm -rf "$LIVE_TMP"' EXIT
source "$ROOT_DIR/scripts/live-tests/common.sh"
source "$ROOT_DIR/scripts/live-tests/contacts.sh"
gog() {
  case "$*" in
    "contacts create"*)
      count=$(cat "$COUNT_FILE")
      count=$((count + 1))
      printf '%d\n' "$count" >"$COUNT_FILE"
      printf '{"resourceName":"people/%d"}\n' "$count"
      ;;
    "contacts dedupe --resource people/1 --resource people/2 --match email --apply --dry-run --json")
      printf '{"dry_run":true,"op":"contacts.dedupe.apply","request":{"groups_merged":2,"contacts_deleted":2,"groups":[]}}\n'
      ;;
    *"--force"*)
      echo "apply must not run" >&2
      return 99
      ;;
    *)
      return 1
      ;;
  esac
}
run_contacts_dedupe_apply_test
`

	output, err := exec.CommandContext(t.Context(), "bash", "-c", script, "bash", root).CombinedOutput()
	if err == nil {
		t.Fatalf("expected unrelated groups to stop live apply:\n%s", output)
	}

	text := string(output)
	if !strings.Contains(text, "unrelated duplicate groups") {
		t.Fatalf("output missing refusal reason:\n%s", text)
	}

	if strings.Contains(text, "apply must not run") {
		t.Fatalf("unsafe live apply ran:\n%s", text)
	}
}
