//nolint:err113,wsl_v5 // Parser errors include the exact invalid syntax for CLI diagnostics.
package docssed

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const axisOperationAppend = "append"

// CellReference identifies one table cell, range, wildcard, or row/column operation.
type CellReference struct {
	TableIndex      int
	Row             int
	Column          int
	Subpattern      string
	RowOperation    string
	ColumnOperation string
	OperationTarget int
	EndRow          int
	EndColumn       int
}

// TableReference is the semantic form of a pattern-side {T=...} reference.
type TableReference struct {
	TableIndex int

	IsCreate   bool
	CreateRows int
	CreateCols int
	HasHeader  bool

	Row int
	Col int

	HasRange bool
	EndRow   int
	EndCol   int

	IsAllCells bool
	RowWild    bool
	ColWild    bool

	RowOp string
	ColOp string
}

// ImageReference identifies existing document images by position or alt-text regex.
type ImageReference struct {
	ByPosition bool
	Position   int
	AllImages  bool
	ByAlt      bool
	Pattern    string
	AltRegex   *regexp.Regexp
}

// TableCreateSpec describes explicit or pipe-table creation syntax.
type TableCreateSpec struct {
	Rows    int
	Columns int
	Header  bool
	Cells   [][]string
}

// ParseTableReference parses a bare table reference such as |1|, |-1|, or |*|.
func ParseTableReference(value string) *TableReference {
	value = strings.TrimSpace(value)
	if len(value) < 3 || value[0] != '|' || value[len(value)-1] != '|' {
		return nil
	}
	inner := value[1 : len(value)-1]
	if strings.ContainsAny(inner, "xX") {
		return nil
	}
	if inner == "*" {
		return &TableReference{}
	}
	index, err := strconv.Atoi(inner)
	if err != nil || index == 0 {
		return nil
	}
	return &TableReference{TableIndex: index}
}

// ParseTableCellReference parses references such as |1|[2,3], |1|[A1], and row/column operations.
func ParseTableCellReference(value string) *CellReference {
	if value == "" || value[0] != '|' {
		return nil
	}
	separator := strings.Index(value[1:], "|")
	if separator < 0 {
		return nil
	}
	tableValue := value[1 : separator+1]
	rest := value[separator+2:]

	tableIndex, err := strconv.Atoi(tableValue)
	if err != nil || rest == "" || rest[0] != '[' {
		return nil
	}
	bracketEnd := strings.Index(rest, "]")
	if bracketEnd < 0 {
		return nil
	}
	cellValue := rest[1:bracketEnd]
	after := rest[bracketEnd+1:]

	if strings.HasPrefix(cellValue, "row:") || strings.HasPrefix(cellValue, "col:") {
		return parseBracketAxisOperation(tableIndex, cellValue)
	}

	var row, column, endRow, endColumn int
	switch {
	case strings.Index(cellValue, ":") > 0:
		start, end, ok := strings.Cut(cellValue, ":")
		if !ok {
			return nil
		}
		row, column, ok = parseNumericCell(start)
		if !ok {
			return nil
		}
		endRow, endColumn, ok = parseNumericCell(end)
		if !ok {
			return nil
		}
	case strings.Contains(cellValue, ","):
		var ok bool
		row, column, ok = parseBracketRowColumn(cellValue)
		if !ok {
			return nil
		}
		if row == appendAxisSentinel || column == appendAxisSentinel {
			ref := &CellReference{TableIndex: tableIndex}
			if row == appendAxisSentinel {
				ref.RowOperation = axisOperationAppend
				ref.OperationTarget = appendAxisTarget(cellValue, true)
				ref.Column = column
			} else {
				ref.ColumnOperation = axisOperationAppend
				ref.OperationTarget = appendAxisTarget(cellValue, false)
				ref.Row = row
			}
			return ref
		}
	default:
		var ok bool
		row, column, ok = ParseExcelReference(cellValue)
		if !ok {
			return nil
		}
	}

	ref := &CellReference{
		TableIndex: tableIndex,
		Row:        row,
		Column:     column,
		EndRow:     endRow,
		EndColumn:  endColumn,
	}
	if strings.HasPrefix(after, ":") {
		ref.Subpattern = after[1:]
	}
	return ref
}

const appendAxisSentinel = math.MinInt

func parseBracketAxisOperation(tableIndex int, value string) *CellReference {
	isRow := strings.HasPrefix(value, "row:")
	operationValue := value[4:]
	ref := &CellReference{TableIndex: tableIndex}

	switch {
	case strings.HasPrefix(operationValue, "+"):
		target, err := strconv.Atoi(operationValue[1:])
		if err != nil {
			return nil
		}
		if isRow {
			ref.RowOperation = "insert"
		} else {
			ref.ColumnOperation = "insert"
		}
		ref.OperationTarget = target
	case operationValue == "$+":
		if isRow {
			ref.RowOperation = axisOperationAppend
		} else {
			ref.ColumnOperation = axisOperationAppend
		}
	default:
		target, err := strconv.Atoi(operationValue)
		if err != nil {
			return nil
		}
		if isRow {
			ref.RowOperation = "delete"
		} else {
			ref.ColumnOperation = "delete"
		}
		ref.OperationTarget = target
	}
	return ref
}

