package sheetsvalidation

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

const (
	conditionOneOfList = "ONE_OF_LIST"
	columnTypeDropdown = "DROPDOWN"
	columnTypeText     = "TEXT"
)

type ValidationError string

func (e ValidationError) Error() string {
	return string(e)
}

type Span struct {
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

func BuildCondition(rawType string, rawValues []string) (*sheets.BooleanCondition, error) {
	conditionType := strings.ToUpper(strings.TrimSpace(rawType))
	conditionType = strings.NewReplacer("-", "_", " ", "_").Replace(conditionType)

	if conditionType == "" {
		return nil, invalid("empty --type")
	}

	minValues, maxValues, ok := conditionArity(conditionType)
	if !ok {
		return nil, invalidf("unsupported validation --type %q", rawType)
	}

	values := append([]string(nil), rawValues...)
	if len(values) < minValues || (maxValues >= 0 && len(values) > maxValues) {
		switch {
		case minValues == maxValues:
			return nil, invalidf("%s requires exactly %d --value flag(s)", conditionType, minValues)
		case maxValues < 0:
			return nil, invalidf("%s requires at least %d --value flag(s)", conditionType, minValues)
		default:
			return nil, invalidf("%s accepts %d to %d --value flag(s)", conditionType, minValues, maxValues)
		}
	}

	if conditionType == conditionOneOfList {
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") {
				return nil, invalid("ONE_OF_LIST values cannot be formulas")
			}
		}
	}

	if conditionType == "ONE_OF_RANGE" {
		values[0] = strings.TrimSpace(values[0])
		if values[0] == "" || values[0] == "=" || values[0] == "+" {
			return nil, invalid("ONE_OF_RANGE requires a non-empty range value")
		}

		if !strings.HasPrefix(values[0], "=") && !strings.HasPrefix(values[0], "+") {
			values[0] = "=" + values[0]
		}
	}

	if conditionType == "CUSTOM_FORMULA" {
		values[0] = strings.TrimSpace(values[0])
		if !strings.HasPrefix(values[0], "=") && !strings.HasPrefix(values[0], "+") {
			return nil, invalid("CUSTOM_FORMULA value must begin with = or +")
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

func BuildClearRequests(target *sheets.GridRange, spans []Span) ([]*sheets.Request, error) {
	if target == nil {
		return nil, nil
	}
	type clearGroup struct {
		columns []*sheets.TableColumnProperties
		indexes map[int64]struct{}
	}

	groups := make(map[string]*clearGroup)

	for _, span := range spans {
		if span.Rule == nil || span.SheetID != target.SheetId {
			continue
		}

		if _, _, ok := IntersectGridIndexes(
			span.StartRow,
			span.EndRow,
			target.StartRowIndex,
			target.EndRowIndex,
		); !ok {
			continue
		}

		if _, _, ok := IntersectGridIndexes(
			span.StartCol,
			span.EndCol,
			target.StartColumnIndex,
			target.EndColumnIndex,
		); !ok {
			continue
		}

		if !GridRangeCoversSpan(target, span) {
			return nil, invalidf(
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
		columns := CloneTableColumnProperties(group.columns, group.indexes, nil)
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

func BuildSetRequests(
	target *sheets.GridRange,
	spans []Span,
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

		if _, _, ok := IntersectGridIndexes(
			span.StartRow,
			span.EndRow,
			target.StartRowIndex,
			target.EndRowIndex,
		); !ok {
			continue
		}

		if _, _, ok := IntersectGridIndexes(
			span.StartCol,
			span.EndCol,
			target.StartColumnIndex,
			target.EndColumnIndex,
		); !ok {
			continue
		}

		if condition.Type != conditionOneOfList {
			return nil, invalidf(
				"table column %d in table %s only supports ONE_OF_LIST dropdown validation",
				span.ColumnIndex+1,
				span.TableID,
			)
		}

		if !GridRangeCoversSpan(target, span) {
			return nil, invalidf(
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
					ColumnProperties: CloneTableColumnProperties(group.columns, group.indexes, condition),
				},
				Fields: "columnProperties",
			},
		})
	}

	return requests, nil
}

func CloneTableColumnProperties(
	columns []*sheets.TableColumnProperties,
	updateIndexes map[int64]struct{},
	condition *sheets.BooleanCondition,
) []*sheets.TableColumnProperties {
	updates := make(map[int64]*sheets.BooleanCondition, len(updateIndexes))
	for index := range updateIndexes {
		updates[index] = condition
	}

	return CloneTableColumnPropertiesWithConditions(columns, updates)
}

func CloneTableColumnPropertiesWithConditions(
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
			if condition != nil && condition.Type == conditionOneOfList {
				item.ColumnType = columnTypeDropdown
				item.DataValidationRule = &sheets.TableColumnDataValidationRule{
					Condition: CloneCondition(condition),
				}
			} else {
				item.ColumnType = columnTypeText
				item.DataValidationRule = nil
				item.NullFields = []string{"DataValidationRule"}
			}
		}

		cloned = append(cloned, item)
	}

	return cloned
}

func GridRangeCoversSpan(target *sheets.GridRange, span Span) bool {
	if target == nil || target.SheetId != span.SheetID {
		return false
	}
	rowsCovered := target.StartRowIndex <= span.StartRow && (target.EndRowIndex == 0 || target.EndRowIndex >= span.EndRow)
	colsCovered := target.StartColumnIndex <= span.StartCol && (target.EndColumnIndex == 0 || target.EndColumnIndex >= span.EndCol)

	return rowsCovered && colsCovered
}

func SubtractSpans(target *sheets.GridRange, spans []Span) []*sheets.GridRange {
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

		next := make([]*sheets.GridRange, 0, len(ranges))
		for _, current := range ranges {
			next = append(next, SubtractRange(current, cut)...)
		}
		ranges = next
	}

	return ranges
}

func SubtractRange(current, cut *sheets.GridRange) []*sheets.GridRange {
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

func CloneCondition(condition *sheets.BooleanCondition) *sheets.BooleanCondition {
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

func IntersectGridIndexes(aStart, aEnd, bStart, bEnd int64) (int64, int64, bool) {
	start := max(aStart, bStart)
	end := aEnd

	if bEnd > 0 && (end == 0 || bEnd < end) {
		end = bEnd
	}

	return start, end, end > start
}

func conditionArity(conditionType string) (int, int, bool) {
	switch conditionType {
	case "TEXT_IS_EMAIL", "TEXT_IS_URL", "DATE_IS_VALID":
		return 0, 0, true
	case "BOOLEAN":
		return 0, 2, true
	case conditionOneOfList:
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

func invalid(message string) error {
	return ValidationError(message)
}

func invalidf(format string, args ...any) error {
	return ValidationError(fmt.Sprintf(format, args...))
}
