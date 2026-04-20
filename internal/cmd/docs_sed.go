package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

// DocsSedCmd implements sed-like find-and-replace operations on Google Docs.
// It supports text replacement, regex, table operations, image insertion, and formatting.
type DocsSedCmd struct {
	DocID       string   `arg:"" name:"docId" help:"Doc ID"`
	Expression  string   `arg:"" optional:"" name:"expression" help:"sed expression: s/pattern/replacement/flags"`
	Expressions []string `short:"e" help:"Additional sed expressions (repeatable)"`
	File        string   `short:"f" help:"Read sed expressions from file (one per line, # comments)"`
	Tab         string   `name:"tab" help:"Tab title or ID for paragraph addressing"`
}

// parseExpressionLines splits data into trimmed non-empty, non-comment lines.
func parseExpressionLines(data []byte) []string {
	var exprs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		exprs = append(exprs, line)
	}
	return exprs
}

// collectExpressions gathers sed expressions from positional arg, -e flags, -f file, and stdin.
func (c *DocsSedCmd) collectExpressions() ([]string, error) {
	var exprs []string

	// 1. Positional argument
	if c.Expression != "" {
		exprs = append(exprs, c.Expression)
	}

	// 2. -e flags
	exprs = append(exprs, c.Expressions...)

	// 3. -f file
	if c.File != "" {
		data, err := os.ReadFile(c.File)
		if err != nil {
			return nil, fmt.Errorf("read sed file: %w", err)
		}
		exprs = append(exprs, parseExpressionLines(data)...)
	}

	// 4. Stdin (only if no expressions from other sources and stdin is not a terminal)
	if len(exprs) == 0 {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return nil, fmt.Errorf("read stdin: %w", err)
			}
			exprs = append(exprs, parseExpressionLines(data)...)
		}
	}

	if len(exprs) == 0 {
		return nil, usage("no sed expressions provided (use positional arg, -e, -f, or stdin)")
	}

	return exprs, nil
}

// sedAddress represents a paragraph-number address prefix on a sed expression.
// Addresses target specific paragraphs by number (1-based), or $ for last.
type sedAddress struct {
	Start    int  // 1-based paragraph number, -1 = last ($)
	End      int  // 0 = same as Start (single paragraph), -1 = last ($)
	HasRange bool // true if comma-separated range was given
}

type sedExpr struct {
	pattern     string
	replacement string // escaped for Go's regexp.ReplaceAllString ($$ = literal $, ${N} = backref)
	global      bool
	nthMatch    int           // >0 means replace only the Nth occurrence (e.g., s/foo/bar/2)
	cellRef     *tableCellRef // non-nil if targeting a specific table cell
	tableRef    int           // non-zero if targeting a whole table (1-indexed, negative from end; math.MinInt32 = all)
	command     byte          // 0 for s//, 'd' for delete, 'a' for append, 'i' for insert, 'y' for transliterate
	brace       *braceExpr    // optional brace expression for SEDMAT v3.5 syntax
	braceSpans  []*braceSpan  // positioned brace spans for inline scoping
	addr        *sedAddress   // optional paragraph address prefix (e.g., 5, 3,7, $)
}

type indexedExpr struct {
	index int
	expr  sedExpr
}

// literalReplacement returns the replacement string with Go regex dollar escaping undone,
// suitable for direct text insertion (not through regexp.ReplaceAllString).
func literalReplacement(repl string) string {
	// $$ → $ (Go regex literal dollar)
	return strings.ReplaceAll(repl, "$$", "$")
}

