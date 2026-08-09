package cmd

import (
	"encoding/json"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsbanding"
	"github.com/openclaw/gogcli/internal/sheetsconditional"
)

type sheetsChartItem struct {
	ChartID    int64  `json:"chartId"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	SheetID    int64  `json:"sheetId"`
	SheetTitle string `json:"sheetTitle"`
}

type sheetsCellLink struct {
	Sheet string `json:"sheet"`
	A1    string `json:"a1"`
	Row   int    `json:"row"`
	Col   int    `json:"col"`
	Value string `json:"value"`
	Link  string `json:"link"`
}

type sheetsCellNote struct {
	Sheet string `json:"sheet"`
	A1    string `json:"a1"`
	Row   int    `json:"row"`
	Col   int    `json:"col"`
	Value string `json:"value"`
	Note  string `json:"note"`
}

func sheetsMetadataColumns() []outfmt.Column[*sheets.Sheet] {
	return []outfmt.Column[*sheets.Sheet]{
		{Header: "ID", Value: func(sheet *sheets.Sheet) string {
			return strconv.FormatInt(sheetsProperties(sheet).SheetId, 10)
		}},
		{Header: "TITLE", Value: func(sheet *sheets.Sheet) string {
			return sheetsProperties(sheet).Title
		}},
		{Header: "ROWS", Value: func(sheet *sheets.Sheet) string {
			return strconv.FormatInt(sheetsGridProperties(sheet).RowCount, 10)
		}},
		{Header: "COLS", Value: func(sheet *sheets.Sheet) string {
			return strconv.FormatInt(sheetsGridProperties(sheet).ColumnCount, 10)
		}},
	}
}

func sheetsChartColumns() []outfmt.Column[sheetsChartItem] {
	return []outfmt.Column[sheetsChartItem]{
		{Header: "CHART_ID", Value: func(item sheetsChartItem) string { return strconv.FormatInt(item.ChartID, 10) }},
		{Header: "TITLE", Value: func(item sheetsChartItem) string { return item.Title }},
		{Header: "TYPE", Value: func(item sheetsChartItem) string { return item.Type }},
		{Header: "SHEET_ID", Value: func(item sheetsChartItem) string { return strconv.FormatInt(item.SheetID, 10) }},
		{Header: "SHEET_TITLE", Value: func(item sheetsChartItem) string { return item.SheetTitle }},
	}
}

func sheetsConditionalColumns() []outfmt.Column[sheetsconditional.RuleItem] {
	return []outfmt.Column[sheetsconditional.RuleItem]{
		{Header: "SHEET", Value: func(item sheetsconditional.RuleItem) string { return item.SheetTitle }},
		{Header: "INDEX", Value: func(item sheetsconditional.RuleItem) string { return strconv.Itoa(item.Index) }},
		{Header: "TYPE", Value: func(item sheetsconditional.RuleItem) string { return item.Type }},
		{Header: "RANGES", Value: func(item sheetsconditional.RuleItem) string { return strings.Join(item.Ranges, ",") }},
	}
}

func sheetsBandingColumns() []outfmt.Column[sheetsbanding.Item] {
	return []outfmt.Column[sheetsbanding.Item]{
		{Header: "BANDED_RANGE_ID", Value: func(item sheetsbanding.Item) string {
			return strconv.FormatInt(item.BandedRangeID, 10)
		}},
		{Header: "SHEET", Value: func(item sheetsbanding.Item) string { return item.SheetTitle }},
		{Header: "RANGE", Value: func(item sheetsbanding.Item) string { return item.A1 }},
	}
}

func sheetsLinkColumns() []outfmt.Column[sheetsCellLink] {
	return []outfmt.Column[sheetsCellLink]{
		{Header: "A1", Value: func(item sheetsCellLink) string { return oneLine(item.A1) }},
		{Header: "VALUE", Value: func(item sheetsCellLink) string { return oneLine(item.Value) }},
		{Header: "LINK", Value: func(item sheetsCellLink) string { return oneLine(item.Link) }},
	}
}

func sheetsNoteColumns() []outfmt.Column[sheetsCellNote] {
	return []outfmt.Column[sheetsCellNote]{
		{Header: "A1", Value: func(item sheetsCellNote) string { return oneLine(item.A1) }},
		{Header: "VALUE", Value: func(item sheetsCellNote) string { return oneLine(item.Value) }},
		{Header: "NOTE", Value: func(item sheetsCellNote) string { return oneLine(item.Note) }},
	}
}

func sheetsCellFormatColumns() []outfmt.Column[sheetsCellFormat] {
	return []outfmt.Column[sheetsCellFormat]{
		{Header: "A1", Value: func(item sheetsCellFormat) string { return oneLine(item.A1) }},
		{Header: "VALUE", Value: func(item sheetsCellFormat) string { return oneLine(item.Value) }},
		{Header: "FORMAT", Value: func(item sheetsCellFormat) string { return sheetsFormatJSON(item.Format) }},
	}
}

func sheetsNamedRangeColumns() []outfmt.Column[namedRangeItem] {
	return []outfmt.Column[namedRangeItem]{
		{Header: "NAME", Value: func(item namedRangeItem) string { return item.Name }},
		{Header: "ID", Value: func(item namedRangeItem) string { return item.NamedRangeID }},
		{Header: "SHEET_ID", Value: func(item namedRangeItem) string { return strconv.FormatInt(item.SheetID, 10) }},
		{Header: "SHEET_TITLE", Value: func(item namedRangeItem) string { return item.SheetTitle }},
		{Header: "START_ROW", Value: func(item namedRangeItem) string { return strconv.FormatInt(item.StartRowIndex, 10) }},
		{Header: "END_ROW", Value: func(item namedRangeItem) string { return strconv.FormatInt(item.EndRowIndex, 10) }},
		{Header: "START_COL", Value: func(item namedRangeItem) string { return strconv.FormatInt(item.StartColIndex, 10) }},
		{Header: "END_COL", Value: func(item namedRangeItem) string { return strconv.FormatInt(item.EndColIndex, 10) }},
		{Header: "A1", Value: func(item namedRangeItem) string { return item.A1 }},
	}
}

func sheetsTableColumns() []outfmt.Column[sheetsTableItem] {
	return []outfmt.Column[sheetsTableItem]{
		{Header: "NAME", Value: func(item sheetsTableItem) string { return item.Name }},
		{Header: "TABLE_ID", Value: func(item sheetsTableItem) string { return item.TableID }},
		{Header: "SHEET_ID", Value: func(item sheetsTableItem) string { return strconv.FormatInt(item.SheetID, 10) }},
		{Header: "SHEET_TITLE", Value: func(item sheetsTableItem) string { return item.SheetTitle }},
		{Header: "A1", Value: func(item sheetsTableItem) string { return item.A1 }},
		{Header: "COLUMNS", Value: func(item sheetsTableItem) string { return strconv.Itoa(len(item.Columns)) }},
	}
}

func sheetsValidationColumns() []outfmt.Column[sheetsCellValidation] {
	return []outfmt.Column[sheetsCellValidation]{
		{Header: "A1", Value: func(item sheetsCellValidation) string { return oneLine(item.A1) }},
		{Header: "TYPE", Value: func(item sheetsCellValidation) string {
			if item.Rule == nil || item.Rule.Condition == nil {
				return ""
			}
			return oneLine(item.Rule.Condition.Type)
		}},
		{Header: "VALUES", Value: func(item sheetsCellValidation) string {
			values := make([]string, 0)
			if item.Rule != nil && item.Rule.Condition != nil {
				for _, value := range item.Rule.Condition.Values {
					if value != nil {
						values = append(values, value.UserEnteredValue)
					}
				}
			}
			encoded, _ := json.Marshal(values)
			return string(encoded)
		}},
		{Header: "STRICT", Value: func(item sheetsCellValidation) string {
			return strconv.FormatBool(item.Rule != nil && item.Rule.Strict)
		}},
		{Header: "SHOW_CUSTOM_UI", Value: func(item sheetsCellValidation) string {
			return strconv.FormatBool(item.Rule != nil && item.Rule.ShowCustomUi)
		}},
		{Header: "INPUT_MESSAGE", Value: func(item sheetsCellValidation) string {
			return oneLine(validationInputMessage(item.Rule))
		}},
	}
}

func sheetsProperties(sheet *sheets.Sheet) *sheets.SheetProperties {
	if sheet == nil || sheet.Properties == nil {
		return &sheets.SheetProperties{}
	}
	return sheet.Properties
}

func sheetsGridProperties(sheet *sheets.Sheet) *sheets.GridProperties {
	properties := sheetsProperties(sheet)
	if properties.GridProperties == nil {
		return &sheets.GridProperties{}
	}
	return properties.GridProperties
}

func sheetsFormatJSON(format *sheets.CellFormat) string {
	encoded, err := json.Marshal(format)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
