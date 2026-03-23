package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsTableCmd struct {
	List   SheetsTableListCmd   `cmd:"" name:"list" aliases:"ls" help:"List all tables in a spreadsheet"`
	Get    SheetsTableGetCmd    `cmd:"" name:"get" aliases:"read,show" help:"Get details of a specific table"`
	Create SheetsTableCreateCmd `cmd:"" name:"create" aliases:"new" help:"Create a new table"`
	Update SheetsTableUpdateCmd `cmd:"" name:"update" aliases:"edit" help:"Update table properties"`
	Append SheetsTableAppendCmd `cmd:"" name:"append" aliases:"add" help:"Append rows to a table"`
	Clear  SheetsTableClearCmd  `cmd:"" name:"clear" help:"Clear table contents"`
	Delete SheetsTableDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove" help:"Delete a table"`
}

// SheetsTableListCmd lists all tables in a spreadsheet
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

	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.properties.title,sheets.tables").Do()
	if err != nil {
		return err
	}

	var tables []map[string]any
	for _, sheet := range resp.Sheets {
		for _, table := range sheet.Tables {
			hasFooter := table.RowsProperties != nil && table.RowsProperties.FooterColorStyle != nil
			tables = append(tables, map[string]any{
				"tableId":     table.TableId,
				"tableName":   table.Name,
				"sheetId":     table.Range.SheetId,
				"sheetName":   sheet.Properties.Title,
				"range":       formatGridRange(table.Range, sheet.Properties.Title),
				"columnCount": len(table.ColumnProperties),
				"hasFooter":   hasFooter,
			})
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tables":        tables,
		})
	}

	if len(tables) == 0 {
		u.Out().Println("No tables found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TABLE_ID\tTABLE_NAME\tSHEET\tRANGE\tCOLUMNS\tHAS_FOOTER")
	for _, t := range tables {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%v\n",
			t["tableId"],
			t["tableName"],
			t["sheetName"],
			t["range"],
			t["columnCount"],
			t["hasFooter"],
		)
	}
	return nil
}

// SheetsTableGetCmd gets details of a specific table
type SheetsTableGetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string `arg:"" name:"tableId" help:"Table ID"`
}

func (c *SheetsTableGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.TableID == "" {
		return usage("empty tableId")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.properties.title,sheets.tables").Do()
	if err != nil {
		return err
	}

	var foundTable *sheets.Table
	var sheetName string
	for _, sheet := range resp.Sheets {
		for _, table := range sheet.Tables {
			if table.TableId == c.TableID {
				foundTable = table
				sheetName = sheet.Properties.Title
				break
			}
		}
		if foundTable != nil {
			break
		}
	}

	if foundTable == nil {
		return fmt.Errorf("table %s not found in spreadsheet %s", c.TableID, spreadsheetID)
	}

	columns := make([]map[string]any, len(foundTable.ColumnProperties))
	for i, col := range foundTable.ColumnProperties {
		colInfo := map[string]any{
			"columnIndex": col.ColumnIndex,
			"columnName":  col.ColumnName,
			"columnType":  col.ColumnType,
		}
		if col.DataValidationRule != nil {
			colInfo["dataValidation"] = col.DataValidationRule
		}
		columns[i] = colInfo
	}

	if outfmt.IsJSON(ctx) {
		hasFooter := foundTable.RowsProperties != nil && foundTable.RowsProperties.FooterColorStyle != nil
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tableId":       foundTable.TableId,
			"tableName":     foundTable.Name,
			"sheetName":     sheetName,
			"range":         formatGridRange(foundTable.Range, sheetName),
			"hasFooter":     hasFooter,
			"columns":       columns,
		})
	}

	u.Out().Printf("Table ID: %s", foundTable.TableId)
	u.Out().Printf("Name: %s", foundTable.Name)
	u.Out().Printf("Sheet: %s", sheetName)
	u.Out().Printf("Range: %s", formatGridRange(foundTable.Range, sheetName))
	u.Out().Printf("Has Footer: %v", foundTable.RowsProperties != nil && foundTable.RowsProperties.FooterColorStyle != nil)
	u.Out().Println("")
	u.Out().Println("Columns:")

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "INDEX\tNAME\tTYPE")
	for _, col := range columns {
		fmt.Fprintf(w, "%d\t%s\t%s\n", col["columnIndex"], col["columnName"], col["columnType"])
	}

	return nil
}

