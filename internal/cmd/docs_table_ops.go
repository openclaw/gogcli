package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docstable"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsTableRowCmd struct {
	Insert    DocsTableRowInsertCmd `cmd:"" name:"insert" aliases:"add,append" help:"Insert a native table row"`
	Delete    DocsTableRowDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a native table row"`
	Style     DocsTableRowStyleCmd  `cmd:"" name:"style" help:"Set native table row height and overflow styles"`
	PinHeader DocsTablePinHeaderCmd `cmd:"" name:"pin-header" help:"Pin or unpin leading table header rows"`
}

type DocsTableColumnCmd struct {
	Insert DocsTableColumnInsertCmd `cmd:"" name:"insert" aliases:"add,append" help:"Insert a native table column"`
	Delete DocsTableColumnDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a native table column"`
}

type DocsTableRowInsertCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Table      string `name:"table" help:"Table selector: index, exact first-cell text, *, or text:VALUE for numeric/syntax-looking text" default:"1"`
	At         string `name:"at" help:"Insert before this 1-based row, use a negative index from the end, or end" default:"end"`
	ValuesJSON string `name:"values-json" help:"Optional JSON string array containing the new row values"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type DocsTableRowDeleteCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Table string `name:"table" help:"Table selector: index, exact first-cell text, *, or text:VALUE for numeric/syntax-looking text" default:"1"`
	Row   int    `name:"row" required:"" help:"1-based row number; negative indexes count from the end"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type DocsTableColumnInsertCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Table string `name:"table" help:"Table selector: index, exact first-cell text, *, or text:VALUE for numeric/syntax-looking text" default:"1"`
	At    string `name:"at" help:"Insert before this 1-based column, use a negative index from the end, or end" default:"end"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type DocsTableColumnDeleteCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Table string `name:"table" help:"Table selector: index, exact first-cell text, *, or text:VALUE for numeric/syntax-looking text" default:"1"`
	Col   int    `name:"col" required:"" help:"1-based column number; negative indexes count from the end"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type DocsTableMergeCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Table string `name:"table" help:"Table selector: index, exact first-cell text, *, or text:VALUE for numeric/syntax-looking text" default:"1"`
	Range string `name:"range" required:"" help:"1-based cell range r1,c1:r2,c2"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type DocsTableUnmergeCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Table string `name:"table" help:"Table selector: index, exact first-cell text, *, or text:VALUE for numeric/syntax-looking text" default:"1"`
	Cell  string `name:"cell" required:"" help:"1-based cell r,c inside the merged region"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type docsSelectedTable struct {
	tableWithIndex
	index int
}

type docsTableMutationResult struct {
	TableIndex int    `json:"tableIndex"`
	Index      int    `json:"index,omitempty"`
	Range      string `json:"range,omitempty"`
}

