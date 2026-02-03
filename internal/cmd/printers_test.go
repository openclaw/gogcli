package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPrintersListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/chrome/printers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"printers": []map[string]any{
				{"id": "p1", "displayName": "HQ Printer", "uri": "ipp://printer"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "HQ Printer") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestPrintersCreateCmd(t *testing.T) {
	var gotName string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/chrome/printers") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			DisplayName string `json:"displayName"`
			Uri         string `json:"uri"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotName = payload.DisplayName
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "p2",
			"displayName": payload.DisplayName,
			"uri":         payload.Uri,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersCreateCmd{Name: "Lab Printer", URI: "ipp://lab"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotName != "Lab Printer" {
		t.Fatalf("unexpected name: %q", gotName)
	}
	if !strings.Contains(out, "Created printer") {
		t.Fatalf("unexpected output: %s", out)
	}
}
