package cmd

import (
	"context"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// SheetsReorderTabCmd moves a tab to a specific 0-based position in the
// spreadsheet via spreadsheets.batchUpdate -> updateSheetProperties with field
// mask `index`. Existing tab management (add/rename/delete) does not expose
// this; see #603.
type SheetsReorderTabCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Tab           string `name:"tab" required:"" help:"Target tab by name or numeric sheet ID (see sheets metadata)"`
	To            *int64 `name:"to" required:"" help:"Destination final 0-based tab index"`
}

func (c *SheetsReorderTabCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	tab := strings.TrimSpace(c.Tab)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if tab == "" {
		return usage("--tab is required")
	}
	if c.To == nil {
		return usage("--to is required")
	}
	if *c.To < 0 {
		return usage("--to must be >= 0")
	}

	if err := dryRunExit(ctx, flags, "sheets.reorder-tab", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"tab":            tab,
		"to":             *c.To,
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

	target, err := resolveSheetTab(ctx, svc, spreadsheetID, tab)
	if err != nil {
		return err
	}
	if *c.To >= int64(target.Count) {
		return usagef("--to must be between 0 and %d", target.Count-1)
	}

	apiIndex := *c.To
	if *c.To > target.Index {
		apiIndex++
	}

	props := &sheets.SheetProperties{
		SheetId:         target.ID,
		Index:           apiIndex,
		ForceSendFields: []string{"Index"},
	}
	forceSendSheetPropertiesSheetID(props)

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
					Properties: props,
					Fields:     "index",
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"spreadsheetId": spreadsheetID,
			"sheetId":       target.ID,
			"index":         *c.To,
		}
		if target.Title != "" {
			payload["title"] = target.Title
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	if target.Title != "" {
		u.Out().Linef("Moved tab %q (sheetId %d) to index %d in spreadsheet %s", target.Title, target.ID, *c.To, spreadsheetID)
	} else {
		u.Out().Linef("Moved sheetId %d to index %d in spreadsheet %s", target.ID, *c.To, spreadsheetID)
	}
	return nil
}

type sheetTabTarget struct {
	ID    int64
	Title string
	Index int64
	Count int
}

// resolveSheetTab accepts either a tab title or a numeric sheet ID.
func resolveSheetTab(ctx context.Context, svc *sheets.Service, spreadsheetID, tab string) (sheetTabTarget, error) {
	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return sheetTabTarget{}, err
	}

	for _, props := range catalog.Sheets {
		if props != nil && props.Title == tab {
			return sheetTabTarget{ID: props.SheetId, Title: props.Title, Index: props.Index, Count: len(catalog.Sheets)}, nil
		}
	}

	if id, err := strconv.ParseInt(tab, 10, 64); err == nil {
		for _, props := range catalog.Sheets {
			if props != nil && props.SheetId == id {
				return sheetTabTarget{ID: id, Title: props.Title, Index: props.Index, Count: len(catalog.Sheets)}, nil
			}
		}
		return sheetTabTarget{}, usagef("unknown sheetId %d", id)
	}
	return sheetTabTarget{}, usagef("unknown tab %q", tab)
}
