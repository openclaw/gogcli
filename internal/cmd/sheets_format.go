package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsFormatCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
	FormatJSON    string `name:"format-json" help:"Cell format as JSON (Sheets API CellFormat)"`
	FormatFields  string `name:"format-fields" help:"Format field mask (eg. userEnteredFormat.textFormat.bold or textFormat.bold)"`
}

func (c *SheetsFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}
	if strings.TrimSpace(c.FormatJSON) == "" {
		return fmt.Errorf("provide format JSON via --format-json")
	}
	formatFields := strings.TrimSpace(c.FormatFields)
	if formatFields == "" {
		return fmt.Errorf("provide format fields via --format-fields")
	}

	var err error
	var format sheets.CellFormat
	b, err := resolveInlineOrFileBytes(c.FormatJSON)
	if err != nil {
		return fmt.Errorf("read --format-json: %w", err)
	}
	if err = json.Unmarshal(b, &format); err != nil {
		return fmt.Errorf("invalid format JSON: %w", err)
	}

	normalizedFields, formatJSONPaths := normalizeFormatMask(formatFields)
	if normalizedFields != "" {
		formatFields = normalizedFields
	}
	if err = applyForceSendFields(&format, formatJSONPaths); err != nil {
		return err
	}

	rangeInfo, err := parseSheetRange(rangeSpec, "format")
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.format", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"fields":         formatFields,
		"format":         format,
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
	gridRange, err := gridRangeFromMap(rangeInfo, sheetIDs, "format")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				RepeatCell: &sheets.RepeatCellRequest{
					Range: gridRange,
					Cell: &sheets.CellData{
						UserEnteredFormat: &format,
					},
					Fields: formatFields,
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"range":  rangeSpec,
			"fields": formatFields,
		})
	}

	u.Out().Printf("Formatted %s", rangeSpec)
	return nil
}
