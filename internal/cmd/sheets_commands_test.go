package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestSheetsCommands_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":append") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updates": map[string]any{"updatedCells": 1},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":clear") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"clearedRange": "Sheet1!A1",
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedCells": 2,
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"range":  "Sheet1!A1:B2",
				"values": [][]any{{"a", "b"}},
			})
			return
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "s1",
				"spreadsheetUrl": "http://example.com/s1",
				"properties": map[string]any{
					"title":    "Sheet",
					"locale":   "en",
					"timeZone": "UTC",
				},
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "gridProperties": map[string]any{"rowCount": 10, "columnCount": 5}}},
				},
			})
			return
		case path == "/spreadsheets" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "s2",
				"spreadsheetUrl": "http://example.com/s2",
				"properties": map[string]any{
					"title": "New Sheet",
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	_ = captureStdout(t, func() {
		cmd := &SheetsGetCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2"}, ctx, flags); err != nil {
			t.Fatalf("get: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		cmd := &SheetsUpdateCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1", "--values-json", `[["a","b"]]`}, ctx, flags); err != nil {
			t.Fatalf("update: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		cmd := &SheetsAppendCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1", "--values-json", `[["a"]]`}, ctx, flags); err != nil {
			t.Fatalf("append: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		cmd := &SheetsClearCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1"}, ctx, flags); err != nil {
			t.Fatalf("clear: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		cmd := &SheetsMetadataCmd{}
		if err := runKong(t, cmd, []string{"s1"}, ctx, flags); err != nil {
			t.Fatalf("metadata: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		cmd := &SheetsCreateCmd{}
		if err := runKong(t, cmd, []string{"New Sheet", "--sheets", "Sheet1,Sheet2"}, ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}
	})
}

func TestSheetsCommands_Text(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":append") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updates": map[string]any{"updatedCells": 1},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":clear") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"clearedRange": "Sheet1!A1",
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedCells": 2,
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"range":  "Sheet1!A1:B2",
				"values": [][]any{{"a", "b"}},
			})
			return
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "s1",
				"spreadsheetUrl": "http://example.com/s1",
				"properties": map[string]any{
					"title":    "Sheet",
					"locale":   "en",
					"timeZone": "UTC",
				},
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "gridProperties": map[string]any{"rowCount": 10, "columnCount": 5}}},
				},
			})
			return
		case path == "/spreadsheets" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "s2",
				"spreadsheetUrl": "http://example.com/s2",
				"properties": map[string]any{
					"title": "New Sheet",
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

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		if err := runKong(t, &SheetsGetCmd{}, []string{"s1", "Sheet1!A1:B2"}, ctx, flags); err != nil {
			t.Fatalf("get: %v", err)
		}

		if err := runKong(t, &SheetsUpdateCmd{}, []string{"s1", "Sheet1!A1", "--values-json", `[["a","b"]]`}, ctx, flags); err != nil {
			t.Fatalf("update: %v", err)
		}

		if err := runKong(t, &SheetsAppendCmd{}, []string{"s1", "Sheet1!A1", "--values-json", `[["a"]]`}, ctx, flags); err != nil {
			t.Fatalf("append: %v", err)
		}

		if err := runKong(t, &SheetsClearCmd{}, []string{"s1", "Sheet1!A1"}, ctx, flags); err != nil {
			t.Fatalf("clear: %v", err)
		}

		if err := runKong(t, &SheetsMetadataCmd{}, []string{"s1"}, ctx, flags); err != nil {
			t.Fatalf("metadata: %v", err)
		}

		if err := runKong(t, &SheetsCreateCmd{}, []string{"New Sheet"}, ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}
	})
	if !strings.Contains(out, "Sheet1") || !strings.Contains(out, "Created spreadsheet") {
		t.Fatalf("unexpected output: %q", out)
	}
}
