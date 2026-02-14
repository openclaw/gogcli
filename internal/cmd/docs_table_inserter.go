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
func (ti *TableInserter) InsertNativeTable(ctx context.Context, tableIndex int64, cells [][]string) (int64, error) {
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
	doc, err := ti.svc.Documents.Get(ti.docID).Context(ctx).Do()
	if err != nil {
		return tableIndex, fmt.Errorf("get document after table insert: %w", err)
	}

	// Step 3: Find the table in the document and get cell indices
	cellIndices, tableEndIndex, err := ti.getTableCellIndices(doc, tableIndex, rows, cols)
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

			// Insert text into cell
			insertTextReq := &docs.Request{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{
						Index: cellIdx,
					},
					Text: cellContent,
				},
			}

			// Make text bold if it's a header row
			var boldReq *docs.Request
			if rowIdx == 0 {
				boldReq = &docs.Request{
					UpdateTextStyle: &docs.UpdateTextStyleRequest{
						Range: &docs.Range{
							StartIndex: cellIdx,
							EndIndex:   cellIdx + utf16Len(cellContent),
						},
						TextStyle: &docs.TextStyle{
							Bold: true,
						},
						Fields: "bold",
					},
				}
			}

			requests := []*docs.Request{insertTextReq}
			if boldReq != nil {
				requests = append(requests, boldReq)
			}

			_, err := ti.svc.Documents.BatchUpdate(ti.docID, &docs.BatchUpdateDocumentRequest{
				Requests: requests,
			}).Context(ctx).Do()
			if err != nil {
				return tableEndIndex, fmt.Errorf("insert cell text: %w", err)
			}

			// Update indices for subsequent cells (they shift by the content length)
			ti.updateIndicesAfter(cellIdx, utf16Len(cellContent), cellIndices, &tableEndIndex)
		}
	}

	return tableEndIndex, nil
}

// getTableCellIndices extracts the start index for each cell in a table
func (ti *TableInserter) getTableCellIndices(doc *docs.Document, tableStartIndex int64, rows, cols int64) ([][]int64, int64, error) {
	cellIndices := make([][]int64, rows)
	for i := range cellIndices {
		cellIndices[i] = make([]int64, cols)
	}

	var tableEndIndex int64

	// Find the table in the document
	if doc.Body == nil {
		return cellIndices, tableEndIndex, fmt.Errorf("document body is nil")
	}

	// Look for table element starting near tableStartIndex
	for _, element := range doc.Body.Content {
		if element.Table != nil {
			// Check if this is our table (starts near the expected index)
			if element.StartIndex >= tableStartIndex-2 && element.StartIndex <= tableStartIndex+2 {
				tableEndIndex = element.EndIndex

				// Extract cell indices from table
				for rowIdx, row := range element.Table.TableRows {
					if rowIdx >= int(rows) {
						break
					}
					for colIdx, cell := range row.TableCells {
						if colIdx >= int(cols) {
							break
						}
						// Cell content starts at StartIndex + 1 (after the cell start marker)
						if len(cell.Content) > 0 {
							cellIndices[rowIdx][colIdx] = cell.Content[0].StartIndex
						}
					}
				}
				break
			}
		}
	}

	if tableEndIndex == 0 {
		return cellIndices, tableEndIndex, fmt.Errorf("table not found near index %d", tableStartIndex)
	}

	return cellIndices, tableEndIndex, nil
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
