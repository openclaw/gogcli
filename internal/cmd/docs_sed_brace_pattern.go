// Package cmd provides CLI commands for Google Docs operations.
package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// braceTableRef represents a parsed {T=...} table reference for pattern-side addressing.
// It captures table index, cell references, ranges, wildcards, and row/col operations.
type braceTableRef struct {
	TableIndex int  // 1-indexed, negative from end, 0 for * (all tables), math.MinInt32 reserved
	IsCreate   bool // {T=3x4} — table creation spec
	CreateRows int
	CreateCols int
	HasHeader  bool // {T=3x4:header}

	// Cell addressing
	Cell      *tableCellRef // reuse existing struct for single cell
	Row       int           // 1-indexed row for cell ref, 0 for wildcard
	Col       int           // 1-indexed col for cell ref, 0 for wildcard
	IsExcel   bool          // true if Excel-style (A1), false if row,col
	ExcelCell string        // original Excel ref (e.g., "A1")

	// Range addressing
	HasRange bool // A1:C3 or 1,1:3,3
	EndRow   int  // end row for range
	EndCol   int  // end col for range

	// Wildcards
	IsAllCells bool // {T=1!*} — all cells in table
	RowWild    bool // {T=1!1,*} — entire row
	ColWild    bool // {T=1!*,2} — entire column

	// Row/column operations
	RowOp string // "+2" (insert before 2), "$+" (append), "2" (delete row 2), "-1" (delete last)
	ColOp string // same semantics for columns
}

// braceImgRef represents a parsed {img=...} image reference for pattern-side addressing.
type braceImgRef struct {
	Index   int            // 1-indexed, negative from end, 0 for pattern match
	IsAll   bool           // {img=*}
	Pattern string         // regex pattern for alt text matching
	Regex   *regexp.Regexp // compiled pattern (nil if positional)
}

// parseBraceTableRef parses a {T=...} table reference spec.
// Supports: {T=1}, {T=-1}, {T=*}, {T=3x4}, {T=3x4:header},
// {T=1!A1}, {T=1!1,2}, {T=1!1,*}, {T=1!*,2}, {T=1!*},
// {T=1!A1:C3}, {T=1!row=+2}, {T=1!row=$+}, {T=1!col=+3}, etc.
func parseBraceTableRef(spec string) (*braceTableRef, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty table spec")
	}

	ref := &braceTableRef{}

	// Check for table creation: NxM or NxM:header
	if isTableCreateSpec(spec) {
		return parseTableCreateBrace(spec)
	}

	// Split by ! to separate table index from cell spec
	parts := strings.SplitN(spec, "!", 2)
	tableSpec := parts[0]
	var cellSpec string
	if len(parts) > 1 {
		cellSpec = parts[1]
	}

	// Parse table index
	if tableSpec == "*" {
		ref.TableIndex = 0 // 0 means all tables
	} else {
		idx, err := strconv.Atoi(tableSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid table index %q: %w", tableSpec, err)
		}
		if idx == 0 {
			return nil, fmt.Errorf("table index cannot be 0; use * for all")
		}
		ref.TableIndex = idx
	}

	// If no cell spec, we're done
	if cellSpec == "" {
		return ref, nil
	}

	// Parse cell spec
	return parseBraceCellSpec(ref, cellSpec)
}

// isTableCreateSpec checks if spec looks like a table creation (NxM or NxM:header).
func isTableCreateSpec(spec string) bool {
	// Must contain 'x' and no '!'
	if strings.Contains(spec, "!") {
		return false
	}
	lower := strings.ToLower(spec)
	if !strings.Contains(lower, "x") {
		return false
	}
	// Must start with digit
	if len(spec) == 0 || spec[0] < '0' || spec[0] > '9' {
		return false
	}
	return true
}

// parseTableCreateBrace parses {T=3x4} or {T=3x4:header}.
func parseTableCreateBrace(spec string) (*braceTableRef, error) {
	ref := &braceTableRef{IsCreate: true}

	// Check for :header suffix
	if idx := strings.Index(spec, ":"); idx >= 0 {
		suffix := strings.ToLower(strings.TrimSpace(spec[idx+1:]))
		if suffix != "header" {
			return nil, fmt.Errorf("invalid table create suffix %q (expected 'header')", suffix)
		}
		ref.HasHeader = true
		spec = spec[:idx]
	}

	// Parse RxC
	lower := strings.ToLower(spec)
	parts := strings.SplitN(lower, "x", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid table create spec %q", spec)
	}

	rows, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || rows < 1 || rows > 100 {
		return nil, fmt.Errorf("invalid row count in %q (must be 1-100)", spec)
	}
	cols, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || cols < 1 || cols > 26 {
		return nil, fmt.Errorf("invalid column count in %q (must be 1-26)", spec)
	}

	ref.CreateRows = rows
	ref.CreateCols = cols
	return ref, nil
}

