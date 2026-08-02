package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"
)

type sheetsUpdatePositionalRecorder struct {
	updateCalls int
	values      [][]interface{}
}

func newSheetsUpdatePositionalTestContext(
	t *testing.T,
	recorder *sheetsUpdatePositionalRecorder,
) context.Context {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPut:
			recorder.updateCalls++
			var req sheets.ValueRange
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode update request: %v", err)
			}
			recorder.values = req.Values
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedRange": "Sheet1!G31",
				"updatedCells": 1,
			})
			return
		case path == "/spreadsheets/s1" && r.Method == http.MethodGet:
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
				"namedRanges": []map[string]any{
					{
						"namedRangeId": "named-single-id",
						"name":         "NamedSingle",
						"range": map[string]any{
							"sheetId":          0,
							"startRowIndex":    30,
							"endRowIndex":      31,
							"startColumnIndex": 6,
							"endColumnIndex":   7,
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc := newSheetsServiceFromServer(t, srv)
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	return withSheetsTestService(ctx, svc)
}

func TestSheetsUpdatePositionalSingleCellPreservesCommas(t *testing.T) {
	recorder := &sheetsUpdatePositionalRecorder{}
	ctx := newSheetsUpdatePositionalTestContext(t, recorder)

	err := runKong(
		t,
		&SheetsUpdateCmd{},
		[]string{"s1", "G31", "text, with, commas"},
		ctx,
		&RootFlags{Account: "a@b.com"},
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if recorder.updateCalls != 1 {
		t.Fatalf("update calls = %d, want 1", recorder.updateCalls)
	}
	if len(recorder.values) != 1 || len(recorder.values[0]) != 1 ||
		recorder.values[0][0] != "text, with, commas" {
		t.Fatalf("values = %#v, want one comma-containing cell", recorder.values)
	}
}

func TestSheetsUpdatePositionalNamedSingleCellPreservesCommas(t *testing.T) {
	recorder := &sheetsUpdatePositionalRecorder{}
	ctx := newSheetsUpdatePositionalTestContext(t, recorder)

	err := runKong(
		t,
		&SheetsUpdateCmd{},
		[]string{"s1", "NamedSingle", "named, cell, value"},
		ctx,
		&RootFlags{Account: "a@b.com"},
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if recorder.updateCalls != 1 {
		t.Fatalf("update calls = %d, want 1", recorder.updateCalls)
	}
	if len(recorder.values) != 1 || len(recorder.values[0]) != 1 ||
		recorder.values[0][0] != "named, cell, value" {
		t.Fatalf("values = %#v, want one comma-containing cell", recorder.values)
	}
}

func TestSheetsUpdatePositionalNamedRangeDryRunRejectsUnresolvedPreview(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	err := runKong(
		t,
		&SheetsUpdateCmd{},
		[]string{"s1", "NamedSingle", "named, cell, value"},
		ctx,
		&RootFlags{Account: "a@b.com", DryRun: true},
	)
	if err == nil || !strings.Contains(err.Error(), "cannot be previewed offline") {
		t.Fatalf("dry-run error = %v, want unresolved-range guidance", err)
	}
	if ExitCode(err) != 2 {
		t.Fatalf("dry-run exit code = %d, want 2", ExitCode(err))
	}
}

func TestSheetsUpdatePositionalPartialMultiCellRangeWritesOneCell(t *testing.T) {
	recorder := &sheetsUpdatePositionalRecorder{}
	ctx := newSheetsUpdatePositionalTestContext(t, recorder)

	err := runKong(
		t,
		&SheetsUpdateCmd{},
		[]string{"s1", "Sheet1!A1:B2", "one"},
		ctx,
		&RootFlags{Account: "a@b.com"},
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if recorder.updateCalls != 1 {
		t.Fatalf("update calls = %d, want 1", recorder.updateCalls)
	}
	if len(recorder.values) != 1 || len(recorder.values[0]) != 1 ||
		recorder.values[0][0] != "one" {
		t.Fatalf("values = %#v, want one-cell matrix", recorder.values)
	}
}

func TestSheetsUpdatePositionalMultiCellExceedsRangeDoesNotWrite(t *testing.T) {
	recorder := &sheetsUpdatePositionalRecorder{}
	ctx := newSheetsUpdatePositionalTestContext(t, recorder)

	err := runKong(
		t,
		&SheetsUpdateCmd{},
		[]string{"s1", "Sheet1!A1:B2", "a,b,c"},
		ctx,
		&RootFlags{Account: "a@b.com"},
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds the update range maximum of 2 rows") {
		t.Fatalf("error = %v, want range-bounds error", err)
	}
	if recorder.updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", recorder.updateCalls)
	}
}
