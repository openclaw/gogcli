package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSitesListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{"id": "site1", "name": "Marketing", "webViewLink": "https://sites.google.com/example.com/marketing", "createdTime": "2026-01-01T00:00:00Z", "owners": []map[string]any{{"emailAddress": "owner@example.com"}}},
			},
		})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &SitesListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Marketing") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSitesDeleteCmd_ResolvesURL(t *testing.T) {
	var deleted string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "site123", "name": "Marketing", "webViewLink": "https://sites.google.com/example.com/marketing"},
				},
			})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/files/site123"):
			deleted = "site123"
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &SitesDeleteCmd{Site: "https://sites.google.com/example.com/marketing"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if deleted != "site123" {
		t.Fatalf("expected delete site123, got %q", deleted)
	}
	if !strings.Contains(out, "Deleted site") {
		t.Fatalf("unexpected output: %s", out)
	}
}
