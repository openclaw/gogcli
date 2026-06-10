package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsTableRowCmd struct {
	Insert DocsTableRowInsertCmd `cmd:"" name:"insert" aliases:"add,append" help:"Insert a native table row"`
	Delete DocsTableRowDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a native table row"`
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
		req, resolved, buildErr := buildDocsTableDimensionRequest(
			target.tableWithIndex, docsTableDimensionRow, opInsert, at, appendAtEnd, loaded.tabID,
		)
		if buildErr != nil {
			return buildErr
		}
		if hasValues && docsTableRowBoundaryCrossesMerge(target.table, resolved) {
			return usagef(
				"cannot insert row %d with --values-json because a vertically merged cell crosses that boundary",
				resolved,
			)
		}
		if hasValues && len(values) != docsTableColumnCount(target.table) {
			return usagef("--values-json has %d values, table has %d columns", len(values), docsTableColumnCount(target.table))
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
		docID: c.DocID, table: c.Table, tab: c.Tab, dimension: docsTableDimensionRow,
		action: opDelete, target: c.Row, dryRunOp: "docs.table-row.delete",
	})
}

func (c *DocsTableColumnInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	at, appendAtEnd, err := parseDocsTableInsertAt(c.At, "column")
	if err != nil {
		return err
	}
	return runDocsTableDimensionCommand(ctx, flags, docsTableDimensionCommand{
		docID: c.DocID, table: c.Table, tab: c.Tab, dimension: docsTableDimensionColumn,
		action: opInsert, target: at, appendAtEnd: appendAtEnd, dryRunOp: "docs.table-column.insert",
	})
}

func (c *DocsTableColumnDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runDocsTableDimensionCommand(ctx, flags, docsTableDimensionCommand{
		docID: c.DocID, table: c.Table, tab: c.Tab, dimension: docsTableDimensionColumn,
		action: opDelete, target: c.Col, dryRunOp: "docs.table-column.delete",
	})
}

type docsTableDimension string

const (
	docsTableDimensionRow    docsTableDimension = "row"
	docsTableDimensionColumn docsTableDimension = "column"
)

