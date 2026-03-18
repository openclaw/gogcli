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

func TestSheetsMergeCmds(t *testing.T) {
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

	t.Run("merge", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsMergeCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2", "--type", "merge_rows"}, ctx, flags); err != nil {
			t.Fatalf("merge: %v", err)
		}
		if gotRequest == nil || gotRequest.MergeCells == nil {
			t.Fatalf("expected merge request, got %#v", gotRequest)
		}
		if gotRequest.MergeCells.MergeType != "MERGE_ROWS" {
			t.Fatalf("unexpected merge type: %#v", gotRequest.MergeCells)
		}
	})

	t.Run("unmerge", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsUnmergeCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2"}, ctx, flags); err != nil {
			t.Fatalf("unmerge: %v", err)
		}
		if gotRequest == nil || gotRequest.UnmergeCells == nil {
			t.Fatalf("expected unmerge request, got %#v", gotRequest)
		}
	})
}
