package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsTableCmd struct {
	List   SheetsTableListCmd   `cmd:"" default:"withargs" help:"List tables in a spreadsheet"`
	Get    SheetsTableGetCmd    `cmd:"" name:"get" aliases:"show,info" help:"Get a table"`
	Create SheetsTableCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a table"`
	Append SheetsTableAppendCmd `cmd:"" name:"append" aliases:"add-row,add-rows" help:"Append rows to a table"`
	Clear  SheetsTableClearCmd  `cmd:"" name:"clear" aliases:"clear-rows" help:"Clear table data rows"`
	Delete SheetsTableDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a table"`
}

type SheetsTableListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
}

func (c *SheetsTableListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	tables, err := fetchSpreadsheetTables(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].Name == tables[j].Name {
			return tables[i].TableID < tables[j].TableID
		}
		return tables[i].Name < tables[j].Name
	})

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"tables": tables})
	}

	if len(tables) == 0 {
		u.Err().Println("No tables")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTABLE_ID\tSHEET_ID\tSHEET_TITLE\tA1\tCOLUMNS")
	for _, table := range tables {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%d\n",
			table.Name,
			table.TableID,
			table.SheetID,
			table.SheetTitle,
			table.A1,
			len(table.Columns),
		)
	}
	return nil
}

type SheetsTableGetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string `arg:"" name:"tableId" help:"Table ID or table name"`
}

func (c *SheetsTableGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	in := strings.TrimSpace(c.TableID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if in == "" {
		return usage("empty tableId")
	}

	svc, err := newSheetsService(ctx, account)
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"table": table})
	}

	u.Out().Printf("name\t%s", table.Name)
	u.Out().Printf("id\t%s", table.TableID)
	u.Out().Printf("sheet\t%s", table.SheetTitle)
	u.Out().Printf("a1\t%s", table.A1)
	for _, col := range table.Columns {
		u.Out().Printf("column\t%d\t%s\t%s", col.ColumnIndex, col.ColumnName, col.ColumnType)
	}
	return nil
}

type SheetsTableCreateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Table range (A1 notation with sheet name, or named range name; e.g. Sheet1!A1:C10 or MyNamedRange)"`
	Name          string `name:"name" help:"Table name" required:""`
	ColumnsJSON   string `name:"columns-json" help:"Column definitions as JSON array or @file (columnName + optional columnType; valid types include TEXT, DOUBLE, BOOLEAN, DATE, DROPDOWN)" required:""`
}

func (c *SheetsTableCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(strings.TrimSpace(c.Range))
	name := strings.TrimSpace(c.Name)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if rangeSpec == "" {
		return usage("empty range")
	}
	if name == "" {
		return usage("empty name")
	}

	columns, err := parseSheetsTableColumnsJSON(c.ColumnsJSON)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.table.create", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"name":           name,
		"columns":        columns,
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

	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	gridRange, err := resolveGridRangeWithCatalog(rangeSpec, catalog, "table")
	if err != nil {
		return err
	}

	table := &sheets.Table{
		Name:             name,
		Range:            gridRange,
		ColumnProperties: columns,
	}
	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddTable: &sheets.AddTableRequest{Table: table},
			},
		},
	}

	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return err
	}

	created := table
	if resp != nil && len(resp.Replies) == 1 && resp.Replies[0] != nil && resp.Replies[0].AddTable != nil && resp.Replies[0].AddTable.Table != nil {
		created = resp.Replies[0].AddTable.Table
	}
	item := sheetsTableToItem(created, catalog)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"table": item})
	}

	u.Out().Printf("created\t%s", item.TableID)
	u.Out().Printf("name\t%s", item.Name)
	u.Out().Printf("a1\t%s", item.A1)
	return nil
}

type SheetsTableDeleteCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string `arg:"" name:"tableId" help:"Table ID or table name"`
}

func (c *SheetsTableDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	in := strings.TrimSpace(c.TableID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if in == "" {
		return usage("empty tableId")
	}

	svc, err := newSheetsService(ctx, account)
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

	if dryRunErr := dryRunExit(ctx, flags, "sheets.table.delete", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"table_id":       table.TableID,
		"name":           table.Name,
	}); dryRunErr != nil {
		return dryRunErr
	}
	if err := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), "delete table "+table.Name); err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				DeleteTable: &sheets.DeleteTableRequest{TableId: table.TableID},
			},
		},
	}
	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"deleted": map[string]any{
				"tableId": table.TableID,
				"name":    table.Name,
			},
		})
	}

	u.Out().Printf("deleted\t%s", table.TableID)
	return nil
}

