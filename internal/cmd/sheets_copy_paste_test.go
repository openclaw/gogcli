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

func TestSheetsCopyPasteCmd(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotRequest *sheets.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 9, "title": "Sheet1"}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 {
				t.Fatalf("expected one request, got %#v", req.Requests)
			}
			gotRequest = req.Requests[0]
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

	t.Run("fill formulas down (formula paste type)", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsCopyPasteCmd{}
		if err := runKong(t, cmd, []string{
			"s1", "Sheet1!A2:H71", "Sheet1!A2:H120", "--type", "FORMULA",
		}, ctx, flags); err != nil {
			t.Fatalf("copy-paste: %v", err)
		}
		if gotRequest == nil || gotRequest.CopyPaste == nil {
			t.Fatalf("expected copyPaste request, got %#v", gotRequest)
		}
		cp := gotRequest.CopyPaste
		if cp.PasteType != "PASTE_FORMULA" {
			t.Fatalf("unexpected paste type: %s", cp.PasteType)
		}
		if cp.PasteOrientation != "NORMAL" {
			t.Fatalf("unexpected orientation: %s", cp.PasteOrientation)
		}
		if cp.Source.SheetId != 9 || cp.Destination.SheetId != 9 {
			t.Fatalf("unexpected sheet ids: src=%d dst=%d", cp.Source.SheetId, cp.Destination.SheetId)
		}
		// A2:H71 -> rows [1,71); A2:H120 -> rows [1,120). Destination is taller (fill-down).
		if cp.Source.StartRowIndex != 1 || cp.Source.EndRowIndex != 71 {
			t.Fatalf("unexpected source rows: %#v", cp.Source)
		}
		if cp.Destination.StartRowIndex != 1 || cp.Destination.EndRowIndex != 120 {
			t.Fatalf("unexpected dest rows: %#v", cp.Destination)
		}
	})

	t.Run("default type is PASTE_NORMAL", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsCopyPasteCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2", "Sheet1!D1:E2"}, ctx, flags); err != nil {
			t.Fatalf("copy-paste: %v", err)
		}
		if gotRequest == nil || gotRequest.CopyPaste == nil {
			t.Fatal("expected copyPaste request")
		}
		if gotRequest.CopyPaste.PasteType != "PASTE_NORMAL" {
			t.Fatalf("unexpected paste type: %s", gotRequest.CopyPaste.PasteType)
		}
	})

	t.Run("transpose sets orientation", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsCopyPasteCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B3", "Sheet1!D1:F2", "--transpose"}, ctx, flags); err != nil {
			t.Fatalf("copy-paste: %v", err)
		}
		if gotRequest == nil || gotRequest.CopyPaste == nil {
			t.Fatal("expected copyPaste request")
		}
		if gotRequest.CopyPaste.PasteOrientation != "TRANSPOSE" {
			t.Fatalf("unexpected orientation: %s", gotRequest.CopyPaste.PasteOrientation)
		}
	})

	t.Run("invalid paste type", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsCopyPasteCmd{}
		err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2", "Sheet1!D1:E2", "--type", "BOGUS"}, ctx, flags)
		if err == nil {
			t.Fatal("expected error for invalid type")
		}
		if !strings.Contains(err.Error(), "invalid --type") {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotRequest != nil {
			t.Fatal("did not expect an API request")
		}
	})

	t.Run("empty source range", func(t *testing.T) {
		cmd := &SheetsCopyPasteCmd{}
		err := runKong(t, cmd, []string{"s1", "  ", "Sheet1!A1"}, ctx, flags)
		if err == nil || !strings.Contains(err.Error(), "empty source range") {
			t.Fatalf("expected empty source error, got %v", err)
		}
	})

	t.Run("empty dest range", func(t *testing.T) {
		cmd := &SheetsCopyPasteCmd{}
		err := runKong(t, cmd, []string{"s1", "Sheet1!A1", "  "}, ctx, flags)
		if err == nil || !strings.Contains(err.Error(), "empty dest range") {
			t.Fatalf("expected empty dest error, got %v", err)
		}
	})
}
