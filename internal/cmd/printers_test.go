package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
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

func TestPrintersListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/chrome/printers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"printers": []map[string]any{
				{"id": "p1", "displayName": "HQ Printer", "uri": "ipp://printer", "orgUnitId": "ou1"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "printers") || !strings.Contains(out, "HQ Printer") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestPrintersListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/chrome/printers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"printers": []map[string]any{},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersListCmd{}

	// Should not return error even with empty list
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestPrintersListCmd_Pagination(t *testing.T) {
	var gotPageToken, gotPageSize string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/chrome/printers") {
			http.NotFound(w, r)
			return
		}
		gotPageToken = r.URL.Query().Get("pageToken")
		gotPageSize = r.URL.Query().Get("pageSize")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"printers": []map[string]any{
				{"id": "p1", "displayName": "Printer 1", "uri": "ipp://p1"},
			},
			"nextPageToken": "next-token",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersListCmd{Max: 10, Page: "token123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageToken != "token123" {
		t.Errorf("expected page token 'token123', got %q", gotPageToken)
	}
	if gotPageSize != "10" {
		t.Errorf("expected page size '10', got %q", gotPageSize)
	}
	if !strings.Contains(out, "Printer 1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestPrintersGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/chrome/printers/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "p1",
			"displayName": "Office Printer",
			"uri":         "ipp://office",
			"orgUnitId":   "ou1",
			"description": "Main office printer",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersGetCmd{PrinterID: "p1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Office Printer") || !strings.Contains(out, "ID:") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestPrintersGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/chrome/printers/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "p1",
			"displayName": "Office Printer",
			"uri":         "ipp://office",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersGetCmd{PrinterID: "p1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "displayName") || !strings.Contains(out, "Office Printer") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestPrintersGetCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersGetCmd{PrinterID: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty printer ID")
	}
	if !strings.Contains(err.Error(), "printer ID is required") {
		t.Fatalf("unexpected error: %v", err)
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

func TestPrintersCreateCmd_JSON(t *testing.T) {
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

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "id") || !strings.Contains(out, "p2") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestPrintersCreateCmd_MissingName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersCreateCmd{Name: "", URI: "ipp://test"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "--name and --uri are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintersCreateCmd_MissingURI(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersCreateCmd{Name: "Test", URI: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing URI")
	}
	if !strings.Contains(err.Error(), "--name and --uri are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintersUpdateCmd(t *testing.T) {
	var gotName, gotURI string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/chrome/printers/") {
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
		gotURI = payload.Uri
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "p1",
			"displayName": payload.DisplayName,
			"uri":         payload.Uri,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated Printer"
	newURI := "ipp://updated"
	cmd := &PrintersUpdateCmd{PrinterID: "p1", Name: &newName, URI: &newURI}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotName != "Updated Printer" {
		t.Errorf("unexpected name: %q", gotName)
	}
	if gotURI != "ipp://updated" {
		t.Errorf("unexpected URI: %q", gotURI)
	}
	if !strings.Contains(out, "Updated printer") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestPrintersUpdateCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Test"
	cmd := &PrintersUpdateCmd{PrinterID: "   ", Name: &newName}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty printer ID")
	}
	if !strings.Contains(err.Error(), "printer ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintersUpdateCmd_NoUpdates(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &PrintersUpdateCmd{PrinterID: "p1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no updates")
	}
	if !strings.Contains(err.Error(), "no updates specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintersDeleteCmd(t *testing.T) {
	var deletedID string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/chrome/printers/") {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(r.URL.Path, "/chrome/printers/")
		if len(parts) > 1 {
			deletedID = parts[1]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &PrintersDeleteCmd{PrinterID: "p1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if deletedID != "p1" {
		t.Errorf("unexpected deleted ID: %q", deletedID)
	}
	if !strings.Contains(out, "Deleted printer") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestPrintersDeleteCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &PrintersDeleteCmd{PrinterID: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty printer ID")
	}
	if !strings.Contains(err.Error(), "printer ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintersDeleteCmd_RequiresConfirmation(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &PrintersDeleteCmd{PrinterID: "p1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when --force not set and non-interactive")
	}
	if !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrinterResourceName(t *testing.T) {
	tests := []struct {
		id       string
		expected string
	}{
		{"p1", "customers/my_customer/chrome/printers/p1"},
		{"customers/my_customer/chrome/printers/p2", "customers/my_customer/chrome/printers/p2"},
		{"  p3  ", "customers/my_customer/chrome/printers/p3"},
	}

	for _, tt := range tests {
		got := printerResourceName(tt.id)
		if got != tt.expected {
			t.Errorf("printerResourceName(%q) = %q, want %q", tt.id, got, tt.expected)
		}
	}
}

func TestPrinterParent(t *testing.T) {
	expected := "customers/my_customer"
	got := printerParent()
	if got != expected {
		t.Errorf("printerParent() = %q, want %q", got, expected)
	}
}
