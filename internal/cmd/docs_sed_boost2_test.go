package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// =============================================================================
// processCellExprs coverage
// =============================================================================

func TestProcessCellExprs_RowOp(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	cellExprs := []indexedExpr{
		{index: 0, expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, rowOp: opInsert, opTarget: 1}}},
	}
	count, err := cmd.processCellExprs(context.Background(), u, "", "test-doc-id", cellExprs)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessCellExprs_SingleCell(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"old", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	cellExprs := []indexedExpr{
		{index: 0, expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}, replacement: "new"}},
	}
	count, err := cmd.processCellExprs(context.Background(), u, "", "test-doc-id", cellExprs)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessCellExprs_BatchCells(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	cellExprs := []indexedExpr{
		{index: 0, expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}, replacement: "A"}},
		{index: 1, expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 2}, replacement: "B"}},
		{index: 2, expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 2, col: 1}, replacement: "C"}},
	}
	count, err := cmd.processCellExprs(context.Background(), u, "", "test-doc-id", cellExprs)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestProcessCellExprs_MergeOp(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	cellExprs := []indexedExpr{
		{index: 0, expr: sedExpr{
			cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1, endRow: 2, endCol: 2},
			replacement: "merge",
		}},
	}
	count, err := cmd.processCellExprs(context.Background(), u, "", "test-doc-id", cellExprs)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessCellExprs_TableRef(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	cellExprs := []indexedExpr{
		{index: 0, expr: sedExpr{tableRef: 1, replacement: ""}},
	}
	count, err := cmd.processCellExprs(context.Background(), u, "", "test-doc-id", cellExprs)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

// =============================================================================
// applyDeferredBullets coverage
// =============================================================================

func TestApplyDeferredBullets_NoBullets(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "no bullets\n"}, StartIndex: 1, EndIndex: 12},
				},
			}, StartIndex: 1, EndIndex: 12},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	err := cmd.applyDeferredBullets(context.Background(), svc, "test-doc")
	assert.NoError(t, err)
}

func TestApplyDeferredBullets_WithTabs(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "- Item 1\n"}, StartIndex: 1, EndIndex: 10},
				},
			}, StartIndex: 1, EndIndex: 10},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "\t- Sub item\n"}, StartIndex: 10, EndIndex: 22},
				},
			}, StartIndex: 10, EndIndex: 22},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	err := cmd.applyDeferredBullets(context.Background(), svc, "test-doc")
	assert.NoError(t, err)
}

func TestApplyDeferredBullets_NilBody(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	err := cmd.applyDeferredBullets(context.Background(), svc, "test-doc")
	assert.NoError(t, err)
}

// =============================================================================
// runManualInner coverage — more paths
// =============================================================================

func TestRunManualInner_NoMatch(t *testing.T) {
	doc := buildDoc(para(plain("hello world")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "nonexistent", replacement: "nope"}
	count, bulletReqs, err := cmd.runManualInner(context.Background(), svc, "test-doc-id", expr)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Nil(t, bulletReqs)
}

func TestRunManualInner_SimpleReplace(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "hello world\n"}, StartIndex: 1, EndIndex: 13},
				},
			}, StartIndex: 1, EndIndex: 13},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "hello", replacement: "goodbye"}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_WithFormatting(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "hello world\n"}, StartIndex: 1, EndIndex: 13},
				},
			}, StartIndex: 1, EndIndex: 13},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "hello", replacement: "**goodbye**"}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_WithHeading(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Title\n"}, StartIndex: 1, EndIndex: 7},
				},
			}, StartIndex: 1, EndIndex: 7},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "Title", replacement: "## Section"}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_ImageReplace(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "LOGO\n"}, StartIndex: 1, EndIndex: 6},
				},
			}, StartIndex: 1, EndIndex: 6},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "LOGO", replacement: "!(https://example.com/logo.png)"}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_GlobalMultiMatch(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "foo bar foo baz foo\n"}, StartIndex: 1, EndIndex: 21},
				},
			}, StartIndex: 1, EndIndex: 21},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "foo", replacement: "**qux**", global: true}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestRunManualInner_DeleteMatch(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "remove THIS word\n"}, StartIndex: 1, EndIndex: 18},
				},
			}, StartIndex: 1, EndIndex: 18},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "THIS ", replacement: ""}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_NthMatch(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "aaa bbb aaa ccc aaa\n"}, StartIndex: 1, EndIndex: 21},
				},
			}, StartIndex: 1, EndIndex: 21},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "aaa", replacement: "XXX", nthMatch: 2, global: true}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_BulletList(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "item\n"}, StartIndex: 1, EndIndex: 6},
				},
			}, StartIndex: 1, EndIndex: 6},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "item", replacement: "- First bullet"}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

