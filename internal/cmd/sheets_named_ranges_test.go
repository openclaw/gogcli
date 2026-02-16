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

func TestSheetsNamedRangesAdd(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotAdd *sheets.AddNamedRangeRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
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
			if len(req.Requests) != 1 || req.Requests[0].AddNamedRange == nil {
				t.Fatalf("expected addNamedRange request, got %#v", req.Requests)
			}
			gotAdd = req.Requests[0].AddNamedRange
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"replies": []map[string]any{
					{
						"addNamedRange": map[string]any{
							"namedRange": map[string]any{
								"namedRangeId": "nr1",
								"name":         "MyNamedRange",
								"range": map[string]any{
									"sheetId":          1,
									"startRowIndex":    1,
									"endRowIndex":      3,
									"startColumnIndex": 1,
									"endColumnIndex":   3,
								},
							},
						},
					},
				},
			})
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
	cmd := &SheetsNamedRangesAddCmd{}
	if err := runKong(t, cmd, []string{"s1", "MyNamedRange", "Sheet1!B2:C3"}, ctx, flags); err != nil {
		t.Fatalf("add: %v", err)
	}

	if gotAdd == nil || gotAdd.NamedRange == nil || gotAdd.NamedRange.Range == nil {
		t.Fatalf("missing add payload: %#v", gotAdd)
	}
	if gotAdd.NamedRange.Name != "MyNamedRange" {
		t.Fatalf("unexpected name: %q", gotAdd.NamedRange.Name)
	}
	if gotAdd.NamedRange.Range.SheetId != 1 {
		t.Fatalf("unexpected sheet id: %d", gotAdd.NamedRange.Range.SheetId)
	}
	if gotAdd.NamedRange.Range.StartRowIndex != 1 || gotAdd.NamedRange.Range.EndRowIndex != 3 {
		t.Fatalf("unexpected row range: %#v", gotAdd.NamedRange.Range)
	}
	if gotAdd.NamedRange.Range.StartColumnIndex != 1 || gotAdd.NamedRange.Range.EndColumnIndex != 3 {
		t.Fatalf("unexpected col range: %#v", gotAdd.NamedRange.Range)
	}
}

func TestSheetsNamedRangesUpdateName(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotUpdate *sheets.UpdateNamedRangeRequest
	getCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			getCount++
			name := "OldName"
			if getCount >= 2 {
				name = "NewName"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1"}},
				},
				"namedRanges": []map[string]any{
					{
						"namedRangeId": "nr1",
						"name":         name,
						"range": map[string]any{
							"sheetId":          1,
							"startRowIndex":    0,
							"endRowIndex":      1,
							"startColumnIndex": 0,
							"endColumnIndex":   1,
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
			if len(req.Requests) != 1 || req.Requests[0].UpdateNamedRange == nil {
				t.Fatalf("expected updateNamedRange request, got %#v", req.Requests)
			}
			gotUpdate = req.Requests[0].UpdateNamedRange
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
	cmd := &SheetsNamedRangesUpdateCmd{}
	if err := runKong(t, cmd, []string{"s1", "OldName", "--name", "NewName"}, ctx, flags); err != nil {
		t.Fatalf("update: %v", err)
	}

	if gotUpdate == nil || gotUpdate.NamedRange == nil {
		t.Fatalf("missing update payload: %#v", gotUpdate)
	}
	if gotUpdate.NamedRange.NamedRangeId != "nr1" {
		t.Fatalf("unexpected id: %q", gotUpdate.NamedRange.NamedRangeId)
	}
	if gotUpdate.Fields != "name" {
		t.Fatalf("unexpected fields: %q", gotUpdate.Fields)
	}
	if gotUpdate.NamedRange.Name != "NewName" {
		t.Fatalf("unexpected name: %q", gotUpdate.NamedRange.Name)
	}
}

func TestSheetsNamedRangesDelete(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotDelete *sheets.DeleteNamedRangeRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"namedRanges": []map[string]any{
					{"namedRangeId": "nr1", "name": "ToDelete"},
				},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].DeleteNamedRange == nil {
				t.Fatalf("expected deleteNamedRange request, got %#v", req.Requests)
			}
			gotDelete = req.Requests[0].DeleteNamedRange
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
	cmd := &SheetsNamedRangesDeleteCmd{}
	if err := runKong(t, cmd, []string{"s1", "ToDelete"}, ctx, flags); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if gotDelete == nil {
		t.Fatalf("missing delete payload")
	}
	if gotDelete.NamedRangeId != "nr1" {
		t.Fatalf("unexpected id: %q", gotDelete.NamedRangeId)
	}
}

func TestGridRangeToA1_PreservesSheetTitleWhitespace(t *testing.T) {
	got := gridRangeToA1("  Sheet One  ", &sheets.GridRange{
		SheetId:          1,
		StartRowIndex:    0,
		EndRowIndex:      1,
		StartColumnIndex: 0,
		EndColumnIndex:   1,
	})
	if got != "'  Sheet One  '!A1" {
		t.Fatalf("unexpected a1: %q", got)
	}
}

func TestGridRangeToA1_OpenEndedStartBoundRangeDoesNotCollapseToWholeSheet(t *testing.T) {
	got := gridRangeToA1("Sheet1", &sheets.GridRange{
		SheetId:          1,
		StartRowIndex:    5,
		StartColumnIndex: 0,
	})
	if got != "" {
		t.Fatalf("expected empty a1 for unrepresentable open-ended range, got %q", got)
	}
}
