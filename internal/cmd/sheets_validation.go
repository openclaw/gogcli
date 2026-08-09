package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/sheetsvalidation"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsValidationCmd struct {
	Get   SheetsValidationGetCmd   `cmd:"" default:"withargs" aliases:"list,show" help:"Get data validation rules from a range"`
	Set   SheetsValidationSetCmd   `cmd:"" name:"set" aliases:"add,create" help:"Set a data validation rule on a range"`
	Clear SheetsValidationClearCmd `cmd:"" name:"clear" aliases:"delete,remove,rm" help:"Clear data validation rules; fully selected table dropdown columns become text columns"`
}

type SheetsValidationGetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (A1 notation or named range name; e.g. Sheet1!A1:B10 or MyNamedRange)"`
}

type sheetsCellValidation struct {
	Sheet string                     `json:"sheet"`
	A1    string                     `json:"a1"`
	Row   int                        `json:"row"`
	Col   int                        `json:"col"`
	Rule  *sheets.DataValidationRule `json:"rule"`
}

func (c *SheetsValidationGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	_, svc, err := requireSheetsService(ctx, flags)
	if err != nil {
		return err
	}
	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	apiRange, targetRange, err := resolveValidationReadRange(rangeSpec, catalog)
	if err != nil {
		return err
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Ranges(apiRange).
		IncludeGridData(true).
		Fields("sheets(properties(title),data(startRow,startColumn,rowData(values(dataValidation))))").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	validations := collectCellValidations(resp)
	tableSpans, err := fetchTableValidationSpans(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	validations = appendTableCellValidations(validations, tableSpans, targetRange, catalog.SheetTitlesByID)
	sort.Slice(validations, func(i, j int) bool {
		if validations[i].Sheet != validations[j].Sheet {
			return validations[i].Sheet < validations[j].Sheet
		}
		if validations[i].Row != validations[j].Row {
			return validations[i].Row < validations[j].Row
		}
		return validations[i].Col < validations[j].Col
	})
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"validations":   validations,
		})
	}
	if len(validations) == 0 {
		u.Err().Println("No data validation rules found")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), validations, sheetsValidationColumns())
}

func resolveValidationReadRange(input string, catalog *spreadsheetRangeCatalog) (string, *sheets.GridRange, error) {
	rangeSpec := cleanRange(strings.TrimSpace(input))
	if !strings.Contains(rangeSpec, "!") {
		namedRange, found, err := resolveNamedRangeByNameOrID(rangeSpec, catalog.NamedRanges)
		if err != nil {
			return "", nil, err
		}
		if found && namedRange != nil {
			gridRange, err := resolveGridRangeWithCatalog(rangeSpec, catalog, "validation")
			if err != nil {
				return "", nil, err
			}
			canonicalName := strings.TrimSpace(namedRange.Name)
			if canonicalName == "" {
				return "", nil, usagef("validation named range %q has no name", rangeSpec)
			}
			return canonicalName, gridRange, nil
		}
	}
	parsed, parseErr := sheetsa1.Parse(rangeSpec)
	if parseErr == nil && parsed.SheetName == "" {
		_, sheetTitle, err := resolveSheetIDByNameOrFirstWithCatalog(catalog, "")
		if err != nil {
			return "", nil, err
		}
		parsed.SheetName = sheetTitle
		gridRange, err := gridRangeFromMap(parsed, catalog.SheetIDsByTitle, "validation")
		if err != nil {
			return "", nil, err
		}
		return sheetsa1.SheetPrefix(sheetTitle) + rangeSpec, gridRange, nil
	}
	gridRange, err := resolveGridRangeWithCatalog(rangeSpec, catalog, "validation")
	if err != nil {
		return "", nil, err
	}
	return rangeSpec, gridRange, nil
}