func (c *DocsSedCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	// Collect all expressions
	rawExprs, err := c.collectExpressions()
	if err != nil {
		return fmt.Errorf("collect expressions: %w", err)
	}

	// Parse all expressions
	var parsed []sedExpr
	for i, raw := range rawExprs {
		expr, parseErr := parseFullExpr(raw)
		if parseErr != nil {
			return fmt.Errorf("expression %d (%q): %w", i+1, raw, parseErr)
		}
		parsed = append(parsed, expr)
	}

	if flags != nil && flags.DryRun {
		return c.runDryRun(ctx, u, parsed)
	}

	account, err := requireAccount(flags)
	if err != nil {
		return fmt.Errorf("require account: %w", err)
	}

	// Single expression: use optimized paths (native, image ref, etc.)
	if len(parsed) == 1 {
		return c.runSingle(ctx, u, account, id, parsed[0])
	}

	// Multiple expressions: batch them
	return c.runBatch(ctx, u, account, id, parsed)
}

// runPositionalInsert handles ^, $, and ^$ patterns for prepend, append, and empty-doc insert.
func (c *DocsSedCmd) runPositionalInsert(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) (bool, error) {
	if expr.pattern != "^$" && expr.pattern != "^" && expr.pattern != "$" {
		return false, nil
	}

	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return true, fmt.Errorf("create docs service: %w", err)
	}

	var doc *docs.Document
	err = retryOnQuota(ctx, func() error {
		var e error
		doc, e = docsSvc.Documents.Get(id).Context(ctx).Do()
		return e
	})
	if err != nil {
		return true, fmt.Errorf("get document: %w", err)
	}

	// Find document body end index
	var bodyEnd int64
	if doc.Body != nil && len(doc.Body.Content) > 0 {
		last := doc.Body.Content[len(doc.Body.Content)-1]
		bodyEnd = last.EndIndex
	}
	if bodyEnd < 2 {
		bodyEnd = 2 // minimum: index 1 is start, trailing \n at end
	}

	// Determine if document is empty (only whitespace/newlines)
	isEmpty := true
	if doc.Body != nil {
		for _, elem := range doc.Body.Content {
			if elem.Paragraph != nil {
				for _, pe := range elem.Paragraph.Elements {
					if pe.TextRun != nil && strings.TrimSpace(pe.TextRun.Content) != "" {
						isEmpty = false
						break
					}
				}
			}
			if elem.Table != nil {
				isEmpty = false
			}
			if !isEmpty {
				break
			}
		}
	}

	switch expr.pattern {
	case "^$":
		if expr.replacement == "" && !isEmpty {
			// s/^$// on a non-empty doc = clear all content
			deleteEnd := bodyEnd - 1
			if deleteEnd < 2 {
				return true, sedOutputOK(ctx, u, id, sedOutputKV{"cleared", 0})
			}
			err = retryOnQuota(ctx, func() error {
				_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
					Requests: []*docs.Request{{
						DeleteContentRange: &docs.DeleteContentRangeRequest{
							Range: &docs.Range{
								StartIndex: 1,
								EndIndex:   deleteEnd,
							},
						},
					}},
				}).Context(ctx).Do()
				return e
			})
			if err != nil {
				return true, fmt.Errorf("clearing document: %w", err)
			}
			return true, sedOutputOK(ctx, u, id, sedOutputKV{"cleared", deleteEnd - 1})
		}
		if !isEmpty {
			// Not empty — no match, report 0 replaced
			return true, sedOutputOK(ctx, u, id, sedOutputKV{"replaced", 0}, sedOutputKV{"message", "document is not empty"})
		}
		// Empty doc with empty replacement = no-op (already empty)
		if expr.replacement == "" {
			return true, sedOutputOK(ctx, u, id, sedOutputKV{"cleared", 0})
		}
		// Empty doc — insert at index 1
		return true, c.doPositionalInsert(ctx, docsSvc, u, id, 1, literalReplacement(expr.replacement))

	case "^":
		// Prepend — insert at index 1 (beginning of body)
		return true, c.doPositionalInsert(ctx, docsSvc, u, id, 1, literalReplacement(expr.replacement))

	case "$":
		// Append — insert before the final newline
		insertIdx := bodyEnd - 1
		if insertIdx < 1 {
			insertIdx = 1
		}
		return true, c.doPositionalInsert(ctx, docsSvc, u, id, insertIdx, literalReplacement(expr.replacement))
	}

	return false, nil
}

