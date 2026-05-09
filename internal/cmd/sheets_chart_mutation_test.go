package cmd

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestSheetsChartCreate_JSON(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	specJSON := `{"title":"Test Chart","basicChart":{"chartType":"BAR"}}`

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

	addChart := recorder.requests[0]["addChart"].(map[string]any)
	chart := addChart["chart"].(map[string]any)
	spec := chart["spec"].(map[string]any)
	if spec["title"] != "Test Chart" {
		t.Errorf("expected spec title Test Chart, got %v", spec["title"])
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

	if anchor["rowIndex"] != float64(9) {
		t.Errorf("expected rowIndex 9, got %v", anchor["rowIndex"])
	}
	if anchor["columnIndex"] != float64(4) {
		t.Errorf("expected columnIndex 4, got %v", anchor["columnIndex"])
	}
	if anchor["sheetId"] != float64(123) {
		t.Errorf("expected sheetId 123, got %v", anchor["sheetId"])
	}
}

func TestSheetsChartCreate_RemapsSourceRangeWithoutAnchor(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	specJSON := `{"title":"Source Chart","basicChart":{"chartType":"LINE","domains":[{"domain":{"sourceRange":{"sources":[{"sheetId":0,"startRowIndex":0,"endRowIndex":3}]}}}],"series":[{"series":{"sourceRange":{"sources":[{"sheetId":0,"startRowIndex":0,"endRowIndex":3}]}}}]}}`

	captureStdout(t, func() {
		if err := runKong(t, &SheetsChartCreateCmd{}, []string{
			"s1", "--spec-json", specJSON,
		}, ctx, flags); err != nil {
			t.Fatalf("chart create: %v", err)
		}
	})

	addChart, ok := recorder.requests[0]["addChart"].(map[string]any)
	if !ok {
		t.Fatalf("expected addChart, got %v", recorder.requests[0])
	}
	chart := addChart["chart"].(map[string]any)
	spec := chart["spec"].(map[string]any)
	source := basicChartDomainSource(t, spec)
	if source["sheetId"] != float64(123) {
		t.Fatalf("expected remapped sheetId 123, got %v", source["sheetId"])
	}
}

func TestSheetsChartCreate_PreservesSheetIDZeroWhenSpreadsheetHasZero(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	specJSON := `{"title":"Zero Chart","basicChart":{"chartType":"LINE","domains":[{"domain":{"sourceRange":{"sources":[{"sheetId":0,"startRowIndex":0,"endRowIndex":3}]}}}],"series":[{"series":{"sourceRange":{"sources":[{"sheetId":0,"startRowIndex":0,"endRowIndex":3}]}}}]}}`

	captureStdout(t, func() {
		if err := runKong(t, &SheetsChartCreateCmd{}, []string{
			"zero", "--spec-json", specJSON, "--sheet", "Sheet1", "--anchor", "E10",
		}, ctx, flags); err != nil {
			t.Fatalf("chart create: %v", err)
		}
	})

	addChart, ok := recorder.requests[0]["addChart"].(map[string]any)
	if !ok {
		t.Fatalf("expected addChart, got %v", recorder.requests[0])
	}
	chart := addChart["chart"].(map[string]any)
	spec := chart["spec"].(map[string]any)
	source := basicChartDomainSource(t, spec)
	if source["sheetId"] != float64(0) {
		t.Fatalf("expected preserved source sheetId 0, got %v", source["sheetId"])
	}

	pos := chart["position"].(map[string]any)
	overlay := pos["overlayPosition"].(map[string]any)
	anchor := overlay["anchorCell"].(map[string]any)
	if anchor["sheetId"] != float64(0) {
		t.Fatalf("expected anchor sheetId 0, got %v", anchor["sheetId"])
	}
}

func TestSheetsChartUpdate_JSON(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	specJSON := `{"title":"Updated Title","basicChart":{"chartType":"COLUMN","domains":[{"domain":{"sourceRange":{"sources":[{"sheetId":0,"startRowIndex":1,"endRowIndex":4}]}}}],"series":[{"series":{"sourceRange":{"sources":[{"sheetId":0,"startRowIndex":1,"endRowIndex":4}]}}}]}}`

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
	spec := updateSpec["spec"].(map[string]any)
	source := basicChartDomainSource(t, spec)
	if source["sheetId"] != float64(123) {
		t.Errorf("expected remapped sheetId 123, got %v", source["sheetId"])
	}
}

func TestSheetsChartUpdate_PreservesSheetIDZeroWhenSpreadsheetHasZero(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	specJSON := `{"title":"Updated Title","basicChart":{"chartType":"COLUMN","domains":[{"domain":{"sourceRange":{"sources":[{"sheetId":0,"startRowIndex":1,"endRowIndex":4}]}}}],"series":[{"series":{"sourceRange":{"sources":[{"sheetId":0,"startRowIndex":1,"endRowIndex":4}]}}}]}}`

	captureStdout(t, func() {
		if err := runKong(t, &SheetsChartUpdateCmd{}, []string{
			"zero", "100", "--spec-json", specJSON,
		}, ctx, flags); err != nil {
			t.Fatalf("chart update: %v", err)
		}
	})

	updateSpec, ok := recorder.requests[0]["updateChartSpec"].(map[string]any)
	if !ok {
		t.Fatalf("expected updateChartSpec request, got %v", recorder.requests[0])
	}
	spec := updateSpec["spec"].(map[string]any)
	source := basicChartDomainSource(t, spec)
	if source["sheetId"] != float64(0) {
		t.Fatalf("expected preserved sheetId 0, got %v", source["sheetId"])
	}
}

func TestSheetsChartUpdate_AcceptsEmbeddedChartJSON(t *testing.T) {
	recorder := &chartRecorder{}
	ctx, flags, cleanup := newChartTestContext(t, recorder)
	defer cleanup()

	specJSON := `{"chartId":100,"spec":{"title":"Updated Title","basicChart":{"chartType":"LINE"}}}`

	if err := runKong(t, &SheetsChartUpdateCmd{}, []string{
		"s1", "100", "--spec-json", specJSON,
	}, ctx, flags); err != nil {
		t.Fatalf("chart update: %v", err)
	}

	updateSpec, ok := recorder.requests[0]["updateChartSpec"].(map[string]any)
	if !ok {
		t.Fatalf("expected updateChartSpec request, got %v", recorder.requests[0])
	}
	spec, ok := updateSpec["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec in request, got %v", updateSpec)
	}
	if spec["title"] != "Updated Title" {
		t.Errorf("expected updated title, got %v", spec["title"])
	}
}

