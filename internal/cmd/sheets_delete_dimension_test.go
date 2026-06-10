package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestSheetsDeleteDimensionCmdTableAwareRows(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

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

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		cmd := &SheetsDeleteDimensionCmd{}
		err = runKong(t, cmd, []string{
			"s1", "Data", "--dimension", "ROWS", "--start", "2", "--end", "3",
		}, newCmdJSONContext(t), &RootFlags{Account: "a@b.com", Force: true})
	})
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
	if !strings.Contains(out, `"beforeA1": "Data!A1:C5"`) || !strings.Contains(out, `"afterA1": "Data!A1:C3"`) {
		t.Fatalf("output = %s", out)
	}
}

func TestSheetsDeleteDimensionCmdRangeTargetColumns(t *testing.T) {
	literalSheet, err := parseSheetsDeleteDimensionSpec("Q1!Q2", "columns", 2, 4)
	if err != nil {
		t.Fatalf("parse literal sheet target: %v", err)
	}
	if literalSheet.SheetName != "Q1!Q2" || literalSheet.StartIndex != 1 || literalSheet.EndIndex != 4 {
		t.Fatalf("literal sheet spec = %#v", literalSheet)
	}

	spec, err := parseSheetsDeleteDimensionSpec("'Data Sheet'!B:C", "columns", 0, 0)
	if err != nil {
		t.Fatalf("parse range target: %v", err)
	}
	if spec.SheetName != "Data Sheet" || spec.Dimension != "COLUMNS" || spec.StartIndex != 1 || spec.EndIndex != 3 {
		t.Fatalf("spec = %#v", spec)
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

	updates, err := planSheetsDeleteDimensionTables([]*sheets.Table{table}, 7, "Data", sheetsDeleteDimensionSpec{
		Dimension:  "COLUMNS",
		Label:      "columns",
		StartIndex: 2,
		EndIndex:   4,
	})
	if err != nil {
		t.Fatalf("plan columns: %v", err)
	}
	if len(updates) != 1 || updates[0].AfterA1 != "Data!B3:C8" {
		t.Fatalf("updates = %#v", updates)
	}

	_, err = planSheetsDeleteDimensionTables([]*sheets.Table{table}, 7, "Data", sheetsDeleteDimensionSpec{
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
	spec, err := parseSheetsDeleteDimensionSpec("Data", "rows", 2, 4)
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
			_, gotErr := parseSheetsDeleteDimensionSpec(tc.target, tc.dimension, tc.start, tc.end)
			if gotErr == nil || !strings.Contains(gotErr.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", gotErr, tc.want)
			}
		})
	}

	err = validateSheetsDeleteDimensionBounds(spec, &sheets.SheetProperties{
		GridProperties: &sheets.GridProperties{RowCount: 3, ColumnCount: 10},
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds sheet size") {
		t.Fatalf("expected bounds error, got %v", err)
	}
}

func TestSheetsDeleteDimensionDryRunIsOffline(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })
	newSheetsService = func(context.Context, string) (*sheets.Service, error) {
		t.Fatal("dry-run must not create Sheets service")
		return nil, errors.New("unexpected Sheets service creation")
	}

	out := captureStdout(t, func() {
		err := (&SheetsDeleteDimensionCmd{
			SpreadsheetID: "s1",
			Target:        "Data!2:4",
			Dimension:     "ROWS",
		}).Run(newCmdJSONContext(t), &RootFlags{DryRun: true})
		if ExitCode(err) != 0 {
			t.Fatalf("dry-run exit: %v", err)
		}
	})
	if !strings.Contains(out, `"op": "sheets.delete-dimension"`) ||
		!strings.Contains(out, `"start_index": 1`) ||
		!strings.Contains(out, `"end_index": 4`) {
		t.Fatalf("dry-run output = %s", out)
	}
}