// =============================================================================
// runManual coverage
// =============================================================================

func TestRunManual_Simple(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "hello\n"}, StartIndex: 1, EndIndex: 7},
				},
			}, StartIndex: 1, EndIndex: 7},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "hello", replacement: "world"}
	err := cmd.runManual(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

// =============================================================================
// Integration: Run with various expression combos through the top-level
// =============================================================================

func TestSedIntegration_PositionalAppend(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc-id",
		Title:      "Test",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "existing\n"}, StartIndex: 1, EndIndex: 10},
				},
			}, StartIndex: 1, EndIndex: 10},
		}},
	}
	reqs := runSedIntegration(t, doc, "s/$/appended/", nil)
	assert.NotEmpty(t, reqs)
}

func TestSedIntegration_PositionalPrepend(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc-id",
		Title:      "Test",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "existing\n"}, StartIndex: 1, EndIndex: 10},
				},
			}, StartIndex: 1, EndIndex: 10},
		}},
	}
	reqs := runSedIntegration(t, doc, "s/^/prepended/", nil)
	assert.NotEmpty(t, reqs)
}

func TestSedIntegration_EmptyDocClear(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc-id",
		Title:      "Test",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "\n"}, StartIndex: 1, EndIndex: 2},
				},
			}, StartIndex: 1, EndIndex: 2},
		}},
	}
	reqs := runSedIntegration(t, doc, "s/^$/Hello world/", nil)
	// Should insert text into empty doc
	hasInsert := false
	for _, r := range reqs {
		if r.InsertText != nil {
			hasInsert = true
		}
	}
	assert.True(t, hasInsert)
}

func TestSedIntegration_ClearDocument(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc-id",
		Title:      "Test",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "content to clear\n"}, StartIndex: 1, EndIndex: 18},
				},
			}, StartIndex: 1, EndIndex: 18},
		}},
	}
	reqs := runSedIntegration(t, doc, "s/^$//", nil)
	hasDelete := false
	for _, r := range reqs {
		if r.DeleteContentRange != nil {
			hasDelete = true
		}
	}
	assert.True(t, hasDelete)
}

func TestSedIntegration_PipeTableCreate(t *testing.T) {
	doc := buildDoc(para(plain("[TABLE]")))
	reqs := runSedIntegration(t, doc, `s/\[TABLE\]/|A|B|\n|1|2|/`, nil)
	hasTableInsert := false
	for _, r := range reqs {
		if r.InsertTable != nil {
			hasTableInsert = true
		}
	}
	assert.True(t, hasTableInsert)
}

func TestSedIntegration_CellReplace(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"old", "keep"}, {"keep", "keep"}}, "")
	runSedIntegration(t, doc, "s/|1|A1/new/", nil)
}

func TestSedIntegration_DeleteCommand(t *testing.T) {
	doc := buildDoc(
		para(plain("keep this")),
		para(plain("delete this")),
	)
	runSedIntegration(t, doc, "d/delete/", nil)
}

func TestSedIntegration_TransliterateCommand(t *testing.T) {
	doc := buildDoc(para(plain("hello")))
	runSedIntegration(t, doc, "y/helo/HELO/", nil)
}

func TestSedIntegration_ImagePatternDelete(t *testing.T) {
	doc := buildDocWithInlineImage()
	// Mock server for images
	var captured []*docs.Request
	srv := mockDocsServerAdvanced(t, doc, func(reqs []*docs.Request) {
		captured = append(captured, reqs...)
	})
	defer srv.Close()

	origNewDocs := newDocsService
	newDocsService = func(ctx context.Context, account string) (*docs.Service, error) {
		return docs.NewService(ctx,
			option.WithoutAuthentication(),
			option.WithEndpoint(srv.URL+"/"),
		)
	}
	defer func() { newDocsService = origNewDocs }()

	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &DocsSedCmd{
		DocID:      "test-doc",
		Expression: "s/!(1)//",
	}
	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	assert.NoError(t, err)
}

// =============================================================================
// classifyExprForBatch coverage
// =============================================================================

