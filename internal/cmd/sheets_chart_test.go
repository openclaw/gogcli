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

// chartRecorder captures batchUpdate requests.
type chartRecorder struct {
	requests []map[string]any
}

func chartHandler(recorder *chartRecorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		// Metadata GET for chart list and sheet ID resolution.
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
						"charts": []map[string]any{
							{
								"chartId": 100,
								"spec": map[string]any{
									"title": "Revenue",
									"basicChart": map[string]any{
										"chartType": "COLUMN",
									},
								},
							},
							{
								"chartId": 200,
								"spec": map[string]any{
									"title": "Expenses",
									"basicChart": map[string]any{
										"chartType": "LINE",
									},
								},
							},
						},
					},
				},
			})
			return
		}

		// BatchUpdate POST.
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

			// Build reply for addChart.
			replies := make([]map[string]any, len(requests))
			for i, req := range requests {
				reqMap, _ := req.(map[string]any)
				if _, ok := reqMap["addChart"]; ok {
					replies[i] = map[string]any{
						"addChart": map[string]any{
							"chart": map[string]any{
								"chartId": 999,
							},
						},
					}
				} else {
					replies[i] = map[string]any{}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"replies":       replies,
			})
			return
		}

		http.NotFound(w, r)
	})
}

func newChartTestContext(t *testing.T, recorder *chartRecorder) (context.Context, *RootFlags, func()) {
	t.Helper()

	origNew := newSheetsService
	srv := httptest.NewServer(chartHandler(recorder))

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

	cleanup := func() {
		srv.Close()
		newSheetsService = origNew
	}
	return ctx, flags, cleanup
}

func TestSheetsChartList_JSON(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsChartListCmd{}, []string{"s1"}, ctx, flags); err != nil {
			t.Fatalf("chart list: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	charts, ok := result["charts"].([]any)
	if !ok {
		t.Fatalf("expected charts array, got %T", result["charts"])
	}
	if len(charts) != 2 {
		t.Fatalf("expected 2 charts, got %d", len(charts))
	}

	first := charts[0].(map[string]any)
	if first["chartId"] != float64(100) {
		t.Errorf("expected chartId 100, got %v", first["chartId"])
	}
	if first["title"] != "Revenue" {
		t.Errorf("expected title Revenue, got %v", first["title"])
	}
	if first["type"] != "COLUMN" {
		t.Errorf("expected type COLUMN, got %v", first["type"])
	}
}

func TestSheetsChartList_Text(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx = ui.WithUI(ctx, u)

		if err := runKong(t, &SheetsChartListCmd{}, []string{"s1"}, ctx, flags); err != nil {
			t.Fatalf("chart list: %v", err)
		}
	})

	// Text output goes through tableWriter which writes to stdout via the writer.
	// Just ensure no error occurred; the test server returns charts.
	_ = out
}

func TestSheetsChartGet_JSON(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsChartGetCmd{}, []string{"s1", "100"}, ctx, flags); err != nil {
			t.Fatalf("chart get: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	if result["chartId"] != float64(100) {
		t.Errorf("expected chartId 100, got %v", result["chartId"])
	}

	spec, ok := result["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec in output, got %v", result)
	}
	if spec["title"] != "Revenue" {
		t.Errorf("expected title Revenue, got %v", spec["title"])
	}
}

