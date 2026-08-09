package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/sheetsdimension"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsDeleteDimensionCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Target        string `arg:"" name:"rangeOrSheet" help:"Sheet name, or row/column range such as Sheet1!2:4 or Sheet1!B:C"`
	Dimension     string `name:"dimension" help:"Dimension to delete: ROWS or COLUMNS" required:""`
	Start         int64  `name:"start" help:"First row/column to delete (1-based, inclusive; required with a sheet target)"`
	End           int64  `name:"end" help:"Last row/column to delete (1-based, inclusive; required with a sheet target)"`
}

type sheetsDeleteDimensionSpec = sheetsdimension.DeleteSpec

type sheetsDeleteDimensionTable struct {
	TableID  string            `json:"tableId"`
	Name     string            `json:"name,omitempty"`
	BeforeA1 string            `json:"beforeA1"`
	AfterA1  string            `json:"afterA1"`
	Range    *sheets.GridRange `json:"range"`
}

func (c *SheetsDeleteDimensionCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	spec, err := sheetsdimension.ParseDeleteSpec(c.Target, c.Dimension, c.Start, c.End)
	if err != nil {
		return sheetsDimensionPlannerError(err)
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.delete-dimension", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"target":         strings.TrimSpace(c.Target),
		"sheet":          spec.SheetName,
		"dimension":      spec.Dimension,
		"start":          spec.StartIndex + 1,
		"end":            spec.EndIndex,
		"start_index":    spec.StartIndex,
		"end_index":      spec.EndIndex,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	catalog, tables, err := fetchSheetsDeleteDimensionMetadata(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	sheetID, sheetTitle, err := resolveSheetIDByNameOrFirstWithCatalog(catalog, spec.SheetName)
	if err != nil {
		return err
	}
	props := findSheetPropertiesByID(catalog, sheetID)
	if props == nil {
		return fmt.Errorf("sheet metadata disappeared (id=%d)", sheetID)
	}
	if boundsErr := sheetsdimension.ValidateBounds(spec, props); boundsErr != nil {
		return sheetsDimensionPlannerError(boundsErr)
	}

	plannedUpdates, err := sheetsdimension.PlanTableUpdates(tables, sheetID, spec)
	if err != nil {
		return sheetsDimensionPlannerError(err)
	}
	tableUpdates := sheetsDeleteDimensionTableResults(sheetTitle, plannedUpdates)
	if err := confirmDestructiveChecked(
		ctx,
		flagsWithoutDryRun(flags),
		fmt.Sprintf("delete %s %d-%d from sheet %q", spec.Label, spec.StartIndex+1, spec.EndIndex, sheetTitle),
	); err != nil {
		return err
	}

	dimRange := &sheets.DimensionRange{
		SheetId:    sheetID,
		Dimension:  spec.Dimension,
		StartIndex: spec.StartIndex,
		EndIndex:   spec.EndIndex,
	}
	forceSendDimensionRangeZeroes(dimRange)
	requests := []*sheets.Request{{
		DeleteDimension: &sheets.DeleteDimensionRequest{Range: dimRange},
	}}
	for _, table := range tableUpdates {
		requests = append(requests, &sheets.Request{
			UpdateTable: &sheets.UpdateTableRequest{
				Table: &sheets.Table{
					TableId: table.TableID,
					Range:   table.Range,
				},
				Fields: "range",
			},
		})
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete sheet dimension: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"sheet":         sheetTitle,
			"sheetId":       sheetID,
			"dimension":     spec.Dimension,
			"start":         spec.StartIndex + 1,
			"end":           spec.EndIndex,
			"startIndex":    spec.StartIndex,
			"endIndex":      spec.EndIndex,
			"tables":        tableUpdates,
		})
	}

	count := spec.EndIndex - spec.StartIndex
	u.Out().Linef("Deleted %d %s from %q (%d-%d); resized %d table(s)",
		count, spec.Label, sheetTitle, spec.StartIndex+1, spec.EndIndex, len(tableUpdates))
	return nil
}

func fetchSheetsDeleteDimensionMetadata(
	ctx context.Context,
	svc *sheets.Service,
	spreadsheetID string,
) (*spreadsheetRangeCatalog, []*sheets.Table, error) {
	call := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title,index,gridProperties(rowCount,columnCount)),tables(tableId,name,range))")
	if ctx != nil {
		call = call.Context(ctx)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, nil, fmt.Errorf("get spreadsheet dimensions and tables: %w", err)
	}

	catalog := &spreadsheetRangeCatalog{
		SheetIDsByTitle: make(map[string]int64, len(resp.Sheets)),
		SheetTitlesByID: make(map[int64]string, len(resp.Sheets)),
		Sheets:          make([]*sheets.SheetProperties, 0, len(resp.Sheets)),
	}
	tables := make([]*sheets.Table, 0)
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		props := sheet.Properties
		catalog.Sheets = append(catalog.Sheets, props)
		if props.Title != "" {
			catalog.SheetIDsByTitle[props.Title] = props.SheetId
			catalog.SheetTitlesByID[props.SheetId] = props.Title
		}
		tables = append(tables, sheet.Tables...)
	}
	return catalog, tables, nil
}

func findSheetPropertiesByID(catalog *spreadsheetRangeCatalog, sheetID int64) *sheets.SheetProperties {
	if catalog == nil {
		return nil
	}
	for _, props := range catalog.Sheets {
		if props != nil && props.SheetId == sheetID {
			return props
		}
	}
	return nil
}

func sheetsDimensionPlannerError(err error) error {
	var validationErr sheetsdimension.ValidationError
	if errors.As(err, &validationErr) {
		return usage(validationErr.Error())
	}
	return err
}

func sheetsDeleteDimensionTableResults(
	sheetTitle string,
	updates []sheetsdimension.TableUpdate,
) []sheetsDeleteDimensionTable {
	results := make([]sheetsDeleteDimensionTable, 0, len(updates))
	for _, update := range updates {
		results = append(results, sheetsDeleteDimensionTable{
			TableID:  update.TableID,
			Name:     update.Name,
			BeforeA1: sheetsa1.FormatGridRange(sheetTitle, update.Before),
			AfterA1:  sheetsa1.FormatGridRange(sheetTitle, update.After),
			Range:    update.After,
		})
	}
	return results
}
