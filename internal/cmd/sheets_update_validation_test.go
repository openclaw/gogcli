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

func TestSheetsUpdateCopyValidationFrom(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotCopyPaste *sheets.CopyPasteRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedRange": "Sheet1!A3:B3",
				"updatedCells": 2,
			})
			return
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1"}},
				},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].CopyPaste == nil {
				t.Fatalf("expected copyPaste request, got %#v", req.Requests)
			}
			gotCopyPaste = req.Requests[0].CopyPaste
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
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
	cmd := &SheetsUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"s1",
		"Sheet1!A3:B3",
		"--values-json", `[["a","b"]]`,
		"--copy-validation-from", "Sheet1!A2:B2",
	}, ctx, flags); err != nil {
		t.Fatalf("update: %v", err)
	}

	if gotCopyPaste == nil {
		t.Fatal("expected batchUpdate copyPaste request")
	}
	if gotCopyPaste.PasteType != "PASTE_DATA_VALIDATION" {
		t.Fatalf("unexpected paste type: %s", gotCopyPaste.PasteType)
	}

	if gotCopyPaste.Source == nil || gotCopyPaste.Destination == nil {
		t.Fatalf("missing ranges: %#v", gotCopyPaste)
	}

	if gotCopyPaste.Source.SheetId != 1 || gotCopyPaste.Destination.SheetId != 1 {
		t.Fatalf("unexpected sheet ids: %#v", gotCopyPaste)
	}
	if gotCopyPaste.Source.StartRowIndex != 1 || gotCopyPaste.Source.EndRowIndex != 2 {
		t.Fatalf("unexpected source rows: %#v", gotCopyPaste.Source)
	}
	if gotCopyPaste.Source.StartColumnIndex != 0 || gotCopyPaste.Source.EndColumnIndex != 2 {
		t.Fatalf("unexpected source cols: %#v", gotCopyPaste.Source)
	}
	if gotCopyPaste.Destination.StartRowIndex != 2 || gotCopyPaste.Destination.EndRowIndex != 3 {
		t.Fatalf("unexpected destination rows: %#v", gotCopyPaste.Destination)
	}
	if gotCopyPaste.Destination.StartColumnIndex != 0 || gotCopyPaste.Destination.EndColumnIndex != 2 {
		t.Fatalf("unexpected destination cols: %#v", gotCopyPaste.Destination)
	}
}

func TestSheetsUpdateCopyValidationFromNamedRange(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotCopyPaste *sheets.CopyPasteRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedRange": "Sheet1!A3:B3",
				"updatedCells": 2,
			})
			return
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1"}},
				},
				"namedRanges": []map[string]any{
					{
						"namedRangeId": "nr1",
						"name":         "MyValidationRange",
						"range": map[string]any{
							"sheetId":          1,
							"startRowIndex":    1,
							"endRowIndex":      2,
							"startColumnIndex": 0,
							"endColumnIndex":   2,
						},
					},
				},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].CopyPaste == nil {
				t.Fatalf("expected copyPaste request, got %#v", req.Requests)
			}
			gotCopyPaste = req.Requests[0].CopyPaste
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
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
	cmd := &SheetsUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"s1",
		"Sheet1!A3:B3",
		"--values-json", `[["a","b"]]`,
		"--copy-validation-from", "MyValidationRange",
	}, ctx, flags); err != nil {
		t.Fatalf("update: %v", err)
	}

	if gotCopyPaste == nil || gotCopyPaste.Source == nil || gotCopyPaste.Destination == nil {
		t.Fatalf("missing ranges: %#v", gotCopyPaste)
	}
	if gotCopyPaste.Source.SheetId != 1 || gotCopyPaste.Destination.SheetId != 1 {
		t.Fatalf("unexpected sheet ids: %#v", gotCopyPaste)
	}
	if gotCopyPaste.Source.StartRowIndex != 1 || gotCopyPaste.Source.EndRowIndex != 2 {
		t.Fatalf("unexpected source rows: %#v", gotCopyPaste.Source)
	}
	if gotCopyPaste.Source.StartColumnIndex != 0 || gotCopyPaste.Source.EndColumnIndex != 2 {
		t.Fatalf("unexpected source cols: %#v", gotCopyPaste.Source)
	}
	if gotCopyPaste.Destination.StartRowIndex != 2 || gotCopyPaste.Destination.EndRowIndex != 3 {
		t.Fatalf("unexpected destination rows: %#v", gotCopyPaste.Destination)
	}
	if gotCopyPaste.Destination.StartColumnIndex != 0 || gotCopyPaste.Destination.EndColumnIndex != 2 {
		t.Fatalf("unexpected destination cols: %#v", gotCopyPaste.Destination)
	}
}
