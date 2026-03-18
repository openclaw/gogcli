package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsAddTabCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TabName       string `arg:"" name:"tabName" help:"Name for the new tab/sheet"`
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

	if err := dryRunExit(ctx, flags, "sheets.add-tab", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"tab_name":       tabName,
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

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: tabName,
					},
				},
			},
		},
	}

	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return err
	}

	var newSheetID int64
	if len(resp.Replies) > 0 && resp.Replies[0].AddSheet != nil {
		newSheetID = resp.Replies[0].AddSheet.Properties.SheetId
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tabName":       tabName,
			"sheetId":       newSheetID,
		})
	}

	u.Out().Printf("Added tab %q (sheetId %d) to spreadsheet %s", tabName, newSheetID, spreadsheetID)
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

	svc, err := newSheetsService(ctx, account)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"oldName":       oldName,
			"newName":       newName,
			"sheetId":       sheetID,
		})
	}

	u.Out().Printf("Renamed tab %q to %q in spreadsheet %s", oldName, newName, spreadsheetID)
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
	sheetID, ok := sheetIDs[tabName]
	if !ok {
		return usagef("unknown tab %q", tabName)
	}

	if err := dryRunExit(ctx, flags, "sheets.delete-tab", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"tab_name":       tabName,
		"sheet_id":       sheetID,
	}); err != nil {
		return err
	}

	if err := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), "delete sheet tab "+tabName); err != nil {
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tabName":       tabName,
			"sheetId":       sheetID,
		})
	}

	u.Out().Printf("Deleted tab %q (sheetId %d) from spreadsheet %s", tabName, sheetID, spreadsheetID)
	return nil
}
