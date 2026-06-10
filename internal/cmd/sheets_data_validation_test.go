package cmd

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestBuildDataValidationCondition(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		values     []string
		wantType   string
		wantValues int
		wantErr    string
	}{
		{name: "list", kind: "one-of-list", values: []string{"red", "green"}, wantType: "ONE_OF_LIST", wantValues: 2},
		{name: "literal whitespace", kind: "TEXT_EQ", values: []string{" pending "}, wantType: "TEXT_EQ", wantValues: 1},
		{name: "number between", kind: "NUMBER_BETWEEN", values: []string{"1", "10"}, wantType: "NUMBER_BETWEEN", wantValues: 2},
		{name: "checkbox defaults", kind: "boolean", wantType: "BOOLEAN"},
		{name: "custom checkbox", kind: "BOOLEAN", values: []string{"yes", "no"}, wantType: "BOOLEAN", wantValues: 2},
		{name: "valid date", kind: "DATE_IS_VALID", wantType: "DATE_IS_VALID"},
		{name: "range normalized", kind: "ONE_OF_RANGE", values: []string{"Sheet1!A1:A3"}, wantType: "ONE_OF_RANGE", wantValues: 1},
		{name: "range arity", kind: "ONE_OF_RANGE", wantErr: "requires exactly 1"},
		{name: "empty range", kind: "ONE_OF_RANGE", values: []string{""}, wantErr: "non-empty range"},
		{name: "between arity", kind: "NUMBER_BETWEEN", values: []string{"1"}, wantErr: "requires exactly 2"},
		{name: "checkbox arity", kind: "BOOLEAN", values: []string{"a", "b", "c"}, wantErr: "accepts 0 to 2"},
		{name: "list formula", kind: "ONE_OF_LIST", values: []string{"=A1"}, wantErr: "cannot be formulas"},
		{name: "custom formula syntax", kind: "CUSTOM_FORMULA", values: []string{"A1>0"}, wantErr: "must begin with"},
		{name: "unsupported", kind: "BLANK", wantErr: "unsupported validation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, err := buildDataValidationCondition(tt.kind, tt.values)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected %q error, got %v", tt.wantErr, err)
				}
				if got := ExitCode(err); got != 2 {
					t.Fatalf("ExitCode = %d, want 2", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("build condition: %v", err)
			}
			if condition.Type != tt.wantType || len(condition.Values) != tt.wantValues {
				t.Fatalf("condition = %#v, want type=%s values=%d", condition, tt.wantType, tt.wantValues)
			}
			if tt.name == "range normalized" && condition.Values[0].UserEnteredValue != "=Sheet1!A1:A3" {
				t.Fatalf("range value = %q", condition.Values[0].UserEnteredValue)
			}
			if tt.name == "literal whitespace" && condition.Values[0].UserEnteredValue != " pending " {
				t.Fatalf("literal value = %q", condition.Values[0].UserEnteredValue)
			}
		})
	}
}

func TestSheetsValidationSetAndClear(t *testing.T) {
	ctx, flags, requests, rawRequests, cleanup := newSheetsValidationTestContext(t, false)
	defer cleanup()

	setOut := captureStdout(t, func() {
		if err := runKong(t, &SheetsValidationSetCmd{}, []string{
			"s1", "AllowedColors",
			"--type", "ONE_OF_LIST",
			"--value", "red",
			"--value", "green",
			"--strict",
			"--no-show-custom-ui",
			"--input-message", "Pick a color",
			"--filtered-rows-included",
		}, ctx, flags); err != nil {
			t.Fatalf("validation set: %v", err)
		}
	})
	if !strings.Contains(setOut, `"type": "ONE_OF_LIST"`) || !strings.Contains(setOut, `"showCustomUi": false`) {
		t.Fatalf("unexpected set output: %s", setOut)
	}
	if len(*requests) != 1 || len((*requests)[0].Requests) != 1 {
		t.Fatalf("requests = %#v", *requests)
	}
	setReq := (*requests)[0].Requests[0].SetDataValidation
	if setReq == nil || setReq.Rule == nil || setReq.Rule.Condition == nil {
		t.Fatalf("missing setDataValidation request: %#v", (*requests)[0].Requests[0])
	}
	if setReq.Range.SheetId != 0 || setReq.Range.StartRowIndex != 1 || setReq.Range.EndRowIndex != 5 ||
		setReq.Range.StartColumnIndex != 0 || setReq.Range.EndColumnIndex != 1 {
		t.Fatalf("range = %#v", setReq.Range)
	}
	if !setReq.Rule.Strict || setReq.Rule.ShowCustomUi || !setReq.FilteredRowsIncluded {
		t.Fatalf("rule/request booleans = %#v / %#v", setReq.Rule, setReq)
	}
	if !strings.Contains((*rawRequests)[0], `"sheetId":0`) ||
		!strings.Contains((*rawRequests)[0], `"showCustomUi":false`) ||
		!strings.Contains((*rawRequests)[0], `"filteredRowsIncluded":true`) {
		t.Fatalf("missing force-sent values: %s", (*rawRequests)[0])
	}

	clearOut := captureStdout(t, func() {
		if err := runKong(t, &SheetsValidationClearCmd{}, []string{
			"s1", "Sheet1!B2:B5",
		}, ctx, flags); err != nil {
			t.Fatalf("validation clear: %v", err)
		}
	})
	if !strings.Contains(clearOut, `"cleared": true`) {
		t.Fatalf("unexpected clear output: %s", clearOut)
	}
	if len(*requests) != 2 {
		t.Fatalf("requests = %#v", *requests)
	}
	clearReq := (*requests)[1].Requests[0].SetDataValidation
	if clearReq == nil || clearReq.Rule != nil {
		t.Fatalf("clear request = %#v", clearReq)
	}
	if strings.Contains((*rawRequests)[1], `"rule"`) {
		t.Fatalf("clear request should omit rule: %s", (*rawRequests)[1])
	}
	if !strings.Contains((*rawRequests)[1], `"filteredRowsIncluded":false`) {
		t.Fatalf("clear request should force-send false filteredRowsIncluded: %s", (*rawRequests)[1])
	}
}

