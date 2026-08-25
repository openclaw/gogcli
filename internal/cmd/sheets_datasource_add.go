package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsDataSourceAddCmd struct {
	SpreadsheetID  string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	BillingProject string `name:"billing-project" help:"Billing-enabled BigQuery project charged for source queries"`
	Query          string `name:"query" help:"BigQuery SQL query; mutually exclusive with table flags"`
	TableProject   string `name:"table-project" help:"BigQuery project owning the table; defaults to the billing project"`
	Dataset        string `name:"dataset" help:"BigQuery dataset ID"`
	Table          string `name:"table" help:"BigQuery table ID"`
}

func (c *SheetsDataSourceAddCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	spec, queryProvided, err := c.bigQuerySpec(kctx)
	if err != nil {
		return err
	}
	preview := map[string]any{
		"spreadsheet_id":             spreadsheetID,
		"billing_project":            spec.ProjectId,
		"starts_execution":           true,
		"may_incur_bigquery_charges": true,
		"query_provided":             queryProvided,
	}
	if queryProvided {
		preview["query_bytes"] = len(spec.QuerySpec.RawQuery)
	} else {
		preview["dataset"] = spec.TableSpec.DatasetId
		preview["table"] = spec.TableSpec.TableId
		if spec.TableSpec.TableProjectId != "" {
			preview["table_project"] = spec.TableSpec.TableProjectId
		}
	}
	if dryRunErr := dryRunExit(ctx, flags, "sheets.datasource.add", preview); dryRunErr != nil {
		return dryRunErr
	}

	response, err := executeConnectedSheetsWrite(ctx, flags, spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			AddDataSource: &sheets.AddDataSourceRequest{
				DataSource: &sheets.DataSource{Spec: &sheets.DataSourceSpec{BigQuery: spec}},
			},
		}},
	})
	if err != nil {
		return err
	}
	if response == nil || len(response.Replies) == 0 || response.Replies[0] == nil ||
		response.Replies[0].AddDataSource == nil || response.Replies[0].AddDataSource.DataSource == nil ||
		strings.TrimSpace(response.Replies[0].AddDataSource.DataSource.DataSourceId) == "" {
		return fmt.Errorf("provider may have created a Connected Sheets source without returning its ID; inspect spreadsheet %s with sheets datasource list", spreadsheetID)
	}
	added := response.Replies[0].AddDataSource
	result := map[string]any{
		"spreadsheetId": spreadsheetID,
		"dataSourceId":  added.DataSource.DataSourceId,
	}
	if added.DataSource.SheetId != 0 {
		result["sheetId"] = added.DataSource.SheetId
	}
	if added.DataExecutionStatus != nil {
		result["dataExecutionStatus"] = added.DataExecutionStatus
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), result); err != nil {
			return err
		}
	} else if added.DataExecutionStatus == nil || added.DataExecutionStatus.State != "FAILED" {
		ui.FromContext(ctx).Out().Linef("Created Connected Sheets data source %s", added.DataSource.DataSourceId)
	}
	return connectedSheetsExecutionError("create", added.DataExecutionStatus)
}

func (c *SheetsDataSourceAddCmd) bigQuerySpec(kctx *kong.Context) (*sheets.BigQueryDataSourceSpec, bool, error) {
	project := strings.TrimSpace(c.BillingProject)
	if project == "" {
		return nil, false, usage("--billing-project is required")
	}
	queryProvided := flagProvided(kctx, "query")
	tableProvided := flagProvided(kctx, "table-project") || flagProvided(kctx, "dataset") || flagProvided(kctx, "table")
	if queryProvided && tableProvided {
		return nil, false, usage("--query and table flags are mutually exclusive")
	}
	if !queryProvided && !tableProvided {
		return nil, false, usage("pass --query, or both --dataset and --table")
	}
	spec := &sheets.BigQueryDataSourceSpec{ProjectId: project}
	if queryProvided {
		query := strings.TrimSpace(c.Query)
		if query == "" {
			return nil, false, usage("--query cannot be empty")
		}
		spec.QuerySpec = &sheets.BigQueryQuerySpec{RawQuery: query}
		return spec, true, nil
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "table-project", value: c.TableProject},
		{name: "dataset", value: c.Dataset},
		{name: "table", value: c.Table},
	} {
		if flagProvided(kctx, field.name) && strings.TrimSpace(field.value) == "" {
			return nil, false, usagef("--%s cannot be empty", field.name)
		}
	}
	dataset := strings.TrimSpace(c.Dataset)
	table := strings.TrimSpace(c.Table)
	if dataset == "" || table == "" {
		return nil, false, usage("--dataset and --table are both required")
	}
	spec.TableSpec = &sheets.BigQueryTableSpec{
		TableProjectId: strings.TrimSpace(c.TableProject),
		DatasetId:      dataset,
		TableId:        table,
	}
	return spec, false, nil
}
