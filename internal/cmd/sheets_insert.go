package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsInsertCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `arg:"" name:"sheet" help:"Sheet name (eg. Sheet1)"`
	Dimension     string `arg:"" name:"dimension" help:"Dimension to insert: rows or cols"`
	Start         int64  `arg:"" name:"start" help:"Position before which to insert (1-based; for cols 1=A, 2=B)"`
	Count         int64  `name:"count" help:"Number of rows/columns to insert" default:"1"`
	After         bool   `name:"after" help:"Insert after the position instead of before"`
}

func (c *SheetsInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	sheetName := strings.TrimSpace(c.Sheet)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if sheetName == "" {
		return usage("empty sheet name")
	}

	dim := strings.ToLower(strings.TrimSpace(c.Dimension))
	var apiDimension, dimLabel string
	switch dim {
	case "rows", "row":
		apiDimension = "ROWS"
		dimLabel = "row"
	case "cols", "col", "columns", "column":
		apiDimension = "COLUMNS"
		dimLabel = "column"
	default:
		return usagef("dimension must be rows or cols, got %q", c.Dimension)
	}

	if c.Start < 1 {
		return usage("start must be >= 1")
	}
	if c.Count < 1 {
		return usage("count must be >= 1")
	}

	// Convert 1-based position to 0-based index for the API.
	startIndex := c.Start - 1
	if c.After {
		startIndex = c.Start
	}
	endIndex := startIndex + c.Count
	inheritFromBefore := c.After

	if dryRunErr := dryRunExit(ctx, flags, "sheets.insert", map[string]any{
		"spreadsheet_id":      spreadsheetID,
		"sheet":               sheetName,
		"dimension":           apiDimension,
		"start":               c.Start,
		"count":               c.Count,
		"after":               c.After,
		"start_index":         startIndex,
		"end_index":           endIndex,
		"inherit_from_before": inheritFromBefore,
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
	sheetID, ok := sheetIDs[sheetName]
	if !ok {
		return usagef("unknown sheet %q", sheetName)
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				InsertDimension: &sheets.InsertDimensionRequest{
					Range: &sheets.DimensionRange{
						SheetId:    sheetID,
						Dimension:  apiDimension,
						StartIndex: startIndex,
						EndIndex:   endIndex,
					},
					InheritFromBefore: inheritFromBefore,
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId":     spreadsheetID,
			"sheet":             sheetName,
			"sheetId":           sheetID,
			"dimension":         apiDimension,
			"start":             c.Start,
			"count":             c.Count,
			"after":             c.After,
			"inheritFromBefore": inheritFromBefore,
			"startIndex":        startIndex,
			"endIndex":          endIndex,
		})
	}

	position := "before"
	if c.After {
		position = "after"
	}
	plural := dimLabel + "s"
	if c.Count == 1 {
		plural = dimLabel
	}
	u.Out().Printf("Inserted %d %s %s %s %d in %q", c.Count, plural, position, dimLabel, c.Start, sheetName)
	return nil
}
