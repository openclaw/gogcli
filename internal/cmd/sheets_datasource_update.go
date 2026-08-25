package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsDataSourceUpdateCmd struct {
	SpreadsheetID  string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	DataSourceID   string `arg:"" name:"dataSourceId" help:"Data source ID"`
	BillingProject string `name:"billing-project" help:"New billing-enabled BigQuery execution project"`
	Query          string `name:"query" help:"Replacement SQL for an existing query source"`
	TableProject   string `name:"table-project" help:"Replacement project owning an existing native table"`
	Dataset        string `name:"dataset" help:"Replacement dataset for an existing native table"`
	Table          string `name:"table" help:"Replacement native table ID"`
}

func (c *SheetsDataSourceUpdateCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	dataSourceID := strings.TrimSpace(c.DataSourceID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if dataSourceID == "" {
		return usage("empty dataSourceId")
	}
	for _, arg := range kctx.Args {
		if arg == "--project" || strings.HasPrefix(arg, "--project=") {
			return usage("--project selects output fields; use --billing-project to change the BigQuery execution project")
		}
	}
	spec, fields, err := c.partialBigQuerySpec(kctx)
	if err != nil {
		return err
	}
	preview := map[string]any{
		"spreadsheet_id":             spreadsheetID,
		"data_source_id":             dataSourceID,
		"fields":                     fields,
		"starts_execution":           true,
		"may_incur_bigquery_charges": true,
	}
	if spec.ProjectId != "" {
		preview["billing_project"] = spec.ProjectId
	}
	if spec.QuerySpec != nil {
		preview["query_bytes"] = len(spec.QuerySpec.RawQuery)
	}
	if spec.TableSpec != nil {
		preview["table"] = spec.TableSpec
	}
	if dryRunErr := dryRunExit(ctx, flags, "sheets.datasource.update", preview); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := prepareConnectedSheetsWrite(ctx, flags)
	if err != nil {
		return err
	}
	if preflightErr := validateConnectedSheetsUpdateTarget(ctx, svc, account, spreadsheetID, dataSourceID, spec); preflightErr != nil {
		return preflightErr
	}
	response, err := submitConnectedSheetsWrite(ctx, account, svc, spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			UpdateDataSource: &sheets.UpdateDataSourceRequest{
				DataSource: &sheets.DataSource{
					DataSourceId: dataSourceID,
					Spec:         &sheets.DataSourceSpec{BigQuery: spec},
				},
				Fields: fields,
			},
		}},
	})
	if err != nil {
		return err
	}
	if response == nil || len(response.Replies) == 0 || response.Replies[0] == nil ||
		response.Replies[0].UpdateDataSource == nil {
		return fmt.Errorf("provider may have updated data source %s without returning a result; inspect it before retrying", dataSourceID)
	}
	updated := response.Replies[0].UpdateDataSource
	if updated.DataSource != nil && updated.DataSource.DataSourceId != "" &&
		updated.DataSource.DataSourceId != dataSourceID {
		return fmt.Errorf("provider returned unexpected data source %q after updating %q", updated.DataSource.DataSourceId, dataSourceID)
	}
	result := map[string]any{
		"spreadsheetId": spreadsheetID,
		"dataSourceId":  dataSourceID,
		"fields":        fields,
	}
	if updated.DataExecutionStatus != nil {
		result["dataExecutionStatus"] = updated.DataExecutionStatus
	}
	if outfmt.IsJSON(ctx) {
		if outputErr := outfmt.WriteJSON(ctx, stdoutWriter(ctx), result); outputErr != nil {
			return outputErr
		}
	} else if updated.DataExecutionStatus == nil || updated.DataExecutionStatus.State != "FAILED" {
		ui.FromContext(ctx).Out().Linef("Updated Connected Sheets data source %s", dataSourceID)
	}
	return connectedSheetsExecutionError("update", updated.DataExecutionStatus)
}

func (c *SheetsDataSourceUpdateCmd) partialBigQuerySpec(kctx *kong.Context) (*sheets.BigQueryDataSourceSpec, string, error) {
	queryProvided := flagProvided(kctx, "query")
	tableProvided := flagProvided(kctx, "table-project") || flagProvided(kctx, "dataset") || flagProvided(kctx, "table")
	if queryProvided && tableProvided {
		return nil, "", usage("--query and table flags are mutually exclusive")
	}
	spec := &sheets.BigQueryDataSourceSpec{}
	fields := make([]string, 0, 5)
	if flagProvided(kctx, "billing-project") {
		project := strings.TrimSpace(c.BillingProject)
		if project == "" {
			return nil, "", usage("--billing-project cannot be empty")
		}
		spec.ProjectId = project
		fields = append(fields, "spec.bigQuery.projectId")
	}
	if queryProvided {
		query := strings.TrimSpace(c.Query)
		if query == "" {
			return nil, "", usage("--query cannot be empty")
		}
		spec.QuerySpec = &sheets.BigQueryQuerySpec{RawQuery: query}
		fields = append(fields, "spec.bigQuery.querySpec.rawQuery")
	}
	if tableProvided {
		spec.TableSpec = &sheets.BigQueryTableSpec{}
		for _, field := range []struct {
			name  string
			value string
			path  string
			set   func(string)
		}{
			{name: "table-project", value: c.TableProject, path: "spec.bigQuery.tableSpec.tableProjectId", set: func(value string) { spec.TableSpec.TableProjectId = value }},
			{name: "dataset", value: c.Dataset, path: "spec.bigQuery.tableSpec.datasetId", set: func(value string) { spec.TableSpec.DatasetId = value }},
			{name: "table", value: c.Table, path: "spec.bigQuery.tableSpec.tableId", set: func(value string) { spec.TableSpec.TableId = value }},
		} {
			if !flagProvided(kctx, field.name) {
				continue
			}
			value := strings.TrimSpace(field.value)
			if value == "" {
				return nil, "", usagef("--%s cannot be empty", field.name)
			}
			field.set(value)
			fields = append(fields, field.path)
		}
	}
	if len(fields) == 0 {
		return nil, "", usage("nothing to update: pass --billing-project, --query, --table-project, --dataset, or --table")
	}
	return spec, strings.Join(fields, ","), nil
}

func validateConnectedSheetsUpdateTarget(ctx context.Context, svc *sheets.Service, account, spreadsheetID, dataSourceID string, update *sheets.BigQueryDataSourceSpec) error {
	spreadsheet, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields(googleapi.Field("dataSources(dataSourceId,spec(bigQuery(querySpec,tableSpec)))")).
		Context(ctx).Do()
	if err != nil {
		return connectedSheetsScopeError(err, account)
	}
	for _, source := range spreadsheet.DataSources {
		if source == nil || source.DataSourceId != dataSourceID {
			continue
		}
		if source.Spec == nil || source.Spec.BigQuery == nil {
			return fmt.Errorf("data source %s is not backed by BigQuery", dataSourceID)
		}
		if update.QuerySpec != nil && source.Spec.BigQuery.QuerySpec == nil {
			return fmt.Errorf("cannot apply SQL to non-query BigQuery data source %s", dataSourceID)
		}
		if update.TableSpec != nil && source.Spec.BigQuery.TableSpec == nil {
			return fmt.Errorf("cannot apply table fields to non-table BigQuery data source %s", dataSourceID)
		}
		return nil
	}
	return fmt.Errorf("data source %s was not found in Connected Sheets", dataSourceID)
}
