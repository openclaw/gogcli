package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// TestSheetsGetCmd_PlainOutputTSV verifies that --plain produces real tab
// characters between cells instead of space-padded columns (fixes #209).
func TestSheetsGetCmd_PlainOutputTSV(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

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

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	got := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})

		cmd := &SheetsGetCmd{}
		if err := runKong(t, cmd, []string{"spreadsheet1", "Sheet1!A1:C2"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

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
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

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

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	got := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})

		cmd := &SheetsMetadataCmd{}
		if err := runKong(t, cmd, []string{"id1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	// The sheets table should use tab delimiters in plain mode.
	if !strings.Contains(got, "ID\tTITLE\tROWS\tCOLS") {
		t.Errorf("metadata table header missing tab delimiters in plain mode: %q", got)
	}
}