func basicChartDomainSource(t *testing.T, spec map[string]any) map[string]any {
	t.Helper()

	basicChart, ok := spec["basicChart"].(map[string]any)
	if !ok {
		t.Fatalf("expected basicChart, got %v", spec)
	}
	domains, ok := basicChart["domains"].([]any)
	if !ok || len(domains) == 0 {
		t.Fatalf("expected domains, got %v", basicChart["domains"])
	}
	domain, ok := domains[0].(map[string]any)["domain"].(map[string]any)
	if !ok {
		t.Fatalf("expected domain, got %v", domains[0])
	}
	sourceRange, ok := domain["sourceRange"].(map[string]any)
	if !ok {
		t.Fatalf("expected sourceRange, got %v", domain)
	}
	sources, ok := sourceRange["sources"].([]any)
	if !ok || len(sources) == 0 {
		t.Fatalf("expected sources, got %v", sourceRange["sources"])
	}
	source, ok := sources[0].(map[string]any)
	if !ok {
		t.Fatalf("expected source map, got %v", sources[0])
	}
	return source
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

func TestSheetsChartCreate_EmptySpecJSON(t *testing.T) {
	ctx, _, cleanup := newChartTestContext(t, &chartRecorder{})
	defer cleanup()

	err := runKong(t, &SheetsChartCreateCmd{}, []string{"s1", "--spec-json", "{}"}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for empty chart spec")
	}
	if !strings.Contains(err.Error(), "must contain a ChartSpec") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSheetsChartDelete_RejectsInvalidChartID(t *testing.T) {
	ctx, _, cleanup := newChartTestContext(t, &chartRecorder{})
	defer cleanup()

	err := runKong(t, &SheetsChartDeleteCmd{}, []string{"s1", "0"}, ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil {
		t.Fatal("expected error for invalid chartId")
	}
	if !strings.Contains(err.Error(), "chartId must be greater than 0") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRemapZeroSheetIDsInChartSpec(t *testing.T) {
	spec := &sheets.ChartSpec{
		BasicChart: &sheets.BasicChartSpec{
			Domains: []*sheets.BasicChartDomain{
				{
					Domain: &sheets.ChartData{
						SourceRange: &sheets.ChartSourceRange{
							Sources: []*sheets.GridRange{
								{SheetId: 0, StartRowIndex: 1, EndRowIndex: 4},
							},
						},
					},
				},
			},
			Series: []*sheets.BasicChartSeries{
				{
					Series: &sheets.ChartData{
						SourceRange: &sheets.ChartSourceRange{
							Sources: []*sheets.GridRange{
								{SheetId: 42, StartRowIndex: 1, EndRowIndex: 4},
							},
						},
					},
				},
			},
		},
	}

	remapZeroSheetIDsInChartSpec(spec, 123)

	domainRange := spec.BasicChart.Domains[0].Domain.SourceRange.Sources[0]
	if domainRange.SheetId != 123 {
		t.Fatalf("domain sheetId = %d, want 123", domainRange.SheetId)
	}
	if !slices.Contains(domainRange.ForceSendFields, "SheetId") {
		t.Fatalf("domain ForceSendFields = %v, want SheetId", domainRange.ForceSendFields)
	}

	seriesRange := spec.BasicChart.Series[0].Series.SourceRange.Sources[0]
	if seriesRange.SheetId != 42 {
		t.Fatalf("explicit series sheetId = %d, want unchanged 42", seriesRange.SheetId)
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
