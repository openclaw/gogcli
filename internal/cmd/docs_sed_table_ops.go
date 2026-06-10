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

	target, _, err := resolveDocsTableWithIndex(doc, ref.tableIndex)
	if err != nil {
		return err
	}
	var dimension docsTableDimension
	var action string
	switch {
	case ref.rowOp != "":
		dimension = docsTableDimensionRow
		action = ref.rowOp
	case ref.colOp != "":
		dimension = docsTableDimensionColumn
		action = ref.colOp
	default:
		return usage("no row/column operation to perform")
	}

	builderAction := action
	if builderAction == opAppend {
		builderAction = opInsert
	}
	req, resolved, err := buildDocsTableDimensionRequest(
		target, dimension, builderAction, ref.opTarget, action == opAppend, "",
	)
	if err != nil {
		return err
	}
	if _, err := batchUpdate(ctx, docsSvc, id, []*docs.Request{req}); err != nil {
		return fmt.Errorf("batch update (row/col op): %w", err)
	}

	var opDesc string
	switch action {
	case opDelete:
		opDesc = fmt.Sprintf("deleted %s %d", dimension, resolved)
	case opInsert:
		opDesc = fmt.Sprintf("inserted %s before %s %d", dimension, dimension, resolved)
	case opAppend:
		opDesc = fmt.Sprintf("appended %s at end", dimension)
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

	target, _, err := resolveDocsTableWithIndex(doc, ref.tableIndex)
	if err != nil {
		return err
	}
	repl := strings.TrimSpace(strings.ToLower(expr.replacement))
	var opDesc string

	switch repl {
	case "merge":
		if ref.endRow == 0 || ref.endCol == 0 {
			return usage("merge requires a range: |N|[r1,c1:r2,c2]")
		}
		opDesc = fmt.Sprintf("merged [%d,%d:%d,%d]", ref.row, ref.col, ref.endRow, ref.endCol)
	case unmergeOp, splitOp:
		opDesc = fmt.Sprintf("unmerged [%d,%d]", ref.row, ref.col)
	default:
		return usagef("unknown merge operation %q (expected merge, unmerge, or split)", repl)
	}

	endRow, endCol := ref.endRow, ref.endCol
	if repl == unmergeOp || repl == splitOp {
		endRow, endCol = ref.row, ref.col
	}
	req, err := buildDocsTableMergeRequest(target, repl, ref.row, ref.col, endRow, endCol, "")
	if err != nil {
		return err
	}
	if _, err := batchUpdate(ctx, docsSvc, id, []*docs.Request{req}); err != nil {
		return fmt.Errorf("batch update (%s): %w", opDesc, err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"action", opDesc})
}
