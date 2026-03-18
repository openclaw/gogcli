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

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestSheetsFindReplaceCmd(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotFind *sheets.FindReplaceRequest
	var response any = map[string]any{
		"replies": []map[string]any{
			{
				"findReplace": map[string]any{
					"occurrencesChanged": 5,
					"valuesChanged":      3,
					"formulasChanged":    1,
					"rowsChanged":        2,
					"sheetsChanged":      2,
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 42, "title": "Sheet1"}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].FindReplace == nil {
				t.Fatalf("expected findReplace request, got %#v", req.Requests)
			}
			gotFind = req.Requests[0].FindReplace
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
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

	t.Run("basic replace", func(t *testing.T) {
		gotFind = nil
		cmd := &SheetsFindReplaceCmd{}
		if err := runKong(t, cmd, []string{"s1", "foo", "bar"}, ctx, flags); err != nil {
			t.Fatalf("find-replace: %v", err)
		}
		if gotFind == nil {
			t.Fatal("expected findReplace request")
		}
		if gotFind.Find != "foo" || gotFind.Replacement != "bar" || !gotFind.AllSheets {
			t.Fatalf("unexpected find/replace payload: %#v", gotFind)
		}
	})

	t.Run("sheet scoping", func(t *testing.T) {
		gotFind = nil
		cmd := &SheetsFindReplaceCmd{}
		if err := runKong(t, cmd, []string{"s1", "foo", "bar", "--sheet", "Sheet1"}, ctx, flags); err != nil {
			t.Fatalf("find-replace --sheet: %v", err)
		}
		if gotFind == nil || gotFind.SheetId != 42 || gotFind.AllSheets {
			t.Fatalf("unexpected scoped request: %#v", gotFind)
		}
	})

	t.Run("match entire and formulas", func(t *testing.T) {
		gotFind = nil
		cmd := &SheetsFindReplaceCmd{}
		if err := runKong(t, cmd, []string{"s1", "foo", "bar", "--match-entire", "--formulas"}, ctx, flags); err != nil {
			t.Fatalf("find-replace flags: %v", err)
		}
		if gotFind == nil || !gotFind.MatchEntireCell || !gotFind.IncludeFormulas {
			t.Fatalf("unexpected flags: %#v", gotFind)
		}
	})

	t.Run("json output", func(t *testing.T) {
		gotFind = nil
		jsonCtx := outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		out := captureStdout(t, func() {
			cmd := &SheetsFindReplaceCmd{}
			if err := runKong(t, cmd, []string{"s1", "foo", "bar"}, jsonCtx, flags); err != nil {
				t.Fatalf("find-replace json: %v", err)
			}
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("json decode: %v", err)
		}
		if payload["occurrences_changed"] != float64(5) || payload["formulas_changed"] != float64(1) {
			t.Fatalf("unexpected payload: %#v", payload)
		}
	})

	t.Run("unknown sheet", func(t *testing.T) {
		cmd := &SheetsFindReplaceCmd{}
		if err := runKong(t, cmd, []string{"s1", "foo", "bar", "--sheet", "Missing"}, ctx, flags); err == nil {
			t.Fatalf("expected unknown sheet error")
		}
	})

	t.Run("zero changes", func(t *testing.T) {
		response = map[string]any{
			"replies": []map[string]any{
				{
					"findReplace": map[string]any{
						"occurrencesChanged": 0,
						"valuesChanged":      0,
						"formulasChanged":    0,
						"rowsChanged":        0,
						"sheetsChanged":      0,
					},
				},
			},
		}
		t.Cleanup(func() {
			response = map[string]any{
				"replies": []map[string]any{
					{
						"findReplace": map[string]any{
							"occurrencesChanged": 5,
							"valuesChanged":      3,
							"formulasChanged":    1,
							"rowsChanged":        2,
							"sheetsChanged":      2,
						},
					},
				},
			}
		})
		gotFind = nil
		jsonCtx := outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		out := captureStdout(t, func() {
			cmd := &SheetsFindReplaceCmd{}
			if err := runKong(t, cmd, []string{"s1", "foo", "bar"}, jsonCtx, flags); err != nil {
				t.Fatalf("find-replace zero json: %v", err)
			}
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("json decode: %v", err)
		}
		if payload["occurrences_changed"] != float64(0) {
			t.Fatalf("unexpected zero payload: %#v", payload)
		}
	})
}
