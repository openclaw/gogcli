package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsAddTabCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TabName       string `arg:"" name:"tabName" help:"Name for the new tab/sheet"`
	Index         *int64 `name:"index" help:"Zero-based tab index for the new tab"`
}

func (c *SheetsAddTabCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	tabName := strings.TrimSpace(c.TabName)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if tabName == "" {
		return usage("empty tabName")
	}

	payload := map[string]any{
		"spreadsheet_id": spreadsheetID,
		"tab_name":       tabName,
	}
	if c.Index != nil {
		payload["index"] = *c.Index
	}
	if err := dryRunExit(ctx, flags, "sheets.add-tab", payload); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	props := &sheets.SheetProperties{Title: tabName}
	if c.Index != nil {
		props.Index = *c.Index
		props.ForceSendFields = []string{"Index"}
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: props,
				},
			},
		},
	}

	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return err
	}

	var newSheetID int64
	var newIndex int64
	hasNewIndex := false
	if len(resp.Replies) > 0 && resp.Replies[0].AddSheet != nil && resp.Replies[0].AddSheet.Properties != nil {
		props := resp.Replies[0].AddSheet.Properties
		newSheetID = props.SheetId
		newIndex = props.Index
		hasNewIndex = true
	}

	if outfmt.IsJSON(ctx) {
		out := map[string]any{
			"spreadsheetId": spreadsheetID,
			"tabName":       tabName,
			"title":         tabName,
			"sheetId":       newSheetID,
		}
		if c.Index != nil || hasNewIndex {
			out["index"] = newIndex
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), out)
	}

	if c.Index != nil || hasNewIndex {
		u.Out().Linef("Added tab %q (sheetId %d, index %d) to spreadsheet %s", tabName, newSheetID, newIndex, spreadsheetID)
		return nil
	}
	u.Out().Linef("Added tab %q (sheetId %d) to spreadsheet %s", tabName, newSheetID, spreadsheetID)
	return nil
}

type SheetsRenameTabCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	OldName       string `arg:"" name:"oldName" help:"Current tab name"`
	NewName       string `arg:"" name:"newName" help:"New tab name"`
}

func (c *SheetsRenameTabCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	oldName := strings.TrimSpace(c.OldName)
	newName := strings.TrimSpace(c.NewName)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if oldName == "" {
		return usage("empty oldName")
	}
	if newName == "" {
		return usage("empty newName")
	}

	if err := dryRunExit(ctx, flags, "sheets.rename-tab", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"old_name":       oldName,
		"new_name":       newName,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	sheetID, ok := sheetIDs[oldName]
	if !ok {
		return usagef("unknown tab %q", oldName)
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
					Properties: &sheets.SheetProperties{
						SheetId: sheetID,
						Title:   newName,
					},
					Fields: "title",
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"oldName":       oldName,
			"newName":       newName,
			"oldTitle":      oldName,
			"newTitle":      newName,
			"sheetId":       sheetID,
		})
	}

	u.Out().Linef("Renamed tab %q to %q in spreadsheet %s", oldName, newName, spreadsheetID)
	return nil
}

type SheetsDeleteTabCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TabName       string `arg:"" name:"tabName" help:"Tab name to delete"`
}

func (c *SheetsDeleteTabCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	tabName := strings.TrimSpace(c.TabName)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if tabName == "" {
		return usage("empty tabName")
	}

	if err := dryRunExit(ctx, flags, "sheets.delete-tab", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"tab_name":       tabName,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	sheetID, ok := sheetIDs[tabName]
	if !ok {
		return usagef("unknown tab %q", tabName)
	}

	if err := confirmDestructiveChecked(ctx, flags, fmt.Sprintf("delete sheet tab %s", tabName)); err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				DeleteSheet: &sheets.DeleteSheetRequest{
					SheetId: sheetID,
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"tabName":       tabName,
			"title":         tabName,
			"sheetId":       sheetID,
			"deleted":       true,
		})
	}

	u.Out().Linef("Deleted tab %q (sheetId %d) from spreadsheet %s", tabName, sheetID, spreadsheetID)
	return nil
}