// SheetsTableCreateCmd creates a new table
type SheetsTableCreateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Table range (A1 notation, e.g. Sheet1!A1:D10)"`
	Name          string `name:"name" help:"Table name" required:""`
	ColumnsJSON   string `name:"columns-json" help:"Column definitions as JSON array [{\"columnName\":\"...\",\"columnType\":\"TEXT|NUMBER|DATE|DROPDOWN|CHECKBOX|PERCENT|RATING|CURRENCY|DATE_TIME|TIME|SMART_CHIP\",\"dataValidation\":{...}}]" required:""`
	HasFooter     bool   `name:"footer" help:"Enable table footer" default:"false"`
}

func (c *SheetsTableCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("empty name")
	}

	var columnDefs []struct {
		ColumnName     string                                 `json:"columnName"`
		ColumnType     string                                 `json:"columnType"`
		DataValidation *sheets.TableColumnDataValidationRule  `json:"dataValidation,omitempty"`
	}

	columnsData, err := resolveInlineOrFileBytes(c.ColumnsJSON)
	if err != nil {
		return fmt.Errorf("read --columns-json: %w", err)
	}
	if err := json.Unmarshal(columnsData, &columnDefs); err != nil {
		return fmt.Errorf("invalid columns JSON: %w", err)
	}

	if len(columnDefs) == 0 {
		return fmt.Errorf("at least one column required")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	gridRange, sheetName, err := parseA1ToGridRangeWithLookup(ctx, svc, spreadsheetID, rangeSpec)
	if err != nil {
		return fmt.Errorf("invalid range: %w", err)
	}

	columnProperties := make([]*sheets.TableColumnProperties, len(columnDefs))
	for i, def := range columnDefs {
		colProp := &sheets.TableColumnProperties{
			ColumnIndex: int64(i),
			ColumnName:  def.ColumnName,
			ColumnType:  def.ColumnType,
		}
		if def.DataValidation != nil {
			colProp.DataValidationRule = def.DataValidation
		}
		columnProperties[i] = colProp
	}

	table := &sheets.Table{
		Name:             name,
		Range:            gridRange,
		ColumnProperties: columnProperties,
	}
	if c.HasFooter {
		table.RowsProperties = &sheets.TableRowsProperties{
			FooterColorStyle: &sheets.ColorStyle{},
		}
	}

	if err := dryRunExit(ctx, flags, "sheets.table.create", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"table_name":     name,
		"range":          rangeSpec,
		"sheet":          sheetName,
		"has_footer":     c.HasFooter,
		"columns":        columnDefs,
	}); err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddTable: &sheets.AddTableRequest{
					Table: table,
				},
			},
		},
	}

	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return err
	}

	var tableID string
	if len(resp.Replies) > 0 && resp.Replies[0].AddTable != nil {
		tableID = resp.Replies[0].AddTable.Table.TableId
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tableId":       tableID,
			"tableName":     name,
			"range":         rangeSpec,
			"hasFooter":     c.HasFooter,
		})
	}

	u.Out().Printf("Created table %q (ID: %s) in %s", name, tableID, spreadsheetID)
	return nil
}

// SheetsTableUpdateCmd updates table properties
type SheetsTableUpdateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string `arg:"" name:"tableId" help:"Table ID"`
	Name          string `name:"name" help:"New table name"`
	Range         string `name:"range" help:"New table range (A1 notation)"`
	Footer        string `name:"footer" help:"Toggle footer: true|false"`
}