func TestSheetsChartGet_NotFound(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	err := runKong(t, &SheetsChartGetCmd{}, []string{"s1", "999999"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for unknown chart")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSheetsChartCreate_JSON(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	specJSON := `{"spec":{"title":"Test Chart","basicChart":{"chartType":"BAR"}}}`

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsChartCreateCmd{}, []string{
			"s1", "--spec-json", specJSON,
		}, ctx, flags); err != nil {
			t.Fatalf("chart create: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	if result["chartId"] != float64(999) {
		t.Errorf("expected chartId 999, got %v", result["chartId"])
	}

	if len(recorder.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(recorder.requests))
	}
	if _, ok := recorder.requests[0]["addChart"]; !ok {
		t.Fatalf("expected addChart request, got %v", recorder.requests[0])
	}
}

func TestSheetsChartCreate_WithAnchor(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	specJSON := `{"spec":{"title":"Anchored Chart","basicChart":{"chartType":"LINE"}}}`

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsChartCreateCmd{}, []string{
			"s1", "--spec-json", specJSON, "--sheet", "Sheet1", "--anchor", "E10",
		}, ctx, flags); err != nil {
			t.Fatalf("chart create: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	if len(recorder.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(recorder.requests))
	}

	addChart, ok := recorder.requests[0]["addChart"].(map[string]any)
	if !ok {
		t.Fatalf("expected addChart, got %v", recorder.requests[0])
	}

	chart, ok := addChart["chart"].(map[string]any)
	if !ok {
		t.Fatalf("expected chart in addChart, got %v", addChart)
	}

	pos, ok := chart["position"].(map[string]any)
	if !ok {
		t.Fatalf("expected position, got %v", chart)
	}

	overlay, ok := pos["overlayPosition"].(map[string]any)
	if !ok {
		t.Fatalf("expected overlayPosition, got %v", pos)
	}

	anchor, ok := overlay["anchorCell"].(map[string]any)
	if !ok {
		t.Fatalf("expected anchorCell, got %v", overlay)
	}

	// E10 → row 9 (0-indexed), col 4 (0-indexed)
	if anchor["rowIndex"] != float64(9) {
		t.Errorf("expected rowIndex 9, got %v", anchor["rowIndex"])
	}
	if anchor["columnIndex"] != float64(4) {
		t.Errorf("expected columnIndex 4, got %v", anchor["columnIndex"])
	}
}

func TestSheetsChartUpdate_JSON(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	specJSON := `{"title":"Updated Title","basicChart":{"chartType":"PIE"}}`

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsChartUpdateCmd{}, []string{
			"s1", "100", "--spec-json", specJSON,
		}, ctx, flags); err != nil {
			t.Fatalf("chart update: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	if result["chartId"] != float64(100) {
		t.Errorf("expected chartId 100, got %v", result["chartId"])
	}

	if len(recorder.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(recorder.requests))
	}

	updateSpec, ok := recorder.requests[0]["updateChartSpec"].(map[string]any)
	if !ok {
		t.Fatalf("expected updateChartSpec request, got %v", recorder.requests[0])
	}
	if updateSpec["chartId"] != float64(100) {
		t.Errorf("expected chartId 100 in request, got %v", updateSpec["chartId"])
	}
}

func TestSheetsChartDelete_JSON(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, _, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flagsForce := &RootFlags{Account: "a@b.com", Force: true}

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsChartDeleteCmd{}, []string{"s1", "100"}, ctx, flagsForce); err != nil {
			t.Fatalf("chart delete: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, out)
	}

	if result["chartId"] != float64(100) {
		t.Errorf("expected chartId 100, got %v", result["chartId"])
	}

	if len(recorder.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(recorder.requests))
	}

	delReq, ok := recorder.requests[0]["deleteEmbeddedObject"].(map[string]any)
	if !ok {
		t.Fatalf("expected deleteEmbeddedObject request, got %v", recorder.requests[0])
	}
	if delReq["objectId"] != float64(100) {
		t.Errorf("expected objectId 100, got %v", delReq["objectId"])
	}
}

func TestSheetsChartDelete_RequiresConfirmation(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, _, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	// NoInput + no Force → should refuse.
	flags := &RootFlags{Account: "a@b.com", NoInput: true}

	err := runKong(t, &SheetsChartDeleteCmd{}, []string{"s1", "100"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error without --force")
	}
	if !strings.Contains(err.Error(), "without --force") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSheetsChartDelete_DryRun(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, _, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true}

	err := runKong(t, &SheetsChartDeleteCmd{}, []string{"s1", "100"}, ctx, flags)
	if ExitCode(err) != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}
	if len(recorder.requests) != 0 {
		t.Fatalf("expected no mutation during dry-run, got %d requests", len(recorder.requests))
	}
}

func TestSheetsChartCreate_EmptySpreadsheetID(t *testing.T) {
	ctx, _, cleanup := newChartTestContext(t, &chartRecorder{})
	defer cleanup()

	err := runKong(t, &SheetsChartCreateCmd{}, []string{"", "--spec-json", `{}`}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for empty spreadsheetId")
	}
	if !strings.Contains(err.Error(), "empty spreadsheetId") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSheetsChartCreate_InvalidSpecJSON(t *testing.T) {
	ctx, _, cleanup := newChartTestContext(t, &chartRecorder{})
	defer cleanup()

	err := runKong(t, &SheetsChartCreateCmd{}, []string{"s1", "--spec-json", "not json"}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid --spec-json") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseA1Cell(t *testing.T) {
	tests := []struct {
		input   string
		wantRow int
		wantCol int
		wantErr bool
	}{
		{"A1", 1, 1, false},
		{"B5", 5, 2, false},
		{"Z26", 26, 26, false},
		{"AA1", 1, 27, false},
		{"E10", 10, 5, false},
		{"", 0, 0, true},
		{"1A", 0, 0, true},
		{"A", 0, 0, true},
		{"A0", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseA1Cell(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got.row != tt.wantRow || got.col != tt.wantCol {
				t.Errorf("parseA1Cell(%q) = {row:%d col:%d}, want {row:%d col:%d}", tt.input, got.row, got.col, tt.wantRow, tt.wantCol)
			}
		})
	}
}
