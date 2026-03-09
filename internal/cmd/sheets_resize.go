package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var (
	columnsRangeRe = regexp.MustCompile(`^([A-Za-z]+)(?::([A-Za-z]+))?$`)
	rowsRangeRe    = regexp.MustCompile(`^([0-9]+)(?::([0-9]+))?$`)
)

type SheetsResizeColumnsCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Columns       string `arg:"" name:"columns" help:"Columns range (eg. Sheet1!A:C)"`
	Width         int64  `name:"width" help:"Column width in pixels"`
	Auto          bool   `name:"auto" help:"Auto-fit columns to content"`
}

func (c *SheetsResizeColumnsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

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

	if dryRunErr := dryRunExit(ctx, flags, "sheets.resize-columns", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"columns":        columnsSpec,
		"sheet":          span.SheetName,
		"start_index":    span.StartIndex,
		"end_index":      span.EndIndex,
		"auto":           c.Auto,
		"width":          c.Width,
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

	sheetID, resolvedSheet, err := resolveSheetIDByNameOrFirst(ctx, svc, spreadsheetID, span.SheetName)
	if err != nil {
		return err
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
	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"sheet":       resolvedSheet,
			"sheet_id":    sheetID,
			"start_index": span.StartIndex,
			"end_index":   span.EndIndex,
			"auto":        c.Auto,
			"width":       c.Width,
		})
	}

	if c.Auto {
		u.Out().Printf("Auto-resized columns %s", columnsSpec)
		return nil
	}
	u.Out().Printf("Resized columns %s to %dpx", columnsSpec, c.Width)
	return nil
}

type SheetsResizeRowsCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Rows          string `arg:"" name:"rows" help:"Rows range (eg. Sheet1!1:10)"`
	Height        int64  `name:"height" help:"Row height in pixels"`
	Auto          bool   `name:"auto" help:"Auto-fit rows to content"`
}

func (c *SheetsResizeRowsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

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

	if dryRunErr := dryRunExit(ctx, flags, "sheets.resize-rows", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"rows":           rowsSpec,
		"sheet":          span.SheetName,
		"start_index":    span.StartIndex,
		"end_index":      span.EndIndex,
		"auto":           c.Auto,
		"height":         c.Height,
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

	sheetID, resolvedSheet, err := resolveSheetIDByNameOrFirst(ctx, svc, spreadsheetID, span.SheetName)
	if err != nil {
		return err
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
	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"sheet":       resolvedSheet,
			"sheet_id":    sheetID,
			"start_index": span.StartIndex,
			"end_index":   span.EndIndex,
			"auto":        c.Auto,
			"height":      c.Height,
		})
	}

	if c.Auto {
		u.Out().Printf("Auto-resized rows %s", rowsSpec)
		return nil
	}
	u.Out().Printf("Resized rows %s to %dpx", rowsSpec, c.Height)
	return nil
}

type dimensionSpan struct {
	SheetName  string
	StartIndex int64
	EndIndex   int64
}

func parseColumnsSpan(spec, label string) (dimensionSpan, error) {
	sheetName, part, err := splitA1Sheet(strings.TrimSpace(spec))
	if err != nil {
		return dimensionSpan{}, fmt.Errorf("parse %s range: %w", label, err)
	}
	part = strings.ReplaceAll(strings.TrimSpace(part), "$", "")
	m := columnsRangeRe.FindStringSubmatch(part)
	if m == nil {
		return dimensionSpan{}, fmt.Errorf("invalid %s range %q (expected A:C or Sheet!A:C)", label, spec)
	}

	startCol, err := colLettersToIndex(m[1])
	if err != nil {
		return dimensionSpan{}, err
	}
	endCol := startCol
	if m[2] != "" {
		endCol, err = colLettersToIndex(m[2])
		if err != nil {
			return dimensionSpan{}, err
		}
	}
	if endCol < startCol {
		startCol, endCol = endCol, startCol
	}

	return dimensionSpan{
		SheetName:  sheetName,
		StartIndex: int64(startCol - 1),
		EndIndex:   int64(endCol),
	}, nil
}

func parseRowsSpan(spec, label string) (dimensionSpan, error) {
	sheetName, part, err := splitA1Sheet(strings.TrimSpace(spec))
	if err != nil {
		return dimensionSpan{}, fmt.Errorf("parse %s range: %w", label, err)
	}
	part = strings.ReplaceAll(strings.TrimSpace(part), "$", "")
	m := rowsRangeRe.FindStringSubmatch(part)
	if m == nil {
		return dimensionSpan{}, fmt.Errorf("invalid %s range %q (expected 1:10 or Sheet!1:10)", label, spec)
	}

	startRow, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || startRow <= 0 {
		return dimensionSpan{}, fmt.Errorf("invalid %s start row %q", label, m[1])
	}
	endRow := startRow
	if m[2] != "" {
		endRow, err = strconv.ParseInt(m[2], 10, 64)
		if err != nil || endRow <= 0 {
			return dimensionSpan{}, fmt.Errorf("invalid %s end row %q", label, m[2])
		}
	}
	if endRow < startRow {
		startRow, endRow = endRow, startRow
	}

	return dimensionSpan{
		SheetName:  sheetName,
		StartIndex: startRow - 1,
		EndIndex:   endRow,
	}, nil
}
