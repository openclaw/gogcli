package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsUpsertCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID or URL"`
	Range         string `arg:"" name:"range" help:"Bounded table range including its header row and sheet name"`
	Key           string `name:"key" required:"" help:"Unique string key column header"`
	Columns       string `name:"columns" required:"" help:"Comma-separated column headers to update (key is immutable on existing rows)"`
	RowsJSON      string `name:"rows-json" required:"" help:"Array of objects keyed by header names, inline or @file; null skips a field"`
}

type sheetsUpsertPlan struct {
	Updates  []*sheets.ValueRange `json:"updates"`
	Inserted int                  `json:"inserted"`
	Matched  int                  `json:"matched"`
}

func (c *SheetsUpsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if id == "" {
		return usage("empty spreadsheetId")
	}
	r, err := sheetsa1.Parse(c.Range)
	if err != nil || r.SheetName == "" || r.StartRow < 1 || r.EndRow < 1 || r.StartCol < 1 || r.EndCol < 1 {
		return usage("range must be a bounded A1 rectangle including the header row and sheet name")
	}
	b, err := resolveInlineOrFileBytes(c.RowsJSON, stdinReader(ctx))
	if err != nil {
		return err
	}
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		return usagef("invalid rows JSON: %v", err)
	}
	if len(rows) == 0 {
		return usage("rows must not be empty")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}
	read := func() ([][]any, error) {
		vr, readErr := svc.Spreadsheets.Values.Get(id, c.Range).ValueRenderOption("FORMULA").Context(ctx).Do()
		if readErr != nil {
			return nil, readErr
		}
		return vr.Values, nil
	}
	current, err := read()
	if err != nil {
		return err
	}
	plan, err := planSheetsUpsert(r, current, rows, c.Key, splitCommaList(c.Columns))
	if err != nil {
		return usage(err.Error())
	}
	dryRun := flags != nil && flags.DryRun
	if !dryRun && len(plan.Updates) > 0 {
		latest, readErr := read()
		if readErr != nil {
			return readErr
		}
		if !reflect.DeepEqual(current, latest) {
			return fmt.Errorf("table changed during planning; no updates submitted")
		}
		_, err = svc.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{Data: plan.Updates, ValueInputOption: "RAW"}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("submit upsert (completion may be unknown; read the table before retrying): %w", err)
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"spreadsheetId": id, "dry_run": dryRun, "plan": plan})
	}
	ui.FromContext(ctx).Out().Linef("matched\t%d\ninserted\t%d\ncells\t%d\ndry_run\t%t", plan.Matched, plan.Inserted, len(plan.Updates), dryRun)
	return nil
}

func planSheetsUpsert(r sheetsa1.Range, current [][]any, rows []map[string]any, key string, columns []string) (sheetsUpsertPlan, error) {
	plan := sheetsUpsertPlan{Updates: []*sheets.ValueRange{}}
	if len(current) == 0 {
		return plan, fmt.Errorf("destination header row is empty")
	}
	headers := map[string]int{}
	for j, cell := range current[0] {
		name, ok := cell.(string)
		if !ok || name == "" {
			continue
		}
		if _, exists := headers[name]; exists {
			return plan, fmt.Errorf("duplicate header %q", name)
		}
		headers[name] = j
	}
	keyCol, exists := headers[key]
	if !exists {
		return plan, fmt.Errorf("key header %q not found", key)
	}
	if len(columns) == 0 {
		return plan, fmt.Errorf("--columns must not be empty")
	}
	selected := map[string]bool{}
	for _, name := range columns {
		if _, exists := headers[name]; !exists || name == key || selected[name] {
			return plan, fmt.Errorf("unknown, duplicate, or key column %q", name)
		}
		selected[name] = true
	}
	keys := map[string]int{}
	for i, row := range current[1:] {
		if keyCol >= len(row) || row[keyCol] == "" || row[keyCol] == nil {
			for _, v := range row {
				if v != nil && v != "" {
					return plan, fmt.Errorf("row %d has data but no key", r.StartRow+i+1)
				}
			}
			continue
		}
		value, ok := row[keyCol].(string)
		if !ok || strings.HasPrefix(value, "=") {
			return plan, fmt.Errorf("row %d key must be a literal string", r.StartRow+i+1)
		}
		if _, exists := keys[value]; exists {
			return plan, fmt.Errorf("duplicate destination key %q", value)
		}
		keys[value] = i + 1
	}
	seen := map[string]bool{}
	next := len(current)
	add := func(row, col int, value any) {
		plan.Updates = append(plan.Updates, &sheets.ValueRange{Range: sheetsa1.FormatCell(r.SheetName, r.StartRow+row, r.StartCol+col), Values: [][]any{{value}}})
	}
	for _, record := range rows {
		value, ok := record[key].(string)
		if !ok || value == "" || strings.HasPrefix(value, "=") || seen[value] {
			return plan, fmt.Errorf("missing, non-string, formula-like, or duplicate input key %q", value)
		}
		seen[value] = true
		for name, v := range record {
			if name != key && !selected[name] {
				return plan, fmt.Errorf("input field %q is not selected", name)
			}
			switch v.(type) {
			case nil, string, float64, bool:
			default:
				return plan, fmt.Errorf("field %q must be a scalar or null", name)
			}
		}
		index, found := keys[value]
		if !found {
			index = next
			next++
			if r.StartRow+index > r.EndRow {
				return plan, fmt.Errorf("new rows exceed destination range; enlarge its row bound")
			}
			add(index, keyCol, value)
			plan.Inserted++
		} else {
			plan.Matched++
		}
		for _, name := range columns {
			v, present := record[name]
			if !present || v == nil {
				continue
			}
			column := headers[name]
			var old any = ""
			if found && column < len(current[index]) {
				old = current[index][column]
			}
			if !reflect.DeepEqual(old, v) {
				add(index, column, v)
			}
		}
	}
	return plan, nil
}
