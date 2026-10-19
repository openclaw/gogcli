package cmd

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsReconcileCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID or URL"`
	DataJSON      string `name:"data-json" required:"" help:"ValueRange array, @file, or @-; null skips a cell, empty string clears it"`
	Apply         bool   `name:"apply" help:"Apply differences after rereading the target; default only previews"`
}

type sheetsCellChange struct {
	Range  string `json:"range"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

func (c *SheetsReconcileCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if id == "" {
		return usage("empty spreadsheetId")
	}
	data, err := parseSheetsBatchUpdateData(c.DataJSON, stdinReader(ctx))
	if err != nil {
		return err
	}
	if validationErr := validateSheetsReconcileRanges(data); validationErr != nil {
		return usage(validationErr.Error())
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}
	before, err := readSheetsReconcile(ctx, svc, id, data)
	if err != nil {
		return err
	}
	changes := diffSheetsReconcile(data, before)
	result := map[string]any{"spreadsheetId": id, "changes": changes, "applied": false, "verified": false}
	apply := c.Apply && (flags == nil || !flags.DryRun)
	if !apply {
		return writeSheetsReconcileResult(ctx, result, len(changes), false)
	}
	current, err := readSheetsReconcile(ctx, svc, id, data)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, current) {
		return fmt.Errorf("target changed during planning; no writes submitted, preview again")
	}
	if len(changes) > 0 {
		updates := make([]*sheets.ValueRange, 0, len(changes))
		for _, change := range changes {
			updates = append(updates, &sheets.ValueRange{Range: change.Range, Values: [][]any{{change.After}}})
		}
		_, err = svc.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{ValueInputOption: "RAW", Data: updates}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("apply differences (completion may be unknown; preview before retrying): %w", err)
		}
	}
	after, err := readSheetsReconcile(ctx, svc, id, data)
	if err != nil {
		return fmt.Errorf("values submitted but readback failed: %w", err)
	}
	mismatches := diffSheetsReconcile(data, after)
	result["applied"] = true
	result["verified"] = len(mismatches) == 0
	result["mismatches"] = mismatches
	if err := writeSheetsReconcileResult(ctx, result, len(changes), true); err != nil {
		return err
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("readback differs in %d cells; changes were already submitted; preview again before retrying", len(mismatches))
	}
	return nil
}

func validateSheetsReconcileRanges(data []*sheets.ValueRange) error {
	if len(data) == 0 {
		return fmt.Errorf("empty data")
	}
	seen := map[string]bool{}
	for _, vr := range data {
		r, err := sheetsa1.Parse(vr.Range)
		if err != nil || r.SheetName == "" || r.StartRow < 1 || r.StartCol < 1 || r.EndRow < 1 || r.EndCol < 1 {
			return fmt.Errorf("each range must be a bounded A1 rectangle including its sheet name")
		}
		if vr.MajorDimension != "" && vr.MajorDimension != "ROWS" {
			return fmt.Errorf("only ROWS major dimension is supported")
		}
		if len(vr.Values) > r.EndRow-r.StartRow+1 {
			return fmt.Errorf("values exceed range %s", vr.Range)
		}
		for i, row := range vr.Values {
			if len(row) > r.EndCol-r.StartCol+1 {
				return fmt.Errorf("values exceed range %s", vr.Range)
			}
			for j, value := range row {
				if value == nil {
					continue
				}
				switch value.(type) {
				case string, float64, bool:
				default:
					return fmt.Errorf("values must be strings, numbers, booleans, or null")
				}
				cell := sheetsa1.FormatCell(r.SheetName, r.StartRow+i, r.StartCol+j)
				if seen[cell] {
					return fmt.Errorf("overlapping input cell %s", cell)
				}
				seen[cell] = true
			}
		}
	}
	if len(seen) == 0 {
		return fmt.Errorf("no values to reconcile")
	}
	return nil
}

func readSheetsReconcile(ctx context.Context, svc *sheets.Service, id string, data []*sheets.ValueRange) ([][][]any, error) {
	ranges := make([]string, len(data))
	for i, vr := range data {
		ranges[i] = vr.Range
	}
	response, err := svc.Spreadsheets.Get(id).Ranges(ranges...).IncludeGridData(true).
		Fields("sheets(properties(title),data(startRow,startColumn,rowData(values(effectiveValue))))").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	values := make([][][]any, len(data))
	for n, vr := range data {
		r, _ := sheetsa1.Parse(vr.Range)
		values[n] = make([][]any, len(vr.Values))
		for i, row := range vr.Values {
			values[n][i] = make([]any, len(row))
		}
		foundSheet := false
		for _, sheet := range response.Sheets {
			if sheet == nil || sheet.Properties == nil || sheet.Properties.Title != r.SheetName {
				continue
			}
			foundSheet = true
			for _, grid := range sheet.Data {
				if grid == nil {
					continue
				}
				for i, row := range grid.RowData {
					ri := int(grid.StartRow) + i + 1 - r.StartRow
					if row == nil || ri < 0 || ri >= len(values[n]) {
						continue
					}
					for j, cell := range row.Values {
						ci := int(grid.StartColumn) + j + 1 - r.StartCol
						if cell == nil || ci < 0 || ci >= len(values[n][ri]) || cell.EffectiveValue == nil {
							continue
						}
						v := cell.EffectiveValue
						switch {
						case v.ErrorValue != nil:
							values[n][ri][ci] = v.ErrorValue
						case v.StringValue != nil:
							values[n][ri][ci] = *v.StringValue
						case v.NumberValue != nil:
							values[n][ri][ci] = *v.NumberValue
						case v.BoolValue != nil:
							values[n][ri][ci] = *v.BoolValue
						}
					}
				}
			}
		}
		if !foundSheet {
			return nil, fmt.Errorf("readback is missing sheet %q", r.SheetName)
		}
	}
	return values, nil
}

func diffSheetsReconcile(data []*sheets.ValueRange, current [][][]any) []sheetsCellChange {
	changes := []sheetsCellChange{}
	for n, vr := range data {
		r, _ := sheetsa1.Parse(vr.Range)
		for i, row := range vr.Values {
			for j, desired := range row {
				if desired == nil {
					continue
				}
				var before any = ""
				if i < len(current[n]) && j < len(current[n][i]) {
					before = current[n][i][j]
				}
				if before == nil {
					before = ""
				}
				if !reflect.DeepEqual(before, desired) {
					changes = append(changes, sheetsCellChange{Range: sheetsa1.FormatCell(r.SheetName, r.StartRow+i, r.StartCol+j), Before: before, After: desired})
				}
			}
		}
	}
	return changes
}

func writeSheetsReconcileResult(ctx context.Context, result map[string]any, count int, applied bool) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	for _, change := range result["changes"].([]sheetsCellChange) {
		ui.FromContext(ctx).Out().Linef("%s\t%v\t%v", change.Range, change.Before, change.After)
	}
	if mismatches, ok := result["mismatches"].([]sheetsCellChange); ok && len(mismatches) > 0 {
		for _, change := range mismatches {
			ui.FromContext(ctx).Out().Linef("mismatch\t%s\tactual=%v\tdesired=%v", change.Range, change.Before, change.After)
		}
		ui.FromContext(ctx).Out().Linef("Changes were already submitted; preview again before retrying.")
	}
	ui.FromContext(ctx).Out().Linef("changes\t%d\napplied\t%t\nverified\t%v", count, applied, result["verified"])
	return nil
}
