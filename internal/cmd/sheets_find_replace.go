package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsFindReplaceCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Find          string `arg:"" name:"find" help:"Text to find"`
	Replace       string `arg:"" name:"replace" help:"Replacement text"`
	Sheet         string `name:"sheet" help:"Sheet name to scope the operation"`
	MatchCase     bool   `name:"match-case" help:"Case-sensitive matching"`
	MatchEntire   bool   `name:"match-entire" aliases:"exact" help:"Match entire cell value"`
	Regex         bool   `name:"regex" help:"Treat find text as a regex"`
	FormulasOnly  bool   `name:"formulas" help:"Include formula cells in search"`
}

func (c *SheetsFindReplaceCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.Find == "" {
		return usage("find text cannot be empty")
	}

	sheetName := strings.TrimSpace(c.Sheet)
	if dryRunErr := dryRunExit(ctx, flags, "sheets.find-replace", map[string]any{
		"spreadsheet_id":   spreadsheetID,
		"find":             c.Find,
		"replace":          c.Replace,
		"sheet":            sheetName,
		"match_case":       c.MatchCase,
		"match_entire":     c.MatchEntire,
		"regex":            c.Regex,
		"include_formulas": c.FormulasOnly,
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

	findReq := &sheets.FindReplaceRequest{
		Find:            c.Find,
		Replacement:     c.Replace,
		MatchCase:       c.MatchCase,
		MatchEntireCell: c.MatchEntire,
		SearchByRegex:   c.Regex,
		IncludeFormulas: c.FormulasOnly,
	}

	if sheetName != "" {
		sheetIDs, fetchErr := fetchSheetIDMap(ctx, svc, spreadsheetID)
		if fetchErr != nil {
			return fetchErr
		}

		sheetID, ok := sheetIDs[sheetName]
		if !ok {
			return usagef("unknown sheet %q", sheetName)
		}

		findReq.SheetId = sheetID
		findReq.ForceSendFields = []string{"SheetId"}
	} else {
		findReq.AllSheets = true
	}

	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{FindReplace: findReq}},
	}).Do()
	if err != nil {
		return err
	}

	result := &sheets.FindReplaceResponse{}
	if len(resp.Replies) > 0 && resp.Replies[0] != nil && resp.Replies[0].FindReplace != nil {
		result = resp.Replies[0].FindReplace
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"find":                c.Find,
			"replace":             c.Replace,
			"occurrences_changed": result.OccurrencesChanged,
			"values_changed":      result.ValuesChanged,
			"formulas_changed":    result.FormulasChanged,
			"rows_changed":        result.RowsChanged,
			"sheets_changed":      result.SheetsChanged,
		})
	}

	u.Out().Printf(
		"Replaced %d occurrences across %d values in %d sheets",
		result.OccurrencesChanged,
		result.ValuesChanged,
		result.SheetsChanged,
	)
	return nil
}