func TestSheetsValidationGetJSONAndPlain(t *testing.T) {
	ctx, flags, _, _, cleanup := newSheetsValidationTestContext(t, true)
	defer cleanup()

	jsonOut := captureStdout(t, func() {
		if err := runKong(t, &SheetsValidationGetCmd{}, []string{"s1", "Sheet1!C2:C3"}, ctx, flags); err != nil {
			t.Fatalf("validation get JSON: %v", err)
		}
	})
	if !strings.Contains(jsonOut, `"a1": "Sheet1!C2"`) ||
		!strings.Contains(jsonOut, `"type": "ONE_OF_LIST"`) ||
		!strings.Contains(jsonOut, `"userEnteredValue": "red"`) {
		t.Fatalf("unexpected JSON output: %s", jsonOut)
	}
	unqualifiedOut := captureStdout(t, func() {
		if err := runKong(t, &SheetsValidationGetCmd{}, []string{"s1", "C2:C3"}, ctx, flags); err != nil {
			t.Fatalf("validation get unqualified: %v", err)
		}
	})
	if !strings.Contains(unqualifiedOut, `"a1": "Sheet1!C2"`) {
		t.Fatalf("unexpected unqualified output: %s", unqualifiedOut)
	}

	plainCtx := outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	plainOut := captureStdout(t, func() {
		if err := runKong(t, &SheetsValidationGetCmd{}, []string{"s1", "Sheet1!C2:C3"}, plainCtx, flags); err != nil {
			t.Fatalf("validation get plain: %v", err)
		}
	})
	want := fmt.Sprintf(
		"A1\tTYPE\tVALUES\tSTRICT\tSHOW_CUSTOM_UI\tINPUT_MESSAGE\n"+
			"Sheet1!C2\tONE_OF_LIST\t[\"red\",\"green\"]\t%t\t%t\tPick one\n",
		true,
		true,
	)
	if plainOut != want {
		t.Fatalf("plain output = %q, want %q", plainOut, want)
	}
}

func TestAppendTableCellValidations(t *testing.T) {
	rule := &sheets.DataValidationRule{
		Condition:    &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
		ShowCustomUi: true,
	}
	items := appendTableCellValidations(
		[]sheetsCellValidation{{
			Sheet: "Sheet1",
			A1:    "Sheet1!I2",
			Row:   2,
			Col:   9,
			Rule:  rule,
		}},
		[]tableValidationSpan{{
			SheetID:  7,
			StartRow: 1,
			EndRow:   4,
			StartCol: 8,
			EndCol:   9,
			Rule:     rule,
		}},
		&sheets.GridRange{
			SheetId:          7,
			StartRowIndex:    1,
			EndRowIndex:      4,
			StartColumnIndex: 8,
			EndColumnIndex:   9,
		},
		map[int64]string{7: "Sheet1"},
	)
	if len(items) != 3 {
		t.Fatalf("items = %#v", items)
	}
	if items[1].A1 != "Sheet1!I3" || items[2].A1 != "Sheet1!I4" {
		t.Fatalf("synthesized cells = %#v", items)
	}
	if got := appendTableCellValidations(nil, []tableValidationSpan{{
		SheetID:  7,
		StartRow: 1,
		EndRow:   4,
		StartCol: 9,
		EndCol:   10,
	}}, &sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      4,
		StartColumnIndex: 9,
		EndColumnIndex:   10,
	}, map[int64]string{7: "Sheet1"}); len(got) != 0 {
		t.Fatalf("text table column validations = %#v", got)
	}
}

func TestResolveValidationReadRangePrefersNamedRange(t *testing.T) {
	catalog := &spreadsheetRangeCatalog{
		SheetIDsByTitle: map[string]int64{"Sheet1": 7},
		SheetTitlesByID: map[int64]string{7: "Sheet1"},
		NamedRanges: []*sheets.NamedRange{{
			NamedRangeId: "nr1",
			Name:         "Validation1",
			Range: &sheets.GridRange{
				SheetId:          7,
				StartRowIndex:    1,
				EndRowIndex:      3,
				StartColumnIndex: 2,
				EndColumnIndex:   3,
			},
		}},
		Sheets: []*sheets.SheetProperties{{SheetId: 7, Title: "Sheet1", Index: 0}},
	}
	apiRange, gridRange, err := resolveValidationReadRange("validation1", catalog)
	if err != nil {
		t.Fatalf("resolve named range: %v", err)
	}
	if apiRange != "Validation1" {
		t.Fatalf("api range = %q", apiRange)
	}
	if gridRange.SheetId != 7 || gridRange.StartRowIndex != 1 || gridRange.EndRowIndex != 3 ||
		gridRange.StartColumnIndex != 2 || gridRange.EndColumnIndex != 3 {
		t.Fatalf("grid range = %#v", gridRange)
	}
}

