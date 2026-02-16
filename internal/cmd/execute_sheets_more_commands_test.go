package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestExecute_SheetsMoreCommands(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/v4/spreadsheets/id1/values/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"range":  "Sheet1!A1:B1",
				"values": []any{[]any{"a", "b"}},
			})
			return
		case strings.Contains(path, "/v4/spreadsheets/id1/values/") && strings.Contains(path, ":clear") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"clearedRange": "Sheet1!A1:B1",
			})
			return
		case strings.Contains(path, "/v4/spreadsheets/id1/values/") && strings.Contains(path, ":append") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updates": map[string]any{
					"updatedRange":   "Sheet1!A1:B1",
					"updatedRows":    1,
					"updatedColumns": 2,
					"updatedCells":   2,
				},
			})
			return
		case strings.Contains(path, "/v4/spreadsheets/id1/values/") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedRange":   "Sheet1!A1:B1",
				"updatedRows":    1,
				"updatedColumns": 2,
				"updatedCells":   2,
			})
			return
		case strings.Contains(path, "/v4/spreadsheets/id1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "id1",
				"properties":    map[string]any{"title": "T"},
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 0, "title": "Sheet1"}},
				},
			})
			return
		case strings.Contains(path, "/v4/spreadsheets") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "id2",
				"properties":    map[string]any{"title": "New"},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	t.Setenv("GOG_ACCOUNT", "a@b.com")

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			// Text mode (covers table output).
			if err := Execute([]string{"sheets", "get", "id1", `Sheet1\\!A1:B1`}); err != nil {
				t.Fatalf("get: %v", err)
			}
		})
		if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
			t.Fatalf("unexpected out=%q", out)
		}

		plainOut := captureStdout(t, func() {
			if err := Execute([]string{"--plain", "sheets", "get", "id1", `Sheet1\\!A1:B1`}); err != nil {
				t.Fatalf("get plain: %v", err)
			}
		})
		if plainOut != "a\tb\n" {
			t.Fatalf("unexpected plain out=%q", plainOut)
		}

		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "update", "id1", "Sheet1!A1:B1", "a|b"}); err != nil {
				t.Fatalf("update: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "update", "id1", "Sheet1!A1:B1", "--values-json", `[["a","b"]]`}); err != nil {
				t.Fatalf("update json: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "append", "id1", "Sheet1!A1:B1", "a|b"}); err != nil {
				t.Fatalf("append: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "append", "id1", "Sheet1!A1:B1", "--values-json", `[["a","b"]]`}); err != nil {
				t.Fatalf("append json: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "clear", "id1", "Sheet1!A1:B1"}); err != nil {
				t.Fatalf("clear: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "metadata", "id1"}); err != nil {
				t.Fatalf("metadata: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "create", "New", "--sheets", "Income,Expenses"}); err != nil {
				t.Fatalf("create: %v", err)
			}
		})
	})
}
