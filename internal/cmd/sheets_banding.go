package cmd

import (
	"context"
	"io"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsbanding"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsBandingCmd struct {
	List  SheetsBandingListCmd  `cmd:"" default:"withargs" help:"List alternating color banded ranges"`
	Set   SheetsBandingSetCmd   `cmd:"" name:"set" aliases:"add,create" help:"Apply alternating colors to a range"`
	Clear SheetsBandingClearCmd `cmd:"" name:"clear" aliases:"delete,rm,remove" help:"Remove alternating color banding"`
}

type SheetsBandingSetCmd struct {
	SpreadsheetID        string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range                string `arg:"" name:"range" help:"A1 range with sheet name (e.g. Sheet1!A1:H20)"`
	RowPropertiesJSON    string `name:"row-properties-json" help:"Sheets API BandingProperties JSON for row colors"`
	ColumnPropertiesJSON string `name:"column-properties-json" help:"Sheets API BandingProperties JSON for column colors"`
}

func (c *SheetsBandingSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}
	parsedRange, err := parseSheetRange(rangeSpec, "banding")
	if err != nil {
		return err
	}
	rowProps, colProps, err := bandingProperties(c.RowPropertiesJSON, c.ColumnPropertiesJSON, stdinReader(ctx))
	if err != nil {
		return err
	}

	if dryErr := dryRunExit(ctx, flags, "sheets.banding.set", map[string]any{
		"spreadsheet_id":    spreadsheetID,
		"range":             rangeSpec,
		"row_properties":    rowProps,
		"column_properties": colProps,
	}); dryErr != nil {
		return dryErr
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
	gridRange, err := gridRangeFromMap(parsedRange, sheetIDs, "banding")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			sheetsbanding.BuildAddRequest(gridRange, rowProps, colProps),
		},
	}
	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}
	var bandedRangeID int64
	if len(resp.Replies) > 0 && resp.Replies[0].AddBanding != nil && resp.Replies[0].AddBanding.BandedRange != nil {
		bandedRangeID = resp.Replies[0].AddBanding.BandedRange.BandedRangeId
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"bandedRangeId": bandedRangeID,
			"range":         rangeSpec,
		})
	}
	u.Out().Linef("Applied banding %d to %s", bandedRangeID, rangeSpec)
	return nil
}

type SheetsBandingListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" help:"Only list banding from this sheet"`
}

func (c *SheetsBandingListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runSheetsSpreadsheetList(
		ctx,
		flags,
		c.SpreadsheetID,
		c.Sheet,
		"sheets(properties(sheetId,title),bandedRanges)",
		"bandedRanges",
		"No banded ranges",
		sheetsbanding.Items,
		sheetsBandingColumns(),
	)
}

type SheetsBandingClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	BandedRangeID int64  `name:"id" help:"Banded range ID to remove"`
	Sheet         string `name:"sheet" help:"Sheet name for --all"`
	All           bool   `name:"all" help:"Remove all banding from the sheet"`
}

func (c *SheetsBandingClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	sheetName := strings.TrimSpace(c.Sheet)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.BandedRangeID > 0 && c.All {
		return usage("use either --id or --all, not both")
	}
	if c.BandedRangeID <= 0 && !c.All {
		return usage("provide --id or --all")
	}
	if c.All && sheetName == "" {
		return usage("--sheet is required with --all")
	}

	if flags != nil && flags.DryRun {
		request := map[string]any{
			"spreadsheet_id":  spreadsheetID,
			"banded_range_id": c.BandedRangeID,
			"sheet":           sheetName,
			"all":             c.All,
		}
		if c.BandedRangeID > 0 {
			request["removed"] = 1
		}
		return dryRunAndConfirmDestructive(ctx, flags, "sheets.banding.clear", request, "remove banding")
	}

	requests := []*sheets.Request{}
	var removed int
	if c.BandedRangeID > 0 {
		requests = append(requests, sheetsbanding.DeleteRequest(c.BandedRangeID))
		removed = 1
	} else {
		account, err := requireAccount(flags)
		if err != nil {
			return err
		}
		svc, err := sheetsService(ctx, account)
		if err != nil {
			return err
		}
		resp, err := svc.Spreadsheets.Get(spreadsheetID).
			Fields("sheets(properties(title),bandedRanges(bandedRangeId))").
			Context(ctx).
			Do()
		if err != nil {
			return err
		}
		ids, found := sheetsbanding.IDsForSheet(resp, sheetName)
		if !found {
			return usagef("unknown sheet %q", sheetName)
		}
		for _, id := range ids {
			requests = append(requests, sheetsbanding.DeleteRequest(id))
		}
		removed = len(requests)
	}

	if len(requests) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"removed": 0})
		}
		ui.FromContext(ctx).Out().Println("No banded ranges to remove")
		return nil
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "sheets.banding.clear", map[string]any{
		"spreadsheet_id":  spreadsheetID,
		"banded_range_id": c.BandedRangeID,
		"sheet":           sheetName,
		"all":             c.All,
		"removed":         removed,
	}, "remove banding"); err != nil {
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
	if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"removed":       removed,
		})
	}
	ui.FromContext(ctx).Out().Linef("Removed %d banded ranges", removed)
	return nil
}

func bandingProperties(rowJSON, columnJSON string, input io.Reader) (*sheets.BandingProperties, *sheets.BandingProperties, error) {
	rowProps, err := parseBandingProperties(rowJSON, sheetsbanding.DefaultRowProperties(), input)
	if err != nil {
		return nil, nil, usagef("invalid --row-properties-json: %v", err)
	}
	colProps, err := parseBandingProperties(columnJSON, nil, input)
	if err != nil {
		return nil, nil, usagef("invalid --column-properties-json: %v", err)
	}
	if rowProps == nil && colProps == nil {
		return nil, nil, usage("provide row or column banding properties")
	}
	return rowProps, colProps, nil
}

func parseBandingProperties(raw string, fallback *sheets.BandingProperties, input io.Reader) (*sheets.BandingProperties, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	b, err := resolveInlineOrFileBytes(raw, input)
	if err != nil {
		return nil, err
	}
	return sheetsbanding.DecodeProperties(b)
}