func TestBuildTableValidationClearRequests(t *testing.T) {
	columns := []*sheets.TableColumnProperties{
		{ColumnIndex: 0, ColumnName: "Choice", ColumnType: "DROPDOWN", DataValidationRule: &sheets.TableColumnDataValidationRule{
			Condition: &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
		}},
		{ColumnIndex: 1, ColumnName: "Other", ColumnType: "TEXT"},
	}
	spans := []tableValidationSpan{{
		SheetID:     7,
		TableID:     "table-1",
		ColumnIndex: 0,
		StartRow:    1,
		EndRow:      4,
		StartCol:    8,
		EndCol:      9,
		Columns:     columns,
		Rule: &sheets.DataValidationRule{
			Condition:    cloneBooleanCondition(columns[0].DataValidationRule.Condition),
			ShowCustomUi: true,
		},
	}}

	requests, err := buildTableValidationClearRequests(&sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      4,
		StartColumnIndex: 8,
		EndColumnIndex:   9,
	}, spans)
	if err != nil {
		t.Fatalf("clear requests: %v", err)
	}
	if len(requests) != 1 || requests[0].UpdateTable == nil {
		t.Fatalf("requests = %#v", requests)
	}
	update := requests[0].UpdateTable
	if update.Fields != "columnProperties" || len(update.Table.ColumnProperties) != 2 {
		t.Fatalf("update = %#v", update)
	}
	cleared := update.Table.ColumnProperties[0]
	if cleared.ColumnName != "Choice" || cleared.ColumnType != "TEXT" || cleared.DataValidationRule != nil {
		t.Fatalf("cleared column = %#v", cleared)
	}
	encoded, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	if !strings.Contains(string(encoded), `"dataValidationRule":null`) {
		t.Fatalf("update should null validation rule: %s", encoded)
	}
	if got := update.Table.ColumnProperties[1]; got.ColumnName != "Other" || got.ColumnType != "TEXT" {
		t.Fatalf("untouched column = %#v", got)
	}

	_, err = buildTableValidationClearRequests(&sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      3,
		StartColumnIndex: 8,
		EndColumnIndex:   9,
	}, spans)
	if err == nil || !strings.Contains(err.Error(), "partially intersects") {
		t.Fatalf("expected partial intersection error, got %v", err)
	}
}

func TestBuildTableValidationSetRequests(t *testing.T) {
	columns := []*sheets.TableColumnProperties{
		{ColumnIndex: 0, ColumnName: "Choice", ColumnType: "DROPDOWN", DataValidationRule: &sheets.TableColumnDataValidationRule{
			Condition: &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
		}},
		{ColumnIndex: 1, ColumnName: "Other", ColumnType: "TEXT"},
	}
	spans := []tableValidationSpan{{
		SheetID:     7,
		TableID:     "table-1",
		ColumnIndex: 0,
		StartRow:    1,
		EndRow:      4,
		StartCol:    8,
		EndCol:      9,
		Columns:     columns,
		Rule: &sheets.DataValidationRule{
			Condition:    cloneBooleanCondition(columns[0].DataValidationRule.Condition),
			ShowCustomUi: true,
		},
	}}
	target := &sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      4,
		StartColumnIndex: 8,
		EndColumnIndex:   9,
	}

	listCondition := &sheets.BooleanCondition{
		Type:   "ONE_OF_LIST",
		Values: []*sheets.ConditionValue{{UserEnteredValue: "new"}},
	}
	requests, err := buildTableValidationSetRequests(target, spans, listCondition)
	if err != nil {
		t.Fatalf("set requests: %v", err)
	}
	if len(requests) != 1 || requests[0].UpdateTable == nil {
		t.Fatalf("requests = %#v", requests)
	}
	updated := requests[0].UpdateTable.Table.ColumnProperties[0]
	if updated.ColumnType != "DROPDOWN" || updated.DataValidationRule == nil ||
		updated.DataValidationRule.Condition.Values[0].UserEnteredValue != "new" {
		t.Fatalf("updated dropdown = %#v", updated)
	}

	requests, err = buildTableValidationSetRequests(target, spans, &sheets.BooleanCondition{
		Type:   "NUMBER_GREATER",
		Values: []*sheets.ConditionValue{{UserEnteredValue: "0"}},
	})
	if err == nil || !strings.Contains(err.Error(), "only supports ONE_OF_LIST") {
		t.Fatalf("expected table condition error, got requests=%#v err=%v", requests, err)
	}

	textSpans := append([]tableValidationSpan(nil), spans...)
	textSpans = append(textSpans, tableValidationSpan{
		SheetID:     7,
		TableID:     "table-1",
		ColumnIndex: 1,
		StartRow:    1,
		EndRow:      4,
		StartCol:    9,
		EndCol:      10,
		Columns:     columns,
	})
	requests, err = buildTableValidationSetRequests(&sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      4,
		StartColumnIndex: 9,
		EndColumnIndex:   10,
	}, textSpans, listCondition)
	if err != nil {
		t.Fatalf("set text table column dropdown: %v", err)
	}
	updated = requests[0].UpdateTable.Table.ColumnProperties[1]
	if updated.ColumnType != "DROPDOWN" || updated.DataValidationRule == nil ||
		updated.DataValidationRule.Condition.Values[0].UserEnteredValue != "new" {
		t.Fatalf("updated text table column = %#v", updated)
	}

	requests, err = buildTableValidationSetRequests(&sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      3,
		StartColumnIndex: 9,
		EndColumnIndex:   10,
	}, textSpans, &sheets.BooleanCondition{
		Type:   "NUMBER_GREATER",
		Values: []*sheets.ConditionValue{{UserEnteredValue: "0"}},
	})
	if err == nil || !strings.Contains(err.Error(), "only supports ONE_OF_LIST") {
		t.Fatalf("expected partial text table condition error, got requests=%#v err=%v", requests, err)
	}

	_, err = buildTableValidationSetRequests(&sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      3,
		StartColumnIndex: 8,
		EndColumnIndex:   9,
	}, spans, listCondition)
	if err == nil || !strings.Contains(err.Error(), "partially intersects") {
		t.Fatalf("expected partial set error, got %v", err)
	}
}

