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

func TestSheetsTabCommands(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotRequests []*sheets.Request
	var gotRequestBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 0, "title": "Sheet1"}},
					{"properties": map[string]any{"sheetId": 42, "title": "OldTab"}},
				},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read batchUpdate body: %v", err)
			}
			gotRequestBody = string(body)
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			gotRequests = req.Requests
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"replies": []map[string]any{
					{"addSheet": map[string]any{"properties": map[string]any{"sheetId": 99, "title": "NewTab", "index": 0}}},
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

	t.Run("add-tab", func(t *testing.T) {
		gotRequests = nil
		gotRequestBody = ""
		cmd := &SheetsAddTabCmd{}
		if err := runKong(t, cmd, []string{"s1", "NewTab", "--index", "0"}, ctx, flags); err != nil {
			t.Fatalf("add-tab: %v", err)
		}
		if len(gotRequests) != 1 || gotRequests[0].AddSheet == nil {
			t.Fatalf("expected addSheet request, got %+v", gotRequests)
		}
		props := gotRequests[0].AddSheet.Properties
		if props.Title != "NewTab" {
			t.Fatalf("unexpected title: %s", props.Title)
		}
		if props.Index != 0 {
			t.Fatalf("unexpected index: %d", props.Index)
		}
		if !strings.Contains(gotRequestBody, `"index":0`) {
			t.Fatalf("expected index 0 in request body, got %s", gotRequestBody)
		}
	})

	t.Run("rename-tab", func(t *testing.T) {
		gotRequests = nil
		cmd := &SheetsRenameTabCmd{}
		if err := runKong(t, cmd, []string{"s1", "OldTab", "RenamedTab"}, ctx, flags); err != nil {
			t.Fatalf("rename-tab: %v", err)
		}
		if len(gotRequests) != 1 || gotRequests[0].UpdateSheetProperties == nil {
			t.Fatalf("expected updateSheetProperties request, got %+v", gotRequests)
		}
		req := gotRequests[0].UpdateSheetProperties
		if req.Properties.SheetId != 42 {
			t.Fatalf("unexpected sheetId: %d, want 42", req.Properties.SheetId)
		}
		if req.Properties.Title != "RenamedTab" {
			t.Fatalf("unexpected title: %s", req.Properties.Title)
		}
		if req.Fields != "title" {
			t.Fatalf("unexpected fields: %s", req.Fields)
		}
	})

	t.Run("delete-tab with force", func(t *testing.T) {
		gotRequests = nil
		cmd := &SheetsDeleteTabCmd{}
		flagsForce := &RootFlags{Account: "a@b.com", Force: true}
		if err := runKong(t, cmd, []string{"s1", "OldTab"}, ctx, flagsForce); err != nil {
			t.Fatalf("delete-tab: %v", err)
		}
		if len(gotRequests) != 1 || gotRequests[0].DeleteSheet == nil {
			t.Fatalf("expected deleteSheet request, got %+v", gotRequests)
		}
		if gotRequests[0].DeleteSheet.SheetId != 42 {
			t.Fatalf("unexpected sheetId: %d, want 42", gotRequests[0].DeleteSheet.SheetId)
		}
	})

	t.Run("rename-tab unknown tab", func(t *testing.T) {
		cmd := &SheetsRenameTabCmd{}
		err := runKong(t, cmd, []string{"s1", "NonExistent", "New"}, ctx, flags)
		if err == nil {
			t.Fatal("expected error for unknown tab")
		}
		if !strings.Contains(err.Error(), "unknown tab") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("delete-tab unknown tab", func(t *testing.T) {
		cmd := &SheetsDeleteTabCmd{}
		flagsForce := &RootFlags{Account: "a@b.com", Force: true}
		err := runKong(t, cmd, []string{"s1", "NonExistent"}, ctx, flagsForce)
		if err == nil {
			t.Fatal("expected error for unknown tab")
		}
		if !strings.Contains(err.Error(), "unknown tab") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("delete-tab unknown tab before confirmation", func(t *testing.T) {
		cmd := &SheetsDeleteTabCmd{}
		err := runKong(t, cmd, []string{"s1", "NonExistent"}, ctx, &RootFlags{Account: "a@b.com", NoInput: true})
		if err == nil {
			t.Fatal("expected error for unknown tab")
		}
		if !strings.Contains(err.Error(), "unknown tab") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("delete-tab dry-run avoids mutation", func(t *testing.T) {
		gotRequests = nil
		cmd := &SheetsDeleteTabCmd{}
		err := runKong(t, cmd, []string{"s1", "OldTab"}, ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
		if ExitCode(err) != 0 {
			t.Fatalf("expected dry-run exit 0, got %v", err)
		}
		if gotRequests != nil {
			t.Fatalf("expected no mutation request during dry-run, got %+v", gotRequests)
		}
	})
}
