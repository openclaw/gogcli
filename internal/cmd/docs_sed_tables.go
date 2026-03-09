package cmd

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

type tableCellRef struct {
	tableIndex int    // 1-indexed, negative means from end (-1 = last)
	row        int    // 1-indexed, 0 means wildcard (*)
	col        int    // 1-indexed, 0 means wildcard (*)
	subPattern string // optional pattern to match within the cell

	// Row/column operations
	rowOp    string // "delete", "insert" — set when [row:N] or [row:+N] syntax used
	colOp    string // "delete", "insert" — set when [col:N] or [col:+N] syntax used
	opTarget int    // target row/col index (1-indexed, negative from end)

	// Merge range: [r1,c1:r2,c2] → merge cells from (row,col) to (endRow,endCol)
	endRow int // 0 means no merge range
	endCol int
}

// parseTableCellRef parses a table cell reference like |1|[2,3] or |1|[A1] or |-1|[1,1]
// Returns nil if the string is not a table cell reference.
// Optionally parses a trailing :pattern for within-cell matching.
func parseTableCellRef(s string) *tableCellRef {
	// Must start with |
	if len(s) == 0 || s[0] != '|' {
		return nil
	}
	// Find second |
	idx := strings.Index(s[1:], "|")
	if idx < 0 {
		return nil
	}
	tableStr := s[1 : 1+idx]
	rest := s[1+idx+1:] // after second |

	tableIdx, err := strconv.Atoi(tableStr)
	if err != nil {
		return nil
	}

	// Must have [...]
	if len(rest) == 0 || rest[0] != '[' {
		return nil
	}
	bracketEnd := strings.Index(rest, "]")
	if bracketEnd < 0 {
		return nil
	}
	cellStr := rest[1:bracketEnd]
	after := rest[bracketEnd+1:]

	// Check for row/col operations: [row:N], [row:+N], [col:N], [col:+N]
	if strings.HasPrefix(cellStr, "row:") || strings.HasPrefix(cellStr, "col:") {
		isRow := strings.HasPrefix(cellStr, "row:")
		valStr := cellStr[4:]
		ref := &tableCellRef{tableIndex: tableIdx}

		if strings.HasPrefix(valStr, "+") {
			// Insert operation
			n, err := strconv.Atoi(valStr[1:])
			if err != nil {
				return nil
			}
			if isRow {
				ref.rowOp = opInsert
				ref.opTarget = n
			} else {
				ref.colOp = opInsert
				ref.opTarget = n
			}
		} else {
			// Delete operation (or special like $+ for append)
			if valStr == "$+" {
				// Append at end
				if isRow {
					ref.rowOp = opAppend
				} else {
					ref.colOp = opAppend
				}
			} else {
				n, err := strconv.Atoi(valStr)
				if err != nil {
					return nil
				}
				if isRow {
					ref.rowOp = opDelete
					ref.opTarget = n
				} else {
					ref.colOp = opDelete
					ref.opTarget = n
				}
			}
		}
		return ref
	}

	var row, col int
	var endRow, endCol int

	// Check for merge range syntax: R1,C1:R2,C2
	if colonIdx := strings.Index(cellStr, ":"); colonIdx > 0 {
		startPart := cellStr[:colonIdx]
		endPart := cellStr[colonIdx+1:]

		// Parse start cell
		startParts := strings.SplitN(startPart, ",", 2)
		if len(startParts) != 2 {
			return nil
		}
		r, err := strconv.Atoi(strings.TrimSpace(startParts[0]))
		if err != nil {
			return nil
		}
		c, err2 := strconv.Atoi(strings.TrimSpace(startParts[1]))
		if err2 != nil {
			return nil
		}
		row, col = r, c

		// Parse end cell
		endParts := strings.SplitN(endPart, ",", 2)
		if len(endParts) != 2 {
			return nil
		}
		er, err3 := strconv.Atoi(strings.TrimSpace(endParts[0]))
		if err3 != nil {
			return nil
		}
		ec, err4 := strconv.Atoi(strings.TrimSpace(endParts[1]))
		if err4 != nil {
			return nil
		}
		endRow, endCol = er, ec
	} else if parts := strings.SplitN(cellStr, ",", 2); len(parts) == 2 {
		// Try R,C format (with wildcard support)
		rStr := strings.TrimSpace(parts[0])
		cStr := strings.TrimSpace(parts[1])

		switch {
		case rStr == "*":
			row = 0 // wildcard
		case strings.HasPrefix(rStr, "+"):
			// +N means append row
			ref := &tableCellRef{tableIndex: tableIdx, rowOp: "append"}
			n, err := strconv.Atoi(rStr[1:])
			if err == nil {
				ref.opTarget = n
			}
			// Parse col for the append target
			if cStr == "*" {
				ref.col = 0
			} else {
				c, err := strconv.Atoi(cStr)
				if err != nil {
					return nil
				}
				ref.col = c
			}
			return ref
		default:
			r, err := strconv.Atoi(rStr)
			if err != nil {
				return nil
			}
			row = r
		}

		switch {
		case cStr == "*":
			col = 0 // wildcard
		case strings.HasPrefix(cStr, "+"):
			// +N means append column
			ref := &tableCellRef{tableIndex: tableIdx, colOp: "append"}
			n, err := strconv.Atoi(cStr[1:])
			if err == nil {
				ref.opTarget = n
			}
			ref.row = row
			return ref
		default:
			c, err := strconv.Atoi(cStr)
			if err != nil {
				return nil
			}
			col = c
		}
	} else {
		// Try Excel-style A1
		r, c, ok := parseExcelRef(cellStr)
		if !ok {
			return nil
		}
		row, col = r, c
	}

	ref := &tableCellRef{tableIndex: tableIdx, row: row, col: col, endRow: endRow, endCol: endCol}

	// Optional :pattern
	if strings.HasPrefix(after, ":") {
		ref.subPattern = after[1:]
	}

	return ref
}

