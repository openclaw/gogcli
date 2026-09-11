package cmd

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsDuplicateTabCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	SourceTab     string `arg:"" name:"sourceTab" help:"Source tab title or numeric sheet ID"`
	NewName       string `arg:"" name:"newName" help:"Name of the duplicated tab"`
	Index         *int64 `name:"index" help:"Zero-based insertion index (default: directly after the source tab)"`
}

func (c *SheetsDuplicateTabCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	sourceTab := strings.TrimSpace(c.SourceTab)
	newName := strings.TrimSpace(c.NewName)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if sourceTab == "" || newName == "" {
		return usage("sourceTab and newName must not be empty")
	}
	if c.Index != nil && *c.Index < 0 {
		return usage("--index must be >= 0")
	}
	if err := dryRunExit(ctx, flags, "sheets.duplicate-tab", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"source_tab":     sourceTab,
		"new_name":       newName,
		"index":          c.Index,
	}); err != nil {
		return err
	}
	_, svc, err := requireSheetsService(ctx, flags)
	if err != nil {
		return err
	}
	target, err := resolveSheetTab(ctx, svc, spreadsheetID, sourceTab)
	if err != nil {
		return err
	}
	index := target.Index + 1
	if c.Index != nil {
		index = *c.Index
	}
	if index > int64(target.Count) {
		return usagef("--index must be between 0 and %d", target.Count)
	}
	request := &sheets.DuplicateSheetRequest{
		SourceSheetId:    target.ID,
		NewSheetName:     newName,
		InsertSheetIndex: index,
		ForceSendFields:  []string{"SourceSheetId", "InsertSheetIndex"},
	}
	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{DuplicateSheet: request}},
	}).Context(ctx).Do()
	if err != nil {
		return err
	}
	if resp == nil || len(resp.Replies) != 1 || resp.Replies[0] == nil || resp.Replies[0].DuplicateSheet == nil || resp.Replies[0].DuplicateSheet.Properties == nil {
		return errors.New("duplicate-tab: Google returned no new tab properties; inspect the spreadsheet before retrying")
	}
	props := resp.Replies[0].DuplicateSheet.Properties
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"sourceSheetId": target.ID,
			"sheetId":       props.SheetId,
			"title":         props.Title,
			"index":         props.Index,
		})
	}
	ui.FromContext(ctx).Out().Linef("Duplicated tab %q as %q (sheetId %d, index %d)", target.Title, props.Title, props.SheetId, props.Index)
	return nil
}