type sheetsTableItem struct {
	Name       string                  `json:"name"`
	TableID    string                  `json:"tableId"`
	SheetID    int64                   `json:"sheetId"`
	SheetTitle string                  `json:"sheetTitle"`
	A1         string                  `json:"a1"`
	DataA1     string                  `json:"dataA1,omitempty"`
	Range      *sheets.GridRange       `json:"range,omitempty"`
	HasFooter  bool                    `json:"hasFooter,omitempty"`
	Columns    []sheetsTableColumnItem `json:"columns,omitempty"`
}

type sheetsTableColumnItem struct {
	ColumnIndex int64  `json:"columnIndex"`
	ColumnName  string `json:"columnName"`
	ColumnType  string `json:"columnType"`
}

func fetchSpreadsheetTables(ctx context.Context, svc *sheets.Service, spreadsheetID string) ([]sheetsTableItem, error) {
	call := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title),tables(tableId,name,range,rowsProperties(footerColorStyle),columnProperties(columnIndex,columnName,columnType,dataValidationRule)))")
	if ctx != nil {
		call = call.Context(ctx)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("get spreadsheet tables: %w", err)
	}

	catalog := &spreadsheetRangeCatalog{
		SheetIDsByTitle: make(map[string]int64, len(resp.Sheets)),
		SheetTitlesByID: make(map[int64]string, len(resp.Sheets)),
	}
	tables := make([]sheetsTableItem, 0)
	for _, sh := range resp.Sheets {
		if sh == nil {
			continue
		}
		if sh.Properties != nil {
			catalog.SheetIDsByTitle[sh.Properties.Title] = sh.Properties.SheetId
			catalog.SheetTitlesByID[sh.Properties.SheetId] = sh.Properties.Title
		}
		for _, table := range sh.Tables {
			if table == nil {
				continue
			}
			tables = append(tables, sheetsTableToItem(table, catalog))
		}
	}
	return tables, nil
}

func sheetsTableToItem(table *sheets.Table, catalog *spreadsheetRangeCatalog) sheetsTableItem {
	if table == nil {
		return sheetsTableItem{}
	}
	item := sheetsTableItem{
		Name:    strings.TrimSpace(table.Name),
		TableID: strings.TrimSpace(table.TableId),
		Range:   table.Range,
	}
	if table.Range != nil {
		item.SheetID = table.Range.SheetId
		if catalog != nil {
			item.SheetTitle = catalog.SheetTitlesByID[table.Range.SheetId]
		}
		if item.SheetTitle != "" {
			item.A1 = gridRangeToA1(item.SheetTitle, table.Range)
			if dataA1, ok := sheetsTableDataRangeA1(item.SheetTitle, table); ok {
				item.DataA1 = dataA1
			}
		}
	}
	item.HasFooter = sheetsTableHasFooter(table)
	for _, col := range table.ColumnProperties {
		if col == nil {
			continue
		}
		item.Columns = append(item.Columns, sheetsTableColumnItem{
			ColumnIndex: col.ColumnIndex,
			ColumnName:  strings.TrimSpace(col.ColumnName),
			ColumnType:  strings.TrimSpace(col.ColumnType),
		})
	}
	sort.Slice(item.Columns, func(i, j int) bool {
		return item.Columns[i].ColumnIndex < item.Columns[j].ColumnIndex
	})
	return item
}

func resolveSheetsTable(input string, tables []sheetsTableItem) (sheetsTableItem, bool, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return sheetsTableItem{}, false, nil
	}

	for _, table := range tables {
		if table.TableID == in {
			return table, true, nil
		}
	}

	var matches []sheetsTableItem
	for _, table := range tables {
		if strings.EqualFold(table.Name, in) {
			matches = append(matches, table)
		}
	}
	switch len(matches) {
	case 0:
		return sheetsTableItem{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		parts := make([]string, 0, len(matches))
		for _, match := range matches {
			parts = append(parts, fmt.Sprintf("%s (%s)", match.Name, match.TableID))
		}
		return sheetsTableItem{}, false, usagef("ambiguous table %q; matches: %s", in, strings.Join(parts, ", "))
	}
}