// parseBraceCellSpec parses the cell specification after the ! separator.
func parseBraceCellSpec(ref *braceTableRef, spec string) (*braceTableRef, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ref, nil
	}

	// Check for wildcard: *
	if spec == "*" {
		ref.IsAllCells = true
		return ref, nil
	}

	// Check for row operation: row=+2, row=$+, row=2, row=-1
	if strings.HasPrefix(spec, "row=") {
		return parseRowOp(ref, spec[4:])
	}

	// Check for column operation: col=+3, col=$+, col=3, col=-1
	if strings.HasPrefix(spec, "col=") {
		return parseColOp(ref, spec[4:])
	}

	// Check for range: A1:C3 or 1,1:3,3
	if strings.Contains(spec, ":") {
		return parseCellRange(ref, spec)
	}

	// Check for row,col format with wildcards
	if strings.Contains(spec, ",") {
		return parseRowColRef(ref, spec)
	}

	// Try Excel-style reference (A1, B12, etc.)
	row, col, ok := parseExcelRef(spec)
	if ok {
		ref.Row = row
		ref.Col = col
		ref.IsExcel = true
		ref.ExcelCell = spec
		return ref, nil
	}

	return nil, fmt.Errorf("invalid cell spec %q", spec)
}

// parseRowOp parses row operations: +2 (insert before 2), $+ (append), 2 (delete), -1 (delete last).
func parseRowOp(ref *braceTableRef, val string) (*braceTableRef, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil, fmt.Errorf("empty row operation")
	}
	ref.RowOp = val
	return ref, nil
}

// parseColOp parses column operations: +3 (insert before 3), $+ (append), 3 (delete), -1 (delete last).
func parseColOp(ref *braceTableRef, val string) (*braceTableRef, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil, fmt.Errorf("empty column operation")
	}
	ref.ColOp = val
	return ref, nil
}

// parseCellRange parses A1:C3 or 1,1:3,3 range syntax.
func parseCellRange(ref *braceTableRef, spec string) (*braceTableRef, error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range %q", spec)
	}

	startSpec := strings.TrimSpace(parts[0])
	endSpec := strings.TrimSpace(parts[1])

	// Parse start cell
	startRow, startCol, err := parseCellCoord(startSpec)
	if err != nil {
		return nil, fmt.Errorf("invalid range start %q: %w", startSpec, err)
	}

	// Parse end cell
	endRow, endCol, err := parseCellCoord(endSpec)
	if err != nil {
		return nil, fmt.Errorf("invalid range end %q: %w", endSpec, err)
	}

	ref.Row = startRow
	ref.Col = startCol
	ref.EndRow = endRow
	ref.EndCol = endCol
	ref.HasRange = true

	return ref, nil
}

// parseCellCoord parses a single cell coordinate (A1 or 1,2 format).
func parseCellCoord(s string) (row, col int, err error) {
	s = strings.TrimSpace(s)

	// Try row,col format
	if strings.Contains(s, ",") {
		parts := strings.SplitN(s, ",", 2)
		row, err = strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid row: %w", err)
		}
		col, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid col: %w", err)
		}
		return row, col, nil
	}

	// Try Excel-style
	row, col, ok := parseExcelRef(s)
	if !ok {
		return 0, 0, fmt.Errorf("invalid cell reference %q", s)
	}
	return row, col, nil
}

// parseRowColRef parses R,C format with wildcard support.
func parseRowColRef(ref *braceTableRef, spec string) (*braceTableRef, error) {
	parts := strings.SplitN(spec, ",", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid row,col spec %q", spec)
	}

	rowStr := strings.TrimSpace(parts[0])
	colStr := strings.TrimSpace(parts[1])

	// Parse row
	if rowStr == "*" {
		ref.ColWild = true // *,N means entire column N
		ref.Row = 0
	} else {
		row, err := strconv.Atoi(rowStr)
		if err != nil {
			return nil, fmt.Errorf("invalid row %q: %w", rowStr, err)
		}
		ref.Row = row
	}

	// Parse col
	if colStr == "*" {
		ref.RowWild = true // N,* means entire row N
		ref.Col = 0
	} else {
		col, err := strconv.Atoi(colStr)
		if err != nil {
			return nil, fmt.Errorf("invalid col %q: %w", colStr, err)
		}
		ref.Col = col
	}

	return ref, nil
}

