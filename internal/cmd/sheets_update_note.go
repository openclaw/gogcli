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

	parsed, err := parseSheetRange(rangeSpec, "note")
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.update-note", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"note":           noteText,
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

	gridRange, err := gridRangeFromMap(parsed, sheetIDs, "note")
	if err != nil {
		return err
	}

	cellCount := (parsed.EndRow - parsed.StartRow + 1) * (parsed.EndCol - parsed.StartCol + 1)
	batchReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				RepeatCell: &sheets.RepeatCellRequest{
					Range: gridRange,
					Cell: &sheets.CellData{
						Note: noteText,
					},
					Fields: "note",
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, batchReq).Do(); err != nil {
		return fmt.Errorf("update note: %w", err)
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