// runSingle executes a single sed expression, routing to the appropriate handler
// based on the expression type (command, table, positional, cell, image, native, or manual).
func (c *DocsSedCmd) runSingle(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	// Handle addressed expressions (paragraph-number targeting)
	if expr.addr != nil {
		switch expr.command {
		case 'd':
			return c.runAddressedDelete(ctx, u, account, id, c.Tab, expr)
		case 'a':
			return c.runAddressedAppend(ctx, u, account, id, c.Tab, expr)
		case 'i':
			return c.runAddressedInsert(ctx, u, account, id, c.Tab, expr)
		case 0:
			// s// substitution scoped to addressed paragraphs
			return c.runAddressedSubstitute(ctx, u, account, id, c.Tab, expr)
		default:
			return fmt.Errorf("addressed %c command not supported", expr.command)
		}
	}

	// Handle non-substitution commands
	switch expr.command {
	case 'd':
		return c.runDeleteCommand(ctx, u, account, id, expr)
	case 'a':
		return c.runAppendCommand(ctx, u, account, id, expr)
	case 'i':
		return c.runInsertCommand(ctx, u, account, id, expr)
	case 'y':
		return c.runTransliterate(ctx, u, account, id, expr)
	}

	// Check table-level operations (s/|1|//, s/|*|//, etc.)
	if expr.tableRef != 0 {
		return c.runTableOp(ctx, u, account, id, expr)
	}

	// Check positional patterns first: ^$ (empty), ^ (prepend), $ (append)
	if handled, err := c.runPositionalInsert(ctx, u, account, id, expr); handled {
		return err
	}

	// Check if this targets a specific table cell
	if expr.cellRef != nil {
		// Check for merge/split operations
		repl := strings.TrimSpace(strings.ToLower(expr.replacement))
		if repl == mergeOp || repl == unmergeOp || repl == splitOp {
			return c.runTableMerge(ctx, u, account, id, expr)
		}
		return c.runTableCellReplace(ctx, u, account, id, expr)
	}

	// Check if replacement is a table creation spec (|RxC|, |RxC:header|, or pipe-table)
	if tableSpec := parseTableCreate(expr.replacement); tableSpec != nil {
		return c.runTableCreate(ctx, u, account, id, expr, tableSpec)
	}
	if tableSpec := parseTableFromPipes(expr.replacement); tableSpec != nil {
		return c.runTableCreate(ctx, u, account, id, expr, tableSpec)
	}

	// Check if pattern is an image reference (!(n), ![regex], etc.)
	imgRef := parseImageRefPattern(expr.pattern)
	if imgRef != nil {
		return c.runImageReplace(ctx, u, account, id, imgRef, expr.replacement, expr.global)
	}

	// Check if we can use native API (no formatting in replacement)
	if canUseNativeReplace(expr.replacement) && expr.global && expr.nthMatch <= 0 {
		return c.runNative(ctx, u, account, id, expr.pattern, expr.replacement)
	}

	return c.runManual(ctx, u, account, id, expr)
}