func parseBracketRowColumn(value string) (int, int, bool) {
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	row, ok := parseBracketAxis(strings.TrimSpace(parts[0]))
	if !ok {
		return 0, 0, false
	}
	column, ok := parseBracketAxis(strings.TrimSpace(parts[1]))
	if !ok {
		return 0, 0, false
	}
	if row == appendAxisSentinel && column == appendAxisSentinel {
		return 0, 0, false
	}
	return row, column, true
}

func parseBracketAxis(value string) (int, bool) {
	switch {
	case value == "*":
		return 0, true
	case strings.HasPrefix(value, "+"):
		if _, err := strconv.Atoi(value[1:]); err != nil {
			return 0, false
		}
		return appendAxisSentinel, true
	default:
		parsed, err := strconv.Atoi(value)
		return parsed, err == nil
	}
}

func appendAxisTarget(value string, row bool) int {
	parts := strings.SplitN(value, ",", 2)
	index := 1
	if row {
		index = 0
	}
	target, _ := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(parts[index]), "+"))
	return target
}

func parseNumericCell(value string) (int, int, bool) {
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	row, rowErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	column, columnErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	return row, column, rowErr == nil && columnErr == nil
}

// ParseExcelReference parses an Excel-style cell reference such as A1 or AA10.
func ParseExcelReference(value string) (row, column int, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false
	}
	index := 0
	for index < len(value) &&
		((value[index] >= 'A' && value[index] <= 'Z') || (value[index] >= 'a' && value[index] <= 'z')) {
		index++
	}
	if index == 0 || index == len(value) {
		return 0, 0, false
	}
	row, err := strconv.Atoi(value[index:])
	if err != nil || row < 1 {
		return 0, 0, false
	}
	for _, letter := range strings.ToUpper(value[:index]) {
		column = column*26 + int(letter-'A') + 1
	}
	return row, column, true
}

// ParseBraceTableReference parses a {T=...} table reference body.
func ParseBraceTableReference(spec string) (*TableReference, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty table spec")
	}
	if isTableCreateSpec(spec) {
		return parseBraceTableCreate(spec)
	}

	tableSpec, cellSpec, hasCell := strings.Cut(spec, "!")
	ref := &TableReference{}
	if tableSpec == "*" {
		ref.TableIndex = 0
	} else {
		index, err := strconv.Atoi(tableSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid table index %q: %w", tableSpec, err)
		}
		if index == 0 {
			return nil, fmt.Errorf("table index cannot be 0; use * for all")
		}
		ref.TableIndex = index
	}
	if !hasCell || cellSpec == "" {
		return ref, nil
	}
	return parseBraceCellReference(ref, cellSpec)
}

func isTableCreateSpec(spec string) bool {
	return !strings.Contains(spec, "!") &&
		strings.Contains(strings.ToLower(spec), "x") &&
		spec[0] >= '0' && spec[0] <= '9'
}

func parseBraceTableCreate(spec string) (*TableReference, error) {
	ref := &TableReference{IsCreate: true}
	if index := strings.Index(spec, ":"); index >= 0 {
		suffix := strings.ToLower(strings.TrimSpace(spec[index+1:]))
		if suffix != "header" {
			return nil, fmt.Errorf("invalid table create suffix %q (expected 'header')", suffix)
		}
		ref.HasHeader = true
		spec = spec[:index]
	}
	parts := strings.SplitN(strings.ToLower(spec), "x", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid table create spec %q", spec)
	}
	rows, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || rows < 1 || rows > 100 {
		return nil, fmt.Errorf("invalid row count in %q (must be 1-100)", spec)
	}
	columns, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || columns < 1 || columns > 26 {
		return nil, fmt.Errorf("invalid column count in %q (must be 1-26)", spec)
	}
	ref.CreateRows = rows
	ref.CreateCols = columns
	return ref, nil
}

