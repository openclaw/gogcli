package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsTableClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string `arg:"" name:"tableId" help:"Table ID or table name"`
}

func (c *SheetsTableClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	in := strings.TrimSpace(c.TableID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if in == "" {
		return usage("empty tableId")
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.table.clear", map[string]any{
		"spreadsheet_id":   spreadsheetID,
		"table_id_or_name": in,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	tables, err := fetchSpreadsheetTables(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	table, found, err := resolveSheetsTable(in, tables)
	if err != nil {
		return err
	}
	if !found {
		return usagef("unknown table %q", in)
	}
	dataRange := strings.TrimSpace(table.DataA1)
	if dataRange == "" {
		return fmt.Errorf("table %q has no data rows to clear", table.TableID)
	}

	if flags == nil || !flags.Force {
		return usage("sheets table clear requires --force")
	}
	if confirmErr := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), "clear data rows in table "+table.Name); confirmErr != nil {
		return confirmErr
	}

	resp, err := svc.Spreadsheets.Values.Clear(spreadsheetID, dataRange, &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"tableId":      table.TableID,
			"name":         table.Name,
			"tableRange":   table.A1,
			"clearedRange": resp.ClearedRange,
			"hasFooter":    table.HasFooter,
		})
	}

	u.Out().Linef("Cleared data rows in %s", resp.ClearedRange)
	return nil
}

func sheetsTableHasFooter(table *sheets.Table) bool {
	return table != nil && table.RowsProperties != nil && table.RowsProperties.FooterColorStyle != nil
}

func sheetsTableDataRangeA1(sheetTitle string, table *sheets.Table) (string, bool) {
	if table == nil || table.Range == nil {
		return "", false
	}
	dataRange := *table.Range
	dataRange.StartRowIndex++
	if sheetsTableHasFooter(table) {
		dataRange.EndRowIndex--
	}
	if dataRange.EndRowIndex <= dataRange.StartRowIndex {
		return "", false
	}
	return sheetsa1.FormatGridRange(sheetTitle, &dataRange), true
}