// parseBraceImgRef parses a {img=...} image reference spec.
// Supports: {img=1}, {img=-1}, {img=*}, {img=pattern}
func parseBraceImgRef(spec string) (*braceImgRef, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty image spec")
	}

	ref := &braceImgRef{}

	// Check for all images
	if spec == "*" {
		ref.IsAll = true
		return ref, nil
	}

	// Try to parse as integer (position)
	if idx, err := strconv.Atoi(spec); err == nil {
		ref.Index = idx
		return ref, nil
	}

	// Treat as regex pattern for alt text matching
	regex, err := regexp.Compile(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid image pattern %q: %w", spec, err)
	}
	ref.Pattern = spec
	ref.Regex = regex
	return ref, nil
}

// detectBracePattern detects if a pattern starts with {T=...} or {img=...}.
// Returns the remaining pattern (for cell-level find/replace) and the parsed refs.
// For example: "{T=1!A1}old" returns ("old", braceTableRef, nil, nil)
func detectBracePattern(pattern string) (remaining string, tableRef *braceTableRef, imgRef *braceImgRef, err error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", nil, nil, nil
	}

	// Must start with {
	if !strings.HasPrefix(pattern, "{") {
		return pattern, nil, nil, nil
	}

	// Find closing brace
	closeIdx := findMatchingBrace(pattern, 0)
	if closeIdx < 0 {
		return pattern, nil, nil, nil
	}

	braceContent := pattern[1:closeIdx]
	remaining = strings.TrimSpace(pattern[closeIdx+1:])

	// Check for T= (table reference)
	if strings.HasPrefix(braceContent, "T=") {
		tableSpec := strings.TrimPrefix(braceContent, "T=")
		tableRef, err = parseBraceTableRef(tableSpec)
		if err != nil {
			return "", nil, nil, fmt.Errorf("parse table ref: %w", err)
		}
		return remaining, tableRef, nil, nil
	}

	// Check for img= (image reference)
	if strings.HasPrefix(braceContent, "img=") {
		imgSpec := strings.TrimPrefix(braceContent, "img=")
		imgRef, err = parseBraceImgRef(imgSpec)
		if err != nil {
			return "", nil, nil, fmt.Errorf("parse image ref: %w", err)
		}
		return remaining, nil, imgRef, nil
	}

	// Not a pattern-side brace ref
	return pattern, nil, nil, nil
}

// braceTableToSedExpr bridges braceTableRef to the existing sedExpr fields
// so existing table machinery (runTableCellReplace, runTableRowColOp, etc.) can handle it.
func braceTableToSedExpr(bt *braceTableRef, expr *sedExpr) {
	if bt == nil {
		return
	}

	// Handle table-level reference (no cell spec)
	if !bt.IsAllCells && !bt.HasRange && bt.Row == 0 && bt.Col == 0 &&
		bt.RowOp == "" && bt.ColOp == "" && !bt.IsCreate {
		// Bare table reference: {T=1}, {T=-1}, {T=*}
		if bt.TableIndex == 0 {
			// math.MinInt32 signals "all tables" in the existing code
			expr.tableRef = -2147483648 // math.MinInt32
		} else {
			expr.tableRef = bt.TableIndex
		}
		return
	}

	// Handle table creation
	if bt.IsCreate {
		// Table creation is handled differently — set a marker
		// The actual creation happens via replacement parsing
		return
	}

	// Build tableCellRef for cell-level operations
	cellRef := &tableCellRef{
		tableIndex: bt.TableIndex,
	}

	// Handle row/col operations
	if bt.RowOp != "" {
		cellRef.rowOp, cellRef.opTarget = parseRowColOpValue(bt.RowOp)
	}
	if bt.ColOp != "" {
		cellRef.colOp, cellRef.opTarget = parseRowColOpValue(bt.ColOp)
	}

	// Handle cell addressing
	switch {
	case bt.IsAllCells:
		// All cells: row=0, col=0 (wildcard in existing code)
		cellRef.row = 0
		cellRef.col = 0
	case bt.RowWild:
		// Entire row: col=0
		cellRef.row = bt.Row
		cellRef.col = 0
	case bt.ColWild:
		// Entire column: row=0
		cellRef.row = 0
		cellRef.col = bt.Col
	case bt.HasRange:
		// Range: set start and end
		cellRef.row = bt.Row
		cellRef.col = bt.Col
		cellRef.endRow = bt.EndRow
		cellRef.endCol = bt.EndCol
	default:
		// Single cell
		cellRef.row = bt.Row
		cellRef.col = bt.Col
	}

	expr.cellRef = cellRef
}

