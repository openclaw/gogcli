package cmd

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/api/docs/v1"
)

// TableInserter handles multi-step table insertion for native Google Docs tables
type TableInserter struct {
	svc   *docs.Service
	docID string
}

func NewTableInserter(svc *docs.Service, docID string) *TableInserter {
	return &TableInserter{
		svc:   svc,
		docID: docID,
	}
}

// InsertNativeTable inserts a native Google Docs table and populates it with content
// Returns the end index of the table after insertion
func (ti *TableInserter) InsertNativeTable(ctx context.Context, tableIndex int64, cells [][]string, tabID string) (int64, error) {
	if len(cells) == 0 || len(cells[0]) == 0 {
		return tableIndex, nil
	}

	rows := int64(len(cells))
	cols := int64(len(cells[0]))

	// Step 1: Insert the table structure
	insertTableReq := &docs.Request{
		InsertTable: &docs.InsertTableRequest{
			Rows:    rows,
			Columns: cols,
			Location: &docs.Location{
				Index: tableIndex,
				TabId: tabID,
			},
		},
	}

	_, err := ti.svc.Documents.BatchUpdate(ti.docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{insertTableReq},
	}).Context(ctx).Do()
	if err != nil {
		return tableIndex, fmt.Errorf("insert table: %w", err)
	}

	// Step 2: Fetch the document to get cell indices
	getCall := ti.svc.Documents.Get(ti.docID).Context(ctx)
	if tabID != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Do()
	if err != nil {
		return tableIndex, fmt.Errorf("get document after table insert: %w", err)
	}
	targetDoc := doc
	if tabID != "" {
		tab, tabErr := findTab(flattenTabs(doc.Tabs), tabID)
		if tabErr != nil {
			return tableIndex, tabErr
		}
		if tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
			return tableIndex, fmt.Errorf("tab has no document body: %s", tabID)
		}
		targetDoc = &docs.Document{Body: tab.DocumentTab.Body}
	}

	// Step 3: Find the table in the document and get cell indices
	cellIndices, tableEndIndex, err := ti.getTableCellIndices(targetDoc, tableIndex, rows, cols)
	if err != nil {
		return tableEndIndex, err
	}

	// Step 4: Collect all per-cell requests grouped by cell, sort descending by
	// cellIdx, submit as capped batchUpdate chunks. Reverse-document-order
	// processing is the canonical pattern for "insert at many positions without
	// manual offset bookkeeping" — earlier (lower-index) cell positions remain
	// valid even as the API processes later (higher-index) inserts first. See
	// #699 for the wire-call collapse this enables (was: 1 batchUpdate per cell;
	// now: usually 1 capped cell-content batch for the whole table).
	//
	// Each cell's request group keeps its InsertText-then-style ordering so
	// the style ranges resolve against the just-inserted text.
	type cellGroup struct {
		cellIdx     int64
		insertedLen int64
		requests    []*docs.Request
	}
	groups := make([]cellGroup, 0, int(rows*cols))
	for rowIdx := 0; rowIdx < len(cells); rowIdx++ {
		for colIdx := 0; colIdx < len(cells[rowIdx]); colIdx++ {
			cellContent := cells[rowIdx][colIdx]
			if cellContent == "" {
				continue
			}
			cellIdx := cellIndices[rowIdx][colIdx]
			if cellIdx == 0 {
				continue
			}
			requests, insertedLen, buildErr := buildTableCellRequests(cellContent, cellIdx, rowIdx == 0, tabID)
			if buildErr != nil {
				return tableEndIndex, buildErr
			}
			if len(requests) == 0 {
				continue
			}
			groups = append(groups, cellGroup{cellIdx: cellIdx, insertedLen: insertedLen, requests: requests})
		}
	}
	if len(groups) > 0 {
		// Sort descending so higher-index cells are processed first; lower-
		// index cells remain at their original positions.
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].cellIdx > groups[j].cellIdx
		})
		allReqs := make([]*docs.Request, 0, len(groups)*3)
		for _, g := range groups {
			allReqs = append(allReqs, g.requests...)
		}
		_, err := submitBatchedDocsRequests(ctx, ti.svc, ti.docID, allReqs, nil)
		if err != nil {
			return tableEndIndex, fmt.Errorf("insert cell content: %w", err)
		}
		// tableEndIndex grows by the total content inserted across all cells.
		for _, g := range groups {
			tableEndIndex += g.insertedLen
		}
	}

	return tableEndIndex, nil
}

