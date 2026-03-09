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
// fillTableCells coverage
// =============================================================================

func TestFillTableCells_WithContent(t *testing.T) {
	// Create a doc with a table that has cells starting near index 2
	table := makeTable(2, 2)
	doc := makeDocWithTables(table)
	// Set indices on table cells properly
	idx := int64(2)
	for _, row := range table.TableRows {
		for _, cell := range row.TableCells {
			if len(cell.Content) > 0 {
				cell.Content[0].StartIndex = idx
				cell.Content[0].EndIndex = idx + 5
				idx += 5
			}
		}
	}

	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	spec := &tableCreateSpec{
		rows: 2, cols: 2,
		cells: [][]string{{"A", "B"}, {"C", "D"}},
	}
	err := cmd.fillTableCells(context.Background(), svc, "test-doc", 1, spec)
	assert.NoError(t, err)
}

func TestFillTableCells_NoMatchingTable(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body:       &docs.Body{Content: []*docs.StructuralElement{}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	spec := &tableCreateSpec{rows: 2, cols: 2, cells: [][]string{{"A"}}}
	err := cmd.fillTableCells(context.Background(), svc, "test-doc", 1, spec)
	assert.NoError(t, err) // nil table = skip
}

// =============================================================================
// runTableCreate coverage
// =============================================================================

func TestRunTableCreate(t *testing.T) {
	doc := buildDoc(para(plain("PLACEHOLDER")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "PLACEHOLDER", replacement: "|2x3|"}
	spec := parseTableCreate("|2x3|")
	require.NotNil(t, spec)
	err := cmd.runTableCreate(context.Background(), u, "", "test-doc-id", expr, spec)
	assert.NoError(t, err)
}

func TestRunTableCreate_WithHeader(t *testing.T) {
	doc := buildDoc(para(plain("PLACEHOLDER")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "PLACEHOLDER", replacement: "|3x2:header|"}
	spec := parseTableCreate("|3x2:header|")
	require.NotNil(t, spec)
	err := cmd.runTableCreate(context.Background(), u, "", "test-doc-id", expr, spec)
	assert.NoError(t, err)
}

func TestRunTableCreate_PipeTable(t *testing.T) {
	doc := buildDoc(para(plain("PLACEHOLDER")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	pipeExpr := "|Name|Age|\n|Alice|30|"
	expr := sedExpr{pattern: "PLACEHOLDER", replacement: pipeExpr}
	spec := parseTableFromPipes(pipeExpr)
	require.NotNil(t, spec)
	err := cmd.runTableCreate(context.Background(), u, "", "test-doc-id", expr, spec)
	assert.NoError(t, err)
}

func TestRunTableCreate_NoMatch(t *testing.T) {
	doc := buildDoc(para(plain("no match here")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "ABSENT", replacement: "|2x2|"}
	spec := parseTableCreate("|2x2|")
	require.NotNil(t, spec)
	err := cmd.runTableCreate(context.Background(), u, "", "test-doc-id", expr, spec)
	assert.NoError(t, err)
}

// =============================================================================
// Integration: brace expression through full pipeline
// =============================================================================

func TestSedIntegration_BraceFormatting(t *testing.T) {
	doc := buildDoc(para(plain("highlight this")))
	reqs := runSedIntegration(t, doc, "s/highlight/{b,c=red}highlight/", nil)
	assert.NotEmpty(t, reqs)
}

func TestSedIntegration_BraceHeading(t *testing.T) {
	doc := buildDoc(para(plain("Title Text")))
	reqs := runSedIntegration(t, doc, "s/Title Text/{h1}Title Text/", nil)
	assert.NotEmpty(t, reqs)
}

// =============================================================================
// Integration: batch with table create
// =============================================================================

func TestSedIntegration_BatchTableCreate(t *testing.T) {
	doc := buildDoc(para(plain("TABLE1 and TABLE2")))
	reqs := runSedIntegration(t, doc, "", []string{
		"s/TABLE1/|2x3|/",
		"s/TABLE2/|3x2|/",
	})
	tableInserts := 0
	for _, r := range reqs {
		if r.InsertTable != nil {
			tableInserts++
		}
	}
	assert.Equal(t, 2, tableInserts)
}

// =============================================================================
// runTableOp with replacement (not delete)
// =============================================================================

func TestRunTableOp_AllTables(t *testing.T) {
	doc := buildDocWithTable("pre", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "post")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	// math.MinInt32 = all tables
	expr := sedExpr{tableRef: -2147483648, replacement: ""}
	err := cmd.runTableOp(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// runTableWildcardReplace — column wildcard
// =============================================================================

func TestRunTableCellReplace_WildcardCol(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	// col=0 means wildcard
	expr := sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 0},
		pattern:     ".",
		replacement: "X",
		global:      true,
	}
	err := cmd.runTableCellReplace(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// runTableMerge — more paths (unmerge, split)
// =============================================================================

func TestRunTableMerge_Unmerge(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"a", "b"}, {"c", "d"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1, endRow: 2, endCol: 2},
		replacement: "unmerge",
	}
	err := cmd.runTableMerge(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// runBatch — image expression path
// =============================================================================

func TestRunBatch_ImageExpressions(t *testing.T) {
	doc := buildDoc(para(plain("LOGO here")))
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	exprs := []sedExpr{
		{pattern: "LOGO", replacement: "![logo](https://example.com/logo.png)"},
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc-id", exprs)
	assert.NoError(t, err)
}

// =============================================================================
// applyDeferredBullets — with existing bullets
// =============================================================================

func TestApplyDeferredBullets_ExistingBullets(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Bullet: &docs.Bullet{ListId: "list1"},
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "- Item 1\n"}, StartIndex: 1, EndIndex: 10},
				},
			}, StartIndex: 1, EndIndex: 10},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "\t- Sub item\n"}, StartIndex: 10, EndIndex: 22},
				},
			}, StartIndex: 10, EndIndex: 22},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "- Item 2\n"}, StartIndex: 22, EndIndex: 31},
				},
			}, StartIndex: 22, EndIndex: 31},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	err := cmd.applyDeferredBullets(context.Background(), svc, "test-doc")
	assert.NoError(t, err)
}