// parseRowColOpValue converts brace op string to existing code's format.
// "+2" → ("insert", 2), "$+" → ("append", 0), "2" → ("delete", 2), "-1" → ("delete", -1)
func parseRowColOpValue(op string) (operation string, target int) {
	op = strings.TrimSpace(op)

	if op == "$+" {
		return opAppend, 0
	}

	if strings.HasPrefix(op, "+") {
		n, err := strconv.Atoi(op[1:])
		if err == nil {
			return opInsert, n
		}
		return "", 0
	}

	// Plain number (positive or negative) = delete
	n, err := strconv.Atoi(op)
	if err == nil {
		return opDelete, n
	}

	return "", 0
}

// braceImgToImageRefPattern bridges braceImgRef to the existing ImageRefPattern struct.
func braceImgToImageRefPattern(bi *braceImgRef) *ImageRefPattern {
	if bi == nil {
		return nil
	}

	ref := &ImageRefPattern{}

	if bi.IsAll {
		ref.ByPosition = true
		ref.AllImages = true
		return ref
	}

	if bi.Index != 0 {
		ref.ByPosition = true
		ref.Position = bi.Index
		return ref
	}

	if bi.Pattern != "" && bi.Regex != nil {
		ref.ByAlt = true
		ref.AltRegex = bi.Regex
		return ref
	}

	return nil
}

// braceTableToTableCreateSpec converts a braceTableRef creation spec to tableCreateSpec.
func braceTableToTableCreateSpec(bt *braceTableRef) *tableCreateSpec {
	if bt == nil || !bt.IsCreate {
		return nil
	}
	return &tableCreateSpec{
		rows:   bt.CreateRows,
		cols:   bt.CreateCols,
		header: bt.HasHeader,
	}
}

// String returns a debug representation of braceTableRef.
func (bt *braceTableRef) String() string {
	if bt == nil {
		return "<nil>"
	}
	var parts []string

	if bt.IsCreate {
		parts = append(parts, fmt.Sprintf("create:%dx%d", bt.CreateRows, bt.CreateCols))
		if bt.HasHeader {
			parts = append(parts, "header")
		}
		return "{T=" + strings.Join(parts, " ") + "}"
	}

	if bt.TableIndex == 0 {
		parts = append(parts, "table:*")
	} else {
		parts = append(parts, fmt.Sprintf("table:%d", bt.TableIndex))
	}

	switch {
	case bt.IsAllCells:
		parts = append(parts, "cells:*")
	case bt.HasRange:
		parts = append(parts, fmt.Sprintf("range:[%d,%d:%d,%d]", bt.Row, bt.Col, bt.EndRow, bt.EndCol))
	case bt.RowWild:
		parts = append(parts, fmt.Sprintf("row:%d,*", bt.Row))
	case bt.ColWild:
		parts = append(parts, fmt.Sprintf("col:*,%d", bt.Col))
	case bt.Row > 0 || bt.Col > 0:
		parts = append(parts, fmt.Sprintf("cell:[%d,%d]", bt.Row, bt.Col))
	}

	if bt.RowOp != "" {
		parts = append(parts, "rowOp:"+bt.RowOp)
	}
	if bt.ColOp != "" {
		parts = append(parts, "colOp:"+bt.ColOp)
	}

	return "{T=" + strings.Join(parts, " ") + "}"
}

// String returns a debug representation of braceImgRef.
func (bi *braceImgRef) String() string {
	if bi == nil {
		return "<nil>"
	}
	if bi.IsAll {
		return "{img=*}"
	}
	if bi.Index != 0 {
		return fmt.Sprintf("{img=%d}", bi.Index)
	}
	if bi.Pattern != "" {
		return fmt.Sprintf("{img=%s}", bi.Pattern)
	}
	return "{img=?}"
}
