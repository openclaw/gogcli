package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type SheetsNumberFormatCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
	Type          string `name:"type" help:"Number format type: NUMBER, CURRENCY, PERCENT, DATE, TIME, DATE_TIME, SCIENTIFIC, TEXT" default:"NUMBER"`
	Pattern       string `name:"pattern" help:"Custom number format pattern (eg. $#,##0.00 or yyyy-mm-dd)"`
}

func (c *SheetsNumberFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	numberType, err := normalizeNumberFormatType(c.Type)
	if err != nil {
		return err
	}
	pattern := strings.TrimSpace(c.Pattern)

	rangeInfo, err := parseSheetRange(rangeSpec, "number-format")
	if err != nil {
		return err
	}

	return runSheetsMutation(ctx, flags, "sheets.number-format", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"type":           numberType,
		"pattern":        pattern,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
		if err != nil {
			return nil, "", err
		}
		gridRange, err := gridRangeFromMap(rangeInfo, sheetIDs, "number-format")
		if err != nil {
			return nil, "", err
		}
		numberFormat := &sheets.NumberFormat{Type: numberType}
		if pattern != "" {
			numberFormat.Pattern = pattern
		}
		req := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				RepeatCell: &sheets.RepeatCellRequest{
					Range: gridRange,
					Cell: &sheets.CellData{
						UserEnteredFormat: &sheets.CellFormat{NumberFormat: numberFormat},
					},
					Fields: "userEnteredFormat.numberFormat",
				},
			}},
		}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		text := fmt.Sprintf("Applied number format %s (%s) to %s", numberType, pattern, rangeSpec)
		if pattern == "" {
			text = fmt.Sprintf("Applied number format %s to %s", numberType, rangeSpec)
		}
		return map[string]any{
			"range":   rangeSpec,
			"type":    numberType,
			"pattern": pattern,
		}, text, nil
	})
}

func normalizeNumberFormatType(raw string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(raw))
	if v == "" {
		v = "NUMBER"
	}
	switch v {
	case "NUMBER", "CURRENCY", "PERCENT", "DATE", "TIME", "DATE_TIME", "SCIENTIFIC", "TEXT":
		return v, nil
	default:
		return "", fmt.Errorf("invalid --type %q (expected NUMBER, CURRENCY, PERCENT, DATE, TIME, DATE_TIME, SCIENTIFIC, or TEXT)", raw)
	}
}