type docsTableDimensionCommand struct {
	docID       string
	table       string
	tab         string
	dimension   docsTableDimension
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
		req, resolved, buildErr := buildDocsTableDimensionRequest(
			target.tableWithIndex, command.dimension, command.action, command.target, command.appendAtEnd, loaded.tabID,
		)
		if buildErr != nil {
			return buildErr
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
		req, buildErr := buildDocsTableMergeRequest(
			target.tableWithIndex, command.action,
			command.startRow, command.startCol, command.endRow, command.endCol, loaded.tabID,
		)
		if buildErr != nil {
			return buildErr
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

func buildDocsTableDimensionRequest(
	target tableWithIndex,
	dimension docsTableDimension,
	action string,
	requested int,
	appendAtEnd bool,
	tabID string,
) (*docs.Request, int, error) {
	if target.table == nil {
		return nil, 0, usage("target table is empty")
	}
	count := len(target.table.TableRows)
	if dimension == docsTableDimensionColumn {
		count = docsTableColumnCount(target.table)
	}
	if count < 1 {
		return nil, 0, usagef("target table has no %ss", dimension)
	}

	resolved := requested
	if appendAtEnd {
		resolved = count + 1
	} else if resolved < 0 {
		resolved = count + resolved + 1
	}
	switch action {
	case opDelete:
		if resolved < 1 || resolved > count {
			return nil, 0, usagef("%s %d out of range (table has %d %ss)", dimension, requested, count, dimension)
		}
		if count == 1 {
			return nil, 0, usagef("cannot delete the only %s in a table", dimension)
		}
	case opInsert:
		if !appendAtEnd && (resolved < 1 || resolved > count) {
			return nil, 0, usagef("%s %d out of range for insert (table has %d %ss)", dimension, requested, count, dimension)
		}
	default:
		return nil, 0, usagef("unsupported table %s action %q", dimension, action)
	}

	rowIndex, columnIndex, err := docsTableDimensionReference(
		target.table, dimension, action, resolved, appendAtEnd,
	)
	if err != nil {
		return nil, 0, err
	}
	cellLocation := &docs.TableCellLocation{
		TableStartLocation: &docs.Location{Index: target.startIdx, TabId: tabID},
		RowIndex:           int64(rowIndex),
		ColumnIndex:        int64(columnIndex),
		ForceSendFields:    []string{"RowIndex", "ColumnIndex"},
	}
	if dimension == docsTableDimensionRow {
		if action == opDelete {
			return &docs.Request{DeleteTableRow: &docs.DeleteTableRowRequest{
				TableCellLocation: cellLocation,
			}}, resolved, nil
		}
		return &docs.Request{InsertTableRow: &docs.InsertTableRowRequest{
			TableCellLocation: cellLocation,
			InsertBelow:       appendAtEnd,
		}}, resolved, nil
	}

	if action == opDelete {
		return &docs.Request{DeleteTableColumn: &docs.DeleteTableColumnRequest{
			TableCellLocation: cellLocation,
		}}, resolved, nil
	}
	return &docs.Request{InsertTableColumn: &docs.InsertTableColumnRequest{
		TableCellLocation: cellLocation,
		InsertRight:       appendAtEnd,
	}}, resolved, nil
}

func docsTableDimensionReference(
	table *docs.Table,
	dimension docsTableDimension,
	action string,
	resolved int,
	appendAtEnd bool,
) (int, int, error) {
	if !docsTableHasRectangularRows(table) {
		return 0, 0, usagef(
			"cannot %s table %s on a non-rectangular table; merge/unmerge cells or normalize the table first",
			action, dimension,
		)
	}
	placements := docsTableCellPlacements(table)
	if dimension == docsTableDimensionColumn {
		for _, placement := range placements {
			switch {
			case action == opDelete && placement.columnSpan == 1 && placement.columnStart == resolved:
				return placement.rowStart - 1, resolved - 1, nil
			case action == opInsert && !appendAtEnd && placement.columnStart == resolved:
				return placement.rowStart - 1, resolved - 1, nil
			case action == opInsert && appendAtEnd && placement.columnEnd() == resolved-1:
				return placement.rowStart - 1, resolved - 2, nil
			}
		}
		if action == opDelete {
			return 0, 0, usagef("cannot delete column %d because every reference cell spanning it is merged", resolved)
		}
		return 0, 0, usagef("cannot insert at column %d because the boundary is inside merged cells", resolved)
	}

	for _, placement := range placements {
		switch {
		case action == opDelete && placement.rowSpan == 1 && placement.rowStart == resolved:
			return resolved - 1, placement.columnStart - 1, nil
		case action == opInsert && !appendAtEnd && placement.rowStart == resolved:
			return resolved - 1, placement.columnStart - 1, nil
		case action == opInsert && appendAtEnd && placement.rowEnd() == resolved-1:
			return resolved - 2, placement.columnStart - 1, nil
		}
	}
	if action == opDelete {
		return 0, 0, usagef("cannot delete row %d because every reference cell spanning it is merged", resolved)
	}
	return 0, 0, usagef("cannot insert at row %d because the boundary is inside merged cells", resolved)
}

func docsTableCellColumnSpan(cell *docs.TableCell) int {
	if cell != nil && cell.TableCellStyle != nil && cell.TableCellStyle.ColumnSpan > 0 {
		return int(cell.TableCellStyle.ColumnSpan)
	}
	return 1
}

func docsTableCellRowSpan(cell *docs.TableCell) int {
	if cell != nil && cell.TableCellStyle != nil && cell.TableCellStyle.RowSpan > 0 {
		return int(cell.TableCellStyle.RowSpan)
	}
	return 1
}

type docsTableCellPlacement struct {
	rowStart    int
	columnStart int
	rowSpan     int
	columnSpan  int
}

func (p docsTableCellPlacement) rowEnd() int {
	return p.rowStart + p.rowSpan - 1
}

func (p docsTableCellPlacement) columnEnd() int {
	return p.columnStart + p.columnSpan - 1
}

func docsTableCellPlacements(table *docs.Table) []docsTableCellPlacement {
	if table == nil {
		return nil
	}
	covered := map[[2]int]bool{}
	placements := make([]docsTableCellPlacement, 0)
	for rowIndex, row := range table.TableRows {
		for columnIndex, cell := range row.TableCells {
			position := [2]int{rowIndex + 1, columnIndex + 1}
			if covered[position] {
				continue
			}
			placement := docsTableCellPlacement{
				rowStart:    position[0],
				columnStart: position[1],
				rowSpan:     docsTableCellRowSpan(cell),
				columnSpan:  docsTableCellColumnSpan(cell),
			}
			placements = append(placements, placement)
			for coveredRow := placement.rowStart; coveredRow <= placement.rowEnd(); coveredRow++ {
				for coveredColumn := placement.columnStart; coveredColumn <= placement.columnEnd(); coveredColumn++ {
					if coveredRow == placement.rowStart && coveredColumn == placement.columnStart {
						continue
					}
					covered[[2]int{coveredRow, coveredColumn}] = true
				}
			}
		}
	}
	return placements
}

func docsTableHasRectangularRows(table *docs.Table) bool {
	columns := docsTableColumnCount(table)
	if table == nil || columns < 1 || len(table.TableRows) == 0 {
		return false
	}
	for _, row := range table.TableRows {
		if row == nil || len(row.TableCells) != columns {
			return false
		}
	}
	return true
}

func docsTableRowBoundaryCrossesMerge(table *docs.Table, row int) bool {
	for _, placement := range docsTableCellPlacements(table) {
		if placement.rowStart < row && placement.rowEnd() >= row {
			return true
		}
	}
	return false
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

func buildDocsTableMergeRequest(
	target tableWithIndex,
	action string,
	startRow, startCol, endRow, endCol int,
	tabID string,
) (*docs.Request, error) {
	if err := validateDocsTableRange(target.table, startRow, startCol, endRow, endCol); err != nil {
		return nil, err
	}
	tableRange := &docs.TableRange{
		TableCellLocation: &docs.TableCellLocation{
			TableStartLocation: &docs.Location{Index: target.startIdx, TabId: tabID},
			RowIndex:           int64(startRow - 1),
			ColumnIndex:        int64(startCol - 1),
			ForceSendFields:    []string{"RowIndex", "ColumnIndex"},
		},
		RowSpan:    int64(endRow - startRow + 1),
		ColumnSpan: int64(endCol - startCol + 1),
	}
	switch action {
	case mergeOp:
		return &docs.Request{MergeTableCells: &docs.MergeTableCellsRequest{TableRange: tableRange}}, nil
	case unmergeOp, splitOp:
		tableRange.RowSpan = 1
		tableRange.ColumnSpan = 1
		return &docs.Request{UnmergeTableCells: &docs.UnmergeTableCellsRequest{TableRange: tableRange}}, nil
	default:
		return nil, usagef("unsupported table merge action %q", action)
	}
}

func validateDocsTableRange(table *docs.Table, startRow, startCol, endRow, endCol int) error {
	rows := 0
	if table != nil {
		rows = len(table.TableRows)
	}
	if startRow < 1 || startRow > rows {
		return usagef("row %d out of range (table has %d rows)", startRow, rows)
	}
	if endRow < startRow || endRow > rows {
		return usagef("row %d out of range (table has %d rows)", endRow, rows)
	}
	columns := docsTableColumnCount(table)
	if columns == 0 {
		for _, placement := range docsTableCellPlacements(table) {
			if placement.columnEnd() > columns {
				columns = placement.columnEnd()
			}
		}
	}
	if startCol < 1 || startCol > columns {
		return usagef("col %d out of range (table has %d columns)", startCol, columns)
	}
	if endCol < startCol || endCol > columns {
		return usagef("col %d out of range (table has %d columns)", endCol, columns)
	}
	return nil
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
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
