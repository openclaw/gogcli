package cmd

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/errfmt"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/ui"
)

const connectedSheetsBigQueryScope = "https://www.googleapis.com/auth/bigquery.readonly"

type SheetsDataSourceCmd struct {
	List     SheetsDataSourceListCmd     `cmd:"" default:"withargs" help:"List Connected Sheets data sources"`
	Describe SheetsDataSourceDescribeCmd `cmd:"" name:"describe" aliases:"get,show,info" help:"Describe a Connected Sheets data source"`
	Table    SheetsDataSourceTableCmd    `cmd:"" name:"table" aliases:"tables,extract,extracts" help:"Inspect Connected Sheets data-source tables (extracts)"`
}

type SheetsDataSourceTableCmd struct {
	List     SheetsDataSourceTableListCmd     `cmd:"" default:"withargs" help:"List data-source tables (extracts)"`
	Describe SheetsDataSourceTableDescribeCmd `cmd:"" name:"describe" aliases:"get,show,info" help:"Describe a data-source table at an anchor cell"`
	Read     SheetsDataSourceTableReadCmd     `cmd:"" name:"read" aliases:"values" help:"Read values from a data-source table"`
}

type SheetsDataSourceListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
}

type SheetsDataSourceDescribeCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	DataSourceID  string `arg:"" name:"dataSourceId" help:"Data source ID"`
}

type SheetsDataSourceTableListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	DataSourceID  string `name:"data-source-id" help:"Only tables belonging to this data source ID"`
}

type SheetsDataSourceTableDescribeCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Anchor        string `arg:"" name:"anchor" help:"Table anchor cell including sheet name (for example Extract!A1)"`
}

type SheetsDataSourceTableReadCmd struct {
	SpreadsheetID     string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Anchor            string `arg:"" name:"anchor" help:"Table anchor cell including sheet name (for example Extract!A1)"`
	MaxRows           int    `name:"max-rows" help:"Maximum data rows to read (header row is returned separately)" default:"1000"`
	ValueRenderOption string `name:"render" help:"Value render option: FORMATTED_VALUE, UNFORMATTED_VALUE, or FORMULA" default:"FORMATTED_VALUE" enum:"FORMATTED_VALUE,UNFORMATTED_VALUE,FORMULA"`
}

