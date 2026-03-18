package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsFormatCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (A1 notation with sheet name, or named range name; e.g. Sheet1!A1:B2 or MyNamedRange)"`
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

	if hasBoardersTypo(formatFields) {
		return fmt.Errorf(`invalid --format-fields: found "boarders"; use "borders"`)
	}

	var err error
	var format sheets.CellFormat
	b, err := resolveInlineOrFileBytes(c.FormatJSON)
	if err != nil {
		return fmt.Errorf("read --format-json: %w", err)
	}
	if err = decodeCellFormatJSON(b, &format); err != nil {
		return fmt.Errorf("invalid format JSON: %w", err)
	}

	normalizedFields, formatJSONPaths := normalizeFormatMask(formatFields)
	if normalizedFields != "" {
		formatFields = normalizedFields
	}
	if err = applyForceSendFields(&format, formatJSONPaths); err != nil {
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

	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	gridRange, err := resolveGridRangeWithCatalog(rangeSpec, catalog, "format")
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

func decodeCellFormatJSON(data []byte, dst *sheets.CellFormat) error {
	if dst == nil {
		return fmt.Errorf("format is required")
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func hasBoardersTypo(mask string) bool {
	for _, part := range splitFieldMask(mask) {
		for _, token := range strings.Split(part, ".") {
			if strings.EqualFold(strings.TrimSpace(token), "boarders") {
				return true
			}
		}
	}
	return false
}
