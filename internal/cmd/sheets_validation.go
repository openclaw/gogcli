package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"validations":   validations,
		})
	}
	if len(validations) == 0 {
		u.Err().Println("No data validation rules found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "A1\tTYPE\tVALUES\tSTRICT\tSHOW_CUSTOM_UI\tINPUT_MESSAGE")
	for _, item := range validations {
		conditionType := ""
		values := []string{}
		if item.Rule != nil && item.Rule.Condition != nil {
			conditionType = item.Rule.Condition.Type
			for _, value := range item.Rule.Condition.Values {
				if value != nil {
					values = append(values, value.UserEnteredValue)
				}
			}
		}
		encodedValues, _ := json.Marshal(values)
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%t\t%s\n",
			oneLine(item.A1),
			oneLine(conditionType),
			encodedValues,
			item.Rule != nil && item.Rule.Strict,
			item.Rule != nil && item.Rule.ShowCustomUi,
			oneLine(validationInputMessage(item.Rule)),
		)
	}
	return nil
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
	parsed, parseErr := parseA1Range(rangeSpec)
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
		return formatSheetPrefix(sheetTitle) + rangeSpec, gridRange, nil
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
	condition, err := buildDataValidationCondition(c.Type, c.Values)
	if err != nil {
		return err
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
		tableRequests, err := buildTableValidationSetRequests(gridRange, tableSpans, condition)
		if err != nil {
			return nil, "", err
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
			ordinaryRanges = subtractTableValidationSpans(gridRange, tableSpans)
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
		tableRequests, err := buildTableValidationClearRequests(gridRange, tableSpans)
		if err != nil {
			return nil, "", err
		}
		if len(tableRequests) > 0 && !c.FilteredRowsIncluded {
			return nil, "", usage("clearing table-managed dropdown validation requires --filtered-rows-included")
		}
		ordinaryRanges := subtractTableValidationSpans(gridRange, tableSpans)
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
						A1:    formatA1Cell(title, rowNumber, colNumber),
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

func buildDataValidationCondition(rawType string, rawValues []string) (*sheets.BooleanCondition, error) {
	conditionType := strings.ToUpper(strings.TrimSpace(rawType))
	conditionType = strings.NewReplacer("-", "_", " ", "_").Replace(conditionType)
	if conditionType == "" {
		return nil, usage("empty --type")
	}

	minValues, maxValues, ok := validationConditionArity(conditionType)
	if !ok {
		return nil, usagef("unsupported validation --type %q", rawType)
	}
	values := append([]string(nil), rawValues...)
	if len(values) < minValues || (maxValues >= 0 && len(values) > maxValues) {
		switch {
		case minValues == maxValues:
			return nil, usagef("%s requires exactly %d --value flag(s)", conditionType, minValues)
		case maxValues < 0:
			return nil, usagef("%s requires at least %d --value flag(s)", conditionType, minValues)
		default:
			return nil, usagef("%s accepts %d to %d --value flag(s)", conditionType, minValues, maxValues)
		}
	}
	if conditionType == sheetsConditionOneOfList {
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") {
				return nil, usage("ONE_OF_LIST values cannot be formulas")
			}
		}
	}
	if conditionType == "ONE_OF_RANGE" {
		values[0] = strings.TrimSpace(values[0])
		if values[0] == "" || values[0] == "=" || values[0] == "+" {
			return nil, usage("ONE_OF_RANGE requires a non-empty range value")
		}
		if !strings.HasPrefix(values[0], "=") && !strings.HasPrefix(values[0], "+") {
			values[0] = "=" + values[0]
		}
	}
	if conditionType == "CUSTOM_FORMULA" {
		values[0] = strings.TrimSpace(values[0])
		if !strings.HasPrefix(values[0], "=") && !strings.HasPrefix(values[0], "+") {
			return nil, usage("CUSTOM_FORMULA value must begin with = or +")
		}
	}

	conditionValues := make([]*sheets.ConditionValue, 0, len(values))
	for _, value := range values {
		conditionValues = append(conditionValues, &sheets.ConditionValue{
			UserEnteredValue: value,
			ForceSendFields:  []string{"UserEnteredValue"},
		})
	}
	return &sheets.BooleanCondition{
		Type:   conditionType,
		Values: conditionValues,
	}, nil
}

func validationConditionArity(conditionType string) (int, int, bool) {
	switch conditionType {
	case "TEXT_IS_EMAIL", "TEXT_IS_URL", "DATE_IS_VALID":
		return 0, 0, true
	case "BOOLEAN":
		return 0, 2, true
	case sheetsConditionOneOfList:
		return 1, -1, true
	case "NUMBER_BETWEEN", "NUMBER_NOT_BETWEEN", "DATE_BETWEEN", "DATE_NOT_BETWEEN":
		return 2, 2, true
	case "NUMBER_GREATER", "NUMBER_GREATER_THAN_EQ", "NUMBER_LESS", "NUMBER_LESS_THAN_EQ",
		"NUMBER_EQ", "NUMBER_NOT_EQ", "TEXT_CONTAINS", "TEXT_NOT_CONTAINS", "TEXT_EQ",
		"DATE_EQ", "DATE_BEFORE", "DATE_AFTER", "DATE_ON_OR_BEFORE", "DATE_ON_OR_AFTER",
		"ONE_OF_RANGE", "CUSTOM_FORMULA":
		return 1, 1, true
	default:
		return 0, 0, false
	}
}

type tableValidationSpan struct {
	SheetID     int64
	TableID     string
	ColumnIndex int64
	StartRow    int64
	EndRow      int64
	StartCol    int64
	EndCol      int64
	Rule        *sheets.DataValidationRule
	Columns     []*sheets.TableColumnProperties
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
						Condition:    cloneBooleanCondition(column.DataValidationRule.Condition),
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

func buildTableValidationClearRequests(target *sheets.GridRange, spans []tableValidationSpan) ([]*sheets.Request, error) {
	if target == nil {
		return nil, nil
	}
	type clearGroup struct {
		columns []*sheets.TableColumnProperties
		indexes map[int64]struct{}
	}
	groups := make(map[string]*clearGroup)
	for _, span := range spans {
		if span.Rule == nil {
			continue
		}
		if span.SheetID != target.SheetId {
			continue
		}
		if _, _, ok := intersectGridIndexes(span.StartRow, span.EndRow, target.StartRowIndex, target.EndRowIndex); !ok {
			continue
		}
		if _, _, ok := intersectGridIndexes(span.StartCol, span.EndCol, target.StartColumnIndex, target.EndColumnIndex); !ok {
			continue
		}
		if !gridRangeCoversSpan(target, span) {
			return nil, usagef(
				"range partially intersects table-managed dropdown column %d in table %s; clear the full table data column",
				span.ColumnIndex+1,
				span.TableID,
			)
		}

		group := groups[span.TableID]
		if group == nil {
			group = &clearGroup{
				columns: span.Columns,
				indexes: make(map[int64]struct{}),
			}
			groups[span.TableID] = group
		}
		group.indexes[span.ColumnIndex] = struct{}{}
	}

	tableIDs := make([]string, 0, len(groups))
	for tableID := range groups {
		tableIDs = append(tableIDs, tableID)
	}
	sort.Strings(tableIDs)
	requests := make([]*sheets.Request, 0, len(tableIDs))
	for _, tableID := range tableIDs {
		group := groups[tableID]
		columns := cloneTableColumnProperties(group.columns, group.indexes, nil)
		requests = append(requests, &sheets.Request{
			UpdateTable: &sheets.UpdateTableRequest{
				Table: &sheets.Table{
					TableId:          tableID,
					ColumnProperties: columns,
				},
				Fields: "columnProperties",
			},
		})
	}
	return requests, nil
}

func buildTableValidationSetRequests(
	target *sheets.GridRange,
	spans []tableValidationSpan,
	condition *sheets.BooleanCondition,
) ([]*sheets.Request, error) {
	if target == nil || condition == nil {
		return nil, nil
	}
	type setGroup struct {
		columns []*sheets.TableColumnProperties
		indexes map[int64]struct{}
	}
	groups := make(map[string]*setGroup)
	for _, span := range spans {
		if span.SheetID != target.SheetId {
			continue
		}
		if _, _, ok := intersectGridIndexes(span.StartRow, span.EndRow, target.StartRowIndex, target.EndRowIndex); !ok {
			continue
		}
		if _, _, ok := intersectGridIndexes(span.StartCol, span.EndCol, target.StartColumnIndex, target.EndColumnIndex); !ok {
			continue
		}
		if condition.Type != sheetsConditionOneOfList {
			return nil, usagef(
				"table column %d in table %s only supports ONE_OF_LIST dropdown validation",
				span.ColumnIndex+1,
				span.TableID,
			)
		}
		if !gridRangeCoversSpan(target, span) {
			return nil, usagef(
				"range partially intersects table-managed dropdown column %d in table %s; set validation on the full table data column",
				span.ColumnIndex+1,
				span.TableID,
			)
		}
		group := groups[span.TableID]
		if group == nil {
			group = &setGroup{
				columns: span.Columns,
				indexes: make(map[int64]struct{}),
			}
			groups[span.TableID] = group
		}
		group.indexes[span.ColumnIndex] = struct{}{}
	}

	tableIDs := make([]string, 0, len(groups))
	for tableID := range groups {
		tableIDs = append(tableIDs, tableID)
	}
	sort.Strings(tableIDs)
	requests := make([]*sheets.Request, 0, len(tableIDs))
	for _, tableID := range tableIDs {
		group := groups[tableID]
		requests = append(requests, &sheets.Request{
			UpdateTable: &sheets.UpdateTableRequest{
				Table: &sheets.Table{
					TableId:          tableID,
					ColumnProperties: cloneTableColumnProperties(group.columns, group.indexes, condition),
				},
				Fields: "columnProperties",
			},
		})
	}
	return requests, nil
}

func cloneTableColumnProperties(
	columns []*sheets.TableColumnProperties,
	updateIndexes map[int64]struct{},
	condition *sheets.BooleanCondition,
) []*sheets.TableColumnProperties {
	updates := make(map[int64]*sheets.BooleanCondition, len(updateIndexes))
	for index := range updateIndexes {
		updates[index] = condition
	}
	return cloneTableColumnPropertiesWithConditions(columns, updates)
}

func cloneTableColumnPropertiesWithConditions(
	columns []*sheets.TableColumnProperties,
	updates map[int64]*sheets.BooleanCondition,
) []*sheets.TableColumnProperties {
	cloned := make([]*sheets.TableColumnProperties, 0, len(columns))
	for _, column := range columns {
		if column == nil {
			continue
		}
		item := &sheets.TableColumnProperties{
			ColumnIndex:        column.ColumnIndex,
			ColumnName:         column.ColumnName,
			ColumnType:         column.ColumnType,
			DataValidationRule: column.DataValidationRule,
			ForceSendFields:    []string{"ColumnIndex"},
		}
		if condition, update := updates[column.ColumnIndex]; update {
			if condition != nil && condition.Type == sheetsConditionOneOfList {
				item.ColumnType = sheetsTypeDropdown
				item.DataValidationRule = &sheets.TableColumnDataValidationRule{
					Condition: cloneBooleanCondition(condition),
				}
			} else {
				item.ColumnType = sheetsTypeText
				item.DataValidationRule = nil
				item.NullFields = []string{"DataValidationRule"}
			}
		}
		cloned = append(cloned, item)
	}
	return cloned
}

func gridRangeCoversSpan(target *sheets.GridRange, span tableValidationSpan) bool {
	if target == nil || target.SheetId != span.SheetID {
		return false
	}
	rowsCovered := target.StartRowIndex <= span.StartRow && (target.EndRowIndex == 0 || target.EndRowIndex >= span.EndRow)
	colsCovered := target.StartColumnIndex <= span.StartCol && (target.EndColumnIndex == 0 || target.EndColumnIndex >= span.EndCol)
	return rowsCovered && colsCovered
}

func subtractTableValidationSpans(target *sheets.GridRange, spans []tableValidationSpan) []*sheets.GridRange {
	if target == nil {
		return nil
	}
	ranges := []*sheets.GridRange{target}
	for _, span := range spans {
		if span.SheetID != target.SheetId {
			continue
		}
		cut := &sheets.GridRange{
			SheetId:          span.SheetID,
			StartRowIndex:    span.StartRow,
			EndRowIndex:      span.EndRow,
			StartColumnIndex: span.StartCol,
			EndColumnIndex:   span.EndCol,
		}
		next := make([]*sheets.GridRange, 0, len(ranges)+3)
		for _, current := range ranges {
			next = append(next, subtractGridRange(current, cut)...)
		}
		ranges = next
	}
	return ranges
}

func subtractGridRange(current, cut *sheets.GridRange) []*sheets.GridRange {
	if current == nil || cut == nil || current.SheetId != cut.SheetId {
		return []*sheets.GridRange{current}
	}
	rowStart := max(current.StartRowIndex, cut.StartRowIndex)
	rowEnd := min(current.EndRowIndex, cut.EndRowIndex)
	colStart := max(current.StartColumnIndex, cut.StartColumnIndex)
	colEnd := min(current.EndColumnIndex, cut.EndColumnIndex)
	if rowEnd <= rowStart || colEnd <= colStart {
		return []*sheets.GridRange{current}
	}

	makeRange := func(startRow, endRow, startCol, endCol int64) *sheets.GridRange {
		return &sheets.GridRange{
			SheetId:          current.SheetId,
			StartRowIndex:    startRow,
			EndRowIndex:      endRow,
			StartColumnIndex: startCol,
			EndColumnIndex:   endCol,
			ForceSendFields:  []string{"SheetId"},
		}
	}
	parts := make([]*sheets.GridRange, 0, 4)
	if current.StartRowIndex < rowStart {
		parts = append(parts, makeRange(current.StartRowIndex, rowStart, current.StartColumnIndex, current.EndColumnIndex))
	}
	if rowEnd < current.EndRowIndex {
		parts = append(parts, makeRange(rowEnd, current.EndRowIndex, current.StartColumnIndex, current.EndColumnIndex))
	}
	if current.StartColumnIndex < colStart {
		parts = append(parts, makeRange(rowStart, rowEnd, current.StartColumnIndex, colStart))
	}
	if colEnd < current.EndColumnIndex {
		parts = append(parts, makeRange(rowStart, rowEnd, colEnd, current.EndColumnIndex))
	}
	return parts
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

func cloneBooleanCondition(condition *sheets.BooleanCondition) *sheets.BooleanCondition {
	if condition == nil {
		return nil
	}
	values := make([]*sheets.ConditionValue, 0, len(condition.Values))
	for _, value := range condition.Values {
		if value == nil {
			continue
		}
		values = append(values, &sheets.ConditionValue{
			RelativeDate:     value.RelativeDate,
			UserEnteredValue: value.UserEnteredValue,
			ForceSendFields:  []string{"UserEnteredValue"},
		})
	}
	return &sheets.BooleanCondition{Type: condition.Type, Values: values}
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
		startRow, endRow, ok := intersectGridIndexes(span.StartRow, span.EndRow, target.StartRowIndex, target.EndRowIndex)
		if !ok {
			continue
		}
		startCol, endCol, ok := intersectGridIndexes(span.StartCol, span.EndCol, target.StartColumnIndex, target.EndColumnIndex)
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
					A1:    formatA1Cell(title, int(row+1), int(col+1)),
					Row:   int(row + 1),
					Col:   int(col + 1),
					Rule:  span.Rule,
				})
			}
		}
	}
	return items
}

