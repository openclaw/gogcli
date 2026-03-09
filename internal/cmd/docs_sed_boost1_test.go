package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// --- Helpers for advanced mock server ---

// mockDocsServerWithImages creates a server that handles image-related requests.
func mockDocsServerWithImages(t *testing.T, doc *docs.Document) (*docs.Service, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		if r.Method == http.MethodPost {
			var req docs.BatchUpdateDocumentRequest
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &req)
			resp := &docs.BatchUpdateDocumentResponse{DocumentId: doc.DocumentId}
			for range req.Requests {
				resp.Replies = append(resp.Replies, &docs.Response{})
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	require.NoError(t, err)
	return svc, srv.Close
}

// buildDocWithInlineImage creates a document with inline objects.
const (
	testInlineObjectID  = "img1"
	testInlineObjectAlt = "logo"
)

func buildDocWithInlineImage() *docs.Document {
	return &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "before "}, StartIndex: 1, EndIndex: 8},
					{InlineObjectElement: &docs.InlineObjectElement{
						InlineObjectId: testInlineObjectID,
					}, StartIndex: 8, EndIndex: 9},
					{TextRun: &docs.TextRun{Content: " after\n"}, StartIndex: 9, EndIndex: 16},
				},
			}, StartIndex: 1, EndIndex: 16},
		}},
		InlineObjects: map[string]docs.InlineObject{
			testInlineObjectID: {
				InlineObjectProperties: &docs.InlineObjectProperties{
					EmbeddedObject: &docs.EmbeddedObject{
						Title:       testInlineObjectAlt,
						Description: testInlineObjectAlt,
					},
				},
			},
		},
	}
}

// =============================================================================
// runImageReplace coverage
// =============================================================================

func TestRunImageReplace_NoImages(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "no images\n"}},
				},
			}, StartIndex: 1, EndIndex: 11},
		}},
	}
	svc, cleanup := mockDocsServerWithImages(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	ref := &ImageRefPattern{ByPosition: true, Position: 1}
	err := cmd.runImageReplace(context.Background(), u, "", "test-doc", ref, "text", false)
	assert.NoError(t, err)
}

