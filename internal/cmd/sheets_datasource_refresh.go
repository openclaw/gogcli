package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsDataSourceRefreshCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	DataSourceID  string `arg:"" name:"dataSourceId" help:"Data source ID"`
	ForceRefresh  bool   `name:"force-refresh" help:"Refresh even when the previous execution failed"`
}

func (c *SheetsDataSourceRefreshCmd) Run(ctx context.Context, flags *RootFlags) error {
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
			RefreshDataSource: &sheets.RefreshDataSourceRequest{
				DataSourceId: dataSourceID,
				Force:        c.ForceRefresh,
			},
		}},
	}
	if err := dryRunExit(ctx, flags, "sheets.datasource.refresh", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"data_source_id": dataSourceID,
		"force_refresh":  c.ForceRefresh,
		"batch_update":   request,
	}); err != nil {
		return err
	}
	response, err := executeConnectedSheetsWrite(ctx, flags, spreadsheetID, request)
	if err != nil {
		return err
	}
	if response == nil || len(response.Replies) == 0 || response.Replies[0] == nil ||
		response.Replies[0].RefreshDataSource == nil {
		return fmt.Errorf("provider may have refreshed data source %s without returning its execution reply; inspect it before retrying", dataSourceID)
	}

	statuses := make([]*sheets.RefreshDataSourceObjectExecutionStatus, 0, len(response.Replies[0].RefreshDataSource.Statuses))
	statuses = append(statuses, response.Replies[0].RefreshDataSource.Statuses...)

	var failed *sheets.DataExecutionStatus
	for _, status := range statuses {
		if status != nil && status.DataExecutionStatus != nil && status.DataExecutionStatus.State == "FAILED" {
			failed = status.DataExecutionStatus
			break
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"dataSourceId":  dataSourceID,
			"statuses":      statuses,
		}); err != nil {
			return err
		}
	} else if failed == nil {
		ui.FromContext(ctx).Out().Linef("Refresh requested for data source %s", dataSourceID)
	}

	return connectedSheetsExecutionError("refresh", failed)
}
