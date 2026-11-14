package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drivelabels/v2"

	"github.com/openclaw/gogcli/internal/app"
)

// Google omits empty list fields, so each fake returns {}.
func TestEmptyListJSON(t *testing.T) {
	t.Run("drive labels list", func(t *testing.T) {
		svc, closeSvc := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v2/labels" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}), drivelabels.NewService)
		defer closeSvc()

		assertEmptyListJSON(t, "labels", func(args []string) executeTestResult {
			return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
				DriveLabels: fixedDriveLabelsTestService(svc),
			}})
		}, []string{"--account", "a@example.com", "drive", "labels", "list"})
	})

	t.Run("drive labels file list", func(t *testing.T) {
		svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/files/file1/listLabels" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}))
		defer closeSvc()

		assertEmptyListJSON(t, "labels", func(args []string) executeTestResult {
			return executeWithDriveTestService(t, args, svc)
		}, []string{"--account", "a@example.com", "drive", "labels", "file", "list", "file1"})
	})

	t.Run("docs comments list", func(t *testing.T) {
		svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
			if r.Method != http.MethodGet || path != "/files/empty/comments" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}))
		defer closeSvc()

		assertEmptyListJSON(t, "comments", func(args []string) executeTestResult {
			return executeWithDriveTestService(t, args, svc)
		}, []string{"--account", "a@b.com", "docs", "comments", "list", "empty"})
	})

	t.Run("drive comments list", func(t *testing.T) {
		svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
			if r.Method != http.MethodGet || path != "/files/empty/comments" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}))
		defer closeSvc()

		assertEmptyListJSON(t, "comments", func(args []string) executeTestResult {
			return executeWithDriveTestService(t, args, svc)
		}, []string{"--account", "a@b.com", "drive", "comments", "list", "empty"})
	})
}

func assertEmptyListJSON(t *testing.T, listKey string, run func([]string) executeTestResult, args []string) {
	t.Helper()

	jsonArgs := append([]string{"--json"}, args...)
	result := run(jsonArgs)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q\nstdout=%q", result.err, result.stderr, result.stdout)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\n%s", err, result.stdout)
	}
	raw, ok := parsed[listKey]
	if !ok {
		t.Fatalf("missing %s key: %s", listKey, result.stdout)
	}
	if strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("%s = %s, want []\nstdout=%s", listKey, raw, result.stdout)
	}

	onlyArgs := append([]string{"--json", "--results-only"}, args...)
	result = run(onlyArgs)
	if result.err != nil {
		t.Fatalf("Execute --results-only: %v\nstderr=%q\nstdout=%q", result.err, result.stderr, result.stdout)
	}
	if got := strings.TrimSpace(result.stdout); got != "[]" {
		t.Fatalf("--results-only = %q, want []", result.stdout)
	}
}