func intersectGridIndexes(aStart, aEnd, bStart, bEnd int64) (int64, int64, bool) {
	start := max(aStart, bStart)
	end := aEnd
	if bEnd > 0 && (end == 0 || bEnd < end) {
		end = bEnd
	}
	return start, end, end > start
}

type validationCopySegment struct {
	StartRow int64
	EndRow   int64
	StartCol int64
	EndCol   int64
	RuleKey  string
	Rule     *sheets.DataValidationRule
}

const maxTableValidationCopySegments = 1000

type tableValidationCopyOptions struct {
	ordinarySourceValidationKnown bool
	ordinaryValidatedCells        []validationCellCoordinate
}

type validationCellCoordinate struct {
	Row int64
	Col int64
}

func buildTableValidationCopyRequests(
	source, destination *sheets.GridRange,
	transpose bool,
	spans []tableValidationSpan,
	options ...tableValidationCopyOptions,
) ([]*sheets.Request, error) {
	if source == nil || destination == nil ||
		source.EndRowIndex <= source.StartRowIndex || source.EndColumnIndex <= source.StartColumnIndex ||
		destination.EndRowIndex <= destination.StartRowIndex || destination.EndColumnIndex <= destination.StartColumnIndex {
		return nil, nil
	}

	destination = effectiveCopyDestination(source, destination, transpose)
	sourceSpans := relevantSourceTableValidationSpans(source, spans)
	ordinarySourceRanges := subtractTableValidationSpans(source, sourceSpans)
	destinationSpan, hasDestinationTable := firstIntersectingTableValidationSpan(destination, spans)
	opts := tableValidationCopyOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	if hasDestinationTable && len(ordinarySourceRanges) > 0 {
		if !opts.ordinarySourceValidationKnown {
			return nil, usagef(
				"copying validation into table column %d in table %s requires a table-column source",
				destinationSpan.ColumnIndex+1,
				destinationSpan.TableID,
			)
		}
		for _, candidate := range spans {
			if tableValidationSpanIntersects(destination, candidate) &&
				ordinaryValidationMapsToTableSpan(
					source,
					destination,
					transpose,
					opts.ordinaryValidatedCells,
					ordinarySourceRanges,
					candidate,
				) {
				return nil, usagef(
					"copying ordinary cell validation into table column %d in table %s is not supported",
					candidate.ColumnIndex+1,
					candidate.TableID,
				)
			}
		}
	}
	if len(sourceSpans) == 0 && !hasDestinationTable {
		return nil, nil
	}

	merged := []validationCopySegment{}
	if len(sourceSpans) > 0 {
		segments, err := buildTableValidationCopySegments(source, destination, transpose, sourceSpans)
		if err != nil {
			return nil, err
		}
		merged = mergeValidationCopySegments(segments)
	}

	coverageSegments := append([]validationCopySegment(nil), merged...)
	if hasDestinationTable && len(ordinarySourceRanges) > 0 {
		ordinarySpans := make([]tableValidationSpan, 0, len(ordinarySourceRanges))
		for _, ordinaryRange := range ordinarySourceRanges {
			ordinarySpans = append(ordinarySpans, tableValidationSpan{
				SheetID:  ordinaryRange.SheetId,
				StartRow: ordinaryRange.StartRowIndex,
				EndRow:   ordinaryRange.EndRowIndex,
				StartCol: ordinaryRange.StartColumnIndex,
				EndCol:   ordinaryRange.EndColumnIndex,
			})
		}
		ordinarySegments, err := buildTableValidationCopySegments(
			source,
			destination,
			transpose,
			ordinarySpans,
		)
		if err != nil {
			return nil, err
		}
		coverageSegments = mergeValidationCopySegments(append(coverageSegments, ordinarySegments...))
	}

	tableRequests := []*sheets.Request{}
	protectedSpans := []tableValidationSpan{}
	if hasDestinationTable {
		var err error
		tableRequests, protectedSpans, err = buildDestinationTableValidationCopyRequests(
			destination,
			spans,
			coverageSegments,
		)
		if err != nil {
			return nil, err
		}
	}

	requests := append([]*sheets.Request(nil), tableRequests...)
	for _, segment := range merged {
		ranges := []*sheets.GridRange{{
			SheetId:          destination.SheetId,
			StartRowIndex:    segment.StartRow,
			EndRowIndex:      segment.EndRow,
			StartColumnIndex: segment.StartCol,
			EndColumnIndex:   segment.EndCol,
			ForceSendFields:  []string{"SheetId"},
		}}
		for _, span := range protectedSpans {
			cut := &sheets.GridRange{
				SheetId:          span.SheetID,
				StartRowIndex:    span.StartRow,
				EndRowIndex:      span.EndRow,
				StartColumnIndex: span.StartCol,
				EndColumnIndex:   span.EndCol,
			}
			next := make([]*sheets.GridRange, 0, len(ranges)+3)
			for _, current := range ranges {
				next = append(next, subtractGridRange(current, cut)...)
			}
			ranges = next
		}
		for _, gridRange := range ranges {
			requests = append(requests, &sheets.Request{
				SetDataValidation: &sheets.SetDataValidationRequest{
					Range:                gridRange,
					Rule:                 segment.Rule,
					FilteredRowsIncluded: true,
					ForceSendFields:      []string{"FilteredRowsIncluded"},
				},
			})
		}
	}
	return requests, nil
}

