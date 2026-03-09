package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type SheetsMergeCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
	Type          string `name:"type" help:"Merge type: MERGE_ALL, MERGE_COLUMNS, MERGE_ROWS" default:"MERGE_ALL"`
}

func (c *SheetsMergeCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	return runSheetsMutation(ctx, flags, "sheets.merge", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"type":           mergeType,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
		if err != nil {
			return nil, "", err
		}
		gridRange, err := gridRangeFromMap(rangeInfo, sheetIDs, "merge")
		if err != nil {
			return nil, "", err
		}
		req := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				MergeCells: &sheets.MergeCellsRequest{
					Range:     gridRange,
					MergeType: mergeType,
				},
			}},
		}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		return map[string]any{
			"range": rangeSpec,
			"type":  mergeType,
		}, fmt.Sprintf("Merged %s (%s)", rangeSpec, mergeType), nil
	})
}

type SheetsUnmergeCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
}

func (c *SheetsUnmergeCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	return runSheetsMutation(ctx, flags, "sheets.unmerge", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
		if err != nil {
			return nil, "", err
		}
		gridRange, err := gridRangeFromMap(rangeInfo, sheetIDs, "unmerge")
		if err != nil {
			return nil, "", err
		}
		req := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				UnmergeCells: &sheets.UnmergeCellsRequest{Range: gridRange},
			}},
		}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		return map[string]any{"range": rangeSpec}, fmt.Sprintf("Unmerged %s", rangeSpec), nil
	})
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
