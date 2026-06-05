package cmd

import (
	"context"
	"fmt"

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

	// Step 4: Insert text into each cell
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

			requests, insertedLen := buildTableCellRequests(cellContent, cellIdx, rowIdx == 0, tabID)
			if len(requests) == 0 {
				continue
			}

			_, err := ti.svc.Documents.BatchUpdate(ti.docID, &docs.BatchUpdateDocumentRequest{
				Requests: requests,
			}).Context(ctx).Do()
			if err != nil {
				return tableEndIndex, fmt.Errorf("insert cell text: %w", err)
			}

			// Update indices for subsequent cells (they shift by the content length)
			ti.updateIndicesAfter(cellIdx, insertedLen, cellIndices, &tableEndIndex)
		}
	}

	return tableEndIndex, nil
}

// buildTableCellRequests constructs the batch requests required to populate a
// single table cell, expanding inline markdown (**bold**, *italic*, `code`,
// [links]) into UpdateTextStyle requests on top of the inserted text. Header
// cells additionally receive a whole-cell bold style. Returns the requests and
// the UTF-16 length of the text that will be inserted so callers can keep
// running cell indices in sync. If the cell content strips to an empty string
// (e.g. content was only markers), returns (nil, 0).
func buildTableCellRequests(cellContent string, cellIdx int64, isHeaderRow bool, tabID string) ([]*docs.Request, int64) {
	styles, stripped := ParseInlineFormatting(cellContent)
	if stripped == "" {
		return nil, 0
	}

	insertedLen := utf16Len(stripped)
	requests := []*docs.Request{{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: cellIdx, TabId: tabID},
			Text:     stripped,
		},
	}}

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

	for _, style := range styles {
		if req := buildTextStyleRequest(style, cellIdx, tabID); req != nil {
			requests = append(requests, req)
		}
	}

	return requests, insertedLen
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

// updateIndicesAfter updates cell indices after text insertion
func (ti *TableInserter) updateIndicesAfter(afterIndex, length int64, cellIndices [][]int64, tableEndIndex *int64) {
	for i, row := range cellIndices {
		for j, idx := range row {
			if idx > afterIndex {
				cellIndices[i][j] = idx + length
			}
		}
	}
	if *tableEndIndex > afterIndex {
		*tableEndIndex += length
	}
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

// predictedTableLen returns the UTF-16 index span an empty Docs table with
// the given rows × cols geometry occupies after an InsertTableRequest lands
// at a known location. It mirrors the structural element layout the Docs API
// produces for a freshly-inserted empty table:
//
//   - One leading char before the first row
//   - For each cell: 2 chars (cell separator + the cell's empty paragraph "\n")
//
// Total span = 1 + 2 * rows * cols. This matches the indices returned by the
// existing `getTableCellIndices` server-readback path for the supported
// markdown-table sizes; see #699 for the wire-call collapse that depends on
// this prediction. If Docs ever changes its empty-table layout, this constant
// is the load-bearing piece that will need to follow.
func predictedTableLen(rows, cols int64) int64 {
	return 1 + 2*rows*cols
}

// predictedTableCellIndex returns the StartIndex of cell (r, c)'s first
// paragraph for an empty Docs table whose Table.StartIndex == tableStart.
// Derivation matches predictedTableLen — for each row we step over 2*cols
// chars (cell separator + cell-paragraph) and each cell within a row is 2
// chars; the leading +1 accounts for the row marker that sits before cell
// (r, 0).
func predictedTableCellIndex(tableStart, r, c, cols int64) int64 {
	return tableStart + 1 + r*(2*cols) + 2*c
}

// BuildNativeTableRequests gathers the Docs API requests needed to insert a
// populated table at tableStartIndex without performing any network round
// trips. Returns:
//
//   - requests: the InsertTableRequest followed by per-cell InsertText +
//     formatting requests, in the order they must land in a single
//     batchUpdate. The Docs API applies requests sequentially and shifts
//     subsequent indices automatically, so the per-cell index values are
//     computed against the empty-table layout (predictedTableCellIndex) and
//     remain valid for the whole batch.
//   - tableEndIndex: the predicted end of the table after both the structure
//     and the cell text have been inserted; callers chain this through
//     nextTableInsertOffset for downstream placeholder shifts.
//
// This is the batchable analogue of InsertNativeTable: it replaces the
// historical 1 InsertTable + 1 Documents.Get + N cell-text batchUpdate calls
// per table (see #699) with a single contribution to the caller's combined
// batchUpdate.
func BuildNativeTableRequests(tableStartIndex int64, cells [][]string, tabID string) ([]*docs.Request, int64) {
	if len(cells) == 0 || len(cells[0]) == 0 {
		return nil, tableStartIndex
	}
	rows := int64(len(cells))
	cols := int64(len(cells[0]))

	requests := make([]*docs.Request, 0, 1+int(rows*cols))
	requests = append(requests, &docs.Request{
		InsertTable: &docs.InsertTableRequest{
			Rows:    rows,
			Columns: cols,
			Location: &docs.Location{
				Index: tableStartIndex,
				TabId: tabID,
			},
		},
	})

	// Cells in the same row share an index basis (predicted from the empty
	// table layout). The Docs API automatically shifts subsequent indices
	// within the same batch, so we emit cells in row-major order and let the
	// API renumber.
	//
	// Iterate up to the row's own length rather than cols — the markdown
	// converter can produce ragged rows (row n has fewer cells than the
	// header), and the resulting Docs table has empty cells for the missing
	// columns. The predicted index still uses cols (the table is structurally
	// uniform on the Docs side); we just skip emitting InsertText for the
	// missing cells.
	for r := int64(0); r < rows; r++ {
		rowLen := int64(len(cells[r]))
		for c := int64(0); c < rowLen; c++ {
			content := cells[r][c]
			if content == "" {
				continue
			}
			cellIdx := predictedTableCellIndex(tableStartIndex, r, c, cols)
			cellReqs, _ := buildTableCellRequests(content, cellIdx, r == 0, tabID)
			requests = append(requests, cellReqs...)
		}
	}

	tableEndIndex := tableStartIndex + predictedTableLen(rows, cols)
	return requests, tableEndIndex
}