func parseBraceCellReference(ref *TableReference, spec string) (*TableReference, error) {
	spec = strings.TrimSpace(spec)
	switch {
	case spec == "":
		return ref, nil
	case spec == "*":
		ref.IsAllCells = true
		return ref, nil
	case strings.HasPrefix(spec, "row="):
		ref.RowOp = strings.TrimSpace(spec[4:])
		if ref.RowOp == "" {
			return nil, fmt.Errorf("empty row operation")
		}
		return ref, nil
	case strings.HasPrefix(spec, "col="):
		ref.ColOp = strings.TrimSpace(spec[4:])
		if ref.ColOp == "" {
			return nil, fmt.Errorf("empty column operation")
		}
		return ref, nil
	case strings.Contains(spec, ":"):
		start, end, _ := strings.Cut(spec, ":")
		row, column, err := parseCellCoordinate(start)
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q: %w", strings.TrimSpace(start), err)
		}
		endRow, endColumn, err := parseCellCoordinate(end)
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q: %w", strings.TrimSpace(end), err)
		}
		ref.Row, ref.Col = row, column
		ref.EndRow, ref.EndCol = endRow, endColumn
		ref.HasRange = true
		return ref, nil
	case strings.Contains(spec, ","):
		return parseBraceRowColumn(ref, spec)
	default:
		row, column, ok := ParseExcelReference(spec)
		if !ok {
			return nil, fmt.Errorf("invalid cell spec %q", spec)
		}
		ref.Row, ref.Col = row, column
		return ref, nil
	}
}

func parseCellCoordinate(value string) (int, int, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, ",") {
		parts := strings.SplitN(value, ",", 2)
		row, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid row: %w", err)
		}
		column, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid col: %w", err)
		}
		return row, column, nil
	}
	row, column, ok := ParseExcelReference(value)
	if !ok {
		return 0, 0, fmt.Errorf("invalid cell reference %q", value)
	}
	return row, column, nil
}

func parseBraceRowColumn(ref *TableReference, spec string) (*TableReference, error) {
	parts := strings.SplitN(spec, ",", 2)
	rowValue := strings.TrimSpace(parts[0])
	columnValue := strings.TrimSpace(parts[1])

	if rowValue == "*" {
		ref.ColWild = true
	} else {
		row, err := strconv.Atoi(rowValue)
		if err != nil {
			return nil, fmt.Errorf("invalid row %q: %w", rowValue, err)
		}
		ref.Row = row
	}
	if columnValue == "*" {
		ref.RowWild = true
	} else {
		column, err := strconv.Atoi(columnValue)
		if err != nil {
			return nil, fmt.Errorf("invalid col %q: %w", columnValue, err)
		}
		ref.Col = column
	}
	return ref, nil
}

// ParseBraceImageReference parses a {img=...} image reference body.
func ParseBraceImageReference(spec string) (*ImageReference, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty image spec")
	}
	if spec == "*" {
		return &ImageReference{ByPosition: true, AllImages: true}, nil
	}
	if position, err := strconv.Atoi(spec); err == nil {
		return &ImageReference{ByPosition: true, Position: position}, nil
	}
	expression, err := regexp.Compile(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid image pattern %q: %w", spec, err)
	}
	return &ImageReference{ByAlt: true, Pattern: spec, AltRegex: expression}, nil
}

// DetectBraceReference parses a leading {T=...} or {img=...} reference.
func DetectBraceReference(pattern string) (string, *TableReference, *ImageReference, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern[0] != '{' {
		return pattern, nil, nil, nil
	}
	closeIndex := findClosingBrace(pattern, 0)
	if closeIndex < 0 {
		return pattern, nil, nil, nil
	}
	content := pattern[1:closeIndex]
	remaining := strings.TrimSpace(pattern[closeIndex+1:])
	switch {
	case strings.HasPrefix(content, "T="):
		ref, err := ParseBraceTableReference(strings.TrimPrefix(content, "T="))
		if err != nil {
			return "", nil, nil, fmt.Errorf("parse table ref: %w", err)
		}
		return remaining, ref, nil, nil
	case strings.HasPrefix(content, "img="):
		ref, err := ParseBraceImageReference(strings.TrimPrefix(content, "img="))
		if err != nil {
			return "", nil, nil, fmt.Errorf("parse image ref: %w", err)
		}
		return remaining, nil, ref, nil
	default:
		return pattern, nil, nil, nil
	}
}

