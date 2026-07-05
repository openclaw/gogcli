package sheetsconditional

import (
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"
)

func TestBuildCondition(t *testing.T) {
	conditionType, values, err := BuildCondition("text-contains", "done")
	if err != nil {
		t.Fatalf("BuildCondition() error = %v", err)
	}

	if conditionType != "TEXT_CONTAINS" ||
		len(values) != 1 ||
		values[0].UserEnteredValue != "done" {
		t.Fatalf("condition = %q, values = %#v", conditionType, values)
	}

	conditionType, values, err = BuildCondition("blank", "")
	if err != nil || conditionType != "BLANK" || values != nil {
		t.Fatalf("blank condition = %q, values = %#v, error = %v", conditionType, values, err)
	}

	for _, test := range []struct {
		kind       string
		expression string
		want       string
	}{
		{kind: "blank", expression: "x", want: "not used"},
		{kind: "text-eq", want: "required"},
		{kind: "unknown", want: "unsupported"},
	} {
		if _, _, gotErr := BuildCondition(test.kind, test.expression); gotErr == nil ||
			!strings.Contains(gotErr.Error(), test.want) {
			t.Fatalf("BuildCondition(%q) error = %v", test.kind, gotErr)
		}
	}
}

func TestBuildAddRequest(t *testing.T) {
	gridRange := &sheets.GridRange{SheetId: 7, EndRowIndex: 4, EndColumnIndex: 2}
	format := &sheets.CellFormat{TextFormat: &sheets.TextFormat{Bold: true}}
	request := BuildAddRequest(
		gridRange,
		"TEXT_EQ",
		[]*sheets.ConditionValue{{UserEnteredValue: "done"}},
		format,
		2,
	)

	add := request.AddConditionalFormatRule
	if add == nil ||
		add.Index != 2 ||
		add.Rule == nil ||
		add.Rule.BooleanRule == nil ||
		add.Rule.BooleanRule.Condition.Type != "TEXT_EQ" ||
		add.Rule.BooleanRule.Format != format ||
		len(add.Rule.Ranges) != 1 ||
		add.Rule.Ranges[0] != gridRange {
		t.Fatalf("request = %#v", request)
	}
}

func TestBuildGradientAddRequest(t *testing.T) {
	gridRange := &sheets.GridRange{SheetId: 7, EndRowIndex: 4, EndColumnIndex: 2}
	gradientRule := &sheets.GradientRule{
		Minpoint: &sheets.InterpolationPoint{Type: "MIN"},
		Maxpoint: &sheets.InterpolationPoint{Type: "MAX"},
	}
	request := BuildGradientAddRequest(gridRange, gradientRule, 3)

	add := request.AddConditionalFormatRule
	if add == nil ||
		add.Index != 3 ||
		add.Rule == nil ||
		add.Rule.BooleanRule != nil ||
		add.Rule.GradientRule != gradientRule ||
		len(add.Rule.Ranges) != 1 ||
		add.Rule.Ranges[0] != gridRange {
		t.Fatalf("request = %#v", request)
	}
}

func TestRuleItems(t *testing.T) {
	spreadsheet := &sheets.Spreadsheet{
		Sheets: []*sheets.Sheet{{
			Properties: &sheets.SheetProperties{SheetId: 7, Title: "Data Set"},
			ConditionalFormats: []*sheets.ConditionalFormatRule{{
				Ranges: []*sheets.GridRange{{SheetId: 7, EndRowIndex: 2, EndColumnIndex: 1}},
				BooleanRule: &sheets.BooleanRule{
					Condition: &sheets.BooleanCondition{
						Type:   "TEXT_EQ",
						Values: []*sheets.ConditionValue{{UserEnteredValue: "done"}},
					},
				},
			}, {
				Ranges: []*sheets.GridRange{{SheetId: 7, StartColumnIndex: 1, EndColumnIndex: 2}},
				GradientRule: &sheets.GradientRule{
					Minpoint: &sheets.InterpolationPoint{Type: "MIN"},
					Maxpoint: &sheets.InterpolationPoint{Type: "MAX"},
				},
			}},
		}},
	}

	items := RuleItems(spreadsheet, "")
	if len(items) != 2 ||
		items[0].SheetTitle != "Data Set" ||
		items[0].Type != "TEXT_EQ" ||
		len(items[0].Values) != 1 ||
		items[0].Ranges[0] != "'Data Set'!A1:A2" {
		t.Fatalf("items = %#v", items)
	}

	if items[1].Type != "GRADIENT_RULE" || items[1].Ranges[0] != "'Data Set'!B:B" {
		t.Fatalf("gradient item = %#v", items[1])
	}
}

func TestDeletePlanning(t *testing.T) {
	requests, err := BuildDeleteRequests(7, 3, "", true)
	if err != nil {
		t.Fatalf("BuildDeleteRequests() error = %v", err)
	}

	if len(requests) != 3 ||
		requests[0].DeleteConditionalFormatRule.Index != 2 ||
		requests[2].DeleteConditionalFormatRule.Index != 0 {
		t.Fatalf("requests = %#v", requests)
	}

	if _, err := BuildDeleteRequests(7, 2, "2", false); err == nil ||
		!strings.Contains(err.Error(), "out of range") {
		t.Fatalf("out-of-range error = %v", err)
	}

	if err := ValidateClearIndex("-1"); err == nil {
		t.Fatal("expected invalid index")
	}
}

func TestSheetRuleCount(t *testing.T) {
	spreadsheet := &sheets.Spreadsheet{
		Sheets: []*sheets.Sheet{{
			Properties:         &sheets.SheetProperties{SheetId: 7, Title: "Data"},
			ConditionalFormats: make([]*sheets.ConditionalFormatRule, 2),
		}},
	}

	sheetID, count, err := SheetRuleCount(spreadsheet, "Data")
	if err != nil || sheetID != 7 || count != 2 {
		t.Fatalf("sheetID = %d, count = %d, error = %v", sheetID, count, err)
	}
}
