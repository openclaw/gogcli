package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/errfmt"
	"github.com/openclaw/gogcli/internal/googleapi"
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
	if googleapi.ReadOnly(ctx) || flags.ReadOnly {
		return fmt.Errorf("%w: Connected Sheets refresh is disabled", googleapi.ErrReadOnly)
	}

	account, svc, err := requireConnectedSheetsWriterService(ctx, flags)
	if err != nil {
		return err
	}
	response, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, request).
		Context(googleapi.WithoutRetries(ctx)).Do()
	if err != nil {
		return wrapConnectedSheetsRefreshError(err, account)
	}

	statuses := make([]*sheets.RefreshDataSourceObjectExecutionStatus, 0)
	if response != nil && len(response.Replies) > 0 && response.Replies[0] != nil &&
		response.Replies[0].RefreshDataSource != nil {
		statuses = append(statuses, response.Replies[0].RefreshDataSource.Statuses...)
	}

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

	if failed != nil {
		detail := failed.ErrorCode
		if failed.ErrorMessage != "" {
			detail += ": " + failed.ErrorMessage
		}
		return fmt.Errorf("refresh Connected Sheets data source: %s", detail)
	}
	return nil
}

func wrapConnectedSheetsRefreshError(err error, account string) error {
	if !isConnectedSheetsInsufficientScopeError(err) {
		return err
	}
	return errfmt.NewUserFacingError(
		fmt.Sprintf("Connected Sheets refresh requires writable Sheets authorization plus OAuth scope %s; re-authenticate while preserving this account's existing --services, --drive-scope, and --gmail-scope selections and append --extra-scopes %s --force-consent (for a Sheets-only token: gog auth add %s --services sheets --extra-scopes %s --force-consent)",
			connectedSheetsBigQueryScope, connectedSheetsBigQueryScope, account, connectedSheetsBigQueryScope),
		err,
	)
}