func TestFetchTableValidationSpansExcludesFooter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sheets": []map[string]any{{
				"properties": map[string]any{"sheetId": 7},
				"tables": []map[string]any{{
					"tableId": "table-1",
					"range": map[string]any{
						"sheetId":          7,
						"startRowIndex":    0,
						"endRowIndex":      5,
						"startColumnIndex": 8,
						"endColumnIndex":   10,
					},
					"rowsProperties": map[string]any{
						"footerColorStyle": map[string]any{},
					},
					"columnProperties": []map[string]any{
						{
							"columnIndex": 0,
							"columnName":  "Choice",
							"columnType":  "DROPDOWN",
							"dataValidationRule": map[string]any{
								"condition": map[string]any{"type": "ONE_OF_LIST"},
							},
						},
						{
							"columnIndex": 1,
							"columnName":  "Other",
							"columnType":  "TEXT",
						},
					},
				}},
			}},
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
	spans, err := fetchTableValidationSpans(context.Background(), svc, "s1")
	if err != nil {
		t.Fatalf("fetch spans: %v", err)
	}
	if len(spans) != 2 || spans[0].StartRow != 1 || spans[0].EndRow != 4 || spans[1].Rule != nil {
		t.Fatalf("spans = %#v", spans)
	}
}

func TestBoundGridRangeToSheet(t *testing.T) {
	catalog := &spreadsheetRangeCatalog{Sheets: []*sheets.SheetProperties{{
		SheetId: 7,
		GridProperties: &sheets.GridProperties{
			RowCount:    1000,
			ColumnCount: 26,
		},
	}}}
	got := boundGridRangeToSheet(&sheets.GridRange{
		SheetId:          7,
		StartColumnIndex: 8,
		EndColumnIndex:   9,
	}, catalog)
	if got.EndRowIndex != 1000 || got.EndColumnIndex != 9 {
		t.Fatalf("bounded range = %#v", got)
	}
}

func TestSubtractTableValidationSpans(t *testing.T) {
	target := &sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      5,
		StartColumnIndex: 8,
		EndColumnIndex:   11,
	}
	ranges := subtractTableValidationSpans(target, []tableValidationSpan{{
		SheetID:  7,
		StartRow: 1,
		EndRow:   4,
		StartCol: 8,
		EndCol:   9,
	}})
	if len(ranges) != 2 {
		t.Fatalf("ranges = %#v", ranges)
	}
	if got := ranges[0]; got.StartRowIndex != 4 || got.EndRowIndex != 5 ||
		got.StartColumnIndex != 8 || got.EndColumnIndex != 11 {
		t.Fatalf("bottom range = %#v", got)
	}
	if got := ranges[1]; got.StartRowIndex != 1 || got.EndRowIndex != 4 ||
		got.StartColumnIndex != 9 || got.EndColumnIndex != 11 {
		t.Fatalf("right range = %#v", got)
	}

	if got := subtractTableValidationSpans(&sheets.GridRange{
		SheetId:          7,
		StartRowIndex:    1,
		EndRowIndex:      4,
		StartColumnIndex: 8,
		EndColumnIndex:   9,
	}, []tableValidationSpan{{
		SheetID:  7,
		StartRow: 1,
		EndRow:   4,
		StartCol: 8,
		EndCol:   9,
	}}); len(got) != 0 {
		t.Fatalf("exact subtraction = %#v", got)
	}
}

func TestFormatA1CellPreservesSheetTitleWhitespace(t *testing.T) {
	if got := formatA1Cell("  Sheet One  ", 2, 3); got != "'  Sheet One  '!C2" {
		t.Fatalf("formatA1Cell = %q", got)
	}
}

func TestBuildTableValidationCopyRequests(t *testing.T) {
	rule := &sheets.DataValidationRule{
		Condition:    &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
		ShowCustomUi: true,
	}
	spans := []tableValidationSpan{{
		SheetID:  1,
		StartRow: 1,
		EndRow:   4,
		StartCol: 8,
		EndCol:   9,
		Rule:     rule,
	}}

	t.Run("normal tiling groups one column", func(t *testing.T) {
		requests, err := buildTableValidationCopyRequests(
			&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9},
			&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 7, StartColumnIndex: 10, EndColumnIndex: 11},
			false,
			spans,
		)
		if err != nil {
			t.Fatalf("build requests: %v", err)
		}
		if len(requests) != 1 {
			t.Fatalf("requests = %#v", requests)
		}
		got := requests[0].SetDataValidation.Range
		if got.StartRowIndex != 1 || got.EndRowIndex != 7 || got.StartColumnIndex != 10 || got.EndColumnIndex != 11 {
			t.Fatalf("range = %#v", got)
		}
	})

	t.Run("small destination expands to source footprint", func(t *testing.T) {
		requests, err := buildTableValidationCopyRequests(
			&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9},
			&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 2, StartColumnIndex: 10, EndColumnIndex: 11},
			false,
			spans,
		)
		if err != nil {
			t.Fatalf("build requests: %v", err)
		}
		if len(requests) != 1 {
			t.Fatalf("requests = %#v", requests)
		}
		got := requests[0].SetDataValidation.Range
		if got.StartRowIndex != 1 || got.EndRowIndex != 4 || got.StartColumnIndex != 10 || got.EndColumnIndex != 11 {
			t.Fatalf("range = %#v", got)
		}
	})

	t.Run("non-multiple destination uses one source footprint", func(t *testing.T) {
		requests, err := buildTableValidationCopyRequests(
			&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9},
			&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 5, StartColumnIndex: 10, EndColumnIndex: 11},
			false,
			spans,
		)
		if err != nil {
			t.Fatalf("build requests: %v", err)
		}
		if len(requests) != 1 {
			t.Fatalf("requests = %#v", requests)
		}
		got := requests[0].SetDataValidation.Range
		if got.StartRowIndex != 1 || got.EndRowIndex != 4 {
			t.Fatalf("range = %#v", got)
		}
	})

	t.Run("header gaps remain gaps", func(t *testing.T) {
		requests, err := buildTableValidationCopyRequests(
			&sheets.GridRange{SheetId: 1, StartRowIndex: 0, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9},
			&sheets.GridRange{SheetId: 1, StartRowIndex: 0, EndRowIndex: 8, StartColumnIndex: 10, EndColumnIndex: 11},
			false,
			spans,
		)
		if err != nil {
			t.Fatalf("build requests: %v", err)
		}
		if len(requests) != 2 {
			t.Fatalf("requests = %#v", requests)
		}
		if requests[0].SetDataValidation.Range.StartRowIndex != 1 || requests[0].SetDataValidation.Range.EndRowIndex != 4 ||
			requests[1].SetDataValidation.Range.StartRowIndex != 5 || requests[1].SetDataValidation.Range.EndRowIndex != 8 {
			t.Fatalf("ranges = %#v, %#v", requests[0].SetDataValidation.Range, requests[1].SetDataValidation.Range)
		}
	})

	t.Run("transpose maps source column to destination row", func(t *testing.T) {
		requests, err := buildTableValidationCopyRequests(
			&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 10},
			&sheets.GridRange{SheetId: 2, StartRowIndex: 1, EndRowIndex: 3, StartColumnIndex: 10, EndColumnIndex: 13},
			true,
			spans,
		)
		if err != nil {
			t.Fatalf("build requests: %v", err)
		}
		if len(requests) != 1 {
			t.Fatalf("requests = %#v", requests)
		}
		got := requests[0].SetDataValidation.Range
		if got.SheetId != 2 || got.StartRowIndex != 1 || got.EndRowIndex != 2 ||
			got.StartColumnIndex != 10 || got.EndColumnIndex != 13 {
			t.Fatalf("range = %#v", got)
		}
	})
}