// runBatch executes multiple sed expressions efficiently by batching native replacements
// into a single API call and routing other types to their specialized handlers.
func (c *DocsSedCmd) runBatch(ctx context.Context, u *ui.UI, account, id string, exprs []sedExpr) error {
	// Split into native (plain text replace) and manual (needs formatting/images)
	type indexedTableExpr struct {
		index int
		expr  sedExpr
		spec  *tableCreateSpec
	}
	var nativeExprs []indexedExpr
	var manualExprs []indexedExpr
	var cellExprs []indexedExpr
	var tableCreateExprs []indexedTableExpr

	// Positional patterns (^$, ^, $) must run individually and sequentially
	// because each one changes the document state.
	var positionalExprs []indexedExpr
	// Image replacements must run individually — Google Docs API cannot
	// reliably fetch images when mixed with other batch operations.
	var imageExprs []indexedExpr

	var addressedExprs []indexedExpr

	for i, expr := range exprs {
		ie := indexedExpr{i, expr}
		switch classifyExprForBatch(expr) {
		case exprCatAddressed:
			addressedExprs = append(addressedExprs, ie)
		case exprCatPositional:
			positionalExprs = append(positionalExprs, ie)
		case exprCatImage:
			imageExprs = append(imageExprs, ie)
		case exprCatCommand, exprCatImagePattern:
			manualExprs = append(manualExprs, ie)
		case exprCatCell:
			cellExprs = append(cellExprs, ie)
		case exprCatTableCreate:
			spec := parseTableCreate(expr.replacement)
			if spec == nil {
				spec = parseTableFromPipes(expr.replacement)
			}
			tableCreateExprs = append(tableCreateExprs, indexedTableExpr{i, expr, spec})
		case exprCatNative:
			nativeExprs = append(nativeExprs, ie)
		case exprCatManual:
			manualExprs = append(manualExprs, ie)
		}
	}

	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return fmt.Errorf("create docs service: %w", err)
	}

	totalReplaced := 0

	// Run positional expressions sequentially (each changes doc state)
	for _, ie := range positionalExprs {
		if _, err2 := c.runPositionalInsert(ctx, u, account, id, ie.expr); err2 != nil {
			return fmt.Errorf("expression %d: %w", ie.index+1, err2)
		}
	}

	// Run addressed expressions sequentially (each changes doc state via paragraph map)
	for _, ie := range addressedExprs {
		if singleErr := c.runSingle(ctx, u, account, id, ie.expr); singleErr != nil {
			return fmt.Errorf("expression %d: %w", ie.index+1, singleErr)
		}
		totalReplaced++
	}

	// Batch all native expressions into one API call
	if len(nativeExprs) > 0 {
		var requests []*docs.Request
		for _, ie := range nativeExprs {
			requests = append(requests, &docs.Request{
				ReplaceAllText: &docs.ReplaceAllTextRequest{
					ContainsText: &docs.SubstringMatchCriteria{
						Text:          ie.expr.pattern,
						MatchCase:     true,
						SearchByRegex: true,
					},
					ReplaceText: ie.expr.replacement,
				},
			})
		}

		resp, err2 := batchUpdate(ctx, docsSvc, id, requests)
		if err2 != nil {
			return fmt.Errorf("native batch update: %w", err2)
		}

		if resp != nil {
			for _, reply := range resp.Replies {
				if reply.ReplaceAllText != nil {
					totalReplaced += int(reply.ReplaceAllText.OccurrencesChanged)
				}
			}
		}
	}

	// Process manual expressions sequentially (each may need fresh doc state)
	for _, ie := range manualExprs {
		// Non-substitution commands (d/a/i/y) run through runSingle
		if ie.expr.command != 0 {
			if singleErr := c.runSingle(ctx, u, account, id, ie.expr); singleErr != nil {
				return fmt.Errorf("expression %d: %w", ie.index+1, singleErr)
			}
			totalReplaced++
			continue
		}

		imgRef := parseImageRefPattern(ie.expr.pattern)
		if imgRef != nil {
			if imgErr := c.runImageReplace(ctx, u, account, id, imgRef, ie.expr.replacement, ie.expr.global); imgErr != nil {
				return fmt.Errorf("expression %d: %w", ie.index+1, imgErr)
			}
			totalReplaced++
			continue
		}

		// Manual formatting replace — need to fetch doc each time since indices shift
		count, _, manualErr := c.runManualInner(ctx, docsSvc, id, ie.expr)
		if manualErr != nil {
			return fmt.Errorf("expression %d (pattern=%q repl=%q): %w", ie.index+1, ie.expr.pattern, ie.expr.replacement, manualErr)
		}
		totalReplaced += count
	}

	// Apply deferred nested bullets. Re-fetches doc to find paragraphs with
	// leading \t, groups them with adjacent bulleted paragraphs, and re-creates
	// bullets with merged ranges so Docs interprets tabs as nesting levels.
	if len(manualExprs) > 0 {
		if bulletErr := c.applyDeferredBullets(ctx, docsSvc, id); bulletErr != nil {
			return fmt.Errorf("apply bullets: %w", bulletErr)
		}
	}

	// Process table creation expressions sequentially (each creates a table via API)
	for _, ie := range tableCreateExprs {
		if createErr := c.runTableCreate(ctx, u, account, id, ie.expr, ie.spec); createErr != nil {
			return fmt.Errorf("expression %d: %w", ie.index+1, createErr)
		}
		totalReplaced++
	}

	// Process cell expressions — batch same-table whole-cell replacements
	cellReplaced, err := c.processCellExprs(ctx, u, account, id, cellExprs)
	if err != nil {
		return err
	}
	totalReplaced += cellReplaced

	// Process image replacements individually (each needs its own API call for reliable image fetching).
	// Add a brief pause before each image to avoid rate-limit issues with Google's image fetcher.
	for _, ie := range imageExprs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		if singleErr := c.runSingle(ctx, u, account, id, ie.expr); singleErr != nil {
			// Retry once after a longer pause (image fetch can be flaky under load)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			if retryErr := c.runSingle(ctx, u, account, id, ie.expr); retryErr != nil {
				return fmt.Errorf("expression %d (image): %w", ie.index+1, retryErr)
			}
		}
		totalReplaced++
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"expressions", len(exprs)}, sedOutputKV{"replaced", totalReplaced})
}