func (c *SheetsTableUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.TableID == "" {
		return usage("empty tableId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.tables").Do()
	if err != nil {
		return err
	}

	var existingTable *sheets.Table
	for _, sheet := range resp.Sheets {
		for _, table := range sheet.Tables {
			if table.TableId == c.TableID {
				existingTable = table
				break
			}
		}
		if existingTable != nil {
			break
		}
	}

	if existingTable == nil {
		return fmt.Errorf("table %s not found in spreadsheet %s", c.TableID, spreadsheetID)
	}

	updateReq := &sheets.UpdateTableRequest{
		Table: &sheets.Table{TableId: c.TableID},
	}

	fields := []string{}

	if strings.TrimSpace(c.Name) != "" {
		updateReq.Table.Name = strings.TrimSpace(c.Name)
		fields = append(fields, "name")
	}

	if strings.TrimSpace(c.Range) != "" {
		gridRange, _, err := parseA1ToGridRangeWithLookup(ctx, svc, spreadsheetID, cleanRange(c.Range))
		if err != nil {
			return fmt.Errorf("invalid range: %w", err)
		}
		updateReq.Table.Range = gridRange
		fields = append(fields, "range")
	}

	if strings.TrimSpace(c.Footer) != "" {
		footerVal := strings.ToLower(strings.TrimSpace(c.Footer))
		switch footerVal {
		case "true", "1", "yes":
			updateReq.Table.RowsProperties = &sheets.TableRowsProperties{
				FooterColorStyle: &sheets.ColorStyle{},
			}
			fields = append(fields, "rowsProperties.footerColorStyle")
		case "false", "0", "no":
			updateReq.Table.RowsProperties = &sheets.TableRowsProperties{
				FooterColorStyle: nil,
			}
			fields = append(fields, "rowsProperties.footerColorStyle")
		default:
			return fmt.Errorf("invalid footer value: %s (use true/false)", c.Footer)
		}
	}

	updateReq.Fields = strings.Join(fields, ",")

	if err := dryRunExit(ctx, flags, "sheets.table.update", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"table_id":       c.TableID,
		"updates": map[string]any{
			"name":   updateReq.Table.Name,
			"range":  c.Range,
			"footer": c.Footer,
		},
	}); err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				UpdateTable: updateReq,
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tableId":       c.TableID,
			"updatedFields": fields,
		})
	}

	u.Out().Printf("Updated table %s in %s", c.TableID, spreadsheetID)
	return nil
}