func firstIntersectingTableValidationSpan(
	target *sheets.GridRange,
	spans []tableValidationSpan,
) (tableValidationSpan, bool) {
	if target == nil {
		return tableValidationSpan{}, false
	}
	for _, span := range spans {
		if tableValidationSpanIntersects(target, span) {
			return span, true
		}
	}
	return tableValidationSpan{}, false
}

func ordinaryValidationMapsToTableSpan(
	source, destination *sheets.GridRange,
	transpose bool,
	validatedCells []validationCellCoordinate,
	ordinarySourceRanges []*sheets.GridRange,
	span tableValidationSpan,
) bool {
	if source == nil || destination == nil || !tableValidationSpanIntersects(destination, span) {
		return false
	}
	startRow, endRow, _ := intersectGridIndexes(
		span.StartRow,
		span.EndRow,
		destination.StartRowIndex,
		destination.EndRowIndex,
	)
	startCol, endCol, _ := intersectGridIndexes(
		span.StartCol,
		span.EndCol,
		destination.StartColumnIndex,
		destination.EndColumnIndex,
	)
	patternHeight := source.EndRowIndex - source.StartRowIndex
	patternWidth := source.EndColumnIndex - source.StartColumnIndex
	if transpose {
		patternHeight, patternWidth = patternWidth, patternHeight
	}
	for _, cell := range validatedCells {
		if !gridRangesContainCell(ordinarySourceRanges, source.SheetId, cell.Row, cell.Col) {
			continue
		}
		rowOffset := cell.Row - source.StartRowIndex
		colOffset := cell.Col - source.StartColumnIndex
		if transpose {
			rowOffset, colOffset = colOffset, rowOffset
		}
		if repeatingOffsetIntersects(
			destination.StartRowIndex+rowOffset,
			patternHeight,
			startRow,
			endRow,
		) && repeatingOffsetIntersects(
			destination.StartColumnIndex+colOffset,
			patternWidth,
			startCol,
			endCol,
		) {
			return true
		}
	}
	return false
}

