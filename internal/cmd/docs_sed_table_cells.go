package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/docs/v1"

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

	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	// Handle wildcard ranges: iterate over matching cells
	if ref.row == 0 || ref.col == 0 {
		return c.runTableWildcardReplace(ctx, docsSvc, u, id, doc, expr)
	}

	cell, err := findTableCell(doc, ref)
	if err != nil {
		return fmt.Errorf("find table cell: %w", err)
	}

	cellText, startIdx, endIdx := getCellText(cell)

	var requests []*docs.Request
	var newText string

	if expr.pattern == "" {
		// Whole cell replacement: replace entire cell content
		// Expand ${0} (whole match / sed &) to existing cell text
		trimmedCell := strings.TrimRight(cellText, "\n")
		r := literalReplacement(expr.replacement)
		r = strings.ReplaceAll(r, "${0}", trimmedCell)

		// Parse markdown formatting
		plainText, formats := parseMarkdownReplacement(r)
		newText = plainText

		// Strip trailing newline from cell text (cells always end with \n)
		deleteEnd := endIdx
		if len(cellText) > 0 && cellText[len(cellText)-1] == '\n' {
			deleteEnd = endIdx - 1 // keep the trailing newline
		}
		requests = append(requests, buildCellReplaceRequests(startIdx, deleteEnd, newText, formats)...)
	} else {
		// Sub-pattern replacement within the cell
		re, reErr := expr.compilePattern()
		if reErr != nil {
			return fmt.Errorf("compile pattern: %w", reErr)
		}

		// Find matches within cell text
		type cellMatch struct {
			start, end int64
			newText    string
		}
		var matches []cellMatch

		// Use LiteralString unless replacement contains backreferences like ${1}
		replaceFunc := re.ReplaceAllLiteralString
		if strings.Contains(expr.replacement, "${") || strings.Contains(expr.replacement, "$\\") {
			replaceFunc = re.ReplaceAllString
		}

		if expr.global {
			results := re.FindAllStringIndex(cellText, -1)
			for _, loc := range results {
				oldText := cellText[loc[0]:loc[1]]
				replaced := replaceFunc(oldText, expr.replacement)
				matches = append(matches, cellMatch{
					start:   startIdx + int64(loc[0]),
					end:     startIdx + int64(loc[1]),
					newText: replaced,
				})
			}
		} else {
			loc := re.FindStringIndex(cellText)
			if loc != nil {
				oldText := cellText[loc[0]:loc[1]]
				replaced := replaceFunc(oldText, expr.replacement)
				matches = append(matches, cellMatch{
					start:   startIdx + int64(loc[0]),
					end:     startIdx + int64(loc[1]),
					newText: replaced,
				})
			}
		}

		// Build requests in reverse order
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: m.start,
						EndIndex:   m.end,
					},
				},
			})
			if m.newText != "" {
				requests = append(requests, &docs.Request{
					InsertText: &docs.InsertTextRequest{
						Location: &docs.Location{Index: m.start},
						Text:     m.newText,
					},
				})
			}
		}
	}

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", 0})
	}

	err = retryOnQuota(ctx, func() error {
		_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
			Requests: requests,
		}).Context(ctx).Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("batch update: %w", err)
	}

	replaced := 1
	if expr.pattern != "" && expr.global {
		replaced = (len(requests) + 1) / 2 // each match = delete + insert
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", replaced})
}

// runBatchCellReplace batches multiple whole-cell replacements for the same table into one API call.
func (c *DocsSedCmd) runBatchCellReplace(ctx context.Context, _ *ui.UI, account, id string, exprs []indexedExpr) error {
	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	type cellOp struct {
		startIdx, endIdx int64
		cellText         string
		replacement      string
	}
	var ops []cellOp

	for _, ie := range exprs {
		cell, findErr := findTableCell(doc, ie.expr.cellRef)
		if findErr != nil {
			return fmt.Errorf("expression %d: %w", ie.index+1, findErr)
		}
		cellText, startIdx, endIdx := getCellText(cell)
		ops = append(ops, cellOp{startIdx: startIdx, endIdx: endIdx, cellText: cellText, replacement: ie.expr.replacement})
	}

	// Sort by startIdx descending (reverse document order)
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].startIdx > ops[j].startIdx
	})

	var requests []*docs.Request
	for _, op := range ops {
		trimmedCell := strings.TrimRight(op.cellText, "\n")
		r := literalReplacement(op.replacement)
		r = strings.ReplaceAll(r, "${0}", trimmedCell)
		plainText, formats := parseMarkdownReplacement(r)

		deleteEnd := op.endIdx
		if len(op.cellText) > 0 && op.cellText[len(op.cellText)-1] == '\n' {
			deleteEnd = op.endIdx - 1
		}
		requests = append(requests, buildCellReplaceRequests(op.startIdx, deleteEnd, plainText, formats)...)
	}

	if len(requests) > 0 {
		err = retryOnQuota(ctx, func() error {
			_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{Requests: requests}).Context(ctx).Do()
			return e
		})
		if err != nil {
			return fmt.Errorf("batch cell update: %w", err)
		}
	}

	return nil
}