// SheetsTableAppendCmd appends rows to a table
type SheetsTableAppendCmd struct {
	SpreadsheetID string   `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string   `arg:"" name:"tableId" help:"Table ID"`
	Values        []string `arg:"" optional:"" name:"values" help:"Values (comma-separated rows, pipe-separated cells)"`
	ValuesJSON    string   `name:"values-json" help:"Values as JSON 2D array"`
	ValueInput    string   `name:"input" help:"Value input option: RAW or USER_ENTERED" default:"USER_ENTERED"`
}

func (c *SheetsTableAppendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.TableID == "" {
		return usage("empty tableId")
	}

	var values [][]interface{}

	switch {
	case strings.TrimSpace(c.ValuesJSON) != "":
		b, err := resolveInlineOrFileBytes(c.ValuesJSON)
		if err != nil {
			return fmt.Errorf("read --values-json: %w", err)
		}
		if unmarshalErr := json.Unmarshal(b, &values); unmarshalErr != nil {
			return fmt.Errorf("invalid JSON values: %w", unmarshalErr)
		}
	case len(c.Values) > 0:
		rawValues := strings.Join(c.Values, " ")
		rows := strings.Split(rawValues, ",")
		for _, row := range rows {
			cells := strings.Split(strings.TrimSpace(row), "|")
			rowData := make([]interface{}, len(cells))
			for i, cell := range cells {
				rowData[i] = strings.TrimSpace(cell)
			}
			values = append(values, rowData)
		}
	default:
		return fmt.Errorf("provide values as args or via --values-json")
	}

	valueInputOption := strings.TrimSpace(c.ValueInput)
	if valueInputOption == "" {
		valueInputOption = "USER_ENTERED"
	}

	if err := dryRunExit(ctx, flags, "sheets.table.append", map[string]any{
		"spreadsheet_id":     spreadsheetID,
		"table_id":           c.TableID,
		"values":             values,
		"value_input_option": valueInputOption,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	// Fetch table metadata before append to check for footer
	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.properties.title,sheets.tables").Do()
	if err != nil {
		return fmt.Errorf("fetch table metadata: %w", err)
	}

	var table *sheets.Table
	var sheetName string
	var originalRange string
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		for _, t := range sheet.Tables {
			if t != nil && t.TableId == c.TableID {
				table = t
				sheetName = sheet.Properties.Title
				originalRange = formatGridRange(t.Range, sheetName)
				break
			}
		}
		if table != nil {
			break
		}
	}

	if table == nil {
		return fmt.Errorf("table %s not found in spreadsheet %s", c.TableID, spreadsheetID)
	}

	// Check if table has footer and capture footer formulas
	hasFooter := table.RowsProperties != nil && table.RowsProperties.FooterColorStyle != nil
	var footerFormulas []string
	if hasFooter && table.Range != nil {
		// Calculate footer row (last row of table range)
		footerRow := table.Range.EndRowIndex - 1 // 0-indexed, so -1 gives last row index
		// Fetch footer row values/formulas
		footerRange := fmt.Sprintf("%s!%s%d:%s%d", 
			sheetName,
			columnIndexToLetter(int(table.Range.StartColumnIndex)),
			footerRow+1,
			columnIndexToLetter(int(table.Range.EndColumnIndex-1)),
			footerRow+1)
		valResp, _ := svc.Spreadsheets.Values.Get(spreadsheetID, footerRange).ValueRenderOption("FORMULA").Do()
		if valResp != nil && len(valResp.Values) > 0 {
			for _, cell := range valResp.Values[0] {
				if str, ok := cell.(string); ok && strings.HasPrefix(str, "=") {
					footerFormulas = append(footerFormulas, str)
				} else {
					footerFormulas = append(footerFormulas, "") // No formula in this cell
				}
			}
		}
	}

	// Use Values.Append with the table's actual range (respects sheet location)
	// Build range from table: SheetName!StartCol:EndCol (append will find first empty row)
	appendRange := fmt.Sprintf("%s!%s:%s",
		sheetName,
		columnIndexToLetter(int(table.Range.StartColumnIndex)),
		columnIndexToLetter(int(table.Range.EndColumnIndex-1)))
	
	vr := &sheets.ValueRange{
		Values: values,
	}
	
	appendResp, err := svc.Spreadsheets.Values.Append(spreadsheetID, appendRange, vr).
		ValueInputOption(valueInputOption).
		InsertDataOption("INSERT_ROWS").
		Do()
	if err != nil {
		return fmt.Errorf("append to table: %w", err)
	}

	// Check if table expanded and restore footer formulas if needed
	if hasFooter && len(footerFormulas) > 0 {
		// Fetch updated table to check if range changed
		updatedResp, fetchErr := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.properties.title,sheets.tables").Do()
		if fetchErr == nil {
			var updatedTable *sheets.Table
			var updatedSheetName string
			for _, sheet := range updatedResp.Sheets {
				if sheet == nil || sheet.Properties == nil {
					continue
				}
				for _, t := range sheet.Tables {
					if t != nil && t.TableId == c.TableID {
						updatedTable = t
						updatedSheetName = sheet.Properties.Title
						break
					}
				}
				if updatedTable != nil {
					break
				}
			}
			
			if updatedTable != nil && updatedTable.Range != nil {
				newRange := formatGridRange(updatedTable.Range, updatedSheetName)
				if newRange != originalRange {
					// Table expanded - re-apply footer formulas
					newFooterRow := updatedTable.Range.EndRowIndex - 1
					newFooterRange := fmt.Sprintf("%s!%s%d:%s%d",
						updatedSheetName,
						columnIndexToLetter(int(updatedTable.Range.StartColumnIndex)),
						newFooterRow+1,
						columnIndexToLetter(int(updatedTable.Range.EndColumnIndex-1)),
						newFooterRow+1)
					
					// Re-apply formulas using sheets.Values.Update
					formulaValues := make([][]interface{}, 1)
					formulaValues[0] = make([]interface{}, len(footerFormulas))
					for i, formula := range footerFormulas {
						if formula != "" {
							formulaValues[0][i] = formula
						}
					}
					
					_, _ = svc.Spreadsheets.Values.Update(spreadsheetID, newFooterRange, 
						&sheets.ValueRange{Values: formulaValues}).
						ValueInputOption("USER_ENTERED").
						Do()
				}
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tableId":       c.TableID,
			"appendedRows":  len(values),
			"updatedRange":  appendResp.Updates.UpdatedRange,
		})
	}

	u.Out().Printf("Appended %d rows to table %s", len(values), c.TableID)
	return nil
}

// SheetsTableClearCmd clears table contents
type SheetsTableClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string `arg:"" name:"tableId" help:"Table ID"`
}

func (c *SheetsTableClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.TableID == "" {
		return usage("empty tableId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.properties.title,sheets.tables").Do()
	if err != nil {
		return err
	}

	var table *sheets.Table
	var sheetName string
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		for _, t := range sheet.Tables {
			if t != nil && t.TableId == c.TableID {
				table = t
				sheetName = sheet.Properties.Title
				break
			}
		}
		if table != nil {
			break
		}
	}

	if table == nil {
		return fmt.Errorf("table %s not found in spreadsheet %s", c.TableID, spreadsheetID)
	}

	if err := dryRunExit(ctx, flags, "sheets.table.clear", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"table_id":       c.TableID,
		"table_name":     table.Name,
		"range":          formatGridRange(table.Range, sheetName),
	}); err != nil {
		return err
	}

	if err := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), "clear table "+table.Name); err != nil {
		return err
	}

	clearResp, err := svc.Spreadsheets.Values.Clear(spreadsheetID, formatGridRange(table.Range, sheetName), &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tableId":       c.TableID,
			"clearedRange":  clearResp.ClearedRange,
		})
	}

	u.Out().Printf("Cleared table %s (%s)", c.TableID, clearResp.ClearedRange)
	return nil
}

// SheetsTableDeleteCmd deletes a table
type SheetsTableDeleteCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string `arg:"" name:"tableId" help:"Table ID"`
	KeepData      bool   `name:"keep-data" help:"Remove table formatting only, keep data"`
}

func (c *SheetsTableDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.TableID == "" {
		return usage("empty tableId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.tables").Do()
	if err != nil {
		return err
	}

	var table *sheets.Table
	for _, sheet := range resp.Sheets {
		for _, t := range sheet.Tables {
			if t.TableId == c.TableID {
				table = t
				break
			}
		}
		if table != nil {
			break
		}
	}

	if table == nil {
		return fmt.Errorf("table %s not found in spreadsheet %s", c.TableID, spreadsheetID)
	}

	opName := "delete"
	if c.KeepData {
		opName = "unformat (keep data)"
	}

	if err := dryRunExit(ctx, flags, "sheets.table."+strings.ReplaceAll(opName, " ", "_"), map[string]any{
		"spreadsheet_id": spreadsheetID,
		"table_id":       c.TableID,
		"table_name":     table.Name,
		"keep_data":      c.KeepData,
	}); err != nil {
		return err
	}

	if err := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), opName+" table "+table.Name); err != nil {
		return err
	}

	var req *sheets.BatchUpdateSpreadsheetRequest

	if c.KeepData {
		// Find and remove banding (table formatting)
		bandedRanges, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.bandedRanges").Do()
		if err != nil {
			return err
		}

		var bandingID int64
		for _, sheet := range bandedRanges.Sheets {
			for _, br := range sheet.BandedRanges {
				if rangesEqual(br.Range, table.Range) {
					bandingID = br.BandedRangeId
					break
				}
			}
			if bandingID != 0 {
				break
			}
		}

		if bandingID != 0 {
			req = &sheets.BatchUpdateSpreadsheetRequest{
				Requests: []*sheets.Request{
					{
						DeleteBanding: &sheets.DeleteBandingRequest{
							BandedRangeId: bandingID,
						},
					},
				},
			}
		}
	} else {
		req = &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{
				{
					DeleteTable: &sheets.DeleteTableRequest{
						TableId: c.TableID,
					},
				},
			},
		}
	}

	if req != nil {
		if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"tableId":       c.TableID,
			"deleted":       !c.KeepData,
			"unformatted":   c.KeepData,
		})
	}

	if c.KeepData {
		u.Out().Printf("Removed table formatting from table %s (data preserved)", c.TableID)
	} else {
		u.Out().Printf("Deleted table %s", c.TableID)
	}
	return nil
}

// Helper functions

func formatGridRange(r *sheets.GridRange, sheetName string) string {
	if r == nil {
		return ""
	}
	// Convert to A1 notation
	startCol := columnIndexToLetter(int(r.StartColumnIndex))
	endCol := columnIndexToLetter(int(r.EndColumnIndex - 1))
	if sheetName == "" {
		sheetName = "Sheet1" // Default fallback
	}
	return fmt.Sprintf("%s!%s%d:%s%d", sheetName, startCol, r.StartRowIndex+1, endCol, r.EndRowIndex)
}

func columnIndexToLetter(index int) string {
	if index < 0 {
		return ""
	}
	result := ""
	for index >= 0 {
		result = string(rune('A'+index%26)) + result
		index = index/26 - 1
	}
	return result
}

func resolveSheetID(ctx context.Context, svc *sheets.Service, spreadsheetID string, sheetName string) (int64, error) {
	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.properties").Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("fetch spreadsheet metadata: %w", err)
	}

	for _, sheet := range resp.Sheets {
		if sheet != nil && sheet.Properties != nil && sheet.Properties.Title == sheetName {
			return sheet.Properties.SheetId, nil
		}
	}

	return 0, fmt.Errorf("sheet %q not found in spreadsheet", sheetName)
}

func parseA1ToGridRangeWithLookup(ctx context.Context, svc *sheets.Service, spreadsheetID string, a1 string) (*sheets.GridRange, string, error) {
	// Parse the A1 notation
	parts := strings.Split(a1, "!")
	var sheetName, rangePart string
	if len(parts) == 2 {
		sheetName = parts[0]
		rangePart = parts[1]
	} else {
		rangePart = parts[0]
		sheetName = "Sheet1" // Default
	}

	rangeParts := strings.Split(rangePart, ":")
	if len(rangeParts) != 2 {
		return nil, "", fmt.Errorf("invalid range format, expected Sheet1!A1:D10 or A1:D10")
	}

	startCol, startRow, err := parseCellRef(rangeParts[0])
	if err != nil {
		return nil, "", fmt.Errorf("invalid start cell: %w", err)
	}
	endCol, endRow, err := parseCellRef(rangeParts[1])
	if err != nil {
		return nil, "", fmt.Errorf("invalid end cell: %w", err)
	}

	// Look up sheet ID from name
	sheetID, err := resolveSheetID(ctx, svc, spreadsheetID, sheetName)
	if err != nil {
		return nil, "", err
	}

	return &sheets.GridRange{
		SheetId:          sheetID,
		StartRowIndex:    int64(startRow - 1),
		EndRowIndex:      int64(endRow),
		StartColumnIndex: int64(startCol),
		EndColumnIndex:   int64(endCol + 1),
	}, sheetName, nil
}

func parseCellRef(ref string) (col, row int, err error) {
	col = 0
	row = 0
	i := 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		col = col*26 + int(ref[i]-'A') + 1
		i++
	}
	if i == 0 {
		return 0, 0, fmt.Errorf("no column letter found")
	}
	col-- // Convert to 0-indexed

	for i < len(ref) && ref[i] >= '0' && ref[i] <= '9' {
		row = row*10 + int(ref[i]-'0')
		i++
	}
	if row == 0 {
		return 0, 0, fmt.Errorf("no row number found")
	}

	return col, row, nil
}

func rangesEqual(r1, r2 *sheets.GridRange) bool {
	if r1 == nil || r2 == nil {
		return r1 == r2
	}
	return r1.SheetId == r2.SheetId &&
		r1.StartRowIndex == r2.StartRowIndex &&
		r1.EndRowIndex == r2.EndRowIndex &&
		r1.StartColumnIndex == r2.StartColumnIndex &&
		r1.EndColumnIndex == r2.EndColumnIndex
}
