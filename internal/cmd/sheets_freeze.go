package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/sheets/v4"
)

type SheetsFreezeCmd struct {
	Batch         string `name:"batch" help:"Append requests to a persisted Sheets batch instead of submitting"`
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Rows          int64  `name:"rows" help:"Number of rows to freeze (0 to unfreeze)" default:"-1"`
	Cols          int64  `name:"cols" help:"Number of columns to freeze (0 to unfreeze)" default:"-1"`
	Sheet         string `name:"sheet" help:"Sheet name (defaults to the first sheet)"`
}

func (c *SheetsFreezeCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	rowsProvided := flagProvided(kctx, "rows")
	colsProvided := flagProvided(kctx, "cols")
	if rowsProvided && c.Rows < 0 {
		return usage("--rows must be >= 0")
	}
	if colsProvided && c.Cols < 0 {
		return usage("--cols must be >= 0")
	}
	if !rowsProvided && !colsProvided {
		return usage("provide --rows and/or --cols")
	}

	requestedSheet := strings.TrimSpace(c.Sheet)
	return runSheetsMutation(ctx, flags, c.Batch, spreadsheetID, "sheets.freeze", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet":          requestedSheet,
		"rows":           c.Rows,
		"cols":           c.Cols,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		sheetID, sheetTitle, err := resolveSheetIDByNameOrFirst(ctx, svc, spreadsheetID, requestedSheet)
		if err != nil {
			return nil, "", err
		}
		gridProps := &sheets.GridProperties{}
		fields := make([]string, 0, 2)
		if c.Rows >= 0 {
			gridProps.FrozenRowCount = c.Rows
			fields = append(fields, "gridProperties.frozenRowCount")
			if c.Rows == 0 {
				gridProps.ForceSendFields = append(gridProps.ForceSendFields, "FrozenRowCount")
			}
		}
		if c.Cols >= 0 {
			gridProps.FrozenColumnCount = c.Cols
			fields = append(fields, "gridProperties.frozenColumnCount")
			if c.Cols == 0 {
				gridProps.ForceSendFields = append(gridProps.ForceSendFields, "FrozenColumnCount")
			}
		}
		props := &sheets.SheetProperties{
			SheetId:        sheetID,
			GridProperties: gridProps,
		}
		forceSendSheetPropertiesSheetID(props)
		req := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
					Properties: props,
					Fields:     strings.Join(fields, ","),
				},
			}},
		}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		rowsLabel := "unchanged"
		if c.Rows >= 0 {
			rowsLabel = fmt.Sprintf("%d", c.Rows)
		}
		colsLabel := "unchanged"
		if c.Cols >= 0 {
			colsLabel = fmt.Sprintf("%d", c.Cols)
		}
		return map[string]any{
			"sheet":    sheetTitle,
			"sheet_id": sheetID,
			"rows":     c.Rows,
			"cols":     c.Cols,
		}, fmt.Sprintf("Freeze updated for %q (rows=%s, cols=%s)", sheetTitle, rowsLabel, colsLabel), nil
	})
}
