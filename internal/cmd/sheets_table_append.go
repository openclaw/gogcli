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

type SheetsTableAppendCmd struct {
	SpreadsheetID string   `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	TableID       string   `arg:"" name:"tableId" help:"Table ID or table name"`
	Values        []string `arg:"" optional:"" name:"values" help:"Values (comma-separated rows, pipe-separated cells)"`
	ValueInput    string   `name:"input" help:"Value input option: RAW or USER_ENTERED" default:"USER_ENTERED"`
	ValuesJSON    string   `name:"values-json" help:"Values as JSON 2D array"`
}

func (c *SheetsTableAppendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	in := strings.TrimSpace(c.TableID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if in == "" {
		return usage("empty tableId")
	}

	values, err := parseSheetsAppendValues(c.ValuesJSON, c.Values)
	if err != nil {
		return err
	}
	valueInputOption := strings.TrimSpace(c.ValueInput)
	if valueInputOption == "" {
		valueInputOption = sheetsDefaultValueInputOption
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
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
	if strings.TrimSpace(table.A1) == "" {
		return fmt.Errorf("table %q has no bounded A1 range", table.TableID)
	}
	if widthErr := validateSheetsTableAppendWidth(table, values); widthErr != nil {
		return widthErr
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.table.append", map[string]any{
		"spreadsheet_id":     spreadsheetID,
		"table_id":           table.TableID,
		"name":               table.Name,
		"range":              table.A1,
		"values":             values,
		"value_input_option": valueInputOption,
		"insert_data_option": "INSERT_ROWS",
	}); dryRunErr != nil {
		return dryRunErr
	}

	resp, err := svc.Spreadsheets.Values.Append(spreadsheetID, table.A1, &sheets.ValueRange{Values: values}).
		ValueInputOption(valueInputOption).
		InsertDataOption("INSERT_ROWS").
		Do()
	if err != nil {
		return err
	}
	if resp == nil || resp.Updates == nil {
		return fmt.Errorf("append response missing update metadata")
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"tableId":        table.TableID,
			"name":           table.Name,
			"tableRange":     table.A1,
			"updatedRange":   resp.Updates.UpdatedRange,
			"updatedRows":    resp.Updates.UpdatedRows,
			"updatedColumns": resp.Updates.UpdatedColumns,
			"updatedCells":   resp.Updates.UpdatedCells,
		})
	}

	u.Out().Printf("Appended %d cells to %s", resp.Updates.UpdatedCells, resp.Updates.UpdatedRange)
	return nil
}

func parseSheetsAppendValues(valuesJSON string, values []string) ([][]interface{}, error) {
	switch {
	case strings.TrimSpace(valuesJSON) != "":
		b, err := resolveInlineOrFileBytes(valuesJSON)
		if err != nil {
			return nil, fmt.Errorf("read --values-json: %w", err)
		}
		var parsed [][]interface{}
		if err := json.Unmarshal(b, &parsed); err != nil {
			return nil, fmt.Errorf("invalid JSON values: %w", err)
		}
		if len(parsed) == 0 {
			return nil, fmt.Errorf("provide at least one row")
		}
		return parsed, nil
	case len(values) > 0:
		rawValues := strings.Join(values, " ")
		rows := strings.Split(rawValues, ",")
		parsed := make([][]interface{}, 0, len(rows))
		for _, row := range rows {
			cells := strings.Split(strings.TrimSpace(row), "|")
			rowData := make([]interface{}, len(cells))
			for i, cell := range cells {
				rowData[i] = strings.TrimSpace(cell)
			}
			parsed = append(parsed, rowData)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("provide values as args or via --values-json")
	}
}

func validateSheetsTableAppendWidth(table sheetsTableItem, values [][]interface{}) error {
	if len(table.Columns) == 0 {
		return nil
	}
	width := len(table.Columns)
	for i, row := range values {
		if len(row) > width {
			return usagef("row %d has %d cells, but table %q has %d columns", i+1, len(row), table.Name, width)
		}
	}
	return nil
}