func TestBuildTableValidationCopyRequestsUpdatesDestinationTable(t *testing.T) {
	sourceColumns := []*sheets.TableColumnProperties{{
		ColumnIndex: 0,
		ColumnName:  "Source",
		ColumnType:  "DROPDOWN",
		DataValidationRule: &sheets.TableColumnDataValidationRule{
			Condition: &sheets.BooleanCondition{
				Type:   "ONE_OF_LIST",
				Values: []*sheets.ConditionValue{{UserEnteredValue: "blue"}},
			},
		},
	}}
	destinationColumns := []*sheets.TableColumnProperties{{
		ColumnIndex: 0,
		ColumnName:  "Destination",
		ColumnType:  "DROPDOWN",
		DataValidationRule: &sheets.TableColumnDataValidationRule{
			Condition: &sheets.BooleanCondition{
				Type:   "ONE_OF_LIST",
				Values: []*sheets.ConditionValue{{UserEnteredValue: "old"}},
			},
		},
	}}
	textDestinationColumns := []*sheets.TableColumnProperties{{
		ColumnIndex: 0,
		ColumnName:  "Text destination",
		ColumnType:  "TEXT",
	}}
	typedDestinationColumns := []*sheets.TableColumnProperties{{
		ColumnIndex: 0,
		ColumnName:  "Typed destination",
		ColumnType:  "DATE",
	}}
	spans := []tableValidationSpan{
		{
			SheetID:     1,
			TableID:     "source-table",
			ColumnIndex: 0,
			StartRow:    1,
			EndRow:      4,
			StartCol:    8,
			EndCol:      9,
			Columns:     sourceColumns,
			Rule: &sheets.DataValidationRule{
				Condition:    cloneBooleanCondition(sourceColumns[0].DataValidationRule.Condition),
				ShowCustomUi: true,
			},
		},
		{
			SheetID:     1,
			TableID:     "destination-table",
			ColumnIndex: 0,
			StartRow:    1,
			EndRow:      4,
			StartCol:    10,
			EndCol:      11,
			Columns:     destinationColumns,
			Rule: &sheets.DataValidationRule{
				Condition:    cloneBooleanCondition(destinationColumns[0].DataValidationRule.Condition),
				ShowCustomUi: true,
			},
		},
		{
			SheetID:     1,
			TableID:     "text-destination-table",
			ColumnIndex: 0,
			StartRow:    1,
			EndRow:      4,
			StartCol:    12,
			EndCol:      13,
			Columns:     textDestinationColumns,
		},
		{
			SheetID:     1,
			TableID:     "typed-destination-table",
			ColumnIndex: 0,
			StartRow:    1,
			EndRow:      4,
			StartCol:    14,
			EndCol:      15,
			Columns:     typedDestinationColumns,
		},
	}
	requests, err := buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 10, EndColumnIndex: 11},
		false,
		spans,
	)
	if err != nil {
		t.Fatalf("build requests: %v", err)
	}
	if len(requests) != 1 || requests[0].UpdateTable == nil {
		t.Fatalf("requests = %#v", requests)
	}
	updated := requests[0].UpdateTable.Table.ColumnProperties[0]
	if updated.DataValidationRule == nil ||
		updated.DataValidationRule.Condition.Values[0].UserEnteredValue != "blue" {
		t.Fatalf("updated destination = %#v", updated)
	}

	_, err = buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 2, StartColumnIndex: 8, EndColumnIndex: 9},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 2, StartColumnIndex: 10, EndColumnIndex: 11},
		false,
		spans,
	)
	if err == nil || !strings.Contains(err.Error(), "partially intersects") {
		t.Fatalf("expected different-rule partial destination error, got %v", err)
	}

	sameRuleSpans := append([]tableValidationSpan(nil), spans...)
	sameRuleSpans[1].Rule = spans[0].Rule
	requests, err = buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 2, StartColumnIndex: 8, EndColumnIndex: 9},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 2, StartColumnIndex: 10, EndColumnIndex: 11},
		false,
		sameRuleSpans,
	)
	if err != nil {
		t.Fatalf("same-rule partial destination: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("same-rule partial destination requests = %#v", requests)
	}

	requests, err = buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 12, EndColumnIndex: 13},
		false,
		spans,
	)
	if err != nil {
		t.Fatalf("copy dropdown into text table column: %v", err)
	}
	if len(requests) != 1 || requests[0].UpdateTable == nil {
		t.Fatalf("text destination requests = %#v", requests)
	}
	updated = requests[0].UpdateTable.Table.ColumnProperties[0]
	if updated.ColumnType != "DROPDOWN" || updated.DataValidationRule == nil ||
		updated.DataValidationRule.Condition.Values[0].UserEnteredValue != "blue" {
		t.Fatalf("updated text destination = %#v", updated)
	}

	requests, err = buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 12, EndColumnIndex: 13},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 10, EndColumnIndex: 11},
		false,
		spans,
	)
	if err != nil {
		t.Fatalf("copy text table column into dropdown table column: %v", err)
	}
	if len(requests) != 1 || requests[0].UpdateTable == nil {
		t.Fatalf("clear destination requests = %#v", requests)
	}
	updated = requests[0].UpdateTable.Table.ColumnProperties[0]
	if updated.ColumnType != "TEXT" || updated.DataValidationRule != nil {
		t.Fatalf("cleared destination dropdown = %#v", updated)
	}

	requests, err = buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 12, EndColumnIndex: 13},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 14, EndColumnIndex: 15},
		false,
		spans,
	)
	if err != nil {
		t.Fatalf("copy text table column into typed table column: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("typed destination requests = %#v", requests)
	}
}

