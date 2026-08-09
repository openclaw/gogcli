package sheetschart

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/sheetsa1"
)

var (
	errMissingSpec          = errors.New("--spec-json must contain a ChartSpec or an EmbeddedChart with spec")
	errEmptyCellReference   = errors.New("empty cell reference")
	errInvalidCellReference = errors.New("invalid cell reference")
	errInvalidCellRow       = errors.New("invalid row in cell reference")
	gridRangeType           = reflect.TypeOf(sheets.GridRange{})
)

const sheetIDFieldKey = "SheetId"

type Anchor struct {
	Row int
	Col int
}

func ParseEmbedded(data []byte) (*sheets.EmbeddedChart, error) {
	var chart sheets.EmbeddedChart
	if err := json.Unmarshal(data, &chart); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	if chart.Spec != nil && !specIsZero(chart.Spec) {
		return &chart, nil
	}

	spec, err := ParseSpec(data)
	if err != nil {
		return nil, err
	}

	chart.Spec = spec

	return &chart, nil
}

func ParseSpec(data []byte) (*sheets.ChartSpec, error) {
	var spec sheets.ChartSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	if !specIsZero(&spec) {
		return &spec, nil
	}

	var chart sheets.EmbeddedChart
	if err := json.Unmarshal(data, &chart); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	if chart.Spec != nil && !specIsZero(chart.Spec) {
		return chart.Spec, nil
	}

	return nil, errMissingSpec
}

func NormalizeZeroSheetIDs(spec *sheets.ChartSpec, sheetID int64, preserveZero bool) {
	visitGridRanges(reflect.ValueOf(spec), func(gridRange *sheets.GridRange) {
		if gridRange == nil || gridRange.SheetId != 0 {
			return
		}

		if !preserveZero {
			gridRange.SheetId = sheetID
		}

		gridRange.ForceSendFields = appendForceSendField(gridRange.ForceSendFields, sheetIDFieldKey)
	})
}

func HasZeroSheetIDs(spec *sheets.ChartSpec) bool {
	var found bool

	visitGridRanges(reflect.ValueOf(spec), func(gridRange *sheets.GridRange) {
		if gridRange != nil && gridRange.SheetId == 0 {
			found = true
		}
	})

	return found
}

func ParseAnchor(cell string) (Anchor, error) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return Anchor{}, errEmptyCellReference
	}

	index := 0
	for index < len(cell) &&
		((cell[index] >= 'A' && cell[index] <= 'Z') || (cell[index] >= 'a' && cell[index] <= 'z')) {
		index++
	}

	if index == 0 || index == len(cell) {
		return Anchor{}, fmt.Errorf("%w %q", errInvalidCellReference, cell)
	}

	column, err := sheetsa1.ColumnIndex(strings.ToUpper(cell[:index]))
	if err != nil {
		return Anchor{}, fmt.Errorf("%w", err)
	}

	row, err := strconv.Atoi(cell[index:])
	if err != nil || row < 1 {
		return Anchor{}, fmt.Errorf("%w %q", errInvalidCellRow, cell)
	}

	return Anchor{Row: row, Col: column}, nil
}

func BuildPosition(sheetID int64, anchor string, width, height int64) (*sheets.EmbeddedObjectPosition, error) {
	var rowIndex, columnIndex int64

	if anchor != "" {
		parsed, err := ParseAnchor(anchor)
		if err != nil {
			return nil, err
		}

		rowIndex = int64(parsed.Row - 1)
		columnIndex = int64(parsed.Col - 1)
	}

	return &sheets.EmbeddedObjectPosition{
		OverlayPosition: &sheets.OverlayPosition{
			AnchorCell: &sheets.GridCoordinate{
				SheetId:         sheetID,
				RowIndex:        rowIndex,
				ColumnIndex:     columnIndex,
				ForceSendFields: []string{sheetIDFieldKey, "RowIndex", "ColumnIndex"},
			},
			WidthPixels:  width,
			HeightPixels: height,
		},
	}, nil
}

func specIsZero(spec *sheets.ChartSpec) bool {
	if spec == nil {
		return true
	}

	return reflect.ValueOf(*spec).IsZero()
}

func visitGridRanges(value reflect.Value, visit func(*sheets.GridRange)) {
	if !value.IsValid() {
		return
	}

	switch value.Kind() {
	case reflect.Interface:
		if !value.IsNil() {
			visitGridRanges(value.Elem(), visit)
		}
	case reflect.Ptr:
		if value.IsNil() {
			return
		}

		if value.Type().Elem() == gridRangeType {
			if value.CanInterface() {
				visit(value.Interface().(*sheets.GridRange))
			}

			return
		}

		visitGridRanges(value.Elem(), visit)
	case reflect.Struct:
		if value.Type() == gridRangeType {
			if value.CanAddr() && value.Addr().CanInterface() {
				visit(value.Addr().Interface().(*sheets.GridRange))
			}

			return
		}

		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if field.CanSet() ||
				field.Kind() == reflect.Ptr ||
				field.Kind() == reflect.Slice ||
				field.Kind() == reflect.Interface {
				visitGridRanges(field, visit)
			}
		}
	case reflect.Slice:
		for index := 0; index < value.Len(); index++ {
			visitGridRanges(value.Index(index), visit)
		}
	}
}

func appendForceSendField(fields []string, field string) []string {
	for _, existing := range fields {
		if existing == field {
			return fields
		}
	}

	return append(fields, field)
}
