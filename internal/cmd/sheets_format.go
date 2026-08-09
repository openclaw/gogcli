package cmd

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsformat"
	"github.com/openclaw/gogcli/internal/ui"
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
		return usage("provide format JSON via --format-json")
	}

	var format sheets.CellFormat
	b, err := resolveInlineOrFileBytes(c.FormatJSON, stdinReader(ctx))
	if err != nil {
		return usagef("read --format-json: %v", err)
	}
	if err = sheetsformat.Decode(b, &format); err != nil {
		return usagef("invalid format JSON: %v", err)
	}

	formatFields := strings.TrimSpace(c.FormatFields)
	var formatJSONPaths []string
	if formatFields == "" {
		var inferErr error
		formatFields, formatJSONPaths, inferErr = sheetsformat.InferMask(b)
		if inferErr != nil {
			return sheetsFormatInputError(inferErr)
		}
	} else {
		if sheetsformat.HasBordersTypo(formatFields) {
			return usage(`invalid --format-fields: found "boarders"; use "borders"`)
		}
		normalizedFields, paths := sheetsformat.NormalizeMask(formatFields)
		if normalizedFields != "" {
			formatFields = normalizedFields
		}
		formatJSONPaths = paths
	}
	if err = sheetsformat.ApplyForceSendFields(&format, formatJSONPaths); err != nil {
		return usage(err.Error())
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

	svc, err := sheetsService(ctx, account)
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"range":  rangeSpec,
			"fields": formatFields,
		})
	}

	u.Out().Linef("Formatted %s", rangeSpec)
	return nil
}

func sheetsFormatInputError(err error) error {
	var validationErr sheetsformat.ValidationError
	if errors.As(err, &validationErr) {
		return usage(validationErr.Error())
	}
	return err
}
