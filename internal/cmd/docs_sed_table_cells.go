package cmd

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docssed"
	"github.com/steipete/gogcli/internal/ui"
)

// runTableCellReplace replaces content in a specific table cell, handling both
// whole-cell and sub-pattern replacements with optional formatting.
func (c *DocsSedCmd) runTableCellReplace(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	ref := expr.cellRef

	// Route row/col operations to dedicated handler
	if ref.rowOp != "" || ref.colOp != "" {
		return c.runTableRowColOp(ctx, u, account, id, expr)
	}

	planner, err := docssed.NewCellPlanner(semanticExpressionFromSedExpr(expr))
	if err != nil {
		return err
	}

	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	// Handle wildcard ranges: iterate over matching cells
	if ref.row == 0 || ref.col == 0 {
		return c.runTableWildcardReplace(ctx, docsSvc, u, id, doc, expr, planner)
	}

	cell, err := findTableCell(doc, ref)
	if err != nil {
		return fmt.Errorf("find table cell: %w", err)
	}

	cellText, startIdx, endIdx := getCellText(cell)
	plan := planner.Plan(docssed.CellInput{
		Text:           cellText,
		TextStartIndex: startIdx,
		TextEndIndex:   endIdx,
	})
	requests := buildCellPlanRequests(plan)

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", 0})
	}

	_, err = batchUpdate(ctx, docsSvc, id, requests)
	if err != nil {
		return fmt.Errorf("batch update: %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", plan.MatchCount})
}

// runBatchCellReplace batches multiple whole-cell replacements for the same table into one API call.
func (c *DocsSedCmd) runBatchCellReplace(ctx context.Context, _ *ui.UI, account, id string, exprs []indexedExpr) error {
	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	type cellOp struct {
		input       docssed.CellInput
		replacement string
	}
	var ops []cellOp

	for _, ie := range exprs {
		cell, findErr := findTableCell(doc, ie.expr.cellRef)
		if findErr != nil {
			return fmt.Errorf("expression %d: %w", ie.index+1, findErr)
		}
		cellText, startIdx, endIdx := getCellText(cell)
		ops = append(ops, cellOp{
			input: docssed.CellInput{
				Text:           cellText,
				TextStartIndex: startIdx,
				TextEndIndex:   endIdx,
			},
			replacement: ie.expr.replacement,
		})
	}

	// Sort by startIdx descending (reverse document order)
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].input.TextStartIndex > ops[j].input.TextStartIndex
	})

	var requests []*docs.Request
	for _, op := range ops {
		plan := docssed.PlanWholeCellReplacement(op.input, op.replacement)
		requests = append(requests, buildCellPlanRequests(plan)...)
	}

	if len(requests) > 0 {
		_, err = batchUpdate(ctx, docsSvc, id, requests)
		if err != nil {
			return fmt.Errorf("batch cell update: %w", err)
		}
	}

	return nil
}

// runTableWildcardReplace handles cell references with wildcards: |1|[1,*], |1|[*,2], |1|[*,*]
func (c *DocsSedCmd) runTableWildcardReplace(
	ctx context.Context,
	docsSvc *docs.Service,
	u *ui.UI,
	id string,
	doc *docs.Document,
	expr sedExpr,
	planner *docssed.CellPlanner,
) error {
	ref := expr.cellRef

	tables := collectAllTables(doc)
	if len(tables) == 0 {
		return usage("document has no tables")
	}

	ti := ref.tableIndex
	if ti < 0 {
		ti = len(tables) + ti + 1
	}
	if ti < 1 || ti > len(tables) {
		return usagef("table %d out of range (document has %d tables)", ref.tableIndex, len(tables))
	}
	table := tables[ti-1]
	if err := validateWildcardTableRef(table, ref); err != nil {
		return err
	}

	// Collect all matching cells
	var cells []docssed.CellInput

	for ri, row := range table.TableRows {
		for ci, cell := range row.TableCells {
			// Check if this cell matches the wildcard pattern
			rowMatch := ref.row == 0 || ref.row == ri+1
			colMatch := ref.col == 0 || ref.col == ci+1
			if rowMatch && colMatch {
				text, start, end := getCellText(cell)
				cells = append(cells, docssed.CellInput{
					Text:           text,
					TextStartIndex: start,
					TextEndIndex:   end,
				})
			}
		}
	}

	if len(cells) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", 0})
	}

	sort.Slice(cells, func(i, j int) bool {
		return cells[i].TextStartIndex > cells[j].TextStartIndex
	})

	var requests []*docs.Request
	replaced := 0

	for _, cell := range cells {
		plan := planner.Plan(cell)
		requests = append(requests, buildCellPlanRequests(plan)...)
		replaced += plan.MatchCount
	}

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", 0})
	}

	_, err := batchUpdate(ctx, docsSvc, id, requests)
	if err != nil {
		return fmt.Errorf("batch update (wildcard cell replace): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", replaced})
}

func buildCellPlanRequests(plan docssed.TextPlan) []*docs.Request {
	var requests []*docs.Request
	for index := len(plan.TextEdits) - 1; index >= 0; index-- {
		edit := plan.TextEdits[index]
		if edit.StartIndex < edit.EndIndex {
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{StartIndex: edit.StartIndex, EndIndex: edit.EndIndex},
				},
			})
		}
		if edit.InsertText != "" {
			requests = append(requests, &docs.Request{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{Index: edit.StartIndex},
					Text:     edit.InsertText,
				},
			})
		}
	}
	for _, formatting := range plan.Formatting {
		requests = append(
			requests,
			buildTextStyleRequests(formatting.Formats, formatting.StartIndex, formatting.EndIndex)...,
		)
	}
	return requests
}

func validateWildcardTableRef(table *docs.Table, ref *tableCellRef) error {
	rows := len(table.TableRows)
	if ref.row != 0 && (ref.row < 1 || ref.row > rows) {
		return usagef("row %d out of range (table has %d rows)", ref.row, rows)
	}
	if ref.col == 0 {
		return nil
	}
	maxCols := 0
	for _, row := range table.TableRows {
		if len(row.TableCells) > maxCols {
			maxCols = len(row.TableCells)
		}
	}
	if ref.col < 1 || ref.col > maxCols {
		return usagef("col %d out of range (table has %d columns)", ref.col, maxCols)
	}
	return nil
}