func TestRunImageReplace_DeleteInlineImage(t *testing.T) {
	doc := buildDocWithInlineImage()
	svc, cleanup := mockDocsServerWithImages(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	ref := &ImageRefPattern{ByPosition: true, Position: 1}
	err := cmd.runImageReplace(context.Background(), u, "", "test-doc", ref, "", false)
	assert.NoError(t, err)
}

func TestRunImageReplace_ReplaceWithNewImage(t *testing.T) {
	doc := buildDocWithInlineImage()
	svc, cleanup := mockDocsServerWithImages(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	ref := &ImageRefPattern{ByPosition: true, Position: 1}
	err := cmd.runImageReplace(context.Background(), u, "", "test-doc", ref, "!(https://example.com/new.png)", false)
	assert.NoError(t, err)
}

func TestRunImageReplace_ReplaceWithText(t *testing.T) {
	doc := buildDocWithInlineImage()
	svc, cleanup := mockDocsServerWithImages(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	ref := &ImageRefPattern{ByPosition: true, Position: 1}
	err := cmd.runImageReplace(context.Background(), u, "", "test-doc", ref, "replacement text", false)
	assert.NoError(t, err)
}

func TestRunImageReplace_AllImages(t *testing.T) {
	doc := buildDocWithInlineImage()
	svc, cleanup := mockDocsServerWithImages(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	ref := &ImageRefPattern{ByPosition: true, AllImages: true}
	err := cmd.runImageReplace(context.Background(), u, "", "test-doc", ref, "", true)
	assert.NoError(t, err)
}

func TestRunImageReplace_NoMatch(t *testing.T) {
	doc := buildDocWithInlineImage()
	svc, cleanup := mockDocsServerWithImages(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	ref := &ImageRefPattern{ByPosition: true, Position: 99}
	err := cmd.runImageReplace(context.Background(), u, "", "test-doc", ref, "text", false)
	assert.NoError(t, err)
}

// =============================================================================
// runTableOp coverage
// =============================================================================

func TestRunTableOp_DeleteTable(t *testing.T) {
	doc := buildDocWithTable("pre", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "post")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{tableRef: 1, replacement: ""}
	err := cmd.runTableOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableOp_OutOfRange(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{tableRef: 99, replacement: ""}
	err := cmd.runTableOp(context.Background(), u, "", "test-doc-id", expr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestRunTableOp_NoTables(t *testing.T) {
	doc := buildDoc(para(plain("no tables")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{tableRef: 1, replacement: ""}
	err := cmd.runTableOp(context.Background(), u, "", "test-doc-id", expr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tables")
}

// =============================================================================
// runTableCellReplace coverage
// =============================================================================

func TestRunTableCellReplace_WholeCell(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"old", "keep"}, {"keep", "keep"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1},
		replacement: "new",
	}
	err := cmd.runTableCellReplace(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableCellReplace_WithPattern(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"hello world", "keep"}, {"keep", "keep"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1},
		pattern:     "hello",
		replacement: "goodbye",
	}
	err := cmd.runTableCellReplace(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableCellReplace_CellNotFound(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 99, col: 1},
		replacement: "x",
	}
	err := cmd.runTableCellReplace(context.Background(), u, "", "test-doc-id", expr)
	assert.Error(t, err)
}

// =============================================================================
// runTableRowColOp coverage
// =============================================================================

func TestRunTableRowColOp_InsertRow(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, rowOp: opInsert, opTarget: 1},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableRowColOp_DeleteRow(t *testing.T) {
	doc := buildDocWithTable("", 3, 2, [][]string{{"a", "b"}, {"c", "d"}, {"e", "f"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, rowOp: opDelete, opTarget: 2},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableRowColOp_InsertCol(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, colOp: opInsert, opTarget: 1},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableRowColOp_DeleteCol(t *testing.T) {
	doc := buildDocWithTable("", 2, 3, [][]string{{"a", "b", "c"}, {"d", "e", "f"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, colOp: opDelete, opTarget: 2},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableRowColOp_NoTables(t *testing.T) {
	doc := buildDoc(para(plain("no tables")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, rowOp: opInsert, opTarget: 1},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.Error(t, err)
}

func TestRunTableRowColOp_RowOutOfRange(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, rowOp: opDelete, opTarget: 99},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.Error(t, err)
}

func TestRunTableRowColOp_DeleteOnlyRow(t *testing.T) {
	doc := buildDocWithTable("", 1, 2, [][]string{{"a", "b"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, rowOp: opDelete, opTarget: 1},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.Error(t, err)
}

func TestRunTableRowColOp_AppendRow(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, rowOp: opAppend, opTarget: 0},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableRowColOp_AppendCol(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, colOp: opAppend, opTarget: 0},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableRowColOp_NegativeTableIndex(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: -1, rowOp: opInsert, opTarget: 1},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunTableRowColOp_NegativeRowTarget(t *testing.T) {
	doc := buildDocWithTable("", 3, 2, [][]string{{"a", "b"}, {"c", "d"}, {"e", "f"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, rowOp: opDelete, opTarget: -1},
	}
	err := cmd.runTableRowColOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// runPositionalInsert coverage
// =============================================================================

func TestRunPositionalInsert_EmptyDocInsert(t *testing.T) {
	// ^$ on empty doc with replacement — insert at index 1
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "\n"}, StartIndex: 1, EndIndex: 2},
				},
			}, StartIndex: 1, EndIndex: 2},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "^$", replacement: "Hello new content"}
	handled, err := cmd.runPositionalInsert(context.Background(), u, "", "test-doc", expr)
	assert.True(t, handled)
	assert.NoError(t, err)
}

func TestRunPositionalInsert_ClearNonEmpty(t *testing.T) {
	// s/^$// on a non-empty doc = clear all content
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "some content\n"}, StartIndex: 1, EndIndex: 14},
				},
			}, StartIndex: 1, EndIndex: 14},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "^$", replacement: ""}
	handled, err := cmd.runPositionalInsert(context.Background(), u, "", "test-doc", expr)
	assert.True(t, handled)
	assert.NoError(t, err)
}

func TestRunPositionalInsert_NonEmptyWithReplacement(t *testing.T) {
	// s/^$/text/ on non-empty doc = no match
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "existing\n"}, StartIndex: 1, EndIndex: 10},
				},
			}, StartIndex: 1, EndIndex: 10},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "^$", replacement: "ignored"}
	handled, err := cmd.runPositionalInsert(context.Background(), u, "", "test-doc", expr)
	assert.True(t, handled)
	assert.NoError(t, err)
}

func TestRunPositionalInsert_Prepend(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "existing\n"}, StartIndex: 1, EndIndex: 10},
				},
			}, StartIndex: 1, EndIndex: 10},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "^", replacement: "Prepended: "}
	handled, err := cmd.runPositionalInsert(context.Background(), u, "", "test-doc", expr)
	assert.True(t, handled)
	assert.NoError(t, err)
}

func TestRunPositionalInsert_Append(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "existing\n"}, StartIndex: 1, EndIndex: 10},
				},
			}, StartIndex: 1, EndIndex: 10},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "$", replacement: "\\nAppended text"}
	handled, err := cmd.runPositionalInsert(context.Background(), u, "", "test-doc", expr)
	assert.True(t, handled)
	assert.NoError(t, err)
}

func TestRunPositionalInsert_NotPositional(t *testing.T) {
	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "foo", replacement: "bar"}
	handled, err := cmd.runPositionalInsert(context.Background(), u, "", "test-doc", expr)
	assert.False(t, handled)
	assert.NoError(t, err)
}