// exprCategory describes how an expression should be processed in batch mode.
type exprCategory int

const (
	exprCatPositional   exprCategory = iota // ^, $, ^$ — sequential, changes doc state
	exprCatImage                            // image replacement — individual API calls
	exprCatCommand                          // d/a/i/y — run individually via runSingle
	exprCatCell                             // table cell targeting
	exprCatTableCreate                      // table creation spec
	exprCatImagePattern                     // image pattern in search (!(n), ![re])
	exprCatNative                           // plain text replace via native API
	exprCatManual                           // requires manual formatting path
	exprCatAddressed                        // paragraph-addressed — sequential, changes doc state
)

// classifyExprForBatch determines how an expression should be processed in batch mode.
func classifyExprForBatch(expr sedExpr) exprCategory {
	// Addressed expressions must run sequentially — they change document state
	if expr.addr != nil {
		return exprCatAddressed
	}
	if expr.command == 0 && expr.cellRef == nil && expr.tableRef == 0 &&
		(expr.pattern == "^$" || expr.pattern == "^" || expr.pattern == "$") {
		return exprCatPositional
	}
	if expr.command == 0 && expr.cellRef == nil && expr.tableRef == 0 &&
		(strings.HasPrefix(expr.replacement, "![") ||
			(expr.brace != nil && expr.brace.ImgRef != "")) {
		return exprCatImage
	}
	if expr.command != 0 {
		return exprCatCommand
	}
	if expr.cellRef != nil || expr.tableRef != 0 {
		return exprCatCell
	}
	if parseTableCreate(expr.replacement) != nil || parseTableFromPipes(expr.replacement) != nil {
		return exprCatTableCreate
	}
	if parseImageRefPattern(expr.pattern) != nil {
		return exprCatImagePattern
	}
	if canUseNativeReplace(expr.replacement) && expr.global && expr.nthMatch <= 0 {
		return exprCatNative
	}
	return exprCatManual
}

// isMergeOp returns true if the replacement string (lowered+trimmed) is a merge/unmerge/split op.
func isMergeOp(repl string) bool {
	r := strings.TrimSpace(strings.ToLower(repl))
	return r == mergeOp || r == unmergeOp || r == splitOp
}

