package sheetsdimension

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/openclaw/gogcli/internal/sheetsa1"
)

var (
	columnsRangeRe = regexp.MustCompile(`^([A-Za-z]+)(?::([A-Za-z]+))?$`)
	rowsRangeRe    = regexp.MustCompile(`^([0-9]+)(?::([0-9]+))?$`)
)

type Span struct {
	SheetName  string
	StartIndex int64
	EndIndex   int64
}

func ParseColumns(spec, label string) (Span, error) {
	sheetName, part, err := sheetsa1.Split(strings.TrimSpace(spec))
	if err != nil {
		return Span{}, invalidf("parse %s range: %v", label, err)
	}

	part = strings.ReplaceAll(strings.TrimSpace(part), "$", "")

	match := columnsRangeRe.FindStringSubmatch(part)
	if match == nil {
		return Span{}, invalidf("invalid %s range %q (expected A:C or Sheet!A:C)", label, spec)
	}

	startCol, err := sheetsa1.ColumnIndex(match[1])
	if err != nil {
		return Span{}, invalidf("%v", err)
	}

	endCol := startCol
	if match[2] != "" {
		endCol, err = sheetsa1.ColumnIndex(match[2])
		if err != nil {
			return Span{}, invalidf("%v", err)
		}
	}

	if endCol < startCol {
		startCol, endCol = endCol, startCol
	}

	return Span{
		SheetName:  sheetName,
		StartIndex: int64(startCol - 1),
		EndIndex:   int64(endCol),
	}, nil
}

func ParseRows(spec, label string) (Span, error) {
	sheetName, part, err := sheetsa1.Split(strings.TrimSpace(spec))
	if err != nil {
		return Span{}, invalidf("parse %s range: %v", label, err)
	}

	part = strings.ReplaceAll(strings.TrimSpace(part), "$", "")

	match := rowsRangeRe.FindStringSubmatch(part)
	if match == nil {
		return Span{}, invalidf("invalid %s range %q (expected 1:10 or Sheet!1:10)", label, spec)
	}

	startRow, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || startRow <= 0 {
		return Span{}, invalidf("invalid %s start row %q", label, match[1])
	}

	endRow := startRow
	if match[2] != "" {
		endRow, err = strconv.ParseInt(match[2], 10, 64)
		if err != nil || endRow <= 0 {
			return Span{}, invalidf("invalid %s end row %q", label, match[2])
		}
	}

	if endRow < startRow {
		startRow, endRow = endRow, startRow
	}

	return Span{
		SheetName:  sheetName,
		StartIndex: startRow - 1,
		EndIndex:   endRow,
	}, nil
}

func ParseDeleteSpec(target, dimension string, start, end int64) (DeleteSpec, error) {
	target = strings.ReplaceAll(strings.TrimSpace(target), `\!`, "!")
	if target == "" {
		return DeleteSpec{}, invalidf("empty rangeOrSheet")
	}

	spec := DeleteSpec{}

	switch strings.ToUpper(strings.TrimSpace(dimension)) {
	case "ROW", "ROWS":
		spec.Dimension = Rows
		spec.Label = "rows"
	case "COL", "COLS", "COLUMN", "COLUMNS":
		spec.Dimension = Columns
		spec.Label = "columns"
	default:
		return DeleteSpec{}, invalidf("dimension must be ROWS or COLUMNS, got %q", dimension)
	}

	if start == 0 && end == 0 {
		if !strings.Contains(target, "!") {
			return DeleteSpec{}, invalidf("sheet targets require both --start and --end; range targets must include a sheet name")
		}

		var (
			span Span
			err  error
		)

		if spec.Dimension == Rows {
			span, err = ParseRows(target, "delete-dimension")
		} else {
			span, err = ParseColumns(target, "delete-dimension")
		}

		if err != nil {
			return DeleteSpec{}, invalidf(
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
		return DeleteSpec{}, invalidf("provide both --start and --end")
	}

	if start < 1 {
		return DeleteSpec{}, invalidf("start must be >= 1")
	}

	if end < start {
		return DeleteSpec{}, invalidf("end must be >= start")
	}

	spec.SheetName = target
	spec.StartIndex = start - 1
	spec.EndIndex = end

	return spec, nil
}
