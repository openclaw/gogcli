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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
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
		return fmt.Errorf("dimension must be rows or cols, got %q", c.Dimension)
	}

	if c.Start < 1 {
		return fmt.Errorf("start must be >= 1")
	}
	if c.Count < 1 {
		return fmt.Errorf("count must be >= 1")
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
		return fmt.Errorf("unknown sheet %q", sheetName)
	}

	// Convert 1-based position to 0-based index for the API.
	startIndex := c.Start - 1
	if c.After {
		startIndex = c.Start
	}
	endIndex := startIndex + c.Count

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
					InheritFromBefore: c.After,
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"sheet":      sheetName,
			"dimension":  apiDimension,
			"start":      c.Start,
			"count":      c.Count,
			"after":      c.After,
			"startIndex": startIndex,
			"endIndex":   endIndex,
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
