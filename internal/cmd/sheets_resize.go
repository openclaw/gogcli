package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type SheetsResizeColumnsCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Columns       string `arg:"" name:"columns" help:"Columns range (eg. Sheet1!A:C)"`
	Width         int64  `name:"width" help:"Column width in pixels"`
	Auto          bool   `name:"auto" help:"Auto-fit columns to content"`
}

func (c *SheetsResizeColumnsCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	columnsSpec := cleanRange(c.Columns)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(columnsSpec) == "" {
		return usage("empty columns")
	}
	if c.Auto && c.Width > 0 {
		return usage("use either --width or --auto")
	}
	if !c.Auto && c.Width <= 0 {
		return usage("--width must be > 0 when --auto is not set")
	}

	span, err := parseColumnsSpan(columnsSpec, "columns")
	if err != nil {
		return err
	}

	return runSheetsMutation(ctx, flags, "sheets.resize-columns", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"columns":        columnsSpec,
		"sheet":          span.SheetName,
		"start_index":    span.StartIndex,
		"end_index":      span.EndIndex,
		"auto":           c.Auto,
		"width":          c.Width,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		sheetID, resolvedSheet, err := resolveSheetIDByNameOrFirst(ctx, svc, spreadsheetID, span.SheetName)
		if err != nil {
			return nil, "", err
		}
		dimRange := &sheets.DimensionRange{
			SheetId:    sheetID,
			Dimension:  "COLUMNS",
			StartIndex: span.StartIndex,
			EndIndex:   span.EndIndex,
		}
		forceSendDimensionRangeZeroes(dimRange)
		request := &sheets.Request{}
		if c.Auto {
			request.AutoResizeDimensions = &sheets.AutoResizeDimensionsRequest{
				Dimensions: dimRange,
			}
		} else {
			request.UpdateDimensionProperties = &sheets.UpdateDimensionPropertiesRequest{
				Range: dimRange,
				Properties: &sheets.DimensionProperties{
					PixelSize: c.Width,
				},
				Fields: "pixelSize",
			}
		}
		req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{request}}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		text := fmt.Sprintf("Resized columns %s to %dpx", columnsSpec, c.Width)
		if c.Auto {
			text = fmt.Sprintf("Auto-resized columns %s", columnsSpec)
		}
		return map[string]any{
			"sheet":       resolvedSheet,
			"sheet_id":    sheetID,
			"start_index": span.StartIndex,
			"end_index":   span.EndIndex,
			"auto":        c.Auto,
			"width":       c.Width,
		}, text, nil
	})
}

type SheetsResizeRowsCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Rows          string `arg:"" name:"rows" help:"Rows range (eg. Sheet1!1:10)"`
	Height        int64  `name:"height" help:"Row height in pixels"`
	Auto          bool   `name:"auto" help:"Auto-fit rows to content"`
}

func (c *SheetsResizeRowsCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rowsSpec := cleanRange(c.Rows)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rowsSpec) == "" {
		return usage("empty rows")
	}
	if c.Auto && c.Height > 0 {
		return usage("use either --height or --auto")
	}
	if !c.Auto && c.Height <= 0 {
		return usage("--height must be > 0 when --auto is not set")
	}

	span, err := parseRowsSpan(rowsSpec, "rows")
	if err != nil {
		return err
	}

	return runSheetsMutation(ctx, flags, "sheets.resize-rows", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"rows":           rowsSpec,
		"sheet":          span.SheetName,
		"start_index":    span.StartIndex,
		"end_index":      span.EndIndex,
		"auto":           c.Auto,
		"height":         c.Height,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		sheetID, resolvedSheet, err := resolveSheetIDByNameOrFirst(ctx, svc, spreadsheetID, span.SheetName)
		if err != nil {
			return nil, "", err
		}
		dimRange := &sheets.DimensionRange{
			SheetId:    sheetID,
			Dimension:  "ROWS",
			StartIndex: span.StartIndex,
			EndIndex:   span.EndIndex,
		}
		forceSendDimensionRangeZeroes(dimRange)
		request := &sheets.Request{}
		if c.Auto {
			request.AutoResizeDimensions = &sheets.AutoResizeDimensionsRequest{
				Dimensions: dimRange,
			}
		} else {
			request.UpdateDimensionProperties = &sheets.UpdateDimensionPropertiesRequest{
				Range:      dimRange,
				Properties: &sheets.DimensionProperties{PixelSize: c.Height},
				Fields:     "pixelSize",
			}
		}
		req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{request}}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		text := fmt.Sprintf("Resized rows %s to %dpx", rowsSpec, c.Height)
		if c.Auto {
			text = fmt.Sprintf("Auto-resized rows %s", rowsSpec)
		}
		return map[string]any{
			"sheet":       resolvedSheet,
			"sheet_id":    sheetID,
			"start_index": span.StartIndex,
			"end_index":   span.EndIndex,
			"auto":        c.Auto,
			"height":      c.Height,
		}, text, nil
	})
}