func TestBuildTableValidationCopyRequestsLargeFillStaysGrouped(t *testing.T) {
	rule := &sheets.DataValidationRule{
		Condition:    &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
		ShowCustomUi: true,
	}
	requests, err := buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9},
		&sheets.GridRange{SheetId: 2, StartRowIndex: 1, EndRowIndex: 1_000_000, StartColumnIndex: 10, EndColumnIndex: 11},
		false,
		[]tableValidationSpan{{
			SheetID:  1,
			StartRow: 1,
			EndRow:   4,
			StartCol: 8,
			EndCol:   9,
			Rule:     rule,
		}},
	)
	if err != nil {
		t.Fatalf("build requests: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	got := requests[0].SetDataValidation.Range
	if got.StartRowIndex != 1 || got.EndRowIndex != 1_000_000 {
		t.Fatalf("range = %#v", got)
	}
}

func TestBuildTableValidationCopyRequestsMergesAdjacentSourceColumnsBeforeLimit(t *testing.T) {
	rule := &sheets.DataValidationRule{
		Condition:    &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
		ShowCustomUi: true,
	}
	requests, err := buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 10},
		&sheets.GridRange{SheetId: 2, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 10, EndColumnIndex: 1012},
		false,
		[]tableValidationSpan{
			{SheetID: 1, StartRow: 1, EndRow: 4, StartCol: 8, EndCol: 9, Rule: rule},
			{SheetID: 1, StartRow: 1, EndRow: 4, StartCol: 9, EndCol: 10, Rule: rule},
		},
	)
	if err != nil {
		t.Fatalf("build requests: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	got := requests[0].SetDataValidation.Range
	if got.StartColumnIndex != 10 || got.EndColumnIndex != 1012 {
		t.Fatalf("range = %#v", got)
	}
}

func TestBuildTableValidationCopyRequestsRejectsLargeSparseFill(t *testing.T) {
	rule := &sheets.DataValidationRule{
		Condition:    &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
		ShowCustomUi: true,
	}
	_, err := buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 0, EndRowIndex: 2, StartColumnIndex: 8, EndColumnIndex: 10},
		&sheets.GridRange{
			SheetId:          2,
			StartRowIndex:    0,
			EndRowIndex:      2 * (maxTableValidationCopySegments + 1),
			StartColumnIndex: 10,
			EndColumnIndex:   12,
		},
		false,
		[]tableValidationSpan{{
			SheetID:  1,
			StartRow: 1,
			EndRow:   2,
			StartCol: 8,
			EndCol:   9,
			Rule:     rule,
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "narrow the destination") {
		t.Fatalf("expected supplemental range limit error, got %v", err)
	}
}

func TestBuildTableValidationCopyRequestsRejectsOrdinarySourceIntoTable(t *testing.T) {
	source := &sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 0, EndColumnIndex: 1}
	destination := &sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9}
	spans := []tableValidationSpan{{
		SheetID:     1,
		TableID:     "destination-table",
		ColumnIndex: 0,
		StartRow:    1,
		EndRow:      4,
		StartCol:    8,
		EndCol:      9,
	}}
	_, err := buildTableValidationCopyRequests(
		source,
		destination,
		false,
		spans,
		tableValidationCopyOptions{
			ordinarySourceValidationKnown: true,
			ordinaryValidatedCells: []validationCellCoordinate{{
				Row: 1,
				Col: 0,
			}},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "ordinary cell validation") {
		t.Fatalf("expected ordinary-to-table rejection, got %v", err)
	}

	requests, err := buildTableValidationCopyRequests(
		source,
		destination,
		false,
		spans,
		tableValidationCopyOptions{
			ordinarySourceValidationKnown: true,
		},
	)
	if err != nil {
		t.Fatalf("ordinary no-validation source: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("ordinary no-validation requests = %#v", requests)
	}

	dropdownColumns := []*sheets.TableColumnProperties{{
		ColumnIndex: 0,
		ColumnName:  "Destination",
		ColumnType:  "DROPDOWN",
		DataValidationRule: &sheets.TableColumnDataValidationRule{
			Condition: &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
		},
	}}
	requests, err = buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 0, EndColumnIndex: 1},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 9},
		false,
		[]tableValidationSpan{{
			SheetID:     1,
			TableID:     "destination-table",
			ColumnIndex: 0,
			StartRow:    1,
			EndRow:      4,
			StartCol:    8,
			EndCol:      9,
			Columns:     dropdownColumns,
			Rule: &sheets.DataValidationRule{
				Condition:    &sheets.BooleanCondition{Type: "ONE_OF_LIST"},
				ShowCustomUi: true,
			},
		}},
		tableValidationCopyOptions{
			ordinarySourceValidationKnown: true,
		},
	)
	if err != nil {
		t.Fatalf("ordinary no-validation clears dropdown: %v", err)
	}
	if len(requests) != 1 || requests[0].UpdateTable == nil {
		t.Fatalf("ordinary no-validation dropdown requests = %#v", requests)
	}
	updated := requests[0].UpdateTable.Table.ColumnProperties[0]
	if updated.ColumnType != "TEXT" || updated.DataValidationRule != nil {
		t.Fatalf("ordinary no-validation dropdown update = %#v", updated)
	}

	requests, err = buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 0, EndColumnIndex: 2},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 10},
		false,
		[]tableValidationSpan{{
			SheetID:     1,
			TableID:     "destination-table",
			ColumnIndex: 0,
			StartRow:    1,
			EndRow:      4,
			StartCol:    9,
			EndCol:      10,
		}},
		tableValidationCopyOptions{
			ordinarySourceValidationKnown: true,
			ordinaryValidatedCells: []validationCellCoordinate{{
				Row: 1,
				Col: 0,
			}},
		},
	)
	if err != nil {
		t.Fatalf("validation outside mapped table cells: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("mapped ordinary validation requests = %#v", requests)
	}

	requests, err = buildTableValidationCopyRequests(
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 0, EndColumnIndex: 2},
		&sheets.GridRange{SheetId: 1, StartRowIndex: 1, EndRowIndex: 4, StartColumnIndex: 8, EndColumnIndex: 10},
		false,
		[]tableValidationSpan{
			{
				SheetID:  1,
				TableID:  "source-table",
				StartRow: 1,
				EndRow:   4,
				StartCol: 0,
				EndCol:   1,
			},
			{
				SheetID:     1,
				TableID:     "destination-table",
				ColumnIndex: 0,
				StartRow:    1,
				EndRow:      4,
				StartCol:    9,
				EndCol:      10,
			},
		},
		tableValidationCopyOptions{
			ordinarySourceValidationKnown: true,
		},
	)
	if err != nil {
		t.Fatalf("mixed table and ordinary source: %v", err)
	}
	if len(requests) != 1 || requests[0].SetDataValidation == nil {
		t.Fatalf("mixed source requests = %#v", requests)
	}
	if got := requests[0].SetDataValidation.Range; got.StartColumnIndex != 8 || got.EndColumnIndex != 9 {
		t.Fatalf("mixed source supplemental range = %#v", got)
	}
}

