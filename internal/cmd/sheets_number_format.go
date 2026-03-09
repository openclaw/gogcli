package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsNumberFormatCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
	Type          string `name:"type" help:"Number format type: NUMBER, CURRENCY, PERCENT, DATE, TIME, DATE_TIME, SCIENTIFIC, TEXT" default:"NUMBER"`
	Pattern       string `name:"pattern" help:"Custom number format pattern (eg. $#,##0.00 or yyyy-mm-dd)"`
}

func (c *SheetsNumberFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

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

	if dryRunErr := dryRunExit(ctx, flags, "sheets.number-format", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"type":           numberType,
		"pattern":        pattern,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	gridRange, err := gridRangeFromMap(rangeInfo, sheetIDs, "number-format")
	if err != nil {
		return err
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

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"range":   rangeSpec,
			"type":    numberType,
			"pattern": pattern,
		})
	}

	if pattern == "" {
		u.Out().Printf("Applied number format %s to %s", numberType, rangeSpec)
		return nil
	}
	u.Out().Printf("Applied number format %s (%s) to %s", numberType, pattern, rangeSpec)
	return nil
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