func findClosingBrace(value string, position int) int {
	if position >= len(value) || value[position] != '{' {
		return -1
	}
	depth := 1
	for index := position + 1; index < len(value); index++ {
		if value[index] == '\\' {
			index++
			continue
		}
		switch value[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

// ParseImageReference parses existing-image references such as !(1), !(*), and ![alt-regex].
func ParseImageReference(pattern string) *ImageReference {
	if strings.HasPrefix(pattern, "!(") && strings.HasSuffix(pattern, ")") {
		inner := pattern[2 : len(pattern)-1]
		if inner == "*" {
			return &ImageReference{ByPosition: true, AllImages: true}
		}
		if position, err := strconv.Atoi(inner); err == nil {
			return &ImageReference{ByPosition: true, Position: position}
		}
		return nil
	}
	if strings.HasPrefix(pattern, "![](") && strings.HasSuffix(pattern, ")") {
		inner := pattern[4 : len(pattern)-1]
		if inner == "*" {
			return &ImageReference{ByPosition: true, AllImages: true}
		}
		if position, err := strconv.Atoi(inner); err == nil {
			return &ImageReference{ByPosition: true, Position: position}
		}
		return nil
	}
	if strings.HasPrefix(pattern, "![") && strings.HasSuffix(pattern, "]") && !strings.Contains(pattern, "](") {
		regexValue := pattern[2 : len(pattern)-1]
		if regexValue == "" {
			return nil
		}
		expression, err := regexp.Compile(regexValue)
		if err != nil {
			return nil
		}
		return &ImageReference{ByAlt: true, Pattern: regexValue, AltRegex: expression}
	}
	return nil
}

// ParseTableCreate parses explicit |RxC| and |RxC:header| table creation syntax.
func ParseTableCreate(value string) *TableCreateSpec {
	value = strings.TrimSpace(value)
	if len(value) < 4 || value[0] != '|' || value[len(value)-1] != '|' {
		return nil
	}
	inner := value[1 : len(value)-1]
	header := false
	if index := strings.Index(inner, ":"); index >= 0 {
		if strings.ToLower(strings.TrimSpace(inner[index+1:])) != "header" {
			return nil
		}
		header = true
		inner = inner[:index]
	}
	parts := strings.SplitN(strings.ToLower(inner), "x", 2)
	if len(parts) != 2 {
		return nil
	}
	rows, rowErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	columns, columnErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if rowErr != nil || columnErr != nil || rows < 1 || columns < 1 || rows > 100 || columns > 26 {
		return nil
	}
	return &TableCreateSpec{Rows: rows, Columns: columns, Header: header}
}

// ParsePipeTable parses markdown pipe-table creation syntax.
func ParsePipeTable(value string) *TableCreateSpec {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\n", "\n"))
	if !strings.HasPrefix(value, "|") {
		return nil
	}
	var rows [][]string
	columnCount := 0
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			return nil
		}
		parts := strings.Split(line, "|")
		var cells []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if strings.Trim(part, "-: ") == "" {
				cells = nil
				break
			}
			cells = append(cells, part)
		}
		if cells == nil {
			continue
		}
		if len(cells) == 0 {
			return nil
		}
		if columnCount == 0 {
			columnCount = len(cells)
		} else if len(cells) != columnCount {
			for len(cells) < columnCount {
				cells = append(cells, "")
			}
			cells = cells[:columnCount]
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 || columnCount == 0 {
		return nil
	}
	return &TableCreateSpec{Rows: len(rows), Columns: columnCount, Cells: rows}
}

// String returns a compact diagnostic representation of the table reference.
func (ref *TableReference) String() string {
	if ref == nil {
		return "<nil>"
	}
	var parts []string
	if ref.IsCreate {
		parts = append(parts, fmt.Sprintf("create:%dx%d", ref.CreateRows, ref.CreateCols))
		if ref.HasHeader {
			parts = append(parts, "header")
		}
		return "{T=" + strings.Join(parts, " ") + "}"
	}
	if ref.TableIndex == 0 {
		parts = append(parts, "table:*")
	} else {
		parts = append(parts, fmt.Sprintf("table:%d", ref.TableIndex))
	}
	switch {
	case ref.IsAllCells:
		parts = append(parts, "cells:*")
	case ref.HasRange:
		parts = append(parts, fmt.Sprintf("range:[%d,%d:%d,%d]", ref.Row, ref.Col, ref.EndRow, ref.EndCol))
	case ref.RowWild:
		parts = append(parts, fmt.Sprintf("row:%d,*", ref.Row))
	case ref.ColWild:
		parts = append(parts, fmt.Sprintf("col:*,%d", ref.Col))
	case ref.Row > 0 || ref.Col > 0:
		parts = append(parts, fmt.Sprintf("cell:[%d,%d]", ref.Row, ref.Col))
	}
	if ref.RowOp != "" {
		parts = append(parts, "rowOp:"+ref.RowOp)
	}
	if ref.ColOp != "" {
		parts = append(parts, "colOp:"+ref.ColOp)
	}
	return "{T=" + strings.Join(parts, " ") + "}"
}

// String returns a compact brace-syntax representation of the image reference.
func (ref *ImageReference) String() string {
	if ref == nil {
		return "<nil>"
	}
	switch {
	case ref.AllImages:
		return "{img=*}"
	case ref.ByPosition:
		return fmt.Sprintf("{img=%d}", ref.Position)
	case ref.Pattern != "":
		return fmt.Sprintf("{img=%s}", ref.Pattern)
	default:
		return "{img=?}"
	}
}
