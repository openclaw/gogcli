package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// SheetsRenameCmd renames a sheet tab (worksheet) within a Google Spreadsheet.
// The sheet can be identified by its 0-based index or by its current name.
//
// Examples:
//
//	gog sheets rename <spreadsheetId> 0 "New Name"       # rename first tab by index
//	gog sheets rename <spreadsheetId> "OldName" "New Name" # rename by current name
type SheetsRenameCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `arg:"" name:"sheet" help:"Sheet tab to rename: 0-based index or current tab name"`
	NewName       string `arg:"" name:"newName" help:"New name for the sheet tab"`
}

func (c *SheetsRenameCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	sheetRef := strings.TrimSpace(c.Sheet)
	newName := strings.TrimSpace(c.NewName)

	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if sheetRef == "" {
		return usage("empty sheet (provide 0-based index or current sheet name)")
	}
	if newName == "" {
		return usage("empty newName")
	}

	if err := dryRunExit(ctx, flags, "sheets.rename", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet":          sheetRef,
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

	// Fetch sheet metadata to resolve sheetRef → numeric SheetId.
	call := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title))")
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get spreadsheet metadata: %w", err)
	}

	sheetID, oldName, err := resolveSheetRef(resp.Sheets, sheetRef)
	if err != nil {
		return err
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

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"sheetId":       sheetID,
			"oldName":       oldName,
			"newName":       newName,
		})
	}

	u.Out().Printf("Renamed sheet %q → %q", oldName, newName)
	return nil
}

// resolveSheetRef resolves a sheet reference (0-based index string or sheet name) to
// the numeric SheetId used by the Sheets API, plus the current sheet title.
func resolveSheetRef(sheets []*sheets.Sheet, ref string) (sheetID int64, title string, err error) {
	// Try numeric index first.
	if idx, parseErr := strconv.ParseInt(ref, 10, 64); parseErr == nil {
		if idx < 0 || idx >= int64(len(sheets)) {
			return 0, "", fmt.Errorf("sheet index %d out of range (spreadsheet has %d sheet(s))", idx, len(sheets))
		}
		props := sheets[idx].Properties
		return props.SheetId, props.Title, nil
	}

	// Fall back to matching by name.
	for _, s := range sheets {
		if s.Properties == nil {
			continue
		}
		if strings.EqualFold(s.Properties.Title, ref) {
			return s.Properties.SheetId, s.Properties.Title, nil
		}
	}
	return 0, "", fmt.Errorf("no sheet found with name %q", ref)
}
