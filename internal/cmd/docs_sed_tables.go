package cmd

import (
	"context"
	"fmt"
	"math"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docssed"
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

func parseTableCellRef(s string) *tableCellRef {
	return tableCellRefFromParsed(docssed.ParseTableCellReference(s))
}

func (c *DocsSedCmd) runTableOp(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return fmt.Errorf("create docs service: %w", err)
	}

	doc, err := getDoc(ctx, docsSvc, id)
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
		return usage("document has no tables")
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
			return usagef("table %d out of range (document has %d tables)", tIdx, len(tables))
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

		_, err = batchUpdate(ctx, docsSvc, id, requests)
		if err != nil {
			return fmt.Errorf("batch update (delete table): %w", err)
		}

		return sedOutputOK(ctx, u, id, sedOutputKV{"deleted", fmt.Sprintf("%d table(s)", len(targets))})
	}

	// Future: handle pin=N, other table-level operations
	return usagef("unsupported table operation: %q (expected empty replacement for delete)", replacement)
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
		return nil, usage("document has no tables")
	}

	// Resolve table index
	ti := ref.tableIndex
	if ti < 0 {
		ti = len(tables) + ti + 1 // -1 → last
	}
	if ti < 1 || ti > len(tables) {
		return nil, usagef("table %d out of range (document has %d tables)", ref.tableIndex, len(tables))
	}
	table := tables[ti-1]

	// Resolve row
	if ref.row < 1 || ref.row > len(table.TableRows) {
		return nil, usagef("row %d out of range (table has %d rows)", ref.row, len(table.TableRows))
	}
	row := table.TableRows[ref.row-1]

	// Resolve col
	if ref.col < 1 || ref.col > len(row.TableCells) {
		return nil, usagef("col %d out of range (row has %d columns)", ref.col, len(row.TableCells))
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