// parseExcelRef parses an Excel-style cell reference like "A1", "B2", "AA10"
// Returns (row, col, ok) where row and col are 1-indexed.
func parseExcelRef(s string) (row, col int, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, 0, false
	}
	// Split into letter part and number part
	i := 0
	for i < len(s) && ((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
		i++
	}
	if i == 0 || i == len(s) {
		return 0, 0, false
	}
	letters := strings.ToUpper(s[:i])
	numStr := s[i:]
	r, err := strconv.Atoi(numStr)
	if err != nil || r < 1 {
		return 0, 0, false
	}
	// Convert letters to column number
	c := 0
	for _, ch := range letters {
		c = c*26 + int(ch-'A') + 1
	}
	return r, c, true
}

func (c *DocsSedCmd) runTableOp(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return fmt.Errorf("create docs service: %w", err)
	}

	var doc *docs.Document
	err = retryOnQuota(ctx, func() error {
		var e error
		doc, e = docsSvc.Documents.Get(id).Context(ctx).Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("get document: %w", err)
	}

	// Collect all tables with their structural element indices
	type tableInfo struct {
		table    *docs.Table
		startIdx int64
		endIdx   int64
	}
	var tables []tableInfo

	if doc.Body != nil {
		for _, elem := range doc.Body.Content {
			if elem.Table != nil {
				tables = append(tables, tableInfo{
					table:    elem.Table,
					startIdx: elem.StartIndex,
					endIdx:   elem.EndIndex,
				})
			}
		}
	}

	if len(tables) == 0 {
		return fmt.Errorf("document has no tables")
	}

	// Resolve which tables to target
	var targets []tableInfo
	tIdx := expr.tableRef
	if tIdx == math.MinInt32 {
		// |*| — all tables
		targets = tables
	} else {
		resolved := tIdx
		if resolved < 0 {
			resolved = len(tables) + resolved + 1
		}
		if resolved < 1 || resolved > len(tables) {
			return fmt.Errorf("table %d out of range (document has %d tables)", tIdx, len(tables))
		}
		targets = []tableInfo{tables[resolved-1]}
	}

	// Handle the operation based on replacement
	replacement := strings.TrimSpace(expr.replacement)

	if replacement == "" {
		// DELETE tables — process in reverse order to preserve indices
		var requests []*docs.Request
		for i := len(targets) - 1; i >= 0; i-- {
			t := targets[i]
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: t.startIdx,
						EndIndex:   t.endIdx,
					},
				},
			})
		}

		err = retryOnQuota(ctx, func() error {
			_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
				Requests: requests,
			}).Context(ctx).Do()
			return e
		})
		if err != nil {
			return fmt.Errorf("batch update (delete table): %w", err)
		}

		return sedOutputOK(ctx, u, id, sedOutputKV{"deleted", fmt.Sprintf("%d table(s)", len(targets))})
	}

	// Future: handle pin=N, other table-level operations
	return fmt.Errorf("unsupported table operation: %q (expected empty replacement for delete)", replacement)
}

