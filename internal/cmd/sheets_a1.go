package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type a1Range struct {
	SheetName        string
	StartRow, EndRow int
	StartCol, EndCol int
}

var (
	a1CellRe = regexp.MustCompile(`^([A-Za-z]+)([0-9]+)$`)
	a1ColRe  = regexp.MustCompile(`^([A-Za-z]+)$`)
	a1RowRe  = regexp.MustCompile(`^([0-9]+)$`)
)

func parseA1Range(a1 string) (a1Range, error) {
	raw := strings.TrimSpace(a1)
	if raw == "" {
		return a1Range{}, fmt.Errorf("empty A1 range")
	}

	raw = cleanRange(raw)
	sheetName, rangePart, err := splitA1Sheet(raw)
	if err != nil {
		return a1Range{}, err
	}
	if strings.TrimSpace(rangePart) == "" {
		return a1Range{}, fmt.Errorf("missing range in %q", raw)
	}

	rangePart = strings.ReplaceAll(rangePart, "$", "")
	parts := strings.Split(rangePart, ":")
	if len(parts) > 2 {
		return a1Range{}, fmt.Errorf("invalid A1 range %q", raw)
	}

	startRef := strings.TrimSpace(parts[0])
	endRef := startRef
	if len(parts) == 2 {
		endRef = strings.TrimSpace(parts[1])
	}

	type refKind int
	const (
		refUnknown refKind = iota
		refCell
		refCol
		refRow
	)

	parseRef := func(ref string) (kind refKind, col int, row int, err error) {
		if strings.TrimSpace(ref) == "" {
			return refUnknown, 0, 0, fmt.Errorf("empty A1 ref")
		}
		if m := a1CellRe.FindStringSubmatch(ref); m != nil {
			c, err := colLettersToIndex(m[1])
			if err != nil {
				return refUnknown, 0, 0, err
			}
			r, err := strconv.Atoi(m[2])
			if err != nil || r <= 0 {
				return refUnknown, 0, 0, fmt.Errorf("invalid row in %q", ref)
			}
			return refCell, c, r, nil
		}
		if m := a1ColRe.FindStringSubmatch(ref); m != nil {
			c, err := colLettersToIndex(m[1])
			if err != nil {
				return refUnknown, 0, 0, err
			}
			return refCol, c, 0, nil
		}
		if m := a1RowRe.FindStringSubmatch(ref); m != nil {
			r, err := strconv.Atoi(m[1])
			if err != nil || r <= 0 {
				return refUnknown, 0, 0, fmt.Errorf("invalid row in %q", ref)
			}
			return refRow, 0, r, nil
		}
		return refUnknown, 0, 0, fmt.Errorf("invalid A1 ref %q", ref)
	}

	startKind, startCol, startRow, err := parseRef(startRef)
	if err != nil {
		return a1Range{}, err
	}
	endKind, endCol, endRow, err := parseRef(endRef)
	if err != nil {
		return a1Range{}, err
	}

	// Without a ":" separator, A1 notation must be a cell reference (e.g. A1),
	// not a row/column range shorthand.
	if len(parts) == 1 && startKind != refCell {
		return a1Range{}, fmt.Errorf("invalid A1 range %q", raw)
	}

	switch startKind {
	case refCell:
		switch endKind {
		case refCell:
			// ok
		case refCol:
			// A5:C (end row unbounded)
			endRow = 0
		default:
			return a1Range{}, fmt.Errorf("invalid A1 range %q", raw)
		}
	case refCol:
		switch endKind {
		case refCol:
			// A:C (rows unbounded)
			startRow, endRow = 0, 0
		default:
			return a1Range{}, fmt.Errorf("invalid A1 range %q", raw)
		}
	case refRow:
		switch endKind {
		case refRow:
			// 2:10 (cols unbounded)
			startCol, endCol = 0, 0
		default:
			return a1Range{}, fmt.Errorf("invalid A1 range %q", raw)
		}
	default:
		return a1Range{}, fmt.Errorf("invalid A1 range %q", raw)
	}

	if startRow > 0 && endRow > 0 && endRow < startRow {
		startRow, endRow = endRow, startRow
	}
	if startCol > 0 && endCol > 0 && endCol < startCol {
		startCol, endCol = endCol, startCol
	}

	return a1Range{
		SheetName: sheetName,
		StartRow:  startRow,
		EndRow:    endRow,
		StartCol:  startCol,
		EndCol:    endCol,
	}, nil
}

func splitA1Sheet(a1 string) (string, string, error) {
	idx := strings.LastIndex(a1, "!")
	if idx == -1 {
		return "", a1, nil
	}

	sheetPart := strings.TrimSpace(a1[:idx])
	rangePart := strings.TrimSpace(a1[idx+1:])
	if sheetPart == "" || rangePart == "" {
		return "", "", fmt.Errorf("invalid A1 range %q", a1)
	}

	sheetName, err := unquoteSheetName(sheetPart)
	if err != nil {
		return "", "", err
	}
	return sheetName, rangePart, nil
}

func unquoteSheetName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty sheet name")
	}
	if strings.HasPrefix(name, "'") {
		if !strings.HasSuffix(name, "'") || len(name) < 2 {
			return "", fmt.Errorf("invalid sheet name %q", name)
		}
		inner := name[1 : len(name)-1]
		return strings.ReplaceAll(inner, "''", "'"), nil
	}
	return name, nil
}

func parseA1Cell(ref string) (int, int, error) {
	matches := a1CellRe.FindStringSubmatch(ref)
	if matches == nil {
		return 0, 0, fmt.Errorf("invalid A1 cell %q", ref)
	}

	col, err := colLettersToIndex(matches[1])
	if err != nil {
		return 0, 0, err
	}
	row, err := strconv.Atoi(matches[2])
	if err != nil || row <= 0 {
		return 0, 0, fmt.Errorf("invalid row in %q", ref)
	}
	return col, row, nil
}

func colLettersToIndex(letters string) (int, error) {
	letters = strings.ToUpper(strings.TrimSpace(letters))
	if letters == "" {
		return 0, fmt.Errorf("empty column")
	}

	col := 0
	for i := 0; i < len(letters); i++ {
		ch := letters[i]
		if ch < 'A' || ch > 'Z' {
			return 0, fmt.Errorf("invalid column %q", letters)
		}
		col = col*26 + int(ch-'A'+1)
	}
	return col, nil
}
