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

type SheetsDeleteDimensionCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Target        string `arg:"" name:"rangeOrSheet" help:"Sheet name, or row/column range such as Sheet1!2:4 or Sheet1!B:C"`
	Dimension     string `name:"dimension" help:"Dimension to delete: ROWS or COLUMNS" required:""`
	Start         int64  `name:"start" help:"First row/column to delete (1-based, inclusive; required with a sheet target)"`
	End           int64  `name:"end" help:"Last row/column to delete (1-based, inclusive; required with a sheet target)"`
}

type sheetsDeleteDimensionSpec struct {
	SheetName  string
	Dimension  string
	Label      string
	StartIndex int64
	EndIndex   int64
}

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

	spec, err := parseSheetsDeleteDimensionSpec(c.Target, c.Dimension, c.Start, c.End)
	if err != nil {
		return err
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
	svc, err := newSheetsService(ctx, account)
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
	if boundsErr := validateSheetsDeleteDimensionBounds(spec, props); boundsErr != nil {
		return boundsErr
	}

	tableUpdates, err := planSheetsDeleteDimensionTables(tables, sheetID, sheetTitle, spec)
	if err != nil {
		return err
	}
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
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

func parseSheetsDeleteDimensionSpec(target, dimension string, start, end int64) (sheetsDeleteDimensionSpec, error) {
	target = cleanRange(strings.TrimSpace(target))
	if target == "" {
		return sheetsDeleteDimensionSpec{}, usage("empty rangeOrSheet")
	}

	spec := sheetsDeleteDimensionSpec{}
	switch strings.ToUpper(strings.TrimSpace(dimension)) {
	case "ROW", "ROWS":
		spec.Dimension = sheetsDimensionRows
		spec.Label = "rows"
	case "COL", "COLS", "COLUMN", "COLUMNS":
		spec.Dimension = sheetsDimensionColumns
		spec.Label = "columns"
	default:
		return sheetsDeleteDimensionSpec{}, usagef("dimension must be ROWS or COLUMNS, got %q", dimension)
	}

	if start == 0 && end == 0 {
		if !strings.Contains(target, "!") {
			return sheetsDeleteDimensionSpec{}, usage("sheet targets require both --start and --end; range targets must include a sheet name")
		}
		var span dimensionSpan
		var err error
		if spec.Dimension == sheetsDimensionRows {
			span, err = parseRowsSpan(target, "delete-dimension")
		} else {
			span, err = parseColumnsSpan(target, "delete-dimension")
		}
		if err != nil {
			return sheetsDeleteDimensionSpec{}, usagef(
				"sheet targets require both --start and --end; otherwise provide a matching row/column range: %v",
				err,
			)
		}
		spec.SheetName = span.SheetName
		spec.StartIndex = span.StartIndex
		spec.EndIndex = span.EndIndex
		return spec, nil
	}
	if start == 0 || end == 0 {
		return sheetsDeleteDimensionSpec{}, usage("provide both --start and --end")
	}
	if start < 1 {
		return sheetsDeleteDimensionSpec{}, usage("start must be >= 1")
	}
	if end < start {
		return sheetsDeleteDimensionSpec{}, usage("end must be >= start")
	}

	spec.SheetName = target
	spec.StartIndex = start - 1
	spec.EndIndex = end
	return spec, nil
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

func validateSheetsDeleteDimensionBounds(spec sheetsDeleteDimensionSpec, props *sheets.SheetProperties) error {
	if props == nil || props.GridProperties == nil {
		return nil
	}
	var size int64
	if spec.Dimension == sheetsDimensionRows {
		size = props.GridProperties.RowCount
	} else {
		size = props.GridProperties.ColumnCount
	}
	if size <= 0 {
		return nil
	}
	if spec.EndIndex > size {
		return usagef("%s end %d exceeds sheet size %d", spec.Label, spec.EndIndex, size)
	}
	if spec.StartIndex == 0 && spec.EndIndex == size {
		return usagef("cannot delete every %s in a sheet", spec.Label[:len(spec.Label)-1])
	}
	return nil
}

func planSheetsDeleteDimensionTables(
	tables []*sheets.Table,
	sheetID int64,
	sheetTitle string,
	spec sheetsDeleteDimensionSpec,
) ([]sheetsDeleteDimensionTable, error) {
	updates := make([]sheetsDeleteDimensionTable, 0)
	for _, table := range tables {
		if table == nil || table.Range == nil || table.Range.SheetId != sheetID {
			continue
		}
		after, intersects, err := resizeTableRangeAfterDimensionDelete(table.Range, spec)
		if err != nil {
			label := strings.TrimSpace(table.Name)
			if label == "" {
				label = strings.TrimSpace(table.TableId)
			}
			return nil, usagef("cannot delete %s %d-%d: table %q would lose its entire %s extent",
				spec.Label, spec.StartIndex+1, spec.EndIndex, label, spec.Label[:len(spec.Label)-1])
		}
		if !intersects {
			continue
		}
		if after.SheetId == 0 {
			after.ForceSendFields = append(after.ForceSendFields, "SheetId")
		}
		updates = append(updates, sheetsDeleteDimensionTable{
			TableID:  strings.TrimSpace(table.TableId),
			Name:     strings.TrimSpace(table.Name),
			BeforeA1: gridRangeToA1(sheetTitle, table.Range),
			AfterA1:  gridRangeToA1(sheetTitle, after),
			Range:    after,
		})
	}
	sort.Slice(updates, func(i, j int) bool {
		if updates[i].TableID == updates[j].TableID {
			return updates[i].Name < updates[j].Name
		}
		return updates[i].TableID < updates[j].TableID
	})
	return updates, nil
}

func resizeTableRangeAfterDimensionDelete(
	tableRange *sheets.GridRange,
	spec sheetsDeleteDimensionSpec,
) (*sheets.GridRange, bool, error) {
	if tableRange == nil {
		return nil, false, nil
	}
	var tableStart, tableEnd int64
	if spec.Dimension == sheetsDimensionRows {
		tableStart, tableEnd = tableRange.StartRowIndex, tableRange.EndRowIndex
	} else {
		tableStart, tableEnd = tableRange.StartColumnIndex, tableRange.EndColumnIndex
	}
	if tableEnd <= tableStart {
		return nil, false, fmt.Errorf("invalid bounded table range")
	}
	if spec.EndIndex <= tableStart || spec.StartIndex >= tableEnd {
		return nil, false, nil
	}

	afterStart := dimensionBoundaryAfterDelete(tableStart, spec.StartIndex, spec.EndIndex)
	afterEnd := dimensionBoundaryAfterDelete(tableEnd, spec.StartIndex, spec.EndIndex)
	if afterEnd <= afterStart {
		return nil, true, fmt.Errorf("empty table range")
	}

	after := *tableRange
	after.ForceSendFields = append([]string(nil), tableRange.ForceSendFields...)
	if spec.Dimension == sheetsDimensionRows {
		after.StartRowIndex = afterStart
		after.EndRowIndex = afterEnd
	} else {
		after.StartColumnIndex = afterStart
		after.EndColumnIndex = afterEnd
	}
	return &after, true, nil
}

func dimensionBoundaryAfterDelete(boundary, deleteStart, deleteEnd int64) int64 {
	switch {
	case boundary <= deleteStart:
		return boundary
	case boundary >= deleteEnd:
		return boundary - (deleteEnd - deleteStart)
	default:
		return deleteStart
	}
}
