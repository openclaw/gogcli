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

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type updateNoteRecorder struct {
	requests []map[string]any
}

func updateNoteHandler(recorder *updateNoteRecorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		// Handle metadata GET to resolve sheet name → ID.
		if strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet && !strings.Contains(path, "batchUpdate") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{
						"properties": map[string]any{
							"sheetId": 0,
							"title":   "Sheet1",
						},
					},
				},
			})
			return
		}

		// Handle batchUpdate POST.
		if strings.HasPrefix(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			requests, ok := body["requests"].([]any)
			if !ok || len(requests) == 0 {
				http.Error(w, "missing requests", http.StatusBadRequest)
				return
			}

			recorder.requests = recorder.requests[:0]
			for _, req := range requests {
				reqMap, ok := req.(map[string]any)
				if !ok {
					http.Error(w, "expected request object", http.StatusBadRequest)
					return
				}
				recorder.requests = append(recorder.requests, reqMap)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"replies":       make([]any, len(requests)),
			})
			return
		}

		http.NotFound(w, r)
	})
}

func expectRepeatCellRequest(t *testing.T, recorder *updateNoteRecorder, note string, endRow int64, endCol int64) {
	t.Helper()

	if len(recorder.requests) != 1 {
		t.Fatalf("expected 1 batchUpdate request, got %d", len(recorder.requests))
	}

	repeatCell, ok := recorder.requests[0]["repeatCell"].(map[string]any)
	if !ok {
		t.Fatalf("expected repeatCell request, got %#v", recorder.requests[0])
	}

	if repeatCell["fields"] != "note" {
		t.Fatalf("expected fields=note, got %#v", repeatCell["fields"])
	}

	cell, ok := repeatCell["cell"].(map[string]any)
	if !ok {
		t.Fatalf("expected cell payload, got %#v", repeatCell["cell"])
	}

	gotNote, hasNote := cell["note"]
	if note == "" && hasNote {
		t.Fatalf("expected cleared note to omit note field, got %#v", cell)
	}
	if note != "" && (!hasNote || gotNote != note) {
		t.Fatalf("expected note %q, got %#v", note, repeatCell["cell"])
	}

	gotRange, ok := repeatCell["range"].(map[string]any)
	if !ok {
		t.Fatalf("expected range payload, got %#v", repeatCell["range"])
	}

	if v, ok := gotRange["startRowIndex"]; ok && v.(float64) != 0 {
		t.Fatalf("unexpected start row range: %#v", gotRange)
	}
	gotEndRow := gotRange["endRowIndex"]
	if gotEndRow != float64(endRow) {
		t.Fatalf("unexpected row range: %#v", gotRange)
	}

	if v, ok := gotRange["startColumnIndex"]; ok && v.(float64) != 0 {
		t.Fatalf("unexpected start column range: %#v", gotRange)
	}
	gotEndCol := gotRange["endColumnIndex"]
	if gotEndCol != float64(endCol) {
		t.Fatalf("unexpected column range: %#v", gotRange)
	}
}

func TestSheetsUpdateNoteCmd_SingleCell_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	recorder := &updateNoteRecorder{}
	srv := httptest.NewServer(updateNoteHandler(recorder))
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

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsUpdateNoteCmd{}, []string{"s1", "Sheet1!A1", "--note", "Hello world"}, ctx, flags); err != nil {
			t.Fatalf("set-note: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	if result["cellsUpdated"] != float64(1) {
		t.Errorf("expected 1 cell updated, got %v", result["cellsUpdated"])
	}
	if result["note"] != "Hello world" {
		t.Errorf("expected note 'Hello world', got %q", result["note"])
	}

	expectRepeatCellRequest(t, recorder, "Hello world", 1, 1)
}

func TestSheetsUpdateNoteCmd_Range_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	recorder := &updateNoteRecorder{}
	srv := httptest.NewServer(updateNoteHandler(recorder))
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

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsUpdateNoteCmd{}, []string{"s1", "Sheet1!A1:B2", "--note", "Same note"}, ctx, flags); err != nil {
			t.Fatalf("set-note: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	if result["cellsUpdated"] != float64(4) {
		t.Errorf("expected 4 cells updated, got %v", result["cellsUpdated"])
	}

	expectRepeatCellRequest(t, recorder, "Same note", 2, 2)
}

func TestSheetsUpdateNoteCmd_ClearNote_Text(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	recorder := &updateNoteRecorder{}
	srv := httptest.NewServer(updateNoteHandler(recorder))
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

		if err := runKong(t, &SheetsUpdateNoteCmd{}, []string{"s1", "Sheet1!A1", "--note", ""}, ctx, flags); err != nil {
			t.Fatalf("set-note: %v", err)
		}
	})

	if !strings.Contains(out, "Cleared note") {
		t.Errorf("expected 'Cleared note' in output: %q", out)
	}

	expectRepeatCellRequest(t, recorder, "", 1, 1)
}

func TestSheetsUpdateNoteCmd_NoteFile(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	recorder := &updateNoteRecorder{}
	srv := httptest.NewServer(updateNoteHandler(recorder))
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

	// Create temp file with note content.
	tmpFile, err := os.CreateTemp(t.TempDir(), "note-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	noteContent := "Line 1\nLine 2\nLine 3"
	if _, err := tmpFile.WriteString(noteContent); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tmpFile.Close()

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsUpdateNoteCmd{}, []string{"s1", "Sheet1!A1", "--note-file", tmpFile.Name()}, ctx, flags); err != nil {
			t.Fatalf("set-note: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	if result["note"] != noteContent {
		t.Errorf("expected note %q, got %q", noteContent, result["note"])
	}

	expectRepeatCellRequest(t, recorder, noteContent, 1, 1)
}

func TestSheetsUpdateNoteCmd_MissingNote(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err := runKong(t, &SheetsUpdateNoteCmd{}, []string{"s1", "Sheet1!A1"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing --note")
	}
	if !strings.Contains(err.Error(), "provide --note or --note-file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSheetsUpdateNoteCmd_MissingSheetName(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err := runKong(t, &SheetsUpdateNoteCmd{}, []string{"s1", "A1", "--note", "test"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing sheet name")
	}
	if !strings.Contains(err.Error(), "range must include a sheet name") {
		t.Errorf("unexpected error: %v", err)
	}
}