func TestResolveTableValidationCopyOptionsUsesEffectiveDestination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sheets":[]}`))
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
	opts, err := resolveTableValidationCopyOptions(
		context.Background(),
		svc,
		"s1",
		&sheets.GridRange{
			SheetId:          1,
			StartRowIndex:    1,
			EndRowIndex:      4,
			StartColumnIndex: 1,
			EndColumnIndex:   3,
		},
		&sheets.GridRange{
			SheetId:          1,
			StartRowIndex:    1,
			EndRowIndex:      2,
			StartColumnIndex: 23,
			EndColumnIndex:   24,
		},
		[]tableValidationSpan{{
			SheetID:     1,
			TableID:     "expanded-destination-table",
			ColumnIndex: 0,
			StartRow:    1,
			EndRow:      4,
			StartCol:    24,
			EndCol:      25,
		}},
		&spreadsheetRangeCatalog{
			SheetTitlesByID: map[int64]string{1: "Sheet1"},
		},
		false,
	)
	if err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	if !opts.ordinarySourceValidationKnown || len(opts.ordinaryValidatedCells) != 0 {
		t.Fatalf("options = %#v", opts)
	}
}

func TestPasteCarriesDataValidation(t *testing.T) {
	for _, pasteType := range []string{"PASTE_NORMAL", "PASTE_FORMAT", "PASTE_NO_BORDERS", "PASTE_DATA_VALIDATION"} {
		if !pasteCarriesDataValidation(pasteType) {
			t.Fatalf("%s should carry validation", pasteType)
		}
	}
	for _, pasteType := range []string{"PASTE_VALUES", "PASTE_FORMULA", "PASTE_CONDITIONAL_FORMATTING"} {
		if pasteCarriesDataValidation(pasteType) {
			t.Fatalf("%s should not carry validation", pasteType)
		}
	}
}

