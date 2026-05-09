package cmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"
)

func parseEmbeddedChartJSON(b []byte) (*sheets.EmbeddedChart, error) {
	var chart sheets.EmbeddedChart
	if err := json.Unmarshal(b, &chart); err != nil {
		return nil, err
	}
	if chart.Spec != nil && !chartSpecIsZero(chart.Spec) {
		return &chart, nil
	}

	spec, err := parseChartSpecJSON(b)
	if err != nil {
		return nil, err
	}
	chart.Spec = spec
	return &chart, nil
}

func parseChartSpecJSON(b []byte) (*sheets.ChartSpec, error) {
	var spec sheets.ChartSpec
	if err := json.Unmarshal(b, &spec); err != nil {
		return nil, err
	}
	if !chartSpecIsZero(&spec) {
		return &spec, nil
	}

	var chart sheets.EmbeddedChart
	if err := json.Unmarshal(b, &chart); err != nil {
		return nil, err
	}
	if chart.Spec != nil && !chartSpecIsZero(chart.Spec) {
		return chart.Spec, nil
	}
	return nil, usage("--spec-json must contain a ChartSpec or an EmbeddedChart with spec")
}

func chartSpecIsZero(spec *sheets.ChartSpec) bool {
	if spec == nil {
		return true
	}
	return reflect.ValueOf(*spec).IsZero()
}

var gridRangeType = reflect.TypeOf(sheets.GridRange{})

func remapZeroSheetIDsInChartSpec(spec *sheets.ChartSpec, sheetID int64) {
	normalizeZeroSheetIDsInChartSpec(spec, sheetID, false)
}

func normalizeZeroSheetIDsInChartSpec(spec *sheets.ChartSpec, sheetID int64, preserveZero bool) {
	visitGridRanges(reflect.ValueOf(spec), func(gr *sheets.GridRange) {
		if gr == nil || gr.SheetId != 0 {
			return
		}
		if !preserveZero {
			gr.SheetId = sheetID
		}
		gr.ForceSendFields = appendForceSendField(gr.ForceSendFields, "SheetId")
	})
}

func chartSpecHasZeroSheetIDs(spec *sheets.ChartSpec) bool {
	var found bool
	visitGridRanges(reflect.ValueOf(spec), func(gr *sheets.GridRange) {
		if gr != nil && gr.SheetId == 0 {
			found = true
		}
	})
	return found
}

func visitGridRanges(v reflect.Value, visit func(*sheets.GridRange)) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			visitGridRanges(v.Elem(), visit)
		}
	case reflect.Ptr:
		if v.IsNil() {
			return
		}
		if v.Type().Elem() == gridRangeType {
			if v.CanInterface() {
				visit(v.Interface().(*sheets.GridRange))
			}
			return
		}
		visitGridRanges(v.Elem(), visit)
	case reflect.Struct:
		if v.Type() == gridRangeType {
			if v.CanAddr() && v.Addr().CanInterface() {
				visit(v.Addr().Interface().(*sheets.GridRange))
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if field.CanSet() || field.Kind() == reflect.Ptr || field.Kind() == reflect.Slice || field.Kind() == reflect.Interface {
				visitGridRanges(field, visit)
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			visitGridRanges(v.Index(i), visit)
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

type chartSheetResolution struct {
	SheetID        int64
	HasSheetIDZero bool
}

func firstSheetResolution(svc *sheets.Service, spreadsheetID string) (chartSheetResolution, error) {
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title))").
		Do()
	if err != nil {
		return chartSheetResolution{}, err
	}

	var res chartSheetResolution
	var found bool
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		if !found {
			res.SheetID = sheet.Properties.SheetId
			found = true
		}
		if sheet.Properties.SheetId == 0 {
			res.HasSheetIDZero = true
		}
	}
	if found {
		return res, nil
	}
	return chartSheetResolution{}, usage("spreadsheet has no sheets")
}

func findChartSheetResolution(svc *sheets.Service, spreadsheetID string, chartID int64) (chartSheetResolution, error) {
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title),charts(chartId))").
		Do()
	if err != nil {
		return chartSheetResolution{}, err
	}

	var res chartSheetResolution
	var found bool
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		if sheet.Properties.SheetId == 0 {
			res.HasSheetIDZero = true
		}
		for _, chart := range sheet.Charts {
			if chart != nil && chart.ChartId == chartID {
				res.SheetID = sheet.Properties.SheetId
				found = true
			}
		}
	}
	if found {
		return res, nil
	}
	return chartSheetResolution{}, usagef("chart %d not found", chartID)
}

func resolveChartSheetResolution(svc *sheets.Service, spreadsheetID, sheetName string) (chartSheetResolution, error) {
	if sheetName == "" {
		return firstSheetResolution(svc, spreadsheetID)
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title))").
		Do()
	if err != nil {
		return chartSheetResolution{}, err
	}

	var res chartSheetResolution
	var found bool
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		if sheet.Properties.SheetId == 0 {
			res.HasSheetIDZero = true
		}
		if sheet.Properties.Title == sheetName {
			res.SheetID = sheet.Properties.SheetId
			found = true
		}
	}
	if !found {
		return chartSheetResolution{}, usagef("unknown sheet %q", sheetName)
	}
	return res, nil
}

func buildChartPosition(sheetID int64, anchor string, width, height int64) (*sheets.EmbeddedObjectPosition, error) {
	var rowIndex, colIndex int64
	if anchor != "" {
		parsed, err := parseA1Cell(anchor)
		if err != nil {
			return nil, fmt.Errorf("invalid --anchor %q: %w", anchor, err)
		}
		rowIndex = int64(parsed.row - 1)
		colIndex = int64(parsed.col - 1)
	}

	return &sheets.EmbeddedObjectPosition{
		OverlayPosition: &sheets.OverlayPosition{
			AnchorCell: &sheets.GridCoordinate{
				SheetId:         sheetID,
				RowIndex:        rowIndex,
				ColumnIndex:     colIndex,
				ForceSendFields: []string{"SheetId", "RowIndex", "ColumnIndex"},
			},
			WidthPixels:  width,
			HeightPixels: height,
		},
	}, nil
}

type a1Cell struct {
	row int
	col int
}

func parseA1Cell(cell string) (a1Cell, error) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return a1Cell{}, fmt.Errorf("empty cell reference")
	}

	i := 0
	for i < len(cell) && ((cell[i] >= 'A' && cell[i] <= 'Z') || (cell[i] >= 'a' && cell[i] <= 'z')) {
		i++
	}
	if i == 0 || i == len(cell) {
		return a1Cell{}, fmt.Errorf("invalid cell reference %q", cell)
	}

	col, err := colLettersToIndex(strings.ToUpper(cell[:i]))
	if err != nil {
		return a1Cell{}, err
	}
	row, err := strconv.Atoi(cell[i:])
	if err != nil || row < 1 {
		return a1Cell{}, fmt.Errorf("invalid row in cell reference %q", cell)
	}
	return a1Cell{row: row, col: col}, nil
}