// runTableWildcardReplace handles cell references with wildcards: |1|[1,*], |1|[*,2], |1|[*,*]
func (c *DocsSedCmd) runTableWildcardReplace(ctx context.Context, docsSvc *docs.Service, u *ui.UI, id string, doc *docs.Document, expr sedExpr) error {
	ref := expr.cellRef

	tables := collectAllTables(doc)
	if len(tables) == 0 {
		return fmt.Errorf("document has no tables")
	}

	ti := ref.tableIndex
	if ti < 0 {
		ti = len(tables) + ti + 1
	}
	if ti < 1 || ti > len(tables) {
		return fmt.Errorf("table %d out of range (document has %d tables)", ref.tableIndex, len(tables))
	}
	table := tables[ti-1]

	// Collect all matching cells
	type cellInfo struct {
		startIdx int64
		endIdx   int64
		text     string
	}
	var cells []cellInfo

	for ri, row := range table.TableRows {
		for ci, cell := range row.TableCells {
			// Check if this cell matches the wildcard pattern
			rowMatch := ref.row == 0 || ref.row == ri+1
			colMatch := ref.col == 0 || ref.col == ci+1
			if rowMatch && colMatch {
				text, start, end := getCellText(cell)
				cells = append(cells, cellInfo{startIdx: start, endIdx: end, text: text})
			}
		}
	}

	if len(cells) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", 0})
	}

	// Build requests in reverse order (to preserve indices)
	var requests []*docs.Request
	replaced := 0

	for i := len(cells) - 1; i >= 0; i-- {
		cell := cells[i]

		// Parse the replacement per-cell (need to expand ${0} with cell content)
		cellRepl := literalReplacement(expr.replacement)
		if expr.pattern == "" {
			// Expand ${0} (sed &) to existing cell text
			trimmedCell := strings.TrimRight(cell.text, "\n")
			cellRepl = strings.ReplaceAll(cellRepl, "${0}", trimmedCell)
		}
		plainText, formats := parseMarkdownReplacement(cellRepl)

		if expr.pattern == "" {
			// Whole cell replacement
			deleteEnd := cell.endIdx
			if len(cell.text) > 0 && cell.text[len(cell.text)-1] == '\n' {
				deleteEnd = cell.endIdx - 1
			}
			requests = append(requests, buildCellReplaceRequests(cell.startIdx, deleteEnd, plainText, formats)...)
			replaced++
		} else {
			// Sub-pattern replacement within matching cells
			re, err := expr.compilePattern()
			if err != nil {
				return fmt.Errorf("compile pattern: %w", err)
			}
			results := re.FindAllStringIndex(cell.text, -1)
			if !expr.global && len(results) > 1 {
				results = results[:1]
			}
			for j := len(results) - 1; j >= 0; j-- {
				loc := results[j]
				start := cell.startIdx + int64(loc[0])
				end := cell.startIdx + int64(loc[1])
				requests = append(requests, &docs.Request{
					DeleteContentRange: &docs.DeleteContentRangeRequest{
						Range: &docs.Range{StartIndex: start, EndIndex: end},
					},
				})
				if plainText != "" {
					requests = append(requests, &docs.Request{
						InsertText: &docs.InsertTextRequest{
							Location: &docs.Location{Index: start},
							Text:     plainText,
						},
					})
				}
				replaced++
			}
		}
	}

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", 0})
	}

	err := retryOnQuota(ctx, func() error {
		_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
			Requests: requests,
		}).Context(ctx).Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("batch update (wildcard cell replace): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", replaced})
}