func TestClassifyExprForBatch_Extended(t *testing.T) {
	tests := []struct {
		name     string
		expr     sedExpr
		expected exprCategory
	}{
		{"positional ^", sedExpr{pattern: "^", replacement: "x"}, exprCatPositional},
		{"positional $", sedExpr{pattern: "$", replacement: "x"}, exprCatPositional},
		{"positional ^$", sedExpr{pattern: "^$", replacement: "x"}, exprCatPositional},
		{"command d", sedExpr{command: 'd', pattern: "x"}, exprCatCommand},
		{"command a", sedExpr{command: 'a', pattern: "x"}, exprCatCommand},
		{"command i", sedExpr{command: 'i', pattern: "x"}, exprCatCommand},
		{"command y", sedExpr{command: 'y', pattern: "x"}, exprCatCommand},
		{"cell ref", sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}}, exprCatCell},
		{"native", sedExpr{pattern: "foo", replacement: "bar", global: true}, exprCatNative},
		{"manual", sedExpr{pattern: "foo", replacement: "**bar**"}, exprCatManual},
		{"image replacement", sedExpr{pattern: "foo", replacement: "![alt](url)"}, exprCatImage},
		{"image pattern", sedExpr{pattern: "!(1)", replacement: ""}, exprCatImagePattern},
		{"table create", sedExpr{pattern: "foo", replacement: "|3x4|"}, exprCatTableCreate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyExprForBatch(tt.expr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// =============================================================================
// Wildcard table cell replace
// =============================================================================

func TestRunTableCellReplace_WildcardRow(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	// row=0 means wildcard
	expr := sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 0, col: 1},
		pattern:     ".",
		replacement: "X",
		global:      true,
	}
	err := cmd.runTableCellReplace(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// fetchDoc coverage
// =============================================================================

func TestFetchDoc(t *testing.T) {
	doc := buildDoc(para(plain("hello")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	docsSvc, gotDoc, err := fetchDoc(context.Background(), "", "test-doc-id")
	require.NoError(t, err)
	assert.NotNil(t, docsSvc)
	assert.NotNil(t, gotDoc)
}

// =============================================================================
// runInsertAroundMatch — more branches (insert before, with image)
// =============================================================================

func TestRunInsertAroundMatch_InsertWithFormatting(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target line\n"}, StartIndex: 1, EndIndex: 13},
				},
			}, StartIndex: 1, EndIndex: 13},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()

	// Test insert with bold formatting
	expr, _ := parseAICommand("i/target/**bold text**/", 'i')
	err := cmd.runInsertCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunInsertAroundMatch_AppendWithTable(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target line\n"}, StartIndex: 1, EndIndex: 13},
				},
			}, StartIndex: 1, EndIndex: 13},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseAICommand("a/target/|2x3|/", 'a')
	err := cmd.runAppendCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

// =============================================================================
// runTableMerge coverage
// =============================================================================

func TestRunTableMerge(t *testing.T) {
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
	err := cmd.runTableMerge(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// Table ops: replacement with pipe-table content
// =============================================================================

func TestRunTableOp_NegativeIndex(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{tableRef: -1, replacement: ""}
	err := cmd.runTableOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// runDeleteCommand — more paths
// =============================================================================

func TestRunDeleteCommand_GlobalFlag(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{SectionBreak: &docs.SectionBreak{}, StartIndex: 0, EndIndex: 1},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "match line\n"}},
				},
			}, StartIndex: 1, EndIndex: 12},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "keep\n"}},
				},
			}, StartIndex: 12, EndIndex: 17},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "match again\n"}},
				},
			}, StartIndex: 17, EndIndex: 29},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, err := parseDCommand("d/match/")
	require.NoError(t, err)
	expr.global = true

	err = cmd.runDeleteCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

// =============================================================================
// Integration: batch with cell + native mixed
// =============================================================================

