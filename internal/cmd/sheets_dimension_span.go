package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	columnsRangeRe = regexp.MustCompile(`^([A-Za-z]+)(?::([A-Za-z]+))?$`)
	rowsRangeRe    = regexp.MustCompile(`^([0-9]+)(?::([0-9]+))?$`)
)

type dimensionSpan struct {
	SheetName  string
	StartIndex int64
	EndIndex   int64
}

func parseColumnsSpan(spec, label string) (dimensionSpan, error) {
	sheetName, part, err := splitA1Sheet(strings.TrimSpace(spec))
	if err != nil {
		return dimensionSpan{}, fmt.Errorf("parse %s range: %w", label, err)
	}
	part = strings.ReplaceAll(strings.TrimSpace(part), "$", "")
	m := columnsRangeRe.FindStringSubmatch(part)
	if m == nil {
		return dimensionSpan{}, fmt.Errorf("invalid %s range %q (expected A:C or Sheet!A:C)", label, spec)
	}

	startCol, err := colLettersToIndex(m[1])
	if err != nil {
		return dimensionSpan{}, err
	}
	endCol := startCol
	if m[2] != "" {
		endCol, err = colLettersToIndex(m[2])
		if err != nil {
			return dimensionSpan{}, err
		}
	}
	if endCol < startCol {
		startCol, endCol = endCol, startCol
	}

	return dimensionSpan{
		SheetName:  sheetName,
		StartIndex: int64(startCol - 1),
		EndIndex:   int64(endCol),
	}, nil
}

func parseRowsSpan(spec, label string) (dimensionSpan, error) {
	sheetName, part, err := splitA1Sheet(strings.TrimSpace(spec))
	if err != nil {
		return dimensionSpan{}, fmt.Errorf("parse %s range: %w", label, err)
	}
	part = strings.ReplaceAll(strings.TrimSpace(part), "$", "")
	m := rowsRangeRe.FindStringSubmatch(part)
	if m == nil {
		return dimensionSpan{}, fmt.Errorf("invalid %s range %q (expected 1:10 or Sheet!1:10)", label, spec)
	}

	startRow, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || startRow <= 0 {
		return dimensionSpan{}, fmt.Errorf("invalid %s start row %q", label, m[1])
	}
	endRow := startRow
	if m[2] != "" {
		endRow, err = strconv.ParseInt(m[2], 10, 64)
		if err != nil || endRow <= 0 {
			return dimensionSpan{}, fmt.Errorf("invalid %s end row %q", label, m[2])
		}
	}
	if endRow < startRow {
		startRow, endRow = endRow, startRow
	}

	return dimensionSpan{
		SheetName:  sheetName,
		StartIndex: startRow - 1,
		EndIndex:   endRow,
	}, nil
}
