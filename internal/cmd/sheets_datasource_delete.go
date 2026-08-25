package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsDataSourceDeleteCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	DataSourceID  string `arg:"" name:"dataSourceId" help:"Data source ID"`
}

func (c *SheetsDataSourceDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	dataSourceID := strings.TrimSpace(c.DataSourceID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if dataSourceID == "" {
		return usage("empty dataSourceId")
	}
	request := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			DeleteDataSource: &sheets.DeleteDataSourceRequest{DataSourceId: dataSourceID},
		}},
	}
	if err := dryRunExit(ctx, flags, "sheets.datasource.delete", map[string]any{
		"spreadsheet_id":            spreadsheetID,
		"data_source_id":            dataSourceID,
		"deletes_linked_sheet":      true,
		"unlinks_dependent_objects": true,
		"batch_update":              request,
	}); err != nil {
		return err
	}
	if googleapi.ReadOnly(ctx) || flags.ReadOnly {
		return fmt.Errorf("%w: Connected Sheets deletion is disabled", googleapi.ErrReadOnly)
	}
	action := fmt.Sprintf(
		"delete Connected Sheets data source %s from spreadsheet %s, delete its linked DATA_SOURCE sheet, and unlink dependent extracts, charts, and pivot tables",
		dataSourceID, spreadsheetID,
	)
	if err := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), action); err != nil {
		return err
	}
	if _, err := executeConnectedSheetsWrite(ctx, flags, spreadsheetID, request); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"dataSourceId":  dataSourceID,
			"deleted":       true,
		})
	}
	ui.FromContext(ctx).Out().Linef("Deleted Connected Sheets data source %s", dataSourceID)
	return nil
}