func TestSheetsCopyPasteTableValidationSupplement(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var got sheets.BatchUpdateSpreadsheetRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case path == "/spreadsheets/s1" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{{
					"properties": map[string]any{
						"sheetId": 1,
						"title":   "Sheet1",
						"gridProperties": map[string]any{
							"rowCount":    1000,
							"columnCount": 26,
						},
					},
					"tables": []map[string]any{{
						"range": map[string]any{
							"sheetId":          1,
							"startRowIndex":    0,
							"endRowIndex":      4,
							"startColumnIndex": 8,
							"endColumnIndex":   10,
						},
						"columnProperties": []map[string]any{{
							"columnIndex": 0,
							"columnType":  "DROPDOWN",
							"dataValidationRule": map[string]any{
								"condition": map[string]any{
									"type": "ONE_OF_LIST",
									"values": []map[string]any{
										{"userEnteredValue": "red"},
										{"userEnteredValue": "green"},
									},
								},
							},
						}},
					}},
				}},
			})
		case path == "/spreadsheets/s1:batchUpdate" && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode request: %v", err)
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
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	err = runKong(t, &SheetsValidationSetCmd{}, []string{
		"s1", "Sheet1!I2:I4",
		"--type", "ONE_OF_LIST",
		"--value", "blue",
		"--value", "yellow",
		"--filtered-rows-included",
	}, ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("table validation set: %v", err)
	}
	if len(got.Requests) != 1 || got.Requests[0].UpdateTable == nil || got.Requests[0].SetDataValidation != nil {
		t.Fatalf("table set requests = %#v", got.Requests)
	}

	got = sheets.BatchUpdateSpreadsheetRequest{}
	err = runKong(t, &SheetsValidationSetCmd{}, []string{
		"s1", "Sheet1!I2:I4",
		"--type", "NUMBER_BETWEEN",
		"--value", "1",
		"--value", "10",
		"--filtered-rows-included",
	}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "only supports ONE_OF_LIST") {
		t.Fatalf("expected table number validation rejection, got %v", err)
	}
	if len(got.Requests) != 0 {
		t.Fatalf("table non-list set requests = %#v", got.Requests)
	}

	got = sheets.BatchUpdateSpreadsheetRequest{}
	if err := runKong(t, &SheetsCopyPasteCmd{}, []string{
		"s1", "Sheet1!I2:I4", "Sheet1!K2:K4", "--type", "DATA_VALIDATION",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("copy paste: %v", err)
	}
	if len(got.Requests) != 2 || got.Requests[0].CopyPaste == nil || got.Requests[1].SetDataValidation == nil {
		t.Fatalf("requests = %#v", got.Requests)
	}
	supplement := got.Requests[1].SetDataValidation
	if supplement.Range.StartRowIndex != 1 || supplement.Range.EndRowIndex != 4 ||
		supplement.Range.StartColumnIndex != 10 || supplement.Range.EndColumnIndex != 11 {
		t.Fatalf("supplement range = %#v", supplement.Range)
	}
	if supplement.Rule == nil || supplement.Rule.Condition == nil || supplement.Rule.Condition.Type != "ONE_OF_LIST" {
		t.Fatalf("supplement rule = %#v", supplement.Rule)
	}
	if !supplement.FilteredRowsIncluded {
		t.Fatalf("supplement should include filtered rows: %#v", supplement)
	}

	got = sheets.BatchUpdateSpreadsheetRequest{}
	if err := runKong(t, &SheetsCopyPasteCmd{}, []string{
		"s1", "Sheet1!I2:I4", "Sheet1!K2:K4", "--type", "FORMAT",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("format copy paste: %v", err)
	}
	if len(got.Requests) != 2 || got.Requests[0].CopyPaste == nil ||
		got.Requests[0].CopyPaste.PasteType != "PASTE_FORMAT" ||
		got.Requests[1].SetDataValidation == nil {
		t.Fatalf("format requests = %#v", got.Requests)
	}

	got = sheets.BatchUpdateSpreadsheetRequest{}
	if err := copyDataValidation(ctx, svc, "s1", "Sheet1!I2:I4", "Sheet1!K2:K4"); err != nil {
		t.Fatalf("copyDataValidation: %v", err)
	}
	if len(got.Requests) != 2 || got.Requests[0].CopyPaste == nil || got.Requests[1].SetDataValidation == nil {
		t.Fatalf("copyDataValidation requests = %#v", got.Requests)
	}
}

func newSheetsValidationTestContext(
	t *testing.T,
	includeGridData bool,
) (context.Context, *RootFlags, *[]sheets.BatchUpdateSpreadsheetRequest, *[]string, func()) {
	t.Helper()

	origNew := newSheetsService
	requests := []sheets.BatchUpdateSpreadsheetRequest{}
	rawRequests := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case path == "/spreadsheets/s1" && r.Method == http.MethodGet && includeGridData:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{{
					"properties": map[string]any{"sheetId": 0, "title": "Sheet1"},
					"data": []map[string]any{{
						"startRow":    1,
						"startColumn": 2,
						"rowData": []map[string]any{{
							"values": []map[string]any{{
								"dataValidation": map[string]any{
									"condition": map[string]any{
										"type": "ONE_OF_LIST",
										"values": []map[string]any{
											{"userEnteredValue": "red"},
											{"userEnteredValue": "green"},
										},
									},
									"inputMessage": "Pick one",
									"showCustomUi": true,
									"strict":       true,
								},
							}},
						}},
					}},
				}},
			})
		case path == "/spreadsheets/s1" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{{
					"properties": map[string]any{"sheetId": 0, "title": "Sheet1", "index": 0},
				}},
				"namedRanges": []map[string]any{{
					"namedRangeId": "nr1",
					"name":         "AllowedColors",
					"range": map[string]any{
						"sheetId":          0,
						"startRowIndex":    1,
						"endRowIndex":      5,
						"startColumnIndex": 0,
						"endColumnIndex":   1,
					},
				}},
			})
		case path == "/spreadsheets/s1:batchUpdate" && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request: %v", err)
			}
			rawRequests = append(rawRequests, string(body))
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			requests = append(requests, req)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "a@b.com"}
	cleanup := func() {
		newSheetsService = origNew
		srv.Close()
	}
	return ctx, flags, &requests, &rawRequests, cleanup
}