type sheetsDataSourceItem struct {
	DataSourceID    string `json:"dataSourceId"`
	SheetID         int64  `json:"sheetId"`
	SheetTitle      string `json:"sheetTitle,omitempty"`
	Provider        string `json:"provider"`
	ProjectID       string `json:"projectId,omitempty"`
	Source          string `json:"source,omitempty"`
	State           string `json:"state,omitempty"`
	LastRefreshTime string `json:"lastRefreshTime,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
}

type sheetsDataSourceTableItem struct {
	Anchor              string                  `json:"anchor"`
	SheetID             int64                   `json:"sheetId"`
	SheetTitle          string                  `json:"sheetTitle"`
	DataSourceID        string                  `json:"dataSourceId"`
	ColumnSelectionType string                  `json:"columnSelectionType,omitempty"`
	Columns             []string                `json:"columns"`
	RowLimit            int64                   `json:"rowLimit,omitempty"`
	State               string                  `json:"state,omitempty"`
	LastRefreshTime     string                  `json:"lastRefreshTime,omitempty"`
	ErrorCode           string                  `json:"errorCode,omitempty"`
	ErrorMessage        string                  `json:"errorMessage,omitempty"`
	Table               *sheets.DataSourceTable `json:"dataSourceTable,omitempty"`
	row                 int
	column              int
}

type sheetsDataSourceSnapshot struct {
	DataSources []*sheets.DataSource
	Schedules   []*sheets.DataSourceRefreshSchedule
	Sheets      []*sheets.Sheet
}

func (c *SheetsDataSourceListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	account, svc, err := requireConnectedSheetsService(ctx, flags)
	if err != nil {
		return err
	}
	snapshot, err := fetchSheetsDataSourceSnapshot(ctx, svc, spreadsheetID)
	if err != nil {
		return wrapConnectedSheetsReadError(err, account)
	}

	items := make([]sheetsDataSourceItem, 0, len(snapshot.DataSources))
	for _, source := range snapshot.DataSources {
		if source != nil {
			items = append(items, sheetsDataSourceToItem(source, snapshot.Sheets))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DataSourceID < items[j].DataSourceID })

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"dataSources":   items,
		})
	}
	if len(items) == 0 {
		u.Err().Println("No Connected Sheets data sources")
		return nil
	}
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), items, sheetsDataSourceColumns())
}

func (c *SheetsDataSourceDescribeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	dataSourceID := strings.TrimSpace(c.DataSourceID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if dataSourceID == "" {
		return usage("empty dataSourceId")
	}

	account, svc, err := requireConnectedSheetsService(ctx, flags)
	if err != nil {
		return err
	}
	snapshot, err := fetchSheetsDataSourceSnapshot(ctx, svc, spreadsheetID)
	if err != nil {
		return wrapConnectedSheetsReadError(err, account)
	}
	source := findSheetsDataSource(snapshot.DataSources, dataSourceID)
	if source == nil {
		return usagef("data source %q not found", dataSourceID)
	}
	sheet := findSheetsDataSourceSheet(snapshot.Sheets, source)

	if outfmt.IsJSON(ctx) {
		var properties *sheets.SheetProperties
		if sheet != nil {
			properties = sheet.Properties
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId":       spreadsheetID,
			"dataSource":          source,
			"sheet":               properties,
			"dataSourceSchedules": snapshot.Schedules,
		})
	}

	item := sheetsDataSourceToItem(source, snapshot.Sheets)
	u.Out().Linef("dataSourceId\t%s", item.DataSourceID)
	u.Out().Linef("provider\t%s", item.Provider)
	u.Out().Linef("sheetId\t%d", item.SheetID)
	u.Out().Linef("sheetTitle\t%s", item.SheetTitle)
	u.Out().Linef("projectId\t%s", item.ProjectID)
	u.Out().Linef("source\t%s", item.Source)
	u.Out().Linef("state\t%s", item.State)
	u.Out().Linef("lastRefreshTime\t%s", item.LastRefreshTime)
	if item.ErrorCode != "" || item.ErrorMessage != "" {
		u.Out().Linef("error\t%s\t%s", item.ErrorCode, item.ErrorMessage)
	}
	return nil
}

func (c *SheetsDataSourceTableListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	dataSourceID := strings.TrimSpace(c.DataSourceID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	account, svc, err := requireConnectedSheetsService(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := fetchSheetsDataSourceTables(ctx, svc, spreadsheetID, "")
	if err != nil {
		return wrapConnectedSheetsReadError(err, account)
	}
	items := collectSheetsDataSourceTables(resp)
	if dataSourceID != "" {
		filtered := items[:0]
		for _, item := range items {
			if item.DataSourceID == dataSourceID {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"tables":        items,
		})
	}
	if len(items) == 0 {
		u.Err().Println("No Connected Sheets data-source tables")
		return nil
	}
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), items, sheetsDataSourceTableColumns())
}

func (c *SheetsDataSourceTableDescribeCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID, anchor, err := validateSheetsDataSourceTableArgs(c.SpreadsheetID, c.Anchor)
	if err != nil {
		return err
	}
	account, svc, err := requireConnectedSheetsService(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := fetchSheetsDataSourceTables(ctx, svc, spreadsheetID, anchor)
	if err != nil {
		return wrapConnectedSheetsReadError(err, account)
	}
	item := findSheetsDataSourceTable(collectSheetsDataSourceTables(resp), anchor)
	if item == nil {
		return usagef("data-source table not found at %q", anchor)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId":   spreadsheetID,
			"anchor":          item.Anchor,
			"sheetId":         item.SheetID,
			"sheetTitle":      item.SheetTitle,
			"dataSourceTable": item.Table,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("anchor\t%s", item.Anchor)
	u.Out().Linef("dataSourceId\t%s", item.DataSourceID)
	u.Out().Linef("selection\t%s", item.ColumnSelectionType)
	u.Out().Linef("columns\t%s", strings.Join(item.Columns, ","))
	u.Out().Linef("rowLimit\t%d", item.RowLimit)
	u.Out().Linef("state\t%s", item.State)
	u.Out().Linef("lastRefreshTime\t%s", item.LastRefreshTime)
	if item.ErrorCode != "" || item.ErrorMessage != "" {
		u.Out().Linef("error\t%s\t%s", item.ErrorCode, item.ErrorMessage)
	}
	return nil
}

func (c *SheetsDataSourceTableReadCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID, anchor, err := validateSheetsDataSourceTableArgs(c.SpreadsheetID, c.Anchor)
	if err != nil {
		return err
	}
	if c.MaxRows <= 0 {
		return usage("--max-rows must be greater than 0")
	}

	account, svc, err := requireConnectedSheetsService(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := fetchSheetsDataSourceTables(ctx, svc, spreadsheetID, anchor)
	if err != nil {
		return wrapConnectedSheetsReadError(err, account)
	}
	item := findSheetsDataSourceTable(collectSheetsDataSourceTables(resp), anchor)
	if item == nil {
		return usagef("data-source table not found at %q", anchor)
	}
	columnCount := len(item.Columns)
	if columnCount == 0 {
		columnCount = dataSourceColumnCount(resp.Sheets, item.DataSourceID)
	}
	if columnCount == 0 {
		// The ranged lookup above only returns the sheets the anchor intersects,
		// so a SYNC_ALL table's column list — which lives on its separate
		// DATA_SOURCE sheet — is missing. Re-read those columns unranged.
		allSheets, columnsErr := fetchSheetsDataSourceSheetColumns(ctx, svc, spreadsheetID)
		if columnsErr != nil {
			return wrapConnectedSheetsReadError(columnsErr, account)
		}
		columnCount = dataSourceColumnCount(allSheets, item.DataSourceID)
	}
	if columnCount == 0 {
		return usagef("cannot determine columns for data-source table at %q", anchor)
	}

	rows := c.MaxRows
	truncated := item.RowLimit == 0 || item.RowLimit > int64(rows)
	if item.RowLimit > 0 && item.RowLimit < int64(rows) {
		rows = int(item.RowLimit)
		truncated = false
	}
	end := sheetsa1.FormatCell(item.SheetTitle, item.row+rows, item.column+columnCount-1)
	readRange := item.Anchor + ":" + strings.TrimPrefix(end, sheetsa1.SheetPrefix(item.SheetTitle))
	valuesCall := svc.Spreadsheets.Values.Get(spreadsheetID, readRange).
		MajorDimension("ROWS").
		ValueRenderOption(c.ValueRenderOption).
		Context(ctx)
	values, err := valuesCall.Do()
	if err != nil {
		return wrapConnectedSheetsReadError(err, account)
	}
	rowsOut := values.Values
	if rowsOut == nil {
		rowsOut = [][]interface{}{}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"anchor":        item.Anchor,
			"range":         values.Range,
			"dataSourceId":  item.DataSourceID,
			"state":         item.State,
			"truncated":     truncated,
			"values":        rowsOut,
		})
	}
	if len(rowsOut) == 0 {
		u.Err().Println("No data found")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	for _, row := range rowsOut {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = fmt.Sprintf("%v", cell)
		}
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	return nil
}

func fetchSheetsDataSourceSnapshot(ctx context.Context, svc *sheets.Service, spreadsheetID string) (*sheetsDataSourceSnapshot, error) {
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields(googleapi.Field("spreadsheetId,dataSources,dataSourceSchedules,sheets(properties(sheetId,title,index,sheetType,gridProperties(rowCount,columnCount),dataSourceSheetProperties))")).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	return &sheetsDataSourceSnapshot{
		DataSources: resp.DataSources,
		Schedules:   resp.DataSourceSchedules,
		Sheets:      resp.Sheets,
	}, nil
}

// fetchSheetsDataSourceSheetColumns reads just the data-source sheet column
// definitions, which a ranged lookup cannot return. Kept narrower than the full
// snapshot mask so the extra request stays cheap.
func fetchSheetsDataSourceSheetColumns(ctx context.Context, svc *sheets.Service, spreadsheetID string) ([]*sheets.Sheet, error) {
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields(googleapi.Field("sheets(properties(sheetId,title,dataSourceSheetProperties(dataSourceId,columns)))")).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}

	return resp.Sheets, nil
}

func fetchSheetsDataSourceTables(ctx context.Context, svc *sheets.Service, spreadsheetID, anchor string) (*sheets.Spreadsheet, error) {
	call := svc.Spreadsheets.Get(spreadsheetID).
		IncludeGridData(true).
		Fields(googleapi.Field("spreadsheetId,sheets(properties(sheetId,title,index,sheetType,gridProperties(rowCount,columnCount),dataSourceSheetProperties),data(startRow,startColumn,rowData(values(dataSourceTable))))")).
		Context(ctx)
	if anchor != "" {
		call = call.Ranges(anchor)
	}
	return call.Do()
}

func sheetsDataSourceToItem(source *sheets.DataSource, allSheets []*sheets.Sheet) sheetsDataSourceItem {
	item := sheetsDataSourceItem{
		DataSourceID: source.DataSourceId,
		SheetID:      source.SheetId,
		Provider:     "UNKNOWN",
	}
	if source.Spec != nil {
		switch {
		case source.Spec.BigQuery != nil:
			item.Provider = "BIGQUERY"
			item.ProjectID = source.Spec.BigQuery.ProjectId
			switch {
			case source.Spec.BigQuery.TableSpec != nil:
				table := source.Spec.BigQuery.TableSpec
				tableProject := table.TableProjectId
				if tableProject == "" {
					tableProject = item.ProjectID
				}
				item.Source = strings.Join([]string{tableProject, table.DatasetId, table.TableId}, ".")
			case source.Spec.BigQuery.QuerySpec != nil:
				item.Source = "query"
			}
		case source.Spec.Looker != nil:
			item.Provider = "LOOKER"
			item.Source = strings.Join([]string{source.Spec.Looker.InstanceUri, source.Spec.Looker.Model, source.Spec.Looker.Explore}, "/")
		}
	}
	if sheet := findSheetsDataSourceSheet(allSheets, source); sheet != nil && sheet.Properties != nil {
		// Spreadsheet.dataSources[].sheetId is absent from live API responses, so
		// the linked sheet's own properties are the authoritative id.
		item.SheetID = sheet.Properties.SheetId
		item.SheetTitle = sheet.Properties.Title
		status := sheet.Properties.DataSourceSheetProperties
		if status != nil {
			setSheetsDataExecutionStatus(&item.State, &item.LastRefreshTime, &item.ErrorCode, &item.ErrorMessage, status.DataExecutionStatus)
		}
	}
	return item
}

func findSheetsDataSource(sources []*sheets.DataSource, dataSourceID string) *sheets.DataSource {
	for _, source := range sources {
		if source != nil && source.DataSourceId == dataSourceID {
			return source
		}
	}
	return nil
}

func findSheetsDataSourceSheet(allSheets []*sheets.Sheet, source *sheets.DataSource) *sheets.Sheet {
	if source == nil {
		return nil
	}
	// Match on the data source id first: live responses omit
	// Spreadsheet.dataSources[].sheetId, and a zero value would otherwise claim
	// any unrelated sheet that happens to have id 0.
	for _, sheet := range allSheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		properties := sheet.Properties
		if properties.DataSourceSheetProperties != nil && properties.DataSourceSheetProperties.DataSourceId == source.DataSourceId {
			return sheet
		}
	}
	if source.SheetId == 0 {
		return nil
	}
	for _, sheet := range allSheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		if sheet.Properties.SheetId == source.SheetId {
			return sheet
		}
	}
	return nil
}

func collectSheetsDataSourceTables(resp *sheets.Spreadsheet) []sheetsDataSourceTableItem {
	items := make([]sheetsDataSourceTableItem, 0)
	if resp == nil {
		return items
	}
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		for _, grid := range sheet.Data {
			if grid == nil {
				continue
			}
			for rowOffset, rowData := range grid.RowData {
				if rowData == nil {
					continue
				}
				for columnOffset, cell := range rowData.Values {
					if cell == nil || cell.DataSourceTable == nil {
						continue
					}
					row := int(grid.StartRow) + rowOffset + 1
					column := int(grid.StartColumn) + columnOffset + 1
					items = append(items, sheetsDataSourceTableToItem(sheet.Properties, row, column, cell.DataSourceTable))
				}
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SheetID != items[j].SheetID {
			return items[i].SheetID < items[j].SheetID
		}
		if items[i].row != items[j].row {
			return items[i].row < items[j].row
		}
		return items[i].column < items[j].column
	})
	return items
}

func sheetsDataSourceTableToItem(properties *sheets.SheetProperties, row, column int, table *sheets.DataSourceTable) sheetsDataSourceTableItem {
	columns := make([]string, 0, len(table.Columns))
	for _, reference := range table.Columns {
		if reference != nil {
			columns = append(columns, reference.Name)
		}
	}
	item := sheetsDataSourceTableItem{
		Anchor:              sheetsa1.FormatCell(properties.Title, row, column),
		SheetID:             properties.SheetId,
		SheetTitle:          properties.Title,
		DataSourceID:        table.DataSourceId,
		ColumnSelectionType: table.ColumnSelectionType,
		Columns:             columns,
		RowLimit:            table.RowLimit,
		Table:               table,
		row:                 row,
		column:              column,
	}
	setSheetsDataExecutionStatus(&item.State, &item.LastRefreshTime, &item.ErrorCode, &item.ErrorMessage, table.DataExecutionStatus)
	return item
}

func setSheetsDataExecutionStatus(state, lastRefreshTime, errorCode, errorMessage *string, status *sheets.DataExecutionStatus) {
	if status == nil {
		return
	}
	*state = status.State
	*lastRefreshTime = status.LastRefreshTime
	*errorCode = status.ErrorCode
	*errorMessage = status.ErrorMessage
}

func validateSheetsDataSourceTableArgs(rawSpreadsheetID, rawAnchor string) (string, string, error) {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(rawSpreadsheetID))
	if spreadsheetID == "" {
		return "", "", usage("empty spreadsheetId")
	}
	parsed, err := sheetsa1.Parse(cleanRange(strings.TrimSpace(rawAnchor)))
	if err != nil {
		return "", "", usagef("invalid anchor: %v", err)
	}
	if parsed.SheetName == "" || parsed.StartRow == 0 || parsed.StartCol == 0 || parsed.StartRow != parsed.EndRow || parsed.StartCol != parsed.EndCol {
		return "", "", usage("anchor must be one cell and include a sheet name (for example Extract!A1)")
	}
	return spreadsheetID, sheetsa1.FormatCell(parsed.SheetName, parsed.StartRow, parsed.StartCol), nil
}

func findSheetsDataSourceTable(items []sheetsDataSourceTableItem, anchor string) *sheetsDataSourceTableItem {
	for i := range items {
		if items[i].Anchor == anchor {
			return &items[i]
		}
	}
	return nil
}

func dataSourceColumnCount(allSheets []*sheets.Sheet, dataSourceID string) int {
	for _, sheet := range allSheets {
		if sheet == nil || sheet.Properties == nil || sheet.Properties.DataSourceSheetProperties == nil {
			continue
		}
		properties := sheet.Properties.DataSourceSheetProperties
		if properties.DataSourceId == dataSourceID {
			return len(properties.Columns)
		}
	}
	return 0
}

func wrapConnectedSheetsReadError(err error, account string) error {
	if err == nil {
		return nil
	}
	errText := strings.ToLower(err.Error())
	if !strings.Contains(errText, "insufficient authentication scopes") &&
		!strings.Contains(errText, "access_token_scope_insufficient") &&
		!strings.Contains(errText, "insufficientpermissions") {
		return err
	}
	return errfmt.NewUserFacingError(
		fmt.Sprintf("Connected Sheets BigQuery reads require OAuth scope %s; re-authenticate while preserving this account's existing --services selection and append --extra-scopes %s --force-consent (for a Sheets-only token: gog auth add %s --services sheets --extra-scopes %s --force-consent)", connectedSheetsBigQueryScope, connectedSheetsBigQueryScope, account, connectedSheetsBigQueryScope),
		err,
	)
}

func sheetsDataSourceColumns() []outfmt.Column[sheetsDataSourceItem] {
	return []outfmt.Column[sheetsDataSourceItem]{
		{Header: "DATA_SOURCE_ID", Value: func(item sheetsDataSourceItem) string { return item.DataSourceID }},
		{Header: "PROVIDER", Value: func(item sheetsDataSourceItem) string { return item.Provider }},
		{Header: "SHEET", Value: func(item sheetsDataSourceItem) string { return item.SheetTitle }},
		{Header: "SOURCE", Value: func(item sheetsDataSourceItem) string { return item.Source }},
		{Header: "STATE", Value: func(item sheetsDataSourceItem) string { return item.State }},
		{Header: "LAST_REFRESH", Value: func(item sheetsDataSourceItem) string { return item.LastRefreshTime }},
		{Header: "ERROR", Value: func(item sheetsDataSourceItem) string { return item.ErrorCode }},
	}
}

func sheetsDataSourceTableColumns() []outfmt.Column[sheetsDataSourceTableItem] {
	return []outfmt.Column[sheetsDataSourceTableItem]{
		{Header: "ANCHOR", Value: func(item sheetsDataSourceTableItem) string { return item.Anchor }},
		{Header: "DATA_SOURCE_ID", Value: func(item sheetsDataSourceTableItem) string { return item.DataSourceID }},
		{Header: "SELECTION", Value: func(item sheetsDataSourceTableItem) string { return item.ColumnSelectionType }},
		{Header: "COLUMNS", Value: func(item sheetsDataSourceTableItem) string { return strconv.Itoa(len(item.Columns)) }},
		{Header: "ROW_LIMIT", Value: func(item sheetsDataSourceTableItem) string { return strconv.FormatInt(item.RowLimit, 10) }},
		{Header: "STATE", Value: func(item sheetsDataSourceTableItem) string { return item.State }},
		{Header: "LAST_REFRESH", Value: func(item sheetsDataSourceTableItem) string { return item.LastRefreshTime }},
		{Header: "ERROR", Value: func(item sheetsDataSourceTableItem) string { return item.ErrorCode }},
	}
}
