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

type SheetsUpdateNoteCmd struct {
	SpreadsheetID string  `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string  `arg:"" name:"range" help:"A1 cell or range (eg. Sheet1!A1 or Sheet1!A1:B2)"`
	Note          *string `name:"note" help:"Note text to set (use --note '' to clear notes)"`
	NoteFile      string  `name:"note-file" help:"Path to file containing note text" type:"existingfile"`
}

func (c *SheetsUpdateNoteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	// Resolve note text: --note-file takes precedence over --note.
	var noteText string
	hasNote := false
	if c.NoteFile != "" {
		data, err := os.ReadFile(c.NoteFile)
		if err != nil {
			return fmt.Errorf("read note file: %w", err)
		}
		noteText = string(data)
		hasNote = true
	} else if c.Note != nil {
		noteText = *c.Note
		hasNote = true
	}

	if !hasNote {
		return usage("provide --note or --note-file")
	}

	parsed, err := parseA1Range(rangeSpec)
	if err != nil {
		return fmt.Errorf("invalid range: %w", err)
	}

	if parsed.SheetName == "" {
		return usage("range must include a sheet name (eg. Sheet1!A1)")
	}

	if err := dryRunExit(ctx, flags, "sheets.update-note", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"note":           noteText,
	}); err != nil {
		return err
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

	sheetID, ok := sheetIDs[parsed.SheetName]
	if !ok {
		return usagef("unknown sheet %q", parsed.SheetName)
	}

	// Build updateCells requests for each cell in the range.
	var requests []*sheets.Request
	cellCount := 0
	for row := parsed.StartRow; row <= parsed.EndRow; row++ {
		for col := parsed.StartCol; col <= parsed.EndCol; col++ {
			requests = append(requests, &sheets.Request{
				UpdateCells: &sheets.UpdateCellsRequest{
					Rows: []*sheets.RowData{
						{
							Values: []*sheets.CellData{
								{Note: noteText},
							},
						},
					},
					Fields: "note",
					Start: &sheets.GridCoordinate{
						SheetId:     sheetID,
						RowIndex:    int64(row - 1),
						ColumnIndex: int64(col - 1),
					},
				},
			})
			cellCount++
		}
	}

	batchReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, batchReq).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"cellsUpdated":  cellCount,
			"note":          noteText,
		})
	}

	action := "Set"
	if noteText == "" {
		action = "Cleared"
	}
	if cellCount == 1 {
		u.Out().Printf("%s note on %s", action, rangeSpec)
	} else {
		u.Out().Printf("%s note on %d cells in %s", action, cellCount, rangeSpec)
	}
	return nil
}