// buildTableCellRequests constructs the batch requests required to populate a
// single table cell, including block Markdown such as native nested lists.
// Header cells additionally receive a whole-cell bold style. insertedLen is
// the final UTF-16 growth after CreateParagraphBullets consumes nesting tabs.
func buildTableCellRequests(cellContent string, cellIdx int64, isHeaderRow bool, tabID string) ([]*docs.Request, int64, error) {
	formatRequests, text, insertedLen, err := buildMarkdownCellContent(cellContent, cellIdx, tabID)
	if err != nil {
		return nil, 0, err
	}
	if text == "" {
		return nil, 0, nil
	}

	requests := []*docs.Request{{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: cellIdx, TabId: tabID},
			Text:     text,
		},
	}}
	requests = append(requests, formatRequests...)

	if isHeaderRow {
		requests = append(requests, &docs.Request{
			UpdateTextStyle: &docs.UpdateTextStyleRequest{
				Range: &docs.Range{
					StartIndex: cellIdx,
					EndIndex:   cellIdx + insertedLen,
					TabId:      tabID,
				},
				TextStyle: &docs.TextStyle{Bold: true},
				Fields:    "bold",
			},
		})
	}

	return requests, insertedLen, nil
}

// getTableCellIndices extracts the start index for each cell in the table that
// was just inserted near tableStartIndex.
//
// Locating the freshly-inserted table is harder than it looks. The Docs API
// guarantees that InsertTableRequest inserts a newline before the table, and
// our markdown-append path additionally pre-inserts a placeholder "\n" at
// tableStartIndex so the table lands cleanly between structural elements.
// Depending on the surrounding doc state the table's reported StartIndex can
// therefore be tableStartIndex, tableStartIndex+1, or a few code units beyond
// — observed real-world drift exceeds the ±2 window the original
// implementation used, producing
// `insert native table: table not found near index N` on append (#592).
//
// Strategy: find the Table element whose StartIndex is closest to
// tableStartIndex among all tables in the body, with a small tolerance for
// any backward shift. InsertTable can only push existing content forward, so
// the freshly-inserted table's StartIndex is always >= tableStartIndex minus
// at most a small constant; we prefer the nearest match.
func (ti *TableInserter) getTableCellIndices(doc *docs.Document, tableStartIndex int64, rows, cols int64) ([][]int64, int64, error) {
	cellIndices := make([][]int64, rows)
	for i := range cellIndices {
		cellIndices[i] = make([]int64, cols)
	}

	var tableEndIndex int64

	if doc.Body == nil {
		return cellIndices, tableEndIndex, fmt.Errorf("document body is nil")
	}

	matched := pickTableNear(doc.Body.Content, tableStartIndex, rows, cols)
	if matched == nil {
		return cellIndices, tableEndIndex, fmt.Errorf("table not found near index %d", tableStartIndex)
	}

	tableEndIndex = matched.EndIndex
	for rowIdx, row := range matched.Table.TableRows {
		if rowIdx >= int(rows) {
			break
		}
		for colIdx, cell := range row.TableCells {
			if colIdx >= int(cols) {
				break
			}
			// Cell content starts at the cell's first paragraph StartIndex.
			if len(cell.Content) > 0 {
				cellIndices[rowIdx][colIdx] = cell.Content[0].StartIndex
			}
		}
	}

	return cellIndices, tableEndIndex, nil
}

// pickTableNear returns the structural element in content that is most likely
// the table we just asked the Docs API to insert near tableStartIndex. It
// prefers Table elements at or after tableStartIndex (since InsertTable can
// only shift existing content forward), but allows a small backward tolerance
// to absorb any minor index quirks. Among candidates it picks the closest
// StartIndex, which uniquely identifies the freshly-inserted table even if
// the document already contains other tables.
func pickTableNear(content []*docs.StructuralElement, tableStartIndex, rows, cols int64) *docs.StructuralElement {
	// Backward tolerance: 2 keeps us robust against the original ±2 search
	// while still ruling out tables that live far above the insertion point.
	const backwardTolerance int64 = 2

	var best *docs.StructuralElement
	var bestDist int64
	for _, element := range content {
		if element == nil || element.Table == nil {
			continue
		}
		if element.Table.Rows != rows || element.Table.Columns != cols {
			continue
		}
		if element.StartIndex < tableStartIndex-backwardTolerance {
			continue
		}
		dist := element.StartIndex - tableStartIndex
		if dist < 0 {
			dist = -dist
		}
		if best == nil || dist < bestDist {
			best = element
			bestDist = dist
		}
	}
	return best
}

// nextTableInsertOffset returns the running offset to apply to subsequent
// markdown-table placeholder positions after inserting a native table that
// spans [tableIndex, tableEnd). InsertTable inserts the new table before the
// existing character at tableIndex, so the placeholder "\n" we wrote into
// plainText for that table position stays in the doc; every subsequent
// placeholder therefore shifts forward by (tableEnd - tableIndex). The
// previous formula subtracted an extra 1, which accumulated one missing
// character of drift per table; see #607.
func nextTableInsertOffset(currentOffset, tableIndex, tableEnd int64) int64 {
	if tableEnd <= tableIndex {
		return currentOffset
	}
	return currentOffset + (tableEnd - tableIndex)
}