func TestRunPositionalInsert_EmptyDocClearNoOp(t *testing.T) {
	// s/^$// on already-empty doc = no-op
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "\n"}, StartIndex: 1, EndIndex: 2},
				},
			}, StartIndex: 1, EndIndex: 2},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "^$", replacement: ""}
	handled, err := cmd.runPositionalInsert(context.Background(), u, "", "test-doc", expr)
	assert.True(t, handled)
	assert.NoError(t, err)
}

// =============================================================================
// doPositionalInsert coverage — image and table paths
// =============================================================================

func TestDoPositionalInsert_Image(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	err := cmd.doPositionalInsert(context.Background(), svc, u, "test-doc", 1, "!(https://example.com/img.png)")
	assert.NoError(t, err)
}

func TestDoPositionalInsert_Table(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	err := cmd.doPositionalInsert(context.Background(), svc, u, "test-doc", 1, "|3x4|")
	assert.NoError(t, err)
}

func TestDoPositionalInsert_PlainText(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	err := cmd.doPositionalInsert(context.Background(), svc, u, "test-doc", 1, "Hello world")
	assert.NoError(t, err)
}

func TestDoPositionalInsert_FormattedText(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	err := cmd.doPositionalInsert(context.Background(), svc, u, "test-doc", 1, "**bold** text")
	assert.NoError(t, err)
}

func TestDoPositionalInsert_HeadingText(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	err := cmd.doPositionalInsert(context.Background(), svc, u, "test-doc", 1, "# Heading One")
	assert.NoError(t, err)
}

func TestDoPositionalInsert_PipeTable(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}

	// Need a server that returns the doc on re-fetch for fillTableCells
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	err := cmd.doPositionalInsert(context.Background(), svc, u, "test-doc", 1, "|A|B|\n|1|2|")
	assert.NoError(t, err)
}

// =============================================================================
// runSingle — more branches
// =============================================================================

func TestRunSingle_Transliterate(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "abc\n"}},
				},
			}, StartIndex: 1, EndIndex: 5},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseYCommand("y/abc/ABC/")
	err := cmd.runSingle(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunSingle_TableRef(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{tableRef: 1, replacement: ""}
	err := cmd.runSingle(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunSingle_PositionalPrepend(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "text\n"}, StartIndex: 1, EndIndex: 6},
				},
			}, StartIndex: 1, EndIndex: 6},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "^", replacement: "prefix "}
	err := cmd.runSingle(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunSingle_CellRef(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"old", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1},
		replacement: "new",
	}
	err := cmd.runSingle(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunSingle_TableCreate(t *testing.T) {
	doc := buildDoc(para(plain("PLACEHOLDER")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "PLACEHOLDER", replacement: "|3x2|"}
	err := cmd.runSingle(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

func TestRunSingle_ImagePattern(t *testing.T) {
	doc := buildDocWithInlineImage()
	svc, cleanup := mockDocsServerWithImages(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "!(1)", replacement: ""}
	err := cmd.runSingle(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunSingle_InsertCommand(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target\n"}},
				},
			}, StartIndex: 1, EndIndex: 8},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseAICommand("i/target/before/", 'i')
	err := cmd.runSingle(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunSingle_AppendCommand(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target\n"}},
				},
			}, StartIndex: 1, EndIndex: 8},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseAICommand("a/target/after/", 'a')
	err := cmd.runSingle(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunSingle_MergeOp(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1, endRow: 2, endCol: 2},
		replacement: "merge",
	}
	err := cmd.runSingle(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// runBatch — more coverage for cell, table-create, positional, image branches
// =============================================================================

func TestRunBatch_CellExpressions(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"old1", "old2"}, {"old3", "old4"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	exprs := []sedExpr{
		{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}, replacement: "new1"},
		{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 2}, replacement: "new2"},
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc-id", exprs)
	assert.NoError(t, err)
}

func TestRunBatch_WithPositional(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "text\n"}, StartIndex: 1, EndIndex: 6},
				},
			}, StartIndex: 1, EndIndex: 6},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	exprs := []sedExpr{
		{pattern: "$", replacement: "\\nappended"},
		{pattern: "foo", replacement: "bar", global: true},
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc", exprs)
	assert.NoError(t, err)
}

func TestRunBatch_WithManualFormatting(t *testing.T) {
	doc := buildDoc(para(plain("hello world")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	exprs := []sedExpr{
		{pattern: "hello", replacement: "**hello**"},
		{pattern: "world", replacement: "*world*"},
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc-id", exprs)
	assert.NoError(t, err)
}

func TestRunBatch_WithCommand(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "delete me\n"}},
				},
			}, StartIndex: 1, EndIndex: 11},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	exprs := []sedExpr{
		{command: 'd', pattern: "delete"},
		{pattern: "foo", replacement: "bar", global: true},
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc", exprs)
	assert.NoError(t, err)
}

func TestRunBatch_TableCreate(t *testing.T) {
	doc := buildDoc(para(plain("TABLE_HERE")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	exprs := []sedExpr{
		{pattern: "TABLE_HERE", replacement: "|2x3|"},
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc-id", exprs)
	assert.NoError(t, err)
}
