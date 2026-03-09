package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func linksHandler() http.Handler {
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
											{"formattedValue": "Google", "hyperlink": "https://google.com"},
											{"formattedValue": "Age"},
										},
									},
									{
										"values": []map[string]any{
											{"formattedValue": "GitHub", "hyperlink": "https://github.com"},
											{"formattedValue": "30"},
										},
									},
									{
										"values": []map[string]any{
											{"formattedValue": "Bob"},
											{"formattedValue": "Docs", "hyperlink": "https://docs.google.com"},
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

func TestSheetsLinksCmd_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(linksHandler())
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
		cmd := &SheetsLinksCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B3"}, ctx, flags); err != nil {
			t.Fatalf("links: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	links, ok := result["links"].([]any)
	if !ok {
		t.Fatalf("expected links array, got %T", result["links"])
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d", len(links))
	}

	first := links[0].(map[string]any)
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
	if first["link"] != "https://google.com" {
		t.Errorf("expected 'https://google.com', got %q", first["link"])
	}
	if first["value"] != "Google" {
		t.Errorf("expected 'Google', got %q", first["value"])
	}
}

func TestSheetsLinksCmd_Text(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(linksHandler())
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

		if err := runKong(t, &SheetsLinksCmd{}, []string{"s1", "Sheet1!A1:B3"}, ctx, flags); err != nil {
			t.Fatalf("links: %v", err)
		}
	})

	if !strings.Contains(out, "https://google.com") {
		t.Errorf("expected 'https://google.com' in output: %q", out)
	}
	if !strings.Contains(out, "https://docs.google.com") {
		t.Errorf("expected 'https://docs.google.com' in output: %q", out)
	}
	if !strings.Contains(out, "A1") {
		t.Errorf("expected table header in output: %q", out)
	}
}

func TestSheetsLinksCmd_OffsetRange_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(linksHandler())
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
		if err := runKong(t, &SheetsLinksCmd{}, []string{"s1", "Sheet1!B2:C3"}, ctx, flags); err != nil {
			t.Fatalf("links: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	links := result["links"].([]any)
	first := links[0].(map[string]any)
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

func TestSheetsLinksCmd_NoLinks(t *testing.T) {
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

		if err := runKong(t, &SheetsLinksCmd{}, []string{"s1", "Sheet1!A1"}, ctx, flags); err != nil {
			t.Fatalf("links: %v", err)
		}
	})

	if !strings.Contains(errOut, "No links found") {
		t.Errorf("expected 'No links found' on stderr: %q", errOut)
	}
}

func TestSheetsLinksCmd_RichTextRunsAndCellLevelLinks(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sheets": []map[string]any{
				{
					"properties": map[string]any{
						"title": "Sheet1",
					},
					"data": []map[string]any{
						{
							"startRow":    2,
							"startColumn": 2,
							"rowData": []map[string]any{
								{
									"values": []map[string]any{
										{
											"formattedValue": "Rich links",
											"userEnteredFormat": map[string]any{
												"textFormat": map[string]any{
													"link": map[string]any{"uri": "https://cell.example"},
												},
											},
											"textFormatRuns": []map[string]any{
												{"startIndex": 0, "format": map[string]any{"link": map[string]any{"uri": "https://cell.example"}}},
												{"startIndex": 4, "format": map[string]any{"link": map[string]any{"uri": "https://run1.example"}}},
												{"startIndex": 8, "format": map[string]any{"link": map[string]any{"uri": " https://run2.example "}}},
											},
										},
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
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsLinksCmd{}, []string{"s1", "Sheet1!C3"}, ctx, flags); err != nil {
			t.Fatalf("links: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	linksAny, ok := result["links"].([]any)
	if !ok {
		t.Fatalf("expected links array, got %T", result["links"])
	}
	if len(linksAny) != 3 {
		t.Fatalf("expected 3 deduped links, got %d", len(linksAny))
	}

	got := make([]string, 0, len(linksAny))
	for _, entry := range linksAny {
		row := entry.(map[string]any)
		if row["a1"] != "Sheet1!C3" {
			t.Fatalf("unexpected a1: %v", row["a1"])
		}
		got = append(got, row["link"].(string))
	}

	want := []string{"https://cell.example", "https://run1.example", "https://run2.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected links: got %v want %v", got, want)
	}
}

func TestExtractCellLinks(t *testing.T) {
	cell := &sheets.CellData{
		Hyperlink: " https://a.example ",
		UserEnteredFormat: &sheets.CellFormat{
			TextFormat: &sheets.TextFormat{
				Link: &sheets.Link{Uri: "https://a.example"},
			},
		},
		TextFormatRuns: []*sheets.TextFormatRun{
			{Format: &sheets.TextFormat{Link: &sheets.Link{Uri: "https://b.example"}}},
			{Format: &sheets.TextFormat{Link: &sheets.Link{Uri: "https://a.example"}}},
			nil,
			{Format: nil},
			{Format: &sheets.TextFormat{Link: &sheets.Link{Uri: "   "}}},
		},
	}

	got := extractCellLinks(cell)
	want := []string{"https://a.example", "https://b.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected links: got %v want %v", got, want)
	}
}
