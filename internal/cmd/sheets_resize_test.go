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

	"github.com/steipete/gogcli/internal/ui"
)

func TestSheetsResizeCmds(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 0, "title": "Sheet1"}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
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
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	t.Run("resize columns force-sends zero sheet and start index", func(t *testing.T) {
		gotBody = nil
		cmd := &SheetsResizeColumnsCmd{}
		if err := runKong(t, cmd, []string{"s1", "A:C", "--width", "120"}, ctx, flags); err != nil {
			t.Fatalf("resize-columns: %v", err)
		}
		requests := gotBody["requests"].([]any)
		update := requests[0].(map[string]any)["updateDimensionProperties"].(map[string]any)
		rng := update["range"].(map[string]any)
		if _, ok := rng["sheetId"]; !ok {
			t.Fatalf("expected sheetId to be sent: %#v", rng)
		}
		if v, ok := rng["startIndex"]; !ok || v != float64(0) {
			t.Fatalf("expected startIndex=0, got %#v", rng)
		}
		if v, ok := rng["endIndex"]; !ok || v != float64(3) {
			t.Fatalf("expected endIndex=3, got %#v", rng)
		}
	})

	t.Run("resize rows auto", func(t *testing.T) {
		gotBody = nil
		cmd := &SheetsResizeRowsCmd{}
		if err := runKong(t, cmd, []string{"s1", "1:3", "--auto"}, ctx, flags); err != nil {
			t.Fatalf("resize-rows: %v", err)
		}
		requests := gotBody["requests"].([]any)
		auto := requests[0].(map[string]any)["autoResizeDimensions"].(map[string]any)
		rng := auto["dimensions"].(map[string]any)
		if _, ok := rng["sheetId"]; !ok {
			t.Fatalf("expected sheetId to be sent: %#v", rng)
		}
		if v, ok := rng["startIndex"]; !ok || v != float64(0) {
			t.Fatalf("expected startIndex=0, got %#v", rng)
		}
		if v, ok := rng["endIndex"]; !ok || v != float64(3) {
			t.Fatalf("expected endIndex=3, got %#v", rng)
		}
	})
}

func TestParseColumnsSpan(t *testing.T) {
	t.Run("sheet range", func(t *testing.T) {
		span, err := parseColumnsSpan("Sheet 1!$C:$A", "columns")
		if err != nil {
			t.Fatalf("parseColumnsSpan: %v", err)
		}
		if span.SheetName != "Sheet 1" {
			t.Fatalf("SheetName=%q, want %q", span.SheetName, "Sheet 1")
		}
		if span.StartIndex != 0 || span.EndIndex != 3 {
			t.Fatalf("range=%d:%d, want 0:3", span.StartIndex, span.EndIndex)
		}
	})

	t.Run("invalid range", func(t *testing.T) {
		if _, err := parseColumnsSpan("Sheet1!1:3", "columns"); err == nil {
			t.Fatal("expected error for invalid column range")
		}
	})
}

func TestParseRowsSpan(t *testing.T) {
	t.Run("sheet range", func(t *testing.T) {
		span, err := parseRowsSpan("Sheet 1!$4:$2", "rows")
		if err != nil {
			t.Fatalf("parseRowsSpan: %v", err)
		}
		if span.SheetName != "Sheet 1" {
			t.Fatalf("SheetName=%q, want %q", span.SheetName, "Sheet 1")
		}
		if span.StartIndex != 1 || span.EndIndex != 4 {
			t.Fatalf("range=%d:%d, want 1:4", span.StartIndex, span.EndIndex)
		}
	})

	t.Run("invalid row", func(t *testing.T) {
		if _, err := parseRowsSpan("Sheet1!0:2", "rows"); err == nil {
			t.Fatal("expected error for invalid row range")
		}
	})
}
