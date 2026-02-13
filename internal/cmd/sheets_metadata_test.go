package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestSheetsMetadataCmd_TextAndJSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v4/spreadsheets/id1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "id1",
				"spreadsheetUrl": "https://docs.google.com/spreadsheets/d/id1",
				"properties": map[string]any{
					"title":    "Budget",
					"locale":   "en_US",
					"timeZone": "UTC",
				},
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "gridProperties": map[string]any{"rowCount": 10, "columnCount": 5}}},
				},
			})
			return
		}
		http.NotFound(w, r)
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

	var outBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	cmd := &SheetsMetadataCmd{}
	if err := runKong(t, cmd, []string{"id1"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	text := outBuf.String()
	if !strings.Contains(text, "ID\tid1") || !strings.Contains(text, "Sheets:") {
		t.Fatalf("unexpected text: %q", text)
	}

	jsonOut := captureStdout(t, func() {
		u2, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx2 := ui.WithUI(context.Background(), u2)
		ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

		cmd2 := &SheetsMetadataCmd{}
		if err := runKong(t, cmd2, []string{"id1"}, ctx2, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		SpreadsheetID string `json:"spreadsheetId"`
		Title         string `json:"title"`
		Sheets        []any  `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.SpreadsheetID != "id1" || parsed.Title != "Budget" || len(parsed.Sheets) != 1 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}