func (c *DocsTableRowInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID, err := validateDocsTableMutationArgs(c.DocID, c.Table)
	if err != nil {
		return err
	}
	at, appendAtEnd, err := parseDocsTableInsertAt(c.At, "row")
	if err != nil {
		return err
	}
	values, err := parseDocsTableRowValues(c.ValuesJSON)
	if err != nil {
		return err
	}
	hasValues := strings.TrimSpace(c.ValuesJSON) != ""
	if dryRunErr := dryRunExit(ctx, flags, "docs.table-row.insert", map[string]any{
		"documentId": docID,
		"table":      c.Table,
		"at":         c.At,
		"values":     values,
		"tab":        c.Tab,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, loaded, selected, err := loadDocsSelectedTables(ctx, flags, docID, c.Tab, c.Table)
	if err != nil {
		return err
	}
	if hasValues && len(selected) != 1 {
		return usagef("--values-json requires exactly one selected table (matched %d)", len(selected))
	}
	requests := make([]*docs.Request, 0, len(selected))
	results := make([]docsTableMutationResult, 0, len(selected))
	for _, target := range selected {
		req, resolved, buildErr := docstable.BuildDimensionRequest(
			docsTablePlanTarget(target.tableWithIndex), docstable.Row, docstable.Insert, at, appendAtEnd, loaded.tabID,
		)
		if buildErr != nil {
			return usage(buildErr.Error())
		}
		if hasValues && docstable.RowBoundaryCrossesMerge(target.table, resolved) {
			return usagef(
				"cannot insert row %d with --values-json because a vertically merged cell crosses that boundary",
				resolved,
			)
		}
		if hasValues && len(values) != docstable.ColumnCount(target.table) {
			return usagef("--values-json has %d values, table has %d columns", len(values), docstable.ColumnCount(target.table))
		}
		requests = append(requests, req)
		results = append(results, docsTableMutationResult{TableIndex: target.index, Index: resolved})
	}
	insertedRevisionID, err := executeDocsTableRequests(ctx, svc, docID, loaded.full.RevisionId, requests)
	if err != nil {
		return fmt.Errorf("insert table row: %w", err)
	}
	if hasValues {
		if insertedRevisionID == "" {
			return fmt.Errorf("insert table row: provider did not return required revision control")
		}
		if err := populateDocsTableRow(
			ctx, svc, docID, loaded.tabID, insertedRevisionID,
			results[0].TableIndex, results[0].Index, values,
		); err != nil {
			return err
		}
	}
	return writeDocsTableMutationResult(ctx, docID, loaded.tabID, "insert", "row", results)
}

func (c *DocsTableRowDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runDocsTableDimensionCommand(ctx, flags, docsTableDimensionCommand{
		docID: c.DocID, table: c.Table, tab: c.Tab, dimension: docstable.Row,
		action: opDelete, target: c.Row, dryRunOp: "docs.table-row.delete",
	})
}

func (c *DocsTableColumnInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	at, appendAtEnd, err := parseDocsTableInsertAt(c.At, "column")
	if err != nil {
		return err
	}
	return runDocsTableDimensionCommand(ctx, flags, docsTableDimensionCommand{
		docID: c.DocID, table: c.Table, tab: c.Tab, dimension: docstable.Column,
		action: opInsert, target: at, appendAtEnd: appendAtEnd, dryRunOp: "docs.table-column.insert",
	})
}

func (c *DocsTableColumnDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runDocsTableDimensionCommand(ctx, flags, docsTableDimensionCommand{
		docID: c.DocID, table: c.Table, tab: c.Tab, dimension: docstable.Column,
		action: opDelete, target: c.Col, dryRunOp: "docs.table-column.delete",
	})
}

type docsTableDimensionCommand struct {
	docID       string
	table       string
	tab         string
	dimension   docstable.Dimension
	action      string
	target      int
	appendAtEnd bool
	dryRunOp    string
}

func runDocsTableDimensionCommand(ctx context.Context, flags *RootFlags, command docsTableDimensionCommand) error {
	docID, err := validateDocsTableMutationArgs(command.docID, command.table)
	if err != nil {
		return err
	}
	if command.target == 0 && !command.appendAtEnd {
		return usagef("--%s cannot be 0", command.dimension)
	}
	targetValue := any(command.target)
	if command.appendAtEnd {
		targetValue = "end"
	}
	if dryRunErr := dryRunExit(ctx, flags, command.dryRunOp, map[string]any{
		"documentId":              docID,
		"table":                   command.table,
		string(command.dimension): targetValue,
		"tab":                     command.tab,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, loaded, selected, err := loadDocsSelectedTables(ctx, flags, docID, command.tab, command.table)
	if err != nil {
		return err
	}
	requests := make([]*docs.Request, 0, len(selected))
	results := make([]docsTableMutationResult, 0, len(selected))
	for _, target := range selected {
		req, resolved, buildErr := docstable.BuildDimensionRequest(
			docsTablePlanTarget(target.tableWithIndex),
			command.dimension,
			docstable.Action(command.action),
			command.target,
			command.appendAtEnd,
			loaded.tabID,
		)
		if buildErr != nil {
			return usage(buildErr.Error())
		}
		requests = append(requests, req)
		results = append(results, docsTableMutationResult{TableIndex: target.index, Index: resolved})
	}
	if _, err := executeDocsTableRequests(ctx, svc, docID, loaded.full.RevisionId, requests); err != nil {
		return fmt.Errorf("%s table %s: %w", command.action, command.dimension, err)
	}
	return writeDocsTableMutationResult(ctx, docID, loaded.tabID, command.action, string(command.dimension), results)
}

func (c *DocsTableMergeCmd) Run(ctx context.Context, flags *RootFlags) error {
	startRow, startCol, endRow, endCol, err := parseDocsTableRange(c.Range)
	if err != nil {
		return err
	}
	return runDocsTableMergeCommand(ctx, flags, docsTableMergeCommand{
		docID: c.DocID, table: c.Table, tab: c.Tab, action: mergeOp,
		startRow: startRow, startCol: startCol, endRow: endRow, endCol: endCol,
	})
}

func (c *DocsTableUnmergeCmd) Run(ctx context.Context, flags *RootFlags) error {
	row, col, err := parseDocsTableCell(c.Cell, "--cell")
	if err != nil {
		return err
	}
	return runDocsTableMergeCommand(ctx, flags, docsTableMergeCommand{
		docID: c.DocID, table: c.Table, tab: c.Tab, action: unmergeOp,
		startRow: row, startCol: col, endRow: row, endCol: col,
	})
}

type docsTableMergeCommand struct {
	docID              string
	table              string
	tab                string
	action             string
	startRow, startCol int
	endRow, endCol     int
}

func runDocsTableMergeCommand(ctx context.Context, flags *RootFlags, command docsTableMergeCommand) error {
	docID, err := validateDocsTableMutationArgs(command.docID, command.table)
	if err != nil {
		return err
	}
	rangeValue := formatDocsTableRange(command.startRow, command.startCol, command.endRow, command.endCol)
	if command.action == unmergeOp {
		rangeValue = fmt.Sprintf("%d,%d", command.startRow, command.startCol)
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.table-"+command.action, map[string]any{
		"documentId": docID,
		"table":      command.table,
		"range":      rangeValue,
		"tab":        command.tab,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, loaded, selected, err := loadDocsSelectedTables(ctx, flags, docID, command.tab, command.table)
	if err != nil {
		return err
	}
	requests := make([]*docs.Request, 0, len(selected))
	results := make([]docsTableMutationResult, 0, len(selected))
	for _, target := range selected {
		req, buildErr := docstable.BuildMergeRequest(
			docsTablePlanTarget(target.tableWithIndex), docstable.Action(command.action),
			command.startRow, command.startCol, command.endRow, command.endCol, loaded.tabID,
		)
		if buildErr != nil {
			return usage(buildErr.Error())
		}
		requests = append(requests, req)
		results = append(results, docsTableMutationResult{TableIndex: target.index, Range: rangeValue})
	}
	if _, err := executeDocsTableRequests(ctx, svc, docID, loaded.full.RevisionId, requests); err != nil {
		return fmt.Errorf("table %s: %w", command.action, err)
	}
	return writeDocsTableMutationResult(ctx, docID, loaded.tabID, command.action, "cell", results)
}

func validateDocsTableMutationArgs(rawDocID, table string) (string, error) {
	docID := normalizeGoogleID(strings.TrimSpace(rawDocID))
	if docID == "" {
		return "", usage("empty docId")
	}
	if table == "" {
		return "", usage("empty --table")
	}
	return docID, nil
}

func loadDocsSelectedTables(
	ctx context.Context,
	flags *RootFlags,
	docID, tab, selector string,
) (*docs.Service, *docsLoadedTarget, []docsSelectedTable, error) {
	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return nil, nil, nil, err
	}
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, strings.TrimSpace(tab))
	if err != nil {
		return nil, nil, nil, err
	}
	selected, err := resolveDocsTableSelector(loaded.target, selector)
	if err != nil {
		return nil, nil, nil, err
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].startIdx > selected[j].startIdx
	})
	return svc, loaded, selected, nil
}

func resolveDocsTableSelector(doc *docs.Document, raw string) ([]docsSelectedTable, error) {
	tables := collectAllTablesWithIndex(doc)
	if len(tables) == 0 {
		return nil, usage("document has no tables")
	}
	if strings.HasPrefix(raw, "text:") {
		return resolveDocsTableHeaderText(tables, strings.TrimPrefix(raw, "text:"))
	}
	syntax := strings.TrimSpace(raw)
	if syntax == "*" {
		selected := make([]docsSelectedTable, 0, len(tables))
		for i, table := range tables {
			selected = append(selected, docsSelectedTable{tableWithIndex: table, index: i + 1})
		}
		return selected, nil
	}
	if index, err := strconv.Atoi(syntax); err == nil {
		if index == 0 {
			return nil, usage("--table index cannot be 0")
		}
		resolved := index
		if resolved < 0 {
			resolved = len(tables) + resolved + 1
		}
		if resolved < 1 || resolved > len(tables) {
			return nil, usagef("table %d out of range (document has %d tables)", index, len(tables))
		}
		return []docsSelectedTable{{tableWithIndex: tables[resolved-1], index: resolved}}, nil
	} else if docsTableSelectorLooksNumeric(syntax) {
		return nil, usagef("invalid table index %q", syntax)
	}

	return resolveDocsTableHeaderText(tables, raw)
}

func docsTableSelectorLooksNumeric(value string) bool {
	if value == "" {
		return false
	}
	if value[0] == '+' || value[0] == '-' {
		value = value[1:]
	}
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func resolveDocsTableHeaderText(tables []tableWithIndex, selector string) ([]docsSelectedTable, error) {
	var matches []docsSelectedTable
	for i, table := range tables {
		if docsTableFirstCellText(table.table) == selector {
			matches = append(matches, docsSelectedTable{tableWithIndex: table, index: i + 1})
		}
	}
	switch len(matches) {
	case 0:
		return nil, usagef("no table has first-cell text %q", selector)
	case 1:
		return matches, nil
	default:
		return nil, usagef("first-cell text %q matches %d tables; use a numeric --table index", selector, len(matches))
	}
}

func docsTableFirstCellText(table *docs.Table) string {
	if table == nil || len(table.TableRows) == 0 || len(table.TableRows[0].TableCells) == 0 {
		return ""
	}
	text, _, _ := getCellText(table.TableRows[0].TableCells[0])
	return strings.TrimSuffix(text, "\n")
}

func parseDocsTableInsertAt(raw, dimension string) (int, bool, error) {
	value := strings.TrimSpace(raw)
	if strings.EqualFold(value, "end") {
		return 0, true, nil
	}
	index, err := strconv.Atoi(value)
	if err != nil || index == 0 {
		return 0, false, usagef("--at must be a non-zero %s index or end", dimension)
	}
	return index, false, nil
}

func parseDocsTableRowValues(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var encoded []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil {
		return nil, usagef("--values-json must be a JSON string array: %v", err)
	}
	if encoded == nil {
		return nil, usage("--values-json must be a JSON string array")
	}
	values := make([]string, len(encoded))
	for i, value := range encoded {
		value = json.RawMessage(strings.TrimSpace(string(value)))
		if len(value) == 0 || value[0] != '"' {
			return nil, usagef("--values-json element %d must be a string", i)
		}
		if err := json.Unmarshal(value, &values[i]); err != nil {
			return nil, usagef("--values-json element %d must be a string: %v", i, err)
		}
	}
	return values, nil
}

func parseDocsTableRange(raw string) (int, int, int, int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, 0, 0, 0, usage("--range must use r1,c1:r2,c2")
	}
	startRow, startCol, err := parseDocsTableCell(parts[0], "--range")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	endRow, endCol, err := parseDocsTableCell(parts[1], "--range")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if endRow < startRow || endCol < startCol {
		return 0, 0, 0, 0, usage("--range end must not precede its start")
	}
	return startRow, startCol, endRow, endCol, nil
}

func parseDocsTableCell(raw, flag string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	if len(parts) != 2 {
		return 0, 0, usagef("%s must use r,c", flag)
	}
	row, rowErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	col, colErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if rowErr != nil || colErr != nil || row < 1 || col < 1 {
		return 0, 0, usagef("%s coordinates must be positive integers", flag)
	}
	return row, col, nil
}

func executeDocsTableRequests(
	ctx context.Context,
	svc *docs.Service,
	docID, revisionID string,
	requests []*docs.Request,
) (string, error) {
	_, updatedRevisionID, err := submitBatchedDocsRequestsWithRevision(
		ctx, svc, docID, requests, &docs.WriteControl{RequiredRevisionId: revisionID},
	)
	return updatedRevisionID, err
}

func populateDocsTableRow(
	ctx context.Context,
	svc *docs.Service,
	docID, tabID, expectedRevisionID string,
	tableIndex, rowIndex int,
	values []string,
) error {
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, tabID)
	if err != nil {
		return fmt.Errorf("reload inserted table row: %w", err)
	}
	if loaded.full.RevisionId != expectedRevisionID {
		return fmt.Errorf(
			"populate inserted table row: document revision changed after row insertion (expected %s, got %s)",
			expectedRevisionID, loaded.full.RevisionId,
		)
	}
	target, _, err := resolveDocsTableWithIndex(loaded.target, tableIndex)
	if err != nil {
		return err
	}
	if rowIndex < 1 || rowIndex > len(target.table.TableRows) {
		return fmt.Errorf("inserted row %d not found after mutation", rowIndex)
	}
	row := target.table.TableRows[rowIndex-1]
	if len(values) != len(row.TableCells) {
		return fmt.Errorf("inserted row has %d cells, want %d", len(row.TableCells), len(values))
	}
	requests := make([]*docs.Request, 0, len(values))
	for col := len(values) - 1; col >= 0; col-- {
		if values[col] == "" {
			continue
		}
		cell := row.TableCells[col]
		if len(cell.Content) == 0 {
			return fmt.Errorf("inserted row cell %d has no content location", col+1)
		}
		requests = append(requests, &docs.Request{InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: cell.Content[0].StartIndex, TabId: tabID},
			Text:     values[col],
		}})
	}
	if len(requests) == 0 {
		return nil
	}
	if _, err := executeDocsTableRequests(ctx, svc, docID, loaded.full.RevisionId, requests); err != nil {
		return fmt.Errorf("populate inserted table row: %w", err)
	}
	return nil
}

