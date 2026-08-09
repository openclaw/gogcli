package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/sheetsdimension"
)

type SheetsResizeColumnsCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Columns       string `arg:"" name:"columns" help:"Columns range (eg. Sheet1!A:C)"`
	Width         int64  `name:"width" help:"Column width in pixels"`
	Auto          bool   `name:"auto" help:"Auto-fit columns to content"`
}

func (c *SheetsResizeColumnsCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runSheetsResize(ctx, flags, c.SpreadsheetID, c.Columns, c.Width, c.Auto, sheetsResizeAxis{
		op:        "sheets.resize-columns",
		label:     "columns",
		sizeLabel: "width",
		dimension: sheetsdimension.Columns,
		parse:     sheetsdimension.ParseColumns,
	})
}

type SheetsResizeRowsCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Rows          string `arg:"" name:"rows" help:"Rows range (eg. Sheet1!1:10)"`
	Height        int64  `name:"height" help:"Row height in pixels"`
	Auto          bool   `name:"auto" help:"Auto-fit rows to content"`
}

func (c *SheetsResizeRowsCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runSheetsResize(ctx, flags, c.SpreadsheetID, c.Rows, c.Height, c.Auto, sheetsResizeAxis{
		op:        "sheets.resize-rows",
		label:     "rows",
		sizeLabel: "height",
		dimension: sheetsdimension.Rows,
		parse:     sheetsdimension.ParseRows,
	})
}

type sheetsResizeAxis struct {
	op        string
	label     string
	sizeLabel string
	dimension string
	parse     func(string, string) (sheetsdimension.Span, error)
}

func runSheetsResize(
	ctx context.Context,
	flags *RootFlags,
	rawSpreadsheetID string,
	rawRange string,
	size int64,
	auto bool,
	axis sheetsResizeAxis,
) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(rawSpreadsheetID))
	rangeSpec := cleanRange(rawRange)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty " + axis.label)
	}
	if auto && size > 0 {
		return usage("use either --" + axis.sizeLabel + " or --auto")
	}
	if !auto && size <= 0 {
		return usage("--" + axis.sizeLabel + " must be > 0 when --auto is not set")
	}

	span, err := axis.parse(rangeSpec, axis.label)
	if err != nil {
		return newUsageError(err)
	}

	dryRunPayload := map[string]any{
		"spreadsheet_id": spreadsheetID,
		axis.label:       rangeSpec,
		"sheet":          span.SheetName,
		"start_index":    span.StartIndex,
		"end_index":      span.EndIndex,
		"auto":           auto,
		axis.sizeLabel:   size,
	}

	return runSheetsMutation(ctx, flags, axis.op, dryRunPayload, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		sheetID, resolvedSheet, err := resolveSheetIDByNameOrFirst(ctx, svc, spreadsheetID, span.SheetName)
		if err != nil {
			return nil, "", err
		}
		dimRange := &sheets.DimensionRange{
			SheetId:    sheetID,
			Dimension:  axis.dimension,
			StartIndex: span.StartIndex,
			EndIndex:   span.EndIndex,
		}
		forceSendDimensionRangeZeroes(dimRange)
		request := &sheets.Request{}
		if auto {
			request.AutoResizeDimensions = &sheets.AutoResizeDimensionsRequest{Dimensions: dimRange}
		} else {
			request.UpdateDimensionProperties = &sheets.UpdateDimensionPropertiesRequest{
				Range:      dimRange,
				Properties: &sheets.DimensionProperties{PixelSize: size},
				Fields:     "pixelSize",
			}
		}
		req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{request}}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}

		text := fmt.Sprintf("Resized %s %s to %dpx", axis.label, rangeSpec, size)
		if auto {
			text = fmt.Sprintf("Auto-resized %s %s", axis.label, rangeSpec)
		}
		return map[string]any{
			"sheet":        resolvedSheet,
			"sheet_id":     sheetID,
			"start_index":  span.StartIndex,
			"end_index":    span.EndIndex,
			"auto":         auto,
			axis.sizeLabel: size,
		}, text, nil
	})
}
