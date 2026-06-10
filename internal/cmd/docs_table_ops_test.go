package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestResolveDocsTableSelector(t *testing.T) {
	doc := docsTableOpsTestDocument(
		docsTableOpsTestElement(5, "First", 2, 2),
		docsTableOpsTestElement(40, "Second", 3, 2),
	)

	tests := []struct {
		name      string
		selector  string
		wantIndex []int
	}{
		{name: "positive", selector: "1", wantIndex: []int{1}},
		{name: "negative", selector: "-1", wantIndex: []int{2}},
		{name: "header", selector: "Second", wantIndex: []int{2}},
		{name: "all", selector: "*", wantIndex: []int{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, err := resolveDocsTableSelector(doc, tt.selector)
			if err != nil {
				t.Fatalf("resolveDocsTableSelector: %v", err)
			}
			if len(selected) != len(tt.wantIndex) {
				t.Fatalf("selected = %d, want %d", len(selected), len(tt.wantIndex))
			}
			for i, want := range tt.wantIndex {
				if selected[i].index != want {
					t.Fatalf("selected[%d].index = %d, want %d", i, selected[i].index, want)
				}
			}
		})
	}
}

func TestResolveDocsTableSelectorRejectsAmbiguousHeader(t *testing.T) {
	doc := docsTableOpsTestDocument(
		docsTableOpsTestElement(5, "Same", 2, 2),
		docsTableOpsTestElement(40, "Same", 2, 2),
	)
	_, err := resolveDocsTableSelector(doc, "Same")
	if err == nil || !strings.Contains(err.Error(), "matches 2 tables") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestResolveDocsTableSelectorPreservesHeaderWhitespace(t *testing.T) {
	doc := docsTableOpsTestDocument(docsTableOpsTestElement(5, " Status ", 2, 2))
	selected, err := resolveDocsTableSelector(doc, " Status ")
	if err != nil {
		t.Fatalf("resolve exact whitespace: %v", err)
	}
	if len(selected) != 1 || selected[0].index != 1 {
		t.Fatalf("selected = %#v", selected)
	}
	if _, err := resolveDocsTableSelector(doc, "Status"); err == nil {
		t.Fatal("expected trimmed selector not to match")
	}
}

func TestResolveDocsTableSelectorSupportsNumericHeaderText(t *testing.T) {
	doc := docsTableOpsTestDocument(
		docsTableOpsTestElement(5, "First", 2, 2),
		docsTableOpsTestElement(40, "2026", 2, 2),
	)
	selected, err := resolveDocsTableSelector(doc, "text:2026")
	if err != nil {
		t.Fatalf("resolve numeric header: %v", err)
	}
	if len(selected) != 1 || selected[0].index != 2 {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestResolveDocsTableSelectorRejectsNumericOverflow(t *testing.T) {
	const overflow = "999999999999999999999999999999"
	doc := docsTableOpsTestDocument(docsTableOpsTestElement(5, overflow, 2, 2))
	_, err := resolveDocsTableSelector(doc, overflow)
	if err == nil || !strings.Contains(err.Error(), "invalid table index") {
		t.Fatalf("expected numeric overflow error, got %v", err)
	}
}

func TestBuildDocsTableDimensionRequest(t *testing.T) {
	target := tableWithIndex{
		table:    docsTableOpsTestElement(5, "Header", 3, 2).Table,
		startIdx: 5,
	}

	rowReq, rowIndex, err := buildDocsTableDimensionRequest(
		target, docsTableDimensionRow, opInsert, 0, true, "tab-1",
	)
	if err != nil {
		t.Fatalf("append row: %v", err)
	}
	if rowIndex != 4 {
		t.Fatalf("row index = %d, want 4", rowIndex)
	}
	row := rowReq.InsertTableRow
	if row == nil || !row.InsertBelow {
		t.Fatalf("unexpected row request: %#v", rowReq)
	}
	if got := row.TableCellLocation; got.RowIndex != 2 || got.ColumnIndex != 0 ||
		got.TableStartLocation.Index != 5 || got.TableStartLocation.TabId != "tab-1" {
		t.Fatalf("row location = %#v", got)
	}

	colReq, colIndex, err := buildDocsTableDimensionRequest(
		target, docsTableDimensionColumn, opDelete, -1, false, "",
	)
	if err != nil {
		t.Fatalf("delete column: %v", err)
	}
	if colIndex != 2 {
		t.Fatalf("column index = %d, want 2", colIndex)
	}
	col := colReq.DeleteTableColumn
	if col == nil || col.TableCellLocation.ColumnIndex != 1 {
		t.Fatalf("unexpected column request: %#v", colReq)
	}
}

func TestBuildDocsTableDimensionRequestAvoidsMergedDeleteAnchor(t *testing.T) {
	table := docsTableOpsTestElement(5, "Header", 2, 3).Table
	table.TableRows[0].TableCells[0].TableCellStyle = &docs.TableCellStyle{ColumnSpan: 2}
	target := tableWithIndex{table: table, startIdx: 5}

	req, _, err := buildDocsTableDimensionRequest(
		target, docsTableDimensionColumn, opDelete, 1, false, "",
	)
	if err != nil {
		t.Fatalf("delete with safe lower-row anchor: %v", err)
	}
	if got := req.DeleteTableColumn.TableCellLocation.RowIndex; got != 1 {
		t.Fatalf("row index = %d, want safe unmerged row 1", got)
	}
}

func TestBuildDocsTableDimensionRequestRejectsMergedDeleteAndInsertBoundary(t *testing.T) {
	table := docsTableOpsTestElement(5, "Header", 2, 3).Table
	for _, row := range table.TableRows {
		row.TableCells[0].TableCellStyle = &docs.TableCellStyle{ColumnSpan: 2}
	}
	target := tableWithIndex{table: table, startIdx: 5}

	if _, _, err := buildDocsTableDimensionRequest(
		target, docsTableDimensionColumn, opDelete, 1, false, "",
	); err == nil || !strings.Contains(err.Error(), "every reference cell") {
		t.Fatalf("expected merged delete rejection, got %v", err)
	}
	if _, _, err := buildDocsTableDimensionRequest(
		target, docsTableDimensionColumn, opInsert, 2, false, "",
	); err == nil || !strings.Contains(err.Error(), "inside merged cells") {
		t.Fatalf("expected merged insert-boundary rejection, got %v", err)
	}
}

func TestBuildDocsTableDimensionRequestRejectsVerticallyMergedRow(t *testing.T) {
	table := docsTableOpsTestElement(5, "Header", 3, 2).Table
	for _, cell := range table.TableRows[0].TableCells {
		cell.TableCellStyle = &docs.TableCellStyle{RowSpan: 2}
	}
	target := tableWithIndex{table: table, startIdx: 5}

	if _, _, err := buildDocsTableDimensionRequest(
		target, docsTableDimensionRow, opDelete, 2, false, "",
	); err == nil || !strings.Contains(err.Error(), "every reference cell") {
		t.Fatalf("expected merged row delete rejection, got %v", err)
	}
	if _, _, err := buildDocsTableDimensionRequest(
		target, docsTableDimensionRow, opInsert, 2, false, "",
	); err == nil || !strings.Contains(err.Error(), "inside merged cells") {
		t.Fatalf("expected merged row insert rejection, got %v", err)
	}
}

func TestDocsTableCellPlacementsSupportRetainedCoveredCells(t *testing.T) {
	retained := docsTableOpsTestElement(5, "Header", 1, 3).Table
	retained.TableRows[0].TableCells[0].TableCellStyle = &docs.TableCellStyle{ColumnSpan: 2}
	retainedPlacements := docsTableCellPlacements(retained)
	if len(retainedPlacements) != 2 ||
		retainedPlacements[0].columnStart != 1 ||
		retainedPlacements[1].columnStart != 3 {
		t.Fatalf("retained placements = %#v", retainedPlacements)
	}
}

func TestBuildDocsTableDimensionRequestRejectsNonRectangularRows(t *testing.T) {
	omitted := docsTableOpsTestElement(5, "Header", 1, 3).Table
	omitted.TableRows[0].TableCells[0].TableCellStyle = &docs.TableCellStyle{ColumnSpan: 2}
	omitted.TableRows[0].TableCells = []*docs.TableCell{
		omitted.TableRows[0].TableCells[0],
		omitted.TableRows[0].TableCells[2],
	}
	target := tableWithIndex{table: omitted, startIdx: 5}
	if _, _, err := buildDocsTableDimensionRequest(
		target, docsTableDimensionColumn, opDelete, 3, false, "",
	); err == nil || !strings.Contains(err.Error(), "non-rectangular") {
		t.Fatalf("expected non-rectangular rejection, got %v", err)
	}
}

func TestBuildDocsTableDimensionRequestPreflightsOnlyDimension(t *testing.T) {
	rowTarget := tableWithIndex{table: docsTableOpsTestElement(5, "Header", 1, 2).Table, startIdx: 5}
	if _, _, err := buildDocsTableDimensionRequest(
		rowTarget, docsTableDimensionRow, opDelete, 1, false, "",
	); err == nil || !strings.Contains(err.Error(), "only row") {
		t.Fatalf("expected only-row error, got %v", err)
	}

	colTarget := tableWithIndex{table: docsTableOpsTestElement(5, "Header", 2, 1).Table, startIdx: 5}
	if _, _, err := buildDocsTableDimensionRequest(
		colTarget, docsTableDimensionColumn, opDelete, 1, false, "",
	); err == nil || !strings.Contains(err.Error(), "only column") {
		t.Fatalf("expected only-column error, got %v", err)
	}
}

func TestBuildDocsTableMergeRequestValidatesLogicalColumnCount(t *testing.T) {
	element := docsTableOpsTestElement(5, "Header", 2, 2)
	element.Table.Columns = 1
	target := tableWithIndex{table: element.Table, startIdx: 5}

	_, err := buildDocsTableMergeRequest(target, mergeOp, 1, 1, 2, 2, "")
	if err == nil || !strings.Contains(err.Error(), "table has 1 columns") {
		t.Fatalf("expected logical-column validation error, got %v", err)
	}
}

func TestValidateDocsTableRangeCountsMergedCellSpans(t *testing.T) {
	table := docsTableOpsTestElement(5, "Header", 2, 3).Table
	table.TableRows[1].TableCells[0].TableCellStyle = &docs.TableCellStyle{ColumnSpan: 2}

	if err := validateDocsTableRange(table, 2, 3, 2, 3); err != nil {
		t.Fatalf("validate logical third column: %v", err)
	}
}

func TestDocsTableColumnDeleteAllUsesDescendingDocumentOrder(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	doc := docsTableOpsTestDocument(
		docsTableOpsTestElement(5, "First", 2, 2),
		docsTableOpsTestElement(40, "Second", 2, 2),
	)
	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(doc)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			_ = json.NewEncoder(w).Encode(&docs.BatchUpdateDocumentResponse{DocumentId: "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	cmd := &DocsTableColumnDeleteCmd{}
	err := runKong(t, cmd, []string{"doc1", "--table", "*", "--col=-1"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.WriteControl == nil || got.WriteControl.RequiredRevisionId != "rev-1" {
		t.Fatalf("write control = %#v", got.WriteControl)
	}
	if len(got.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(got.Requests))
	}
	if first := got.Requests[0].DeleteTableColumn.TableCellLocation.TableStartLocation.Index; first != 40 {
		t.Fatalf("first table start = %d, want 40", first)
	}
	if second := got.Requests[1].DeleteTableColumn.TableCellLocation.TableStartLocation.Index; second != 5 {
		t.Fatalf("second table start = %d, want 5", second)
	}
}

func TestDocsTableRowInsertPopulatesValues(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	before := docsTableOpsTestDocument(docsTableOpsTestElement(5, "Header", 2, 2))
	after := docsTableOpsTestDocument(docsTableOpsTestElement(5, "Header", 3, 2))
	after.RevisionId = "rev-2"
	var batches []docs.BatchUpdateDocumentRequest
	gets := 0
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			gets++
			if gets == 1 {
				_ = json.NewEncoder(w).Encode(before)
			} else {
				_ = json.NewEncoder(w).Encode(after)
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var batch docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			batches = append(batches, batch)
			revisionID := "rev-2"
			if len(batches) > 1 {
				revisionID = "rev-3"
			}
			_ = json.NewEncoder(w).Encode(&docs.BatchUpdateDocumentResponse{
				DocumentId:   "doc1",
				WriteControl: &docs.WriteControl{RequiredRevisionId: revisionID},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	cmd := &DocsTableRowInsertCmd{}
	err := runKong(t, cmd, []string{
		"doc1", "--table", "*", "--at", "end", "--values-json", `["left","right"]`,
	}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want 2", len(batches))
	}
	if batches[0].Requests[0].InsertTableRow == nil {
		t.Fatalf("first batch = %#v", batches[0].Requests)
	}
	if len(batches[1].Requests) != 2 {
		t.Fatalf("fill requests = %d, want 2", len(batches[1].Requests))
	}
	if batches[1].Requests[0].InsertText.Text != "right" || batches[1].Requests[1].InsertText.Text != "left" {
		t.Fatalf("fill request order = %#v", batches[1].Requests)
	}
	if batches[1].WriteControl == nil || batches[1].WriteControl.RequiredRevisionId != "rev-2" {
		t.Fatalf("fill write control = %#v", batches[1].WriteControl)
	}
}

func TestDocsTableRowInsertValuesRejectsMultipleSelectedTables(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	doc := docsTableOpsTestDocument(
		docsTableOpsTestElement(5, "First", 2, 2),
		docsTableOpsTestElement(40, "Second", 2, 2),
	)
	postCount := 0
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		if r.Method == http.MethodPost {
			postCount++
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	cmd := &DocsTableRowInsertCmd{}
	err := runKong(t, cmd, []string{
		"doc1", "--table", "*", "--at", "end", "--values-json", `["left","right"]`,
	}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "exactly one selected table") {
		t.Fatalf("expected selection-count error, got %v", err)
	}
	if postCount != 0 {
		t.Fatalf("post count = %d, want 0", postCount)
	}
}

func TestDocsTableRowInsertValuesRejectsVerticalMergeBoundary(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	doc := docsTableOpsTestDocument(docsTableOpsTestElement(5, "Header", 3, 2))
	doc.Body.Content[0].Table.TableRows[0].TableCells[0].TableCellStyle = &docs.TableCellStyle{RowSpan: 2}
	postCount := 0
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		if r.Method == http.MethodPost {
			postCount++
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	cmd := &DocsTableRowInsertCmd{}
	err := runKong(t, cmd, []string{
		"doc1", "--table", "1", "--at", "2", "--values-json", `["left","right"]`,
	}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "vertically merged cell") {
		t.Fatalf("expected vertical merge boundary error, got %v", err)
	}
	if postCount != 0 {
		t.Fatalf("post count = %d, want 0", postCount)
	}
}

func TestDocsTableRowInsertValuesRejectsConcurrentRevision(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	before := docsTableOpsTestDocument(docsTableOpsTestElement(5, "Header", 2, 2))
	after := docsTableOpsTestDocument(docsTableOpsTestElement(5, "Header", 3, 2))
	after.RevisionId = "rev-collaborator"
	batchCount := 0
	gets := 0
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			gets++
			if gets == 1 {
				_ = json.NewEncoder(w).Encode(before)
			} else {
				_ = json.NewEncoder(w).Encode(after)
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			batchCount++
			_ = json.NewEncoder(w).Encode(&docs.BatchUpdateDocumentResponse{
				DocumentId:   "doc1",
				WriteControl: &docs.WriteControl{RequiredRevisionId: "rev-2"},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	cmd := &DocsTableRowInsertCmd{}
	err := runKong(t, cmd, []string{
		"doc1", "--table", "1", "--at", "end", "--values-json", `["left","right"]`,
	}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "document revision changed") {
		t.Fatalf("expected concurrent revision error, got %v", err)
	}
	if batchCount != 1 {
		t.Fatalf("batch count = %d, want structural batch only", batchCount)
	}
}

func TestParseDocsTableRowValuesRejectsNull(t *testing.T) {
	if _, err := parseDocsTableRowValues("null"); err == nil {
		t.Fatal("expected null rejection")
	}
	if _, err := parseDocsTableRowValues(`["left",null]`); err == nil || !strings.Contains(err.Error(), "element 1") {
		t.Fatalf("expected null element rejection, got %v", err)
	}
	if _, err := parseDocsTableRowValues(`["left",2]`); err == nil || !strings.Contains(err.Error(), "element 1") {
		t.Fatalf("expected numeric element rejection, got %v", err)
	}
}

func TestExecuteDocsTableRequestsSplitsAtAPICap(t *testing.T) {
	var batches []docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var batch docs.BatchUpdateDocumentRequest
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		batches = append(batches, batch)
		revisionID := "rev-2"
		if len(batches) > 1 {
			revisionID = "rev-3"
		}
		_ = json.NewEncoder(w).Encode(&docs.BatchUpdateDocumentResponse{
			DocumentId:   "doc1",
			WriteControl: &docs.WriteControl{RequiredRevisionId: revisionID},
		})
	})
	defer cleanup()

	requests := make([]*docs.Request, docsBatchUpdateRequestCap+1)
	for i := range requests {
		requests[i] = &docs.Request{InsertTableRow: &docs.InsertTableRowRequest{}}
	}
	if _, err := executeDocsTableRequests(context.Background(), docSvc, "doc1", "rev-1", requests); err != nil {
		t.Fatalf("executeDocsTableRequests: %v", err)
	}
	if len(batches) != 2 || len(batches[0].Requests) != docsBatchUpdateRequestCap || len(batches[1].Requests) != 1 {
		t.Fatalf("batch sizes = %d/%d/%d", len(batches), len(batches[0].Requests), len(batches[1].Requests))
	}
	if batches[0].WriteControl == nil || batches[0].WriteControl.RequiredRevisionId != "rev-1" {
		t.Fatalf("first write control = %#v", batches[0].WriteControl)
	}
	if batches[1].WriteControl == nil || batches[1].WriteControl.RequiredRevisionId != "rev-2" {
		t.Fatalf("second write control = %#v, want rev-2", batches[1].WriteControl)
	}
}

func docsTableOpsTestDocument(elements ...*docs.StructuralElement) *docs.Document {
	return &docs.Document{
		DocumentId: "doc1",
		RevisionId: "rev-1",
		Body:       &docs.Body{Content: elements},
	}
}

func docsTableOpsTestElement(start int64, header string, rows, cols int) *docs.StructuralElement {
	next := start + 1
	tableRows := make([]*docs.TableRow, rows)
	for row := 0; row < rows; row++ {
		cells := make([]*docs.TableCell, cols)
		for col := 0; col < cols; col++ {
			text := ""
			if row == 0 && col == 0 {
				text = header
			}
			cellStart := next
			cellEnd := cellStart + int64(len(text)) + 1
			cells[col] = &docs.TableCell{Content: []*docs.StructuralElement{{
				StartIndex: cellStart,
				EndIndex:   cellEnd,
				Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
					StartIndex: cellStart,
					EndIndex:   cellEnd,
					TextRun:    &docs.TextRun{Content: text + "\n"},
				}}},
			}}}
			next = cellEnd
		}
		tableRows[row] = &docs.TableRow{TableCells: cells}
	}
	return &docs.StructuralElement{
		StartIndex: start,
		EndIndex:   next,
		Table: &docs.Table{
			Rows:      int64(rows),
			Columns:   int64(cols),
			TableRows: tableRows,
		},
	}
}
