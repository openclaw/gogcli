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

func TestSheetsRenameCmd(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	// fakeSheets is a minimal spreadsheet with two tabs.
	fakeSpreadsheet := map[string]any{
		"spreadsheetId": "s1",
		"sheets": []map[string]any{
			{"properties": map[string]any{"sheetId": 0, "title": "Sheet1"}},
			{"properties": map[string]any{"sheetId": 42, "title": "Data"}},
		},
	}

	var gotUpdate *sheets.UpdateSheetPropertiesRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(fakeSpreadsheet)
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].UpdateSheetProperties == nil {
				t.Fatalf("expected updateSheetProperties, got %#v", req.Requests)
			}
			gotUpdate = req.Requests[0].UpdateSheetProperties
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

	t.Run("rename by index 0", func(t *testing.T) {
		gotUpdate = nil
		cmd := &SheetsRenameCmd{}
		if err := runKong(t, cmd, []string{"s1", "0", "Renamed"}, ctx, flags); err != nil {
			t.Fatalf("rename: %v", err)
		}
		if gotUpdate == nil {
			t.Fatal("expected updateSheetProperties request")
		}
		if gotUpdate.Properties.SheetId != 0 {
			t.Fatalf("wrong sheetId: got %d, want 0", gotUpdate.Properties.SheetId)
		}
		if gotUpdate.Properties.Title != "Renamed" {
			t.Fatalf("wrong title: got %q, want %q", gotUpdate.Properties.Title, "Renamed")
		}
		if gotUpdate.Fields != "title" {
			t.Fatalf("fields mask should be 'title', got %q", gotUpdate.Fields)
		}
	})

	t.Run("rename by index 1", func(t *testing.T) {
		gotUpdate = nil
		cmd := &SheetsRenameCmd{}
		if err := runKong(t, cmd, []string{"s1", "1", "NewData"}, ctx, flags); err != nil {
			t.Fatalf("rename: %v", err)
		}
		if gotUpdate == nil {
			t.Fatal("expected updateSheetProperties request")
		}
		if gotUpdate.Properties.SheetId != 42 {
			t.Fatalf("wrong sheetId: got %d, want 42", gotUpdate.Properties.SheetId)
		}
		if gotUpdate.Properties.Title != "NewData" {
			t.Fatalf("wrong title: got %q, want %q", gotUpdate.Properties.Title, "NewData")
		}
	})

	t.Run("rename by name", func(t *testing.T) {
		gotUpdate = nil
		cmd := &SheetsRenameCmd{}
		if err := runKong(t, cmd, []string{"s1", "Data", "Archive"}, ctx, flags); err != nil {
			t.Fatalf("rename: %v", err)
		}
		if gotUpdate == nil {
			t.Fatal("expected updateSheetProperties request")
		}
		if gotUpdate.Properties.SheetId != 42 {
			t.Fatalf("wrong sheetId: got %d, want 42", gotUpdate.Properties.SheetId)
		}
		if gotUpdate.Properties.Title != "Archive" {
			t.Fatalf("wrong title: got %q, want %q", gotUpdate.Properties.Title, "Archive")
		}
	})

	t.Run("rename by name case-insensitive", func(t *testing.T) {
		gotUpdate = nil
		cmd := &SheetsRenameCmd{}
		if err := runKong(t, cmd, []string{"s1", "data", "Lower"}, ctx, flags); err != nil {
			t.Fatalf("rename: %v", err)
		}
		if gotUpdate == nil {
			t.Fatal("expected updateSheetProperties request")
		}
		if gotUpdate.Properties.SheetId != 42 {
			t.Fatalf("wrong sheetId: got %d, want 42", gotUpdate.Properties.SheetId)
		}
	})

	t.Run("index out of range", func(t *testing.T) {
		cmd := &SheetsRenameCmd{}
		err := runKong(t, cmd, []string{"s1", "5", "Oops"}, ctx, flags)
		if err == nil {
			t.Fatal("expected error for out-of-range index")
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unknown sheet name", func(t *testing.T) {
		cmd := &SheetsRenameCmd{}
		err := runKong(t, cmd, []string{"s1", "NoSuchSheet", "Oops"}, ctx, flags)
		if err == nil {
			t.Fatal("expected error for unknown sheet name")
		}
		if !strings.Contains(err.Error(), "no sheet found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty newName rejected", func(t *testing.T) {
		cmd := &SheetsRenameCmd{}
		err := runKong(t, cmd, []string{"s1", "0", ""}, ctx, flags)
		if err == nil {
			t.Fatal("expected error for empty newName")
		}
	})
}

func TestResolveSheetRef(t *testing.T) {
	makeSheets := func(titles ...string) []*sheets.Sheet {
		result := make([]*sheets.Sheet, len(titles))
		for i, title := range titles {
			result[i] = &sheets.Sheet{
				Properties: &sheets.SheetProperties{
					SheetId: int64(i * 10),
					Title:   title,
				},
			}
		}
		return result
	}

	tests := []struct {
		name      string
		ref       string
		sheets    []*sheets.Sheet
		wantID    int64
		wantTitle string
		wantErr   string
	}{
		{name: "index 0", ref: "0", sheets: makeSheets("Alpha", "Beta"), wantID: 0, wantTitle: "Alpha"},
		{name: "index 1", ref: "1", sheets: makeSheets("Alpha", "Beta"), wantID: 10, wantTitle: "Beta"},
		{name: "by name", ref: "Beta", sheets: makeSheets("Alpha", "Beta"), wantID: 10, wantTitle: "Beta"},
		{name: "by name case-insensitive", ref: "beta", sheets: makeSheets("Alpha", "Beta"), wantID: 10, wantTitle: "Beta"},
		{name: "negative index", ref: "-1", sheets: makeSheets("Alpha"), wantErr: "out of range"},
		{name: "index too large", ref: "3", sheets: makeSheets("Alpha"), wantErr: "out of range"},
		{name: "unknown name", ref: "Gamma", sheets: makeSheets("Alpha", "Beta"), wantErr: "no sheet found"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, title, err := resolveSheetRef(tc.sheets, tc.ref)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tc.wantID {
				t.Fatalf("sheetId: got %d, want %d", id, tc.wantID)
			}
			if title != tc.wantTitle {
				t.Fatalf("title: got %q, want %q", title, tc.wantTitle)
			}
		})
	}
}
