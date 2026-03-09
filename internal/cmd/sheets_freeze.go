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

type SheetsFreezeCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Rows          int64  `name:"rows" help:"Number of rows to freeze (0 to unfreeze)" default:"-1"`
	Cols          int64  `name:"cols" help:"Number of columns to freeze (0 to unfreeze)" default:"-1"`
	Sheet         string `name:"sheet" help:"Sheet name (defaults to the first sheet)"`
}

func (c *SheetsFreezeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.Rows < -1 {
		return usage("--rows must be >= 0")
	}
	if c.Cols < -1 {
		return usage("--cols must be >= 0")
	}
	if c.Rows == -1 && c.Cols == -1 {
		return usage("provide --rows and/or --cols")
	}

	requestedSheet := strings.TrimSpace(c.Sheet)
	if dryRunErr := dryRunExit(ctx, flags, "sheets.freeze", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet":          requestedSheet,
		"rows":           c.Rows,
		"cols":           c.Cols,
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

	sheetID, sheetTitle, err := resolveSheetIDByNameOrFirst(ctx, svc, spreadsheetID, requestedSheet)
	if err != nil {
		return err
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

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"sheet":    sheetTitle,
			"sheet_id": sheetID,
			"rows":     c.Rows,
			"cols":     c.Cols,
		})
	}

	rowsLabel := "unchanged"
	if c.Rows >= 0 {
		rowsLabel = fmt.Sprintf("%d", c.Rows)
	}
	colsLabel := "unchanged"
	if c.Cols >= 0 {
		colsLabel = fmt.Sprintf("%d", c.Cols)
	}
	u.Out().Printf("Freeze updated for %q (rows=%s, cols=%s)", sheetTitle, rowsLabel, colsLabel)
	return nil
}