type tableWithIndex struct {
	table    *docs.Table
	startIdx int64
}

// collectAllTablesWithIndex returns all tables in the document along with their
// structural element index, used for operations that need positional context.
func collectAllTablesWithIndex(doc *docs.Document) []tableWithIndex {
	var tables []tableWithIndex
	var walkContent func(content []*docs.StructuralElement)
	walkContent = func(content []*docs.StructuralElement) {
		for _, elem := range content {
			if elem.Table != nil {
				tables = append(tables, tableWithIndex{table: elem.Table, startIdx: elem.StartIndex})
				for _, row := range elem.Table.TableRows {
					for _, cell := range row.TableCells {
						walkContent(cell.Content)
					}
				}
			}
		}
	}
	if doc.Body != nil {
		walkContent(doc.Body.Content)
	}
	return tables
}

// collectAllTables returns all tables in the document in order of appearance.
func collectAllTables(doc *docs.Document) []*docs.Table {
	withIdx := collectAllTablesWithIndex(doc)
	tables := make([]*docs.Table, len(withIdx))
	for i, t := range withIdx {
		tables[i] = t.table
	}
	return tables
}

// findTableCell locates a specific table cell in the document.
// Tables are numbered in document order including nested tables.
func findTableCell(doc *docs.Document, ref *tableCellRef) (*docs.TableCell, error) {
	tables := collectAllTables(doc)

	if len(tables) == 0 {
		return nil, fmt.Errorf("document has no tables")
	}

	// Resolve table index
	ti := ref.tableIndex
	if ti < 0 {
		ti = len(tables) + ti + 1 // -1 → last
	}
	if ti < 1 || ti > len(tables) {
		return nil, fmt.Errorf("table %d out of range (document has %d tables)", ref.tableIndex, len(tables))
	}
	table := tables[ti-1]

	// Resolve row
	if ref.row < 1 || ref.row > len(table.TableRows) {
		return nil, fmt.Errorf("row %d out of range (table has %d rows)", ref.row, len(table.TableRows))
	}
	row := table.TableRows[ref.row-1]

	// Resolve col
	if ref.col < 1 || ref.col > len(row.TableCells) {
		return nil, fmt.Errorf("col %d out of range (row has %d columns)", ref.col, len(row.TableCells))
	}
	return row.TableCells[ref.col-1], nil
}

// getCellText extracts the plain text content from a table cell.
// Returns the concatenated text, the start index of the first text run,
// and the end index of the last text run.
func getCellText(cell *docs.TableCell) (text string, startIdx int64, endIdx int64) {
	var b strings.Builder
	for _, elem := range cell.Content {
		if elem.Paragraph != nil {
			for _, pe := range elem.Paragraph.Elements {
				if pe.TextRun != nil {
					b.WriteString(pe.TextRun.Content)
					if startIdx == 0 && pe.StartIndex > 0 {
						startIdx = pe.StartIndex
					}
					endIdx = pe.EndIndex
				}
			}
		}
	}
	return b.String(), startIdx, endIdx
}

// runTableCellReplace handles sed expressions targeting specific table cells