func gridRangesContainCell(ranges []*sheets.GridRange, sheetID, row, col int64) bool {
	for _, gridRange := range ranges {
		if gridRange != nil &&
			gridRange.SheetId == sheetID &&
			row >= gridRange.StartRowIndex && row < gridRange.EndRowIndex &&
			col >= gridRange.StartColumnIndex && col < gridRange.EndColumnIndex {
			return true
		}
	}
	return false
}

func repeatingOffsetIntersects(base, step, start, end int64) bool {
	if step <= 0 || end <= start || base >= end {
		return false
	}
	if base < start {
		base += ((start - base + step - 1) / step) * step
	}
	return base < end
}

func tableValidationSpanIntersects(target *sheets.GridRange, span tableValidationSpan) bool {
	if target == nil || span.SheetID != target.SheetId {
		return false
	}
	if _, _, ok := intersectGridIndexes(span.StartRow, span.EndRow, target.StartRowIndex, target.EndRowIndex); !ok {
		return false
	}
	_, _, ok := intersectGridIndexes(span.StartCol, span.EndCol, target.StartColumnIndex, target.EndColumnIndex)
	return ok
}

func resolveTableValidationCopyOptions(
	ctx context.Context,
	svc *sheets.Service,
	spreadsheetID string,
	source, destination *sheets.GridRange,
	spans []tableValidationSpan,
	catalog *spreadsheetRangeCatalog,
	transpose bool,
) (tableValidationCopyOptions, error) {
	effectiveDestination := effectiveCopyDestination(source, destination, transpose)
	if _, ok := firstIntersectingTableValidationSpan(effectiveDestination, spans); !ok {
		return tableValidationCopyOptions{}, nil
	}
	sourceSpans := relevantSourceTableValidationSpans(source, spans)
	if len(subtractTableValidationSpans(source, sourceSpans)) == 0 {
		return tableValidationCopyOptions{}, nil
	}
	if catalog == nil {
		return tableValidationCopyOptions{}, fmt.Errorf("missing spreadsheet range catalog")
	}
	sheetTitle, ok := catalog.SheetTitlesByID[source.SheetId]
	if !ok {
		return tableValidationCopyOptions{}, usagef("copy source references unknown sheet ID %d", source.SheetId)
	}
	sourceA1 := gridRangeToA1(sheetTitle, source)
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
		ordinarySourceValidationKnown: true,
		ordinaryValidatedCells:        validatedCells,
	}, nil
}