func TestSedIntegration_BatchMixedCellAndNative(t *testing.T) {
	doc := buildDocWithTable("hello world", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")

	var captured []*docs.Request
	srv := mockDocsServerAdvanced(t, doc, func(reqs []*docs.Request) {
		captured = append(captured, reqs...)
	})
	defer srv.Close()

	origNewDocs := newDocsService
	newDocsService = func(ctx context.Context, account string) (*docs.Service, error) {
		return docs.NewService(ctx,
			option.WithoutAuthentication(),
			option.WithEndpoint(srv.URL+"/"),
		)
	}
	defer func() { newDocsService = origNewDocs }()

	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &DocsSedCmd{
		DocID: "test-doc-id",
		Expressions: []string{
			"s/hello/Hi/g",
			"s/|1|A1/X/",
		},
	}
	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	assert.NoError(t, err)
	assert.NotEmpty(t, captured)
}

// =============================================================================
// helpers coverage — buildTextStyleRequests branches
// =============================================================================

func TestBuildTextStyleRequests_AllFormats(t *testing.T) {
	tests := []struct {
		name    string
		formats []string
	}{
		{"bold", []string{"bold"}},
		{"italic", []string{"italic"}},
		{"strikethrough", []string{"strikethrough"}},
		{"code", []string{"code"}},
		{"underline", []string{"underline"}},
		{"superscript", []string{"superscript"}},
		{"subscript", []string{"subscript"}},
		{"link", []string{"link:https://example.com"}},
		{"color", []string{"color:#ff0000"}},
		{"font", []string{"font:Arial"}},
		{"smallcaps", []string{"smallcaps"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildTextStyleRequests(tt.formats, 0, 10)
			assert.NotEmpty(t, reqs, "expected requests for format %s", tt.name)
		})
	}
}

func TestBuildParagraphStyleRequests_AllFormats(t *testing.T) {
	tests := []struct {
		name    string
		formats []string
	}{
		{"heading1", []string{"heading1"}},
		{"heading2", []string{"heading2"}},
		{"heading3", []string{"heading3"}},
		{"heading4", []string{"heading4"}},
		{"heading5", []string{"heading5"}},
		{"heading6", []string{"heading6"}},
		{"bullet", []string{"bullet"}},
		{"numbered", []string{"numbered"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildParagraphStyleRequests(tt.formats, 0, 10)
			assert.NotEmpty(t, reqs, "expected requests for format %s", tt.name)
		})
	}
}

// =============================================================================
// parseSedExpr — more edge cases for coverage
// =============================================================================

func TestParseSedExpr_CellRefWithPattern(t *testing.T) {
	expr, err := parseFullExpr("s/|1|A1:hello/world/")
	require.NoError(t, err)
	// Cell ref syntax is parsed; verify no error
	assert.Equal(t, "world", expr.replacement)
}

func TestParseSedExpr_NthFlag(t *testing.T) {
	expr, err := parseFullExpr("s/foo/bar/2")
	require.NoError(t, err)
	assert.Equal(t, 2, expr.nthMatch)
}

func TestParseSedExpr_TableRef(t *testing.T) {
	expr, err := parseFullExpr("s/|1|//")
	require.NoError(t, err)
	assert.Equal(t, 1, expr.tableRef)
}

// =============================================================================
// canBatchCell
// =============================================================================

func TestCanBatchCell_MoreCases(t *testing.T) {
	tests := []struct {
		name     string
		ie       indexedExpr
		expected bool
	}{
		{
			"simple cell",
			indexedExpr{expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}, replacement: "x"}},
			true,
		},
		{
			"with pattern",
			indexedExpr{expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}, pattern: "foo", replacement: "x"}},
			false,
		},
		{
			"merge op",
			indexedExpr{expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}, replacement: "merge"}},
			false,
		},
		{
			"wildcard row",
			indexedExpr{expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 0, col: 1}, replacement: "x"}},
			false,
		},
		{
			"row op",
			indexedExpr{expr: sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1, rowOp: opInsert}, replacement: "x"}},
			false,
		},
		{
			"nil cellRef",
			indexedExpr{expr: sedExpr{replacement: "x"}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, canBatchCell(tt.ie))
		})
	}
}

// =============================================================================
// findDocMatches coverage — multiple paragraphs, table content
// =============================================================================

func TestFindDocMatches_AcrossElements(t *testing.T) {
	doc := buildDoc(
		para(bold("Hello "), plain("world")),
		para(plain("Hello again")),
	)
	expr := sedExpr{pattern: "Hello", replacement: "Hi", global: true}
	re, err := expr.compilePattern()
	require.NoError(t, err)

	matches := findDocMatches(doc, re, expr)
	assert.Equal(t, 2, len(matches))
}

func TestFindDocMatches_InTable(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"hello", "world"}, {"foo", "bar"}}, "")
	expr := sedExpr{pattern: "hello", replacement: "HI", global: true}
	re, err := expr.compilePattern()
	require.NoError(t, err)

	matches := findDocMatches(doc, re, expr)
	assert.Equal(t, 1, len(matches))
}

// =============================================================================
// classifyMatch coverage
// =============================================================================

// classifyMatch is tested indirectly through runManualInner tests above.

// =============================================================================
// Integration tests for RunBatch with image expressions
// =============================================================================

func TestSedIntegration_BatchWithImageReplacement(t *testing.T) {
	doc := buildDoc(para(plain("LOGO here and text")))

	var captured []*docs.Request
	srv := mockDocsServerAdvanced(t, doc, func(reqs []*docs.Request) {
		captured = append(captured, reqs...)
	})
	defer srv.Close()

	origNewDocs := newDocsService
	newDocsService = func(ctx context.Context, account string) (*docs.Service, error) {
		return docs.NewService(ctx,
			option.WithoutAuthentication(),
			option.WithEndpoint(srv.URL+"/"),
		)
	}
	defer func() { newDocsService = origNewDocs }()

	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &DocsSedCmd{
		DocID: "test-doc-id",
		Expressions: []string{
			`s/LOGO/![logo](https:\/\/example.com\/logo.png)/`,
			"s/text/TEXT/g",
		},
	}
	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	assert.NoError(t, err)
}