// =============================================================================
// processFootnotes coverage
// =============================================================================

func TestProcessFootnotes_Empty(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	err := processFootnotes(context.Background(), svc, "test-doc", nil)
	assert.NoError(t, err)
}

// =============================================================================
// applyBreakPhase coverage
// =============================================================================

func TestApplyBreakPhase_NilBrace(t *testing.T) {
	expr := sedExpr{pattern: "foo", replacement: "bar"}
	err := applyBreakPhase(context.Background(), nil, "", expr, nil)
	assert.NoError(t, err)
}

func TestApplyBreakPhase_NoBreak(t *testing.T) {
	brace, _ := parseBraceExpr("b")
	expr := sedExpr{pattern: "foo", replacement: "bar", brace: brace}
	err := applyBreakPhase(context.Background(), nil, "", expr, nil)
	assert.NoError(t, err)
}

func TestApplyBreakPhase_EmptyFormatRanges(t *testing.T) {
	brace, _ := parseBraceExpr("+=page")
	expr := sedExpr{pattern: "foo", replacement: "bar", brace: brace}
	err := applyBreakPhase(context.Background(), nil, "", expr, nil)
	assert.NoError(t, err)
}

func TestApplyBreakPhase_WithBreak(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "content here\n"}, StartIndex: 1, EndIndex: 14},
				},
			}, StartIndex: 1, EndIndex: 14},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	brace, _ := parseBraceExpr("+=page")
	expr := sedExpr{pattern: "content", replacement: "content", brace: brace}
	frs := []formatRange{{start: 1, end: 8}}
	err := applyBreakPhase(context.Background(), svc, "test-doc", expr, frs)
	assert.NoError(t, err)
}