func relevantSourceTableValidationSpans(source *sheets.GridRange, spans []tableValidationSpan) []tableValidationSpan {
	relevant := make([]tableValidationSpan, 0)
	for _, span := range spans {
		if span.SheetID != source.SheetId {
			continue
		}
		startRow, endRow, rowsOK := intersectGridIndexes(
			span.StartRow,
			span.EndRow,
			source.StartRowIndex,
			source.EndRowIndex,
		)
		startCol, endCol, colsOK := intersectGridIndexes(
			span.StartCol,
			span.EndCol,
			source.StartColumnIndex,
			source.EndColumnIndex,
		)
		if !rowsOK || !colsOK {
			continue
		}
		clipped := span
		clipped.StartRow = startRow
		clipped.EndRow = endRow
		clipped.StartCol = startCol
		clipped.EndCol = endCol
		relevant = append(relevant, clipped)
	}
	return relevant
}

func buildTableValidationCopySegments(
	source, destination *sheets.GridRange,
	transpose bool,
	spans []tableValidationSpan,
) ([]validationCopySegment, error) {
	sourceHeight := source.EndRowIndex - source.StartRowIndex
	sourceWidth := source.EndColumnIndex - source.StartColumnIndex
	patternHeight, patternWidth := sourceHeight, sourceWidth
	if transpose {
		patternHeight, patternWidth = sourceWidth, sourceHeight
	}
	rowTiles := (destination.EndRowIndex - destination.StartRowIndex) / patternHeight
	colTiles := (destination.EndColumnIndex - destination.StartColumnIndex) / patternWidth
	patternSegments := make([]validationCopySegment, 0, len(spans))
	for _, span := range spans {
		ruleKey, err := validationRuleKey(span.Rule)
		if err != nil {
			return nil, err
		}
		patternSegments = append(patternSegments, validationCopySegment{
			StartRow: span.StartRow,
			EndRow:   span.EndRow,
			StartCol: span.StartCol,
			EndCol:   span.EndCol,
			RuleKey:  ruleKey,
			Rule:     span.Rule,
		})
	}
	patternSegments = mergeValidationCopySegments(patternSegments)

	segments := make([]validationCopySegment, 0, len(patternSegments))
	var err error
	for _, patternSegment := range patternSegments {
		relRowStart := patternSegment.StartRow - source.StartRowIndex
		relRowEnd := patternSegment.EndRow - source.StartRowIndex
		relColStart := patternSegment.StartCol - source.StartColumnIndex
		relColEnd := patternSegment.EndCol - source.StartColumnIndex
		mappedRowStart, mappedRowEnd := relRowStart, relRowEnd
		mappedColStart, mappedColEnd := relColStart, relColEnd
		if transpose {
			mappedRowStart, mappedRowEnd = relColStart, relColEnd
			mappedColStart, mappedColEnd = relRowStart, relRowEnd
		}
		fullRows := mappedRowStart == 0 && mappedRowEnd == patternHeight
		fullCols := mappedColStart == 0 && mappedColEnd == patternWidth
		if fullRows && fullCols {
			segments, err = appendValidationCopySegment(segments, validationCopySegment{
				StartRow: destination.StartRowIndex,
				EndRow:   destination.EndRowIndex,
				StartCol: destination.StartColumnIndex,
				EndCol:   destination.EndColumnIndex,
				RuleKey:  patternSegment.RuleKey,
				Rule:     patternSegment.Rule,
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		if fullRows {
			for colTile := int64(0); colTile < colTiles; colTile++ {
				segments, err = appendValidationCopySegment(segments, validationCopySegment{
					StartRow: destination.StartRowIndex,
					EndRow:   destination.EndRowIndex,
					StartCol: destination.StartColumnIndex + colTile*patternWidth + mappedColStart,
					EndCol:   destination.StartColumnIndex + colTile*patternWidth + mappedColEnd,
					RuleKey:  patternSegment.RuleKey,
					Rule:     patternSegment.Rule,
				})
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		if fullCols {
			for rowTile := int64(0); rowTile < rowTiles; rowTile++ {
				segments, err = appendValidationCopySegment(segments, validationCopySegment{
					StartRow: destination.StartRowIndex + rowTile*patternHeight + mappedRowStart,
					EndRow:   destination.StartRowIndex + rowTile*patternHeight + mappedRowEnd,
					StartCol: destination.StartColumnIndex,
					EndCol:   destination.EndColumnIndex,
					RuleKey:  patternSegment.RuleKey,
					Rule:     patternSegment.Rule,
				})
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		for rowTile := int64(0); rowTile < rowTiles; rowTile++ {
			for colTile := int64(0); colTile < colTiles; colTile++ {
				segment := validationCopySegment{
					StartRow: destination.StartRowIndex + rowTile*patternHeight + mappedRowStart,
					EndRow:   destination.StartRowIndex + rowTile*patternHeight + mappedRowEnd,
					StartCol: destination.StartColumnIndex + colTile*patternWidth + mappedColStart,
					EndCol:   destination.StartColumnIndex + colTile*patternWidth + mappedColEnd,
					RuleKey:  patternSegment.RuleKey,
					Rule:     patternSegment.Rule,
				}
				segments, err = appendValidationCopySegment(segments, segment)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return segments, nil
}

func appendValidationCopySegment(
	segments []validationCopySegment,
	segment validationCopySegment,
) ([]validationCopySegment, error) {
	if len(segments) >= maxTableValidationCopySegments {
		return nil, usagef(
			"copying table-managed validation requires more than %d supplemental ranges; narrow the destination or copy one source footprint",
			maxTableValidationCopySegments,
		)
	}
	return append(segments, segment), nil
}

func mergeValidationCopySegments(segments []validationCopySegment) []validationCopySegment {
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].StartCol != segments[j].StartCol {
			return segments[i].StartCol < segments[j].StartCol
		}
		if segments[i].EndCol != segments[j].EndCol {
			return segments[i].EndCol < segments[j].EndCol
		}
		if segments[i].RuleKey != segments[j].RuleKey {
			return segments[i].RuleKey < segments[j].RuleKey
		}
		return segments[i].StartRow < segments[j].StartRow
	})
	vertical := make([]validationCopySegment, 0, len(segments))
	for _, segment := range segments {
		last := len(vertical) - 1
		if last >= 0 &&
			vertical[last].StartCol == segment.StartCol &&
			vertical[last].EndCol == segment.EndCol &&
			vertical[last].RuleKey == segment.RuleKey &&
			vertical[last].EndRow == segment.StartRow {
			vertical[last].EndRow = segment.EndRow
			continue
		}
		vertical = append(vertical, segment)
	}

	sort.Slice(vertical, func(i, j int) bool {
		if vertical[i].StartRow != vertical[j].StartRow {
			return vertical[i].StartRow < vertical[j].StartRow
		}
		if vertical[i].EndRow != vertical[j].EndRow {
			return vertical[i].EndRow < vertical[j].EndRow
		}
		if vertical[i].RuleKey != vertical[j].RuleKey {
			return vertical[i].RuleKey < vertical[j].RuleKey
		}
		return vertical[i].StartCol < vertical[j].StartCol
	})
	merged := make([]validationCopySegment, 0, len(vertical))
	for _, segment := range vertical {
		last := len(merged) - 1
		if last >= 0 &&
			merged[last].StartRow == segment.StartRow &&
			merged[last].EndRow == segment.EndRow &&
			merged[last].RuleKey == segment.RuleKey &&
			merged[last].EndCol == segment.StartCol {
			merged[last].EndCol = segment.EndCol
			continue
		}
		merged = append(merged, segment)
	}
	return merged
}

func buildDestinationTableValidationCopyRequests(
	destination *sheets.GridRange,
	spans []tableValidationSpan,
	segments []validationCopySegment,
) ([]*sheets.Request, []tableValidationSpan, error) {
	type copyGroup struct {
		columns    []*sheets.TableColumnProperties
		conditions map[int64]*sheets.BooleanCondition
	}
	groups := make(map[string]*copyGroup)
	protected := make([]tableValidationSpan, 0)
	for _, span := range spans {
		if span.SheetID != destination.SheetId {
			continue
		}
		if _, _, ok := intersectGridIndexes(span.StartRow, span.EndRow, destination.StartRowIndex, destination.EndRowIndex); !ok {
			continue
		}
		if _, _, ok := intersectGridIndexes(span.StartCol, span.EndCol, destination.StartColumnIndex, destination.EndColumnIndex); !ok {
			continue
		}
		startRow, endRow, _ := intersectGridIndexes(
			span.StartRow,
			span.EndRow,
			destination.StartRowIndex,
			destination.EndRowIndex,
		)
		startCol, endCol, _ := intersectGridIndexes(
			span.StartCol,
			span.EndCol,
			destination.StartColumnIndex,
			destination.EndColumnIndex,
		)
		condition, ruleKey, covered := validationRuleCoverage(
			segments,
			startRow,
			endRow,
			startCol,
			endCol,
		)
		if !covered {
			return nil, nil, usagef(
				"copy into table column %d in table %s requires a table-column source covering the destination",
				span.ColumnIndex+1,
				span.TableID,
			)
		}
		existingKey, err := validationRuleKey(span.Rule)
		if err != nil {
			return nil, nil, err
		}
		if ruleKey == existingKey {
			protected = append(protected, span)
			continue
		}
		if !gridRangeCoversSpan(destination, span) {
			return nil, nil, usagef(
				"copy destination partially intersects table-managed dropdown column %d in table %s with a different rule",
				span.ColumnIndex+1,
				span.TableID,
			)
		}

		group := groups[span.TableID]
		if group == nil {
			group = &copyGroup{
				columns:    span.Columns,
				conditions: make(map[int64]*sheets.BooleanCondition),
			}
			groups[span.TableID] = group
		}
		group.conditions[span.ColumnIndex] = condition
		protected = append(protected, span)
	}

	tableIDs := make([]string, 0, len(groups))
	for tableID := range groups {
		tableIDs = append(tableIDs, tableID)
	}
	sort.Strings(tableIDs)
	requests := make([]*sheets.Request, 0, len(tableIDs))
	for _, tableID := range tableIDs {
		group := groups[tableID]
		requests = append(requests, &sheets.Request{
			UpdateTable: &sheets.UpdateTableRequest{
				Table: &sheets.Table{
					TableId:          tableID,
					ColumnProperties: cloneTableColumnPropertiesWithConditions(group.columns, group.conditions),
				},
				Fields: "columnProperties",
			},
		})
	}
	return requests, protected, nil
}

func validationRuleCoverage(
	segments []validationCopySegment,
	startRow, endRow, startCol, endCol int64,
) (*sheets.BooleanCondition, string, bool) {
	if endRow <= startRow || endCol <= startCol {
		return nil, "", false
	}
	type interval struct {
		start int64
		end   int64
		key   string
		rule  *sheets.DataValidationRule
	}
	expectedKey := ""
	haveExpectedKey := false
	var expectedCondition *sheets.BooleanCondition
	for col := startCol; col < endCol; col++ {
		intervals := make([]interval, 0)
		for _, segment := range segments {
			if col < segment.StartCol || col >= segment.EndCol {
				continue
			}
			overlapStart := max(startRow, segment.StartRow)
			overlapEnd := min(endRow, segment.EndRow)
			if overlapEnd > overlapStart {
				intervals = append(intervals, interval{
					start: overlapStart,
					end:   overlapEnd,
					key:   segment.RuleKey,
					rule:  segment.Rule,
				})
			}
		}
		sort.Slice(intervals, func(i, j int) bool { return intervals[i].start < intervals[j].start })
		cursor := startRow
		ruleKey := ""
		haveRuleKey := false
		var condition *sheets.BooleanCondition
		for _, item := range intervals {
			if item.start > cursor {
				return nil, "", false
			}
			if item.end <= cursor {
				continue
			}
			if !haveRuleKey {
				ruleKey = item.key
				haveRuleKey = true
				if item.rule != nil {
					condition = item.rule.Condition
				}
			} else if item.key != ruleKey {
				return nil, "", false
			}
			cursor = item.end
			if cursor >= endRow {
				break
			}
		}
		if cursor < endRow {
			return nil, "", false
		}
		if !haveExpectedKey {
			expectedKey = ruleKey
			haveExpectedKey = true
			expectedCondition = condition
		} else if ruleKey != expectedKey {
			return nil, "", false
		}
	}
	return expectedCondition, expectedKey, haveExpectedKey
}

func validationRuleKey(rule *sheets.DataValidationRule) (string, error) {
	if rule == nil || rule.Condition == nil {
		return "", nil
	}
	encoded, err := json.Marshal(rule)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func effectiveCopyDestination(source, destination *sheets.GridRange, transpose bool) *sheets.GridRange {
	if source == nil || destination == nil {
		return destination
	}
	minHeight := source.EndRowIndex - source.StartRowIndex
	minWidth := source.EndColumnIndex - source.StartColumnIndex
	if transpose {
		minHeight, minWidth = minWidth, minHeight
	}
	effective := *destination
	effective.EndRowIndex = effective.StartRowIndex + effectivePasteLength(
		minHeight,
		effective.EndRowIndex-effective.StartRowIndex,
	)
	effective.EndColumnIndex = effective.StartColumnIndex + effectivePasteLength(
		minWidth,
		effective.EndColumnIndex-effective.StartColumnIndex,
	)
	return &effective
}

func effectivePasteLength(sourceLength, destinationLength int64) int64 {
	if destinationLength >= sourceLength && destinationLength%sourceLength == 0 {
		return destinationLength
	}
	return sourceLength
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
	supplemental, err := buildTableValidationCopyRequests(sourceGrid, destGrid, false, spans, copyOptions)
	if err != nil {
		return err
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
	r, err := parseA1Range(a1)
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
