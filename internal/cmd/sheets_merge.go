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

type SheetsMergeCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
	Type          string `name:"type" help:"Merge type: MERGE_ALL, MERGE_COLUMNS, MERGE_ROWS" default:"MERGE_ALL"`
}

func (c *SheetsMergeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	mergeType, err := normalizeMergeType(c.Type)
	if err != nil {
		return err
	}

	rangeInfo, err := parseSheetRange(rangeSpec, "merge")
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.merge", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"type":           mergeType,
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
	gridRange, err := gridRangeFromMap(rangeInfo, sheetIDs, "merge")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			MergeCells: &sheets.MergeCellsRequest{
				Range:     gridRange,
				MergeType: mergeType,
			},
		}},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"range": rangeSpec,
			"type":  mergeType,
		})
	}

	u.Out().Printf("Merged %s (%s)", rangeSpec, mergeType)
	return nil
}

type SheetsUnmergeCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
}

func (c *SheetsUnmergeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	rangeInfo, err := parseSheetRange(rangeSpec, "unmerge")
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.unmerge", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
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
	gridRange, err := gridRangeFromMap(rangeInfo, sheetIDs, "unmerge")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			UnmergeCells: &sheets.UnmergeCellsRequest{Range: gridRange},
		}},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"range": rangeSpec})
	}

	u.Out().Printf("Unmerged %s", rangeSpec)
	return nil
}

func normalizeMergeType(raw string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(raw))
	if v == "" {
		v = "MERGE_ALL"
	}
	switch v {
	case "MERGE_ALL", "MERGE_COLUMNS", "MERGE_ROWS":
		return v, nil
	default:
		return "", fmt.Errorf("invalid --type %q (expected MERGE_ALL, MERGE_COLUMNS, or MERGE_ROWS)", raw)
	}
}
