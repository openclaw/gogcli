package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSheetsGetCmd_PlainOutputTSV verifies that --plain produces real tab
// characters between cells instead of space-padded columns (fixes #209).
func TestSheetsGetCmd_PlainOutputTSV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"range": "Sheet1!A1:C2",
			"values": [][]string{
				{"Name", "Age", "City"},
				{"Alice", "30", "Berlin"},
			},
		})
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)
	result := executeWithSheetsTestService(t, []string{"--plain", "--account", "a@b.com", "sheets", "get", "spreadsheet1", "Sheet1!A1:C2"}, svc)
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}
	got := result.stdout

	// In plain mode, columns must be separated by real tab characters.
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d: %q", len(lines), got)
	}

	for i, line := range lines {
		if !strings.Contains(line, "\t") {
			t.Errorf("line %d missing tab delimiter: %q", i, line)
		}
	}

	// Verify exact content of first row.
	if lines[0] != "Name\tAge\tCity" {
		t.Errorf("expected header %q, got %q", "Name\tAge\tCity", lines[0])
	}
}

// TestSheetsMetadataCmd_PlainOutputTSV verifies that sheets metadata --plain
// uses real tabs in the sheets table (fixes #209).
func TestSheetsMetadataCmd_PlainOutputTSV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spreadsheetId":  "id1",
			"spreadsheetUrl": "https://docs.google.com/spreadsheets/d/id1",
			"properties": map[string]any{
				"title":    "Budget",
				"locale":   "en_US",
				"timeZone": "UTC",
			},
			"sheets": []map[string]any{
				{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "gridProperties": map[string]any{"rowCount": 10, "columnCount": 5}}},
			},
		})
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)
	result := executeWithSheetsTestService(t, []string{"--plain", "--account", "a@b.com", "sheets", "metadata", "id1"}, svc)
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}
	got := result.stdout

	// The sheets table should use tab delimiters in plain mode.
	if !strings.Contains(got, "ID\tTITLE\tROWS\tCOLS") {
		t.Errorf("metadata table header missing tab delimiters in plain mode: %q", got)
	}
}

// TestSheetsGetCmd_PlainKeepsMultilineCellsOnOneRow verifies that a cell with
// line breaks or tabs stays in its own row and column, as the other Sheets
// cell tables (links, notes, format) already do through oneLine.
func TestSheetsGetCmd_PlainKeepsMultilineCellsOnOneRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"range": "Sheet1!A1:D3",
			"values": [][]any{
				{"Name", "Address", "Note"},
				{"Alice", "1 Main St\nBerlin", "a\tb"},
				{"Bob", "2 High St\r\nParis", 3.5, true},
			},
		})
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)
	result := executeWithSheetsTestService(t, []string{"--plain", "--account", "a@b.com", "sheets", "get", "spreadsheet1", "Sheet1!A1:D3"}, svc)
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}
	want := "Name\tAddress\tNote\nAlice\t1 Main St\\nBerlin\ta b\nBob\t2 High St\\nParis\t3.5\ttrue\n"
	if result.stdout != want {
		t.Fatalf("stdout = %q, want %q", result.stdout, want)
	}
}