// =============================================================================
// runInsertAroundMatch — with global flag
// =============================================================================

func TestRunInsertAroundMatch_GlobalAppend(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{SectionBreak: &docs.SectionBreak{}, StartIndex: 0, EndIndex: 1},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target one\n"}, StartIndex: 1, EndIndex: 12},
				},
			}, StartIndex: 1, EndIndex: 12},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "keep\n"}, StartIndex: 12, EndIndex: 17},
				},
			}, StartIndex: 12, EndIndex: 17},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target two\n"}, StartIndex: 17, EndIndex: 28},
				},
			}, StartIndex: 17, EndIndex: 28},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseAICommand("a/target/appended text/", 'a')
	expr.global = true
	err := cmd.runAppendCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

// =============================================================================
// parseSedExpr — more coverage for edge cases
// =============================================================================

func TestParseFullExpr_AllCommands(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"substitute basic", "s/foo/bar/", false},
		{"substitute global", "s/foo/bar/g", false},
		{"delete", "d/pattern/", false},
		{"append", "a/pattern/text/", false},
		{"insert", "i/pattern/text/", false},
		{"transliterate", "y/abc/ABC/", false},
		{"empty", "", true},
		{"invalid", "x/foo/bar/", true},
		{"cell ref", "s/|1|A1/new/", false},
		{"table ref", "s/|1|//", false},
		{"bad regex", "s/[invalid/text/", false}, // bad regex parses fine, fails at compile
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseFullExpr(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Integration: batch with all expression types
// =============================================================================

func TestSedIntegration_BatchAllTypes(t *testing.T) {
	doc := buildDocWithTable("hello world target", 2, 2,
		[][]string{{"old", "keep"}, {"keep", "keep"}}, "footer")

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
			"s/hello/Hi/g",       // native
			"s/world/**World**/", // manual
			"d/target/",          // command
			"s/|1|A1/new/",       // cell
			"s/$/\\nappended/",   // positional
		},
	}
	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	assert.NoError(t, err)
	assert.NotEmpty(t, captured)
}

// =============================================================================
// runBatch — manual with image pattern in pattern field
// =============================================================================

func TestRunBatch_ImagePattern(t *testing.T) {
	doc := buildDocWithInlineImage()
	svc, cleanup := mockDocsServerWithImages(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	exprs := []sedExpr{
		{pattern: "!(1)", replacement: ""},
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc", exprs)
	assert.NoError(t, err)
}

// =============================================================================
// runNative coverage
// =============================================================================

func TestRunNative(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	err := cmd.runNative(context.Background(), u, "", "test-doc", "foo", "bar")
	assert.NoError(t, err)
}

// =============================================================================
// Run — DryRun with multiple expressions
// =============================================================================

func TestSedIntegration_DryRunMulti(t *testing.T) {
	doc := buildDoc(para(plain("hello world")))

	srv := mockDocsServerAdvanced(t, doc, nil)
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
			"s/hello/**world**/",
			"s/world/earth/g",
			"d/nothing/",
			"s/|1|A1/test/",
		},
	}
	flags := &RootFlags{Account: "test@example.com", DryRun: true}
	err := cmd.Run(ctx, flags)
	assert.NoError(t, err)
}

// =============================================================================
// runDeleteCommand — delete in table
// =============================================================================

func TestRunDeleteCommand_InTable(t *testing.T) {
	doc := buildDocWithTable("", 2, 2, [][]string{{"delete me", "keep"}, {"keep", "keep"}}, "")
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, err := parseDCommand("d/delete/")
	require.NoError(t, err)
	err = cmd.runDeleteCommand(context.Background(), u, "", "test-doc-id", expr)
	assert.NoError(t, err)
}

// =============================================================================
// runTransliterate — edge cases
// =============================================================================

func TestRunTransliterate_NoMatch(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "xyz\n"}},
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
	err := cmd.runTransliterate(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}
