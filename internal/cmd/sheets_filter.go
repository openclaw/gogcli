package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type SheetsFilterCmd struct {
	Set SheetsFilterSetCmd `cmd:"" name:"set" aliases:"create,add" help:"Create a basic filter on a range"`
}

type SheetsFilterSetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (A1 notation with sheet name or named range name)"`
}

func (c *SheetsFilterSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	return runSheetsMutation(ctx, flags, "sheets.filter.set", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
		if err != nil {
			return nil, "", err
		}
		gridRange, err := resolveGridRangeWithCatalog(rangeSpec, catalog, "filter")
		if err != nil {
			return nil, "", err
		}
		filter := &sheets.BasicFilter{Range: gridRange}
		req := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				SetBasicFilter: &sheets.SetBasicFilterRequest{
					Filter: filter,
				},
			}},
		}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		return map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"filter":        filter,
		}, fmt.Sprintf("Set basic filter on %s", rangeSpec), nil
	})
}
