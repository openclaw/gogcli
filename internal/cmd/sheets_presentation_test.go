package cmd

import (
	"fmt"
	"testing"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/sheetsbanding"
	"github.com/openclaw/gogcli/internal/sheetsconditional"
)

func TestSheetsPresentationSchemas(t *testing.T) {
	t.Parallel()

	t.Run("metadata", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*sheets.Sheet{{
			Properties: &sheets.SheetProperties{
				SheetId: 7,
				Title:   "Data",
				GridProperties: &sheets.GridProperties{
					RowCount:    20,
					ColumnCount: 8,
				},
			},
		}}, sheetsMetadataColumns())
		assertTableOutput(t, got, "ID\tTITLE\tROWS\tCOLS\n7\tData\t20\t8\n")
	})

	t.Run("charts", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []sheetsChartItem{{
			ChartID:    10,
			Title:      "Revenue",
			Type:       "COLUMN",
			SheetID:    7,
			SheetTitle: "Data",
		}}, sheetsChartColumns())
		assertTableOutput(t, got, "CHART_ID\tTITLE\tTYPE\tSHEET_ID\tSHEET_TITLE\n10\tRevenue\tCOLUMN\t7\tData\n")
	})

	t.Run("conditional", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []sheetsconditional.RuleItem{{
			SheetTitle: "Data",
			Index:      2,
			Type:       "TEXT_EQ",
			Ranges:     []string{"Data!A1", "Data!C1"},
		}}, sheetsConditionalColumns())
		assertTableOutput(t, got, "SHEET\tINDEX\tTYPE\tRANGES\nData\t2\tTEXT_EQ\tData!A1,Data!C1\n")
	})

	t.Run("banding", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []sheetsbanding.Item{{
			BandedRangeID: 99,
			SheetTitle:    "Data",
			A1:            "Data!A1:C5",
		}}, sheetsBandingColumns())
		assertTableOutput(t, got, "BANDED_RANGE_ID\tSHEET\tRANGE\n99\tData\tData!A1:C5\n")
	})

	t.Run("links", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []sheetsCellLink{{
			A1:    "Data!A1",
			Value: "line 1\nline 2",
			Link:  "https://example.com/\tpath",
		}}, sheetsLinkColumns())
		assertTableOutput(t, got, "A1\tVALUE\tLINK\nData!A1\tline 1\\nline 2\thttps://example.com/ path\n")
	})

	t.Run("notes", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []sheetsCellNote{{
			A1:    "Data!B2",
			Value: "value",
			Note:  "first\r\nsecond",
		}}, sheetsNoteColumns())
		assertTableOutput(t, got, "A1\tVALUE\tNOTE\nData!B2\tvalue\tfirst\\nsecond\n")
	})

	t.Run("formats", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []sheetsCellFormat{{
			A1:    "Data!C3",
			Value: "header",
			Format: &sheets.CellFormat{
				TextFormat: &sheets.TextFormat{Bold: true},
			},
		}}, sheetsCellFormatColumns())
		assertTableOutput(t, got, "A1\tVALUE\tFORMAT\nData!C3\theader\t{\"textFormat\":{\"bold\":true}}\n")
	})

	t.Run("named ranges", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []namedRangeItem{{
			Name:          "Input",
			NamedRangeID:  "nr1",
			SheetID:       7,
			SheetTitle:    "Data",
			StartRowIndex: 1,
			EndRowIndex:   4,
			StartColIndex: 2,
			EndColIndex:   5,
			A1:            "Data!C2:E4",
		}}, sheetsNamedRangeColumns())
		assertTableOutput(
			t,
			got,
			"NAME\tID\tSHEET_ID\tSHEET_TITLE\tSTART_ROW\tEND_ROW\tSTART_COL\tEND_COL\tA1\n"+
				"Input\tnr1\t7\tData\t1\t4\t2\t5\tData!C2:E4\n",
		)
	})

	t.Run("tables", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []sheetsTableItem{{
			Name:       "Tasks",
			TableID:    "tbl1",
			SheetID:    7,
			SheetTitle: "Data",
			A1:         "Data!A1:C4",
			Columns: []sheetsTableColumnItem{
				{ColumnIndex: 0, ColumnName: "Task", ColumnType: "TEXT"},
				{ColumnIndex: 1, ColumnName: "Done", ColumnType: "BOOLEAN"},
			},
		}}, sheetsTableColumns())
		assertTableOutput(t, got, "NAME\tTABLE_ID\tSHEET_ID\tSHEET_TITLE\tA1\tCOLUMNS\nTasks\ttbl1\t7\tData\tData!A1:C4\t2\n")
	})

	t.Run("validation", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []sheetsCellValidation{{
			A1: "Data!D2",
			Rule: &sheets.DataValidationRule{
				Condition: &sheets.BooleanCondition{
					Type: "ONE_OF_LIST",
					Values: []*sheets.ConditionValue{
						{UserEnteredValue: "red"},
						{UserEnteredValue: "green"},
					},
				},
				Strict:       true,
				ShowCustomUi: true,
				InputMessage: "Pick\tone",
			},
		}}, sheetsValidationColumns())
		assertTableOutput(
			t,
			got,
			fmt.Sprintf(
				"A1\tTYPE\tVALUES\tSTRICT\tSHOW_CUSTOM_UI\tINPUT_MESSAGE\n"+
					"Data!D2\tONE_OF_LIST\t[\"red\",\"green\"]\t%t\t%t\tPick one\n",
				true,
				true,
			),
		)
	})
}
