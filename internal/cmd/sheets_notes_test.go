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

func notesHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		if strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet {
			if r.URL.Query().Get("includeGridData") != "true" {
				http.Error(w, "expected includeGridData=true", http.StatusBadRequest)
				return
			}

			rangeParam := r.URL.Query().Get("ranges")
			startRow, startCol := 0, 0
			if strings.Contains(rangeParam, "B2") {
				startRow, startCol = 1, 1
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{
					{
						"properties": map[string]any{
							"title": "Sheet1",
						},
						"data": []map[string]any{
							{
								"startRow":    startRow,
								"startColumn": startCol,
								"rowData": []map[string]any{
									{
										"values": []map[string]any{
											{"formattedValue": "Name", "note": "Header note"},
											{"formattedValue": "Age"},
										},
									},
									{
										"values": []map[string]any{
											{"formattedValue": "Alice", "note": "First entry"},
											{"formattedValue": "30"},
										},
									},
									{
										"values": []map[string]any{
											{"formattedValue": "Bob"},
											{"formattedValue": "25", "note": "Estimated"},
										},
									},
								},
							},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
}

func TestSheetsNotesCmd_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(notesHandler())
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
		cmd := &SheetsNotesCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B3"}, ctx, flags); err != nil {
			t.Fatalf("notes: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	notes, ok := result["notes"].([]any)
	if !ok {
		t.Fatalf("expected notes array, got %T", result["notes"])
	}
	if len(notes) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(notes))
	}

	first := notes[0].(map[string]any)
	if first["sheet"] != "Sheet1" {
		t.Errorf("expected sheet 'Sheet1', got %q", first["sheet"])
	}
	if first["a1"] != "Sheet1!A1" {
		t.Errorf("expected a1 'Sheet1!A1', got %q", first["a1"])
	}
	if first["row"] != float64(1) {
		t.Errorf("expected row 1, got %v", first["row"])
	}
	if first["col"] != float64(1) {
		t.Errorf("expected col 1, got %v", first["col"])
	}
	if first["note"] != "Header note" {
		t.Errorf("expected 'Header note', got %q", first["note"])
	}
	if first["value"] != "Name" {
		t.Errorf("expected 'Name', got %q", first["value"])
	}
}

func TestSheetsNotesCmd_Text(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(notesHandler())
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

		if err := runKong(t, &SheetsNotesCmd{}, []string{"s1", "Sheet1!A1:B3"}, ctx, flags); err != nil {
			t.Fatalf("notes: %v", err)
		}
	})

	if !strings.Contains(out, "Header note") {
		t.Errorf("expected 'Header note' in output: %q", out)
	}
	if !strings.Contains(out, "Estimated") {
		t.Errorf("expected 'Estimated' in output: %q", out)
	}
	if !strings.Contains(out, "A1") {
		t.Errorf("expected table header in output: %q", out)
	}
}

func TestSheetsNotesCmd_OffsetRange_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(notesHandler())
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
		if err := runKong(t, &SheetsNotesCmd{}, []string{"s1", "Sheet1!B2:C3"}, ctx, flags); err != nil {
			t.Fatalf("notes: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	notes := result["notes"].([]any)
	first := notes[0].(map[string]any)
	if first["a1"] != "Sheet1!B2" {
		t.Errorf("expected a1 'Sheet1!B2', got %q", first["a1"])
	}
	if first["row"] != float64(2) {
		t.Errorf("expected row 2, got %v", first["row"])
	}
	if first["col"] != float64(2) {
		t.Errorf("expected col 2, got %v", first["col"])
	}
}

func TestSheetsNotesCmd_NoNotes(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sheets": []map[string]any{
				{
					"data": []map[string]any{
						{
							"rowData": []map[string]any{
								{
									"values": []map[string]any{
										{"formattedValue": "Name"},
									},
								},
							},
						},
					},
				},
			},
		})
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
	errOut := captureStderr(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		if err := runKong(t, &SheetsNotesCmd{}, []string{"s1", "Sheet1!A1"}, ctx, flags); err != nil {
			t.Fatalf("notes: %v", err)
		}
	})

	if !strings.Contains(errOut, "No notes found") {
		t.Errorf("expected 'No notes found' on stderr: %q", errOut)
	}
}