func writeDocsTableMutationResult(
	ctx context.Context,
	docID, tabID, action, target string,
	results []docsTableMutationResult,
) error {
	sort.Slice(results, func(i, j int) bool {
		return results[i].TableIndex < results[j].TableIndex
	})
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"action":     action,
			"target":     target,
			"updated":    true,
			"tables":     results,
		}
		if len(results) == 1 {
			payload["tableIndex"] = results[0].TableIndex
			if results[0].Index > 0 {
				payload[target] = results[0].Index
			}
			if results[0].Range != "" {
				payload["range"] = results[0].Range
			}
		}
		if tabID != "" {
			payload["tabId"] = tabID
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("action\t%s", action)
	u.Out().Linef("target\t%s", target)
	for _, result := range results {
		if result.Range != "" {
			u.Out().Linef("table\t%d\t%s\t%s", result.TableIndex, target, result.Range)
		} else {
			u.Out().Linef("table\t%d\t%s\t%d", result.TableIndex, target, result.Index)
		}
	}
	u.Out().Linef("updated\ttrue")
	if tabID != "" {
		u.Out().Linef("tabId\t%s", tabID)
	}
	return nil
}

func formatDocsTableRange(startRow, startCol, endRow, endCol int) string {
	return fmt.Sprintf("%d,%d:%d,%d", startRow, startCol, endRow, endCol)
}

func docsTablePlanTarget(target tableWithIndex) docstable.Target {
	return docstable.Target{Table: target.table, StartIndex: target.startIdx}
}
