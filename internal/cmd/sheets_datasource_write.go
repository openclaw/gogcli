package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/errfmt"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func executeConnectedSheetsWrite(ctx context.Context, flags *RootFlags, spreadsheetID string, request *sheets.BatchUpdateSpreadsheetRequest) (*sheets.BatchUpdateSpreadsheetResponse, error) {
	if googleapi.ReadOnly(ctx) || flags.ReadOnly {
		return nil, fmt.Errorf("%w: Connected Sheets mutations are disabled", googleapi.ErrReadOnly)
	}
	account, svc, err := requireConnectedSheetsWriterService(ctx, flags)
	if err != nil {
		return nil, err
	}
	response, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, request).
		Context(googleapi.WithoutRetries(ctx)).Do()
	if err == nil || !isConnectedSheetsInsufficientScopeError(err) {
		return response, err
	}
	return nil, errfmt.NewUserFacingError(
		fmt.Sprintf("Connected Sheets mutations require writable Sheets authorization plus OAuth scope %s; re-authenticate while preserving this account's existing --services, --drive-scope, and --gmail-scope selections and append --extra-scopes %s --force-consent (for a Sheets-only token: gog auth add %s --services sheets --extra-scopes %s --force-consent)",
			connectedSheetsBigQueryScope, connectedSheetsBigQueryScope, account, connectedSheetsBigQueryScope),
		err,
	)
}

func connectedSheetsExecutionError(operation string, status *sheets.DataExecutionStatus) error {
	if status == nil || status.State != "FAILED" {
		return nil
	}
	detail := status.ErrorCode
	if status.ErrorMessage != "" {
		detail += ": " + status.ErrorMessage
	}
	return fmt.Errorf("%s Connected Sheets data source: %s", operation, detail)
}
