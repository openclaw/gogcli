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

func TestSheetsNumberFormatCmd(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotRepeat *sheets.RepeatCellRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 7, "title": "Sheet1"}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].RepeatCell == nil {
				t.Fatalf("expected repeatCell request, got %#v", req.Requests)
			}
			gotRepeat = req.Requests[0].RepeatCell
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

	gotRepeat = nil
	cmd := &SheetsNumberFormatCmd{}
	if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2", "--type", "CURRENCY", "--pattern", "$#,##0.00"}, ctx, flags); err != nil {
		t.Fatalf("number-format: %v", err)
	}
	if gotRepeat == nil || gotRepeat.Cell == nil || gotRepeat.Cell.UserEnteredFormat == nil || gotRepeat.Cell.UserEnteredFormat.NumberFormat == nil {
		t.Fatalf("missing number format payload: %#v", gotRepeat)
	}
	if gotRepeat.Fields != "userEnteredFormat.numberFormat" {
		t.Fatalf("unexpected fields: %s", gotRepeat.Fields)
	}
	if gotRepeat.Cell.UserEnteredFormat.NumberFormat.Type != "CURRENCY" {
		t.Fatalf("unexpected type: %#v", gotRepeat.Cell.UserEnteredFormat.NumberFormat)
	}
	if gotRepeat.Cell.UserEnteredFormat.NumberFormat.Pattern != "$#,##0.00" {
		t.Fatalf("unexpected pattern: %#v", gotRepeat.Cell.UserEnteredFormat.NumberFormat)
	}
}