// canBatchCell returns true if the expression is a simple whole-cell replacement
// that can be batched with adjacent same-table expressions.
func canBatchCell(ie indexedExpr) bool {
	return ie.expr.cellRef != nil && ie.expr.pattern == "" &&
		ie.expr.cellRef.row > 0 && ie.expr.cellRef.col > 0 &&
		!isMergeOp(ie.expr.replacement) &&
		ie.expr.cellRef.rowOp == "" && ie.expr.cellRef.colOp == ""
}

// processCellExprs handles cell-level expressions: merge/split, row/col ops, table ops,
// and batches consecutive whole-cell replacements for efficiency.
func (c *DocsSedCmd) processCellExprs(ctx context.Context, u *ui.UI, account, id string, cellExprs []indexedExpr) (int, error) {
	totalReplaced := 0
	i := 0
	for i < len(cellExprs) {
		ie := cellExprs[i]

		// Merge/split/unmerge ops run individually
		if isMergeOp(ie.expr.replacement) {
			if err := c.runTableMerge(ctx, u, account, id, ie.expr); err != nil {
				return totalReplaced, fmt.Errorf("expression %d: %w", ie.index+1, err)
			}
			totalReplaced++
			i++
			continue
		}

		// Row/col ops run individually
		if ie.expr.cellRef != nil && (ie.expr.cellRef.rowOp != "" || ie.expr.cellRef.colOp != "") {
			if err := c.runTableCellReplace(ctx, u, account, id, ie.expr); err != nil {
				return totalReplaced, fmt.Errorf("expression %d: %w", ie.index+1, err)
			}
			totalReplaced++
			i++
			continue
		}

		// Table-level ops (delete table) run individually
		if ie.expr.tableRef != 0 {
			if err := c.runTableOp(ctx, u, account, id, ie.expr); err != nil {
				return totalReplaced, fmt.Errorf("expression %d: %w", ie.index+1, err)
			}
			totalReplaced++
			i++
			continue
		}

		// Collect consecutive whole-cell replacements for the same table
		if canBatchCell(ie) {
			tableIdx := ie.expr.cellRef.tableIndex
			batch := []indexedExpr{ie}
			j := i + 1
			for j < len(cellExprs) && canBatchCell(cellExprs[j]) && cellExprs[j].expr.cellRef.tableIndex == tableIdx {
				batch = append(batch, cellExprs[j])
				j++
			}

			if len(batch) > 1 {
				if err := c.runBatchCellReplace(ctx, u, account, id, batch); err != nil {
					return totalReplaced, fmt.Errorf("cell batch (expressions %d-%d): %w", batch[0].index+1, batch[len(batch)-1].index+1, err)
				}
				totalReplaced += len(batch)
			} else {
				if err := c.runTableCellReplace(ctx, u, account, id, ie.expr); err != nil {
					return totalReplaced, fmt.Errorf("expression %d: %w", ie.index+1, err)
				}
				totalReplaced++
			}
			i = j
			continue
		}

		// Wildcard or other cell ops run individually
		if err := c.runTableCellReplace(ctx, u, account, id, ie.expr); err != nil {
			return totalReplaced, fmt.Errorf("expression %d: %w", ie.index+1, err)
		}
		totalReplaced++
		i++
	}
	return totalReplaced, nil
}

func (c *DocsSedCmd) runNative(ctx context.Context, u *ui.UI, account, docID, pattern, replacement string) error {
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return fmt.Errorf("create docs service: %w", err)
	}

	// Build native replace request with regex
	requests := []*docs.Request{
		{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					Text:          pattern,
					MatchCase:     true,
					SearchByRegex: true,
				},
				ReplaceText: replacement,
			},
		},
	}

	resp, err := batchUpdate(ctx, docsSvc, docID, requests)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}

	// Get replacement count from response
	replaced := int64(0)
	if resp != nil && len(resp.Replies) > 0 && resp.Replies[0].ReplaceAllText != nil {
		replaced = resp.Replies[0].ReplaceAllText.OccurrencesChanged
	}

	return sedOutputOK(ctx, u, docID, sedOutputKV{"replaced", replaced}, sedOutputKV{"native", true})
}
