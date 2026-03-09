package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

// runTableRowColOp handles row and column operations: insert, delete, and append.
func (c *DocsSedCmd) runTableRowColOp(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	ref := expr.cellRef

	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	tablesWithIdx := collectAllTablesWithIndex(doc)
	if len(tablesWithIdx) == 0 {
		return fmt.Errorf("document has no tables")
	}

	// Resolve table index
	ti := ref.tableIndex
	if ti < 0 {
		ti = len(tablesWithIdx) + ti + 1
	}
	if ti < 1 || ti > len(tablesWithIdx) {
		return fmt.Errorf("table %d out of range (document has %d tables)", ref.tableIndex, len(tablesWithIdx))
	}
	tw := tablesWithIdx[ti-1]
	table := tw.table
	tableStartLoc := &docs.Location{Index: tw.startIdx}

	var requests []*docs.Request
	opDesc := ""

	if ref.rowOp != "" {
		numRows := len(table.TableRows)
		switch ref.rowOp {
		case opDelete:
			target := ref.opTarget
			if target < 0 {
				target = numRows + target + 1
			}
			if target < 1 || target > numRows {
				return fmt.Errorf("row %d out of range (table has %d rows)", ref.opTarget, numRows)
			}
			if numRows <= 1 {
				return fmt.Errorf("cannot delete the only row in a table")
			}
			cellLoc := &docs.TableCellLocation{
				TableStartLocation: tableStartLoc,
				RowIndex:           int64(target - 1),
				ColumnIndex:        0,
			}
			requests = append(requests, &docs.Request{
				DeleteTableRow: &docs.DeleteTableRowRequest{
					TableCellLocation: cellLoc,
				},
			})
			opDesc = fmt.Sprintf("deleted row %d", target)

		case opInsert:
			target := ref.opTarget
			if target < 0 {
				target = numRows + target + 1
			}
			if target < 1 || target > numRows {
				return fmt.Errorf("row %d out of range for insert (table has %d rows)", ref.opTarget, numRows)
			}
			cellLoc := &docs.TableCellLocation{
				TableStartLocation: tableStartLoc,
				RowIndex:           int64(target - 1),
				ColumnIndex:        0,
			}
			requests = append(requests, &docs.Request{
				InsertTableRow: &docs.InsertTableRowRequest{
					TableCellLocation: cellLoc,
					InsertBelow:       false,
				},
			})
			opDesc = fmt.Sprintf("inserted row before row %d", target)

		case opAppend:
			lastRow := numRows - 1
			cellLoc := &docs.TableCellLocation{
				TableStartLocation: tableStartLoc,
				RowIndex:           int64(lastRow),
				ColumnIndex:        0,
			}
			requests = append(requests, &docs.Request{
				InsertTableRow: &docs.InsertTableRowRequest{
					TableCellLocation: cellLoc,
					InsertBelow:       true,
				},
			})
			opDesc = "appended row at end"
		}
	}

	if ref.colOp != "" {
		numCols := 0
		if len(table.TableRows) > 0 {
			numCols = len(table.TableRows[0].TableCells)
		}
		switch ref.colOp {
		case opDelete:
			target := ref.opTarget
			if target < 0 {
				target = numCols + target + 1
			}
			if target < 1 || target > numCols {
				return fmt.Errorf("col %d out of range (table has %d columns)", ref.opTarget, numCols)
			}
			if numCols <= 1 {
				return fmt.Errorf("cannot delete the only column in a table")
			}
			cellLoc := &docs.TableCellLocation{
				TableStartLocation: tableStartLoc,
				RowIndex:           0,
				ColumnIndex:        int64(target - 1),
			}
			requests = append(requests, &docs.Request{
				DeleteTableColumn: &docs.DeleteTableColumnRequest{
					TableCellLocation: cellLoc,
				},
			})
			opDesc = fmt.Sprintf("deleted column %d", target)

		case opInsert:
			target := ref.opTarget
			if target < 0 {
				target = numCols + target + 1
			}
			if target < 1 || target > numCols {
				return fmt.Errorf("col %d out of range for insert (table has %d columns)", ref.opTarget, numCols)
			}
			cellLoc := &docs.TableCellLocation{
				TableStartLocation: tableStartLoc,
				RowIndex:           0,
				ColumnIndex:        int64(target - 1),
			}
			requests = append(requests, &docs.Request{
				InsertTableColumn: &docs.InsertTableColumnRequest{
					TableCellLocation: cellLoc,
					InsertRight:       false,
				},
			})
			opDesc = fmt.Sprintf("inserted column before column %d", target)

		case opAppend:
			lastCol := numCols - 1
			cellLoc := &docs.TableCellLocation{
				TableStartLocation: tableStartLoc,
				RowIndex:           0,
				ColumnIndex:        int64(lastCol),
			}
			requests = append(requests, &docs.Request{
				InsertTableColumn: &docs.InsertTableColumnRequest{
					TableCellLocation: cellLoc,
					InsertRight:       true,
				},
			})
			opDesc = "appended column at end"
		}
	}

	if len(requests) == 0 {
		return fmt.Errorf("no row/column operation to perform")
	}

	err = retryOnQuota(ctx, func() error {
		_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
			Requests: requests,
		}).Context(ctx).Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("batch update (row/col op): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"op", opDesc})
}

// runTableMerge handles merging or unmerging table cells.
// Merge: s/|1|[1,1:2,3]/merge/ — merges cells from [1,1] to [2,3]
// Unmerge/split: s/|1|[1,1]/unmerge/ or s/|1|[1,1]/split/
func (c *DocsSedCmd) runTableMerge(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	ref := expr.cellRef

	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	tablesWithIdx := collectAllTablesWithIndex(doc)
	tIdx := ref.tableIndex
	if tIdx < 0 {
		tIdx = len(tablesWithIdx) + tIdx + 1
	}
	if tIdx < 1 || tIdx > len(tablesWithIdx) {
		return fmt.Errorf("table index %d out of range (have %d tables)", ref.tableIndex, len(tablesWithIdx))
	}
	tableStartLoc := &docs.Location{Index: tablesWithIdx[tIdx-1].startIdx}

	repl := strings.TrimSpace(strings.ToLower(expr.replacement))
	var requests []*docs.Request
	var opDesc string

	switch repl {
	case "merge":
		if ref.endRow == 0 || ref.endCol == 0 {
			return fmt.Errorf("merge requires a range: |N|[r1,c1:r2,c2]")
		}
		requests = append(requests, &docs.Request{
			MergeTableCells: &docs.MergeTableCellsRequest{
				TableRange: &docs.TableRange{
					TableCellLocation: &docs.TableCellLocation{
						TableStartLocation: tableStartLoc,
						RowIndex:           int64(ref.row - 1),
						ColumnIndex:        int64(ref.col - 1),
					},
					RowSpan:    int64(ref.endRow - ref.row + 1),
					ColumnSpan: int64(ref.endCol - ref.col + 1),
				},
			},
		})
		opDesc = fmt.Sprintf("merged [%d,%d:%d,%d]", ref.row, ref.col, ref.endRow, ref.endCol)
	case unmergeOp, splitOp:
		requests = append(requests, &docs.Request{
			UnmergeTableCells: &docs.UnmergeTableCellsRequest{
				TableRange: &docs.TableRange{
					TableCellLocation: &docs.TableCellLocation{
						TableStartLocation: tableStartLoc,
						RowIndex:           int64(ref.row - 1),
						ColumnIndex:        int64(ref.col - 1),
					},
					// For unmerge, span the minimum (1x1) — Google will unmerge whatever
					// merged region contains this cell
					RowSpan:    1,
					ColumnSpan: 1,
				},
			},
		})
		opDesc = fmt.Sprintf("unmerged [%d,%d]", ref.row, ref.col)
	default:
		return fmt.Errorf("unknown merge operation %q (expected merge, unmerge, or split)", repl)
	}

	err = retryOnQuota(ctx, func() error {
		_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
			Requests: requests,
		}).Context(ctx).Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("batch update (%s): %w", opDesc, err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"action", opDesc})
}
