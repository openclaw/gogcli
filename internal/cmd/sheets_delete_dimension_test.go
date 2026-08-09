package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/sheetsdimension"
)

func TestSheetsDeleteDimensionCmdTableAwareRows(t *testing.T) {
	var got sheets.BatchUpdateSpreadsheetRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/spreadsheets/s1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{{
					"properties": map[string]any{
						"sheetId": 7,
						"title":   "Data",
						"gridProperties": map[string]any{
							"rowCount":    100,
							"columnCount": 20,
						},
					},
					"tables": []map[string]any{{
						"tableId": "tbl1",
						"name":    "Tasks",
						"range": map[string]any{
							"sheetId":          7,
							"startRowIndex":    0,
							"endRowIndex":      5,
							"startColumnIndex": 0,
							"endColumnIndex":   3,
						},
					}},
				}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spreadsheets/s1:batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batch update: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)
	var out bytes.Buffer
	ctx := withSheetsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := &SheetsDeleteDimensionCmd{}
	err := runKong(t, cmd, []string{
		"s1", "Data", "--dimension", "ROWS", "--start", "2", "--end", "3",
	}, ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err != nil {
		t.Fatalf("delete rows: %v", err)
	}
	if len(got.Requests) != 2 || got.Requests[0].DeleteDimension == nil || got.Requests[1].UpdateTable == nil {
		t.Fatalf("requests = %#v", got.Requests)
	}
	dim := got.Requests[0].DeleteDimension.Range
	if dim.SheetId != 7 || dim.Dimension != "ROWS" || dim.StartIndex != 1 || dim.EndIndex != 3 {
		t.Fatalf("delete range = %#v", dim)
	}
	update := got.Requests[1].UpdateTable
	if update.Fields != "range" || update.Table.TableId != "tbl1" {
		t.Fatalf("update table = %#v", update)
	}
	if gotRange := update.Table.Range; gotRange.StartRowIndex != 0 || gotRange.EndRowIndex != 3 ||
		gotRange.StartColumnIndex != 0 || gotRange.EndColumnIndex != 3 {
		t.Fatalf("updated table range = %#v", gotRange)
	}
	if !strings.Contains(out.String(), `"beforeA1": "Data!A1:C5"`) || !strings.Contains(out.String(), `"afterA1": "Data!A1:C3"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestSheetsDeleteDimensionCmdRangeTargetColumns(t *testing.T) {
	cmd := &SheetsDeleteDimensionCmd{
		SpreadsheetID: "s1",
		Target:        "'Data Sheet'!B:C",
		Dimension:     "columns",
	}
	err := cmd.Run(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{DryRun: true})
	if ExitCode(err) != 0 {
		t.Fatalf("range dry-run: %v", err)
	}
}

func TestSheetsDeleteDimensionPlanning(t *testing.T) {
	table := &sheets.Table{
		TableId: "tbl1",
		Name:    "Tasks",
		Range: &sheets.GridRange{
			SheetId:          7,
			StartRowIndex:    2,
			EndRowIndex:      8,
			StartColumnIndex: 1,
			EndColumnIndex:   5,
		},
	}

	updates, err := sheetsdimension.PlanTableUpdates([]*sheets.Table{table}, 7, sheetsDeleteDimensionSpec{
		Dimension:  "COLUMNS",
		Label:      "columns",
		StartIndex: 2,
		EndIndex:   4,
	})
	if err != nil {
		t.Fatalf("plan columns: %v", err)
	}
	if len(updates) != 1 ||
		updates[0].After.StartColumnIndex != 1 ||
		updates[0].After.EndColumnIndex != 3 {
		t.Fatalf("updates = %#v", updates)
	}

	_, err = sheetsdimension.PlanTableUpdates([]*sheets.Table{table}, 7, sheetsDeleteDimensionSpec{
		Dimension:  "COLUMNS",
		Label:      "columns",
		StartIndex: 0,
		EndIndex:   5,
	})
	if err == nil || !strings.Contains(err.Error(), "entire column extent") {
		t.Fatalf("expected whole-table guard, got %v", err)
	}
}

func TestSheetsDeleteDimensionValidation(t *testing.T) {
	spec, err := sheetsdimension.ParseDeleteSpec("Data", "rows", 2, 4)
	if err != nil {
		t.Fatalf("parse sheet target: %v", err)
	}
	if spec.StartIndex != 1 || spec.EndIndex != 4 {
		t.Fatalf("spec = %#v", spec)
	}

	for _, tc := range []struct {
		name      string
		target    string
		dimension string
		start     int64
		end       int64
		want      string
	}{
		{name: "missing spans", target: "Data", dimension: "ROWS", want: "require both --start and --end"},
		{name: "ambiguous bare column", target: "B", dimension: "COLUMNS", want: "must include a sheet name"},
		{name: "ambiguous bare row", target: "2025", dimension: "ROWS", want: "must include a sheet name"},
		{name: "column overflow", target: "Data!GKGWBYLWRXTLPQ", dimension: "COLUMNS", want: "too large"},
		{name: "one bound", target: "Data", dimension: "ROWS", start: 2, want: "provide both"},
		{name: "bad dimension", target: "Data", dimension: "CELLS", start: 2, end: 4, want: "ROWS or COLUMNS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := sheetsdimension.ParseDeleteSpec(tc.target, tc.dimension, tc.start, tc.end)
			if gotErr == nil || !strings.Contains(gotErr.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", gotErr, tc.want)
			}
		})
	}

	err = sheetsdimension.ValidateBounds(spec, &sheets.SheetProperties{
		GridProperties: &sheets.GridProperties{RowCount: 3, ColumnCount: 10},
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds sheet size") {
		t.Fatalf("expected bounds error, got %v", err)
	}
}

func TestSheetsDeleteDimensionDryRunIsOffline(t *testing.T) {
	var out bytes.Buffer
	ctx := withSheetsTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), func(context.Context, string) (*sheets.Service, error) {
		t.Fatal("dry-run must not create Sheets service")
		return nil, errors.New("unexpected Sheets service creation")
	})

	err := (&SheetsDeleteDimensionCmd{
		SpreadsheetID: "s1",
		Target:        "Data!2:4",
		Dimension:     "ROWS",
	}).Run(ctx, &RootFlags{DryRun: true})
	if ExitCode(err) != 0 {
		t.Fatalf("dry-run exit: %v", err)
	}
	if !strings.Contains(out.String(), `"op": "sheets.delete-dimension"`) ||
		!strings.Contains(out.String(), `"start_index": 1`) ||
		!strings.Contains(out.String(), `"end_index": 4`) {
		t.Fatalf("dry-run output = %s", out.String())
	}
}