type SheetsValidationSetCmd struct {
	SpreadsheetID        string   `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range                string   `arg:"" name:"range" help:"Range (A1 notation with sheet name or named range name)"`
	Type                 string   `name:"type" required:"" help:"Condition type (e.g. ONE_OF_LIST, ONE_OF_RANGE, NUMBER_BETWEEN, DATE_AFTER, BOOLEAN)"`
	Values               []string `name:"value" help:"Condition value; repeat for list or between conditions"`
	Strict               bool     `name:"strict" help:"Reject invalid input instead of showing a warning" negatable:""`
	ShowCustomUI         bool     `name:"show-custom-ui" help:"Show dropdown or checkbox UI where supported" default:"true" negatable:""`
	InputMessage         string   `name:"input-message" help:"Message shown when the cell is selected"`
	FilteredRowsIncluded bool     `name:"filtered-rows-included" help:"Apply the rule to filtered rows; required for table-managed dropdown columns" negatable:""`
}

func (c *SheetsValidationSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID, rangeSpec, err := validateSheetsValidationTarget(c.SpreadsheetID, c.Range)
	if err != nil {
		return err
	}
	condition, err := sheetsvalidation.BuildCondition(c.Type, c.Values)
	if err != nil {
		return sheetsValidationPlannerError(err)
	}
	rule := &sheets.DataValidationRule{
		Condition:    condition,
		InputMessage: c.InputMessage,
		ShowCustomUi: c.ShowCustomUI,
		Strict:       c.Strict,
		ForceSendFields: []string{
			"ShowCustomUi",
			"Strict",
		},
	}

	return runSheetsMutation(ctx, flags, "sheets.validation.set", map[string]any{
		"spreadsheet_id":         spreadsheetID,
		"range":                  rangeSpec,
		"rule":                   rule,
		"filtered_rows_included": c.FilteredRowsIncluded,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		gridRange, err := resolveValidationGridRange(ctx, svc, spreadsheetID, rangeSpec)
		if err != nil {
			return nil, "", err
		}
		tableSpans, err := fetchTableValidationSpans(ctx, svc, spreadsheetID)
		if err != nil {
			return nil, "", err
		}
		tableRequests, err := sheetsvalidation.BuildSetRequests(gridRange, tableSpans, condition)
		if err != nil {
			return nil, "", sheetsValidationPlannerError(err)
		}
		if len(tableRequests) > 0 && !c.FilteredRowsIncluded {
			return nil, "", usage("setting table-managed dropdown validation requires --filtered-rows-included")
		}
		if len(tableRequests) > 0 && condition.Type == sheetsConditionOneOfList &&
			(c.Strict || !c.ShowCustomUI || c.InputMessage != "") {
			return nil, "", usage("table-managed dropdowns do not support --strict, --no-show-custom-ui, or --input-message")
		}

		ordinaryRanges := []*sheets.GridRange{gridRange}
		if len(tableRequests) > 0 && condition.Type == sheetsConditionOneOfList {
			ordinaryRanges = sheetsvalidation.SubtractSpans(gridRange, tableSpans)
		}
		requests := append([]*sheets.Request(nil), tableRequests...)
		for _, ordinaryRange := range ordinaryRanges {
			requests = append(requests, &sheets.Request{
				SetDataValidation: &sheets.SetDataValidationRequest{
					Range:                ordinaryRange,
					Rule:                 rule,
					FilteredRowsIncluded: c.FilteredRowsIncluded,
					ForceSendFields:      []string{"FilteredRowsIncluded"},
				},
			})
		}
		req := &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		return map[string]any{
			"spreadsheetId":        spreadsheetID,
			"range":                rangeSpec,
			"rule":                 rule,
			"filteredRowsIncluded": c.FilteredRowsIncluded,
			"tableManagedRules":    len(tableRequests),
		}, fmt.Sprintf("Set %s data validation on %s", condition.Type, rangeSpec), nil
	})
}

type SheetsValidationClearCmd struct {
	SpreadsheetID        string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range                string `arg:"" name:"range" help:"Range (A1 notation with sheet name or named range name)"`
	FilteredRowsIncluded bool   `name:"filtered-rows-included" help:"Clear rules from filtered rows too; required for table-managed dropdown columns" negatable:""`
}

func (c *SheetsValidationClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID, rangeSpec, err := validateSheetsValidationTarget(c.SpreadsheetID, c.Range)
	if err != nil {
		return err
	}

	return runSheetsMutation(ctx, flags, "sheets.validation.clear", map[string]any{
		"spreadsheet_id":         spreadsheetID,
		"range":                  rangeSpec,
		"filtered_rows_included": c.FilteredRowsIncluded,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		gridRange, err := resolveValidationGridRange(ctx, svc, spreadsheetID, rangeSpec)
		if err != nil {
			return nil, "", err
		}
		tableSpans, err := fetchTableValidationSpans(ctx, svc, spreadsheetID)
		if err != nil {
			return nil, "", err
		}
		tableRequests, err := sheetsvalidation.BuildClearRequests(gridRange, tableSpans)
		if err != nil {
			return nil, "", sheetsValidationPlannerError(err)
		}
		if len(tableRequests) > 0 && !c.FilteredRowsIncluded {
			return nil, "", usage("clearing table-managed dropdown validation requires --filtered-rows-included")
		}
		ordinaryRanges := sheetsvalidation.SubtractSpans(gridRange, tableSpans)
		requests := make([]*sheets.Request, 0, len(ordinaryRanges)+len(tableRequests))
		for _, ordinaryRange := range ordinaryRanges {
			requests = append(requests, &sheets.Request{
				SetDataValidation: &sheets.SetDataValidationRequest{
					Range:                ordinaryRange,
					FilteredRowsIncluded: c.FilteredRowsIncluded,
					ForceSendFields:      []string{"FilteredRowsIncluded"},
				},
			})
		}
		requests = append(requests, tableRequests...)
		if len(requests) > 0 {
			req := &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}
			if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
				return nil, "", err
			}
		}
		return map[string]any{
			"spreadsheetId":        spreadsheetID,
			"range":                rangeSpec,
			"cleared":              true,
			"filteredRowsIncluded": c.FilteredRowsIncluded,
			"tableManagedRules":    len(tableRequests),
		}, fmt.Sprintf("Cleared data validation from %s", rangeSpec), nil
	})
}

func collectCellValidations(resp *sheets.Spreadsheet) []sheetsCellValidation {
	items := make([]sheetsCellValidation, 0)
	if resp == nil {
		return items
	}
	for _, sheet := range resp.Sheets {
		if sheet == nil {
			continue
		}
		title := ""
		if sheet.Properties != nil {
			title = sheet.Properties.Title
		}
		for _, data := range sheet.Data {
			if data == nil {
				continue
			}
			for rowOffset, row := range data.RowData {
				if row == nil {
					continue
				}
				for colOffset, cell := range row.Values {
					if cell == nil || cell.DataValidation == nil {
						continue
					}
					rowNumber := int(data.StartRow) + rowOffset + 1
					colNumber := int(data.StartColumn) + colOffset + 1
					items = append(items, sheetsCellValidation{
						Sheet: title,
						A1:    sheetsa1.FormatCell(title, rowNumber, colNumber),
						Row:   rowNumber,
						Col:   colNumber,
						Rule:  cell.DataValidation,
					})
				}
			}
		}
	}
	return items
}

func validationInputMessage(rule *sheets.DataValidationRule) string {
	if rule == nil {
		return ""
	}
	return rule.InputMessage
}

func validateSheetsValidationTarget(rawSpreadsheetID, rawRange string) (string, string, error) {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(rawSpreadsheetID))
	rangeSpec := cleanRange(rawRange)
	if spreadsheetID == "" {
		return "", "", usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return "", "", usage("empty range")
	}
	return spreadsheetID, rangeSpec, nil
}

func resolveValidationGridRange(ctx context.Context, svc *sheets.Service, spreadsheetID, rangeSpec string) (*sheets.GridRange, error) {
	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return nil, err
	}
	gridRange, err := resolveGridRangeWithCatalog(rangeSpec, catalog, "validation")
	if err != nil {
		return nil, err
	}
	return boundGridRangeToSheet(gridRange, catalog), nil
}

type tableValidationSpan = sheetsvalidation.Span

type a1Range = sheetsa1.Range

func sheetsValidationPlannerError(err error) error {
	var validationErr sheetsvalidation.ValidationError
	if errors.As(err, &validationErr) {
		return usage(validationErr.Error())
	}
	return err
}

func fetchTableValidationSpans(ctx context.Context, svc *sheets.Service, spreadsheetID string) ([]tableValidationSpan, error) {
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId),tables(tableId,range,rowsProperties(footerColorStyle),columnProperties(columnIndex,columnName,columnType,dataValidationRule(condition(type,values(userEnteredValue))))))").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("get spreadsheet table validations: %w", err)
	}

	spans := make([]tableValidationSpan, 0)
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		for _, table := range sheet.Tables {
			if table == nil || table.Range == nil {
				continue
			}
			endRow := table.Range.EndRowIndex
			if sheetsTableHasFooter(table) {
				endRow--
			}
			if endRow <= table.Range.StartRowIndex+1 {
				continue
			}
			for _, column := range table.ColumnProperties {
				if column == nil {
					continue
				}
				var rule *sheets.DataValidationRule
				if column.DataValidationRule != nil && column.DataValidationRule.Condition != nil {
					rule = &sheets.DataValidationRule{
						Condition:    sheetsvalidation.CloneCondition(column.DataValidationRule.Condition),
						ShowCustomUi: true,
					}
				}
				spans = append(spans, tableValidationSpan{
					SheetID:     sheet.Properties.SheetId,
					TableID:     table.TableId,
					ColumnIndex: column.ColumnIndex,
					StartRow:    table.Range.StartRowIndex + 1,
					EndRow:      endRow,
					StartCol:    table.Range.StartColumnIndex + column.ColumnIndex,
					EndCol:      table.Range.StartColumnIndex + column.ColumnIndex + 1,
					Columns:     table.ColumnProperties,
					Rule:        rule,
				})
			}
		}
	}
	return spans, nil
}

func boundGridRangeToSheet(grid *sheets.GridRange, catalog *spreadsheetRangeCatalog) *sheets.GridRange {
	if grid == nil || catalog == nil || (grid.EndRowIndex > 0 && grid.EndColumnIndex > 0) {
		return grid
	}
	bounded := *grid
	for _, props := range catalog.Sheets {
		if props == nil || props.SheetId != grid.SheetId || props.GridProperties == nil {
			continue
		}
		if bounded.EndRowIndex == 0 {
			bounded.EndRowIndex = props.GridProperties.RowCount
		}
		if bounded.EndColumnIndex == 0 {
			bounded.EndColumnIndex = props.GridProperties.ColumnCount
		}
		break
	}
	return &bounded
}

func appendTableCellValidations(
	items []sheetsCellValidation,
	spans []tableValidationSpan,
	target *sheets.GridRange,
	titles map[int64]string,
) []sheetsCellValidation {
	if target == nil {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seen[fmt.Sprintf("%s:%d:%d", item.Sheet, item.Row, item.Col)] = struct{}{}
	}

	for _, span := range spans {
		if span.Rule == nil || span.SheetID != target.SheetId {
			continue
		}
		startRow, endRow, ok := sheetsvalidation.IntersectGridIndexes(
			span.StartRow,
			span.EndRow,
			target.StartRowIndex,
			target.EndRowIndex,
		)
		if !ok {
			continue
		}
		startCol, endCol, ok := sheetsvalidation.IntersectGridIndexes(
			span.StartCol,
			span.EndCol,
			target.StartColumnIndex,
			target.EndColumnIndex,
		)
		if !ok {
			continue
		}
		title := titles[span.SheetID]
		for row := startRow; row < endRow; row++ {
			for col := startCol; col < endCol; col++ {
				key := fmt.Sprintf("%s:%d:%d", title, row+1, col+1)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				items = append(items, sheetsCellValidation{
					Sheet: title,
					A1:    sheetsa1.FormatCell(title, int(row+1), int(col+1)),
					Row:   int(row + 1),
					Col:   int(col + 1),
					Rule:  span.Rule,
				})
			}
		}
	}
	return items
}

type tableValidationCopyOptions = sheetsvalidation.CopyOptions

type validationCellCoordinate = sheetsvalidation.CellCoordinate

func resolveTableValidationCopyOptions(
	ctx context.Context,
	svc *sheets.Service,
	spreadsheetID string,
	source, destination *sheets.GridRange,
	spans []tableValidationSpan,
	catalog *spreadsheetRangeCatalog,
	transpose bool,
) (tableValidationCopyOptions, error) {
	effectiveDestination := sheetsvalidation.EffectiveCopyDestination(source, destination, transpose)
	if _, ok := sheetsvalidation.FirstIntersectingSpan(effectiveDestination, spans); !ok {
		return tableValidationCopyOptions{}, nil
	}
	sourceSpans := sheetsvalidation.RelevantSourceSpans(source, spans)
	if len(sheetsvalidation.SubtractSpans(source, sourceSpans)) == 0 {
		return tableValidationCopyOptions{}, nil
	}
	if catalog == nil {
		return tableValidationCopyOptions{}, fmt.Errorf("missing spreadsheet range catalog")
	}
	sheetTitle, ok := catalog.SheetTitlesByID[source.SheetId]
	if !ok {
		return tableValidationCopyOptions{}, usagef("copy source references unknown sheet ID %d", source.SheetId)
	}
	sourceA1 := sheetsa1.FormatGridRange(sheetTitle, source)
	if sourceA1 == "" {
		return tableValidationCopyOptions{}, usage("copy source range cannot be represented in A1 notation")
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Ranges(sourceA1).
		IncludeGridData(true).
		Fields("sheets(data(startRow,startColumn,rowData(values(dataValidation))))").
		Context(ctx).
		Do()
	if err != nil {
		return tableValidationCopyOptions{}, fmt.Errorf("get copy source validations: %w", err)
	}
	validatedCells := make([]validationCellCoordinate, 0)
	for _, sheet := range resp.Sheets {
		if sheet == nil {
			continue
		}
		for _, data := range sheet.Data {
			if data == nil {
				continue
			}
			for rowOffset, row := range data.RowData {
				if row == nil {
					continue
				}
				for colOffset, cell := range row.Values {
					if cell == nil || cell.DataValidation == nil {
						continue
					}
					validatedCells = append(validatedCells, validationCellCoordinate{
						Row: int64(rowOffset) + data.StartRow,
						Col: int64(colOffset) + data.StartColumn,
					})
				}
			}
		}
	}
	return tableValidationCopyOptions{
		OrdinarySourceValidationKnown: true,
		OrdinaryValidatedCells:        validatedCells,
	}, nil
}

func copyDataValidation(ctx context.Context, svc *sheets.Service, spreadsheetID, sourceA1, destA1 string) error {
	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}

	sourceGrid, err := resolveGridRangeWithCatalog(sourceA1, catalog, "copy-validation-from")
	if err != nil {
		return err
	}

	destRange, err := parseSheetRange(destA1, "updated")
	if err != nil {
		return err
	}
	destGrid, err := gridRangeFromMap(destRange, catalog.SheetIDsByTitle, "updated")
	if err != nil {
		return err
	}
	sourceGrid = boundGridRangeToSheet(sourceGrid, catalog)
	destGrid = boundGridRangeToSheet(destGrid, catalog)

	spans, err := fetchTableValidationSpans(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	copyOptions, err := resolveTableValidationCopyOptions(
		ctx,
		svc,
		spreadsheetID,
		sourceGrid,
		destGrid,
		spans,
		catalog,
		false,
	)
	if err != nil {
		return err
	}
	supplemental, err := sheetsvalidation.BuildCopyRequests(sourceGrid, destGrid, false, spans, copyOptions)
	if err != nil {
		return sheetsValidationPlannerError(err)
	}
	requests := make([]*sheets.Request, 0, 1+len(supplemental))
	requests = append(requests, &sheets.Request{
		CopyPaste: &sheets.CopyPasteRequest{
			Source:      sourceGrid,
			Destination: destGrid,
			PasteType:   "PASTE_DATA_VALIDATION",
		},
	})
	requests = append(requests, supplemental...)
	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	_, err = svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return fmt.Errorf("apply data validation: %w", err)
	}
	return nil
}

func fetchSheetIDMap(ctx context.Context, svc *sheets.Service, spreadsheetID string) (map[string]int64, error) {
	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return nil, err
	}
	return catalog.SheetIDsByTitle, nil
}

func toGridRange(r a1Range, sheetID int64) *sheets.GridRange {
	gr := &sheets.GridRange{
		SheetId:          sheetID,
		ForceSendFields:  []string{"SheetId"}, // sheetId can be 0 for the first sheet, but still must be sent.
		StartRowIndex:    0,
		EndRowIndex:      0,
		StartColumnIndex: 0,
		EndColumnIndex:   0,
	}
	if r.StartRow > 0 {
		gr.StartRowIndex = int64(r.StartRow - 1)
	}
	if r.EndRow > 0 {
		gr.EndRowIndex = int64(r.EndRow)
	}
	if r.StartCol > 0 {
		gr.StartColumnIndex = int64(r.StartCol - 1)
	}
	if r.EndCol > 0 {
		gr.EndColumnIndex = int64(r.EndCol)
	}
	return gr
}

func parseSheetRange(a1, label string) (a1Range, error) {
	r, err := sheetsa1.Parse(a1)
	if err != nil {
		return a1Range{}, usagef("parse %s range: %v", label, err)
	}
	if strings.TrimSpace(r.SheetName) == "" {
		return a1Range{}, usagef("%s range must include a sheet name", label)
	}
	return r, nil
}

func gridRangeFromMap(r a1Range, sheetIDs map[string]int64, label string) (*sheets.GridRange, error) {
	sheetID, ok := sheetIDs[r.SheetName]
	if !ok {
		return nil, fmt.Errorf("unknown sheet %q in %s range", r.SheetName, label)
	}
	return toGridRange(r, sheetID), nil
}
