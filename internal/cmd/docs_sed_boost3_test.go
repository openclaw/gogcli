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

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// =============================================================================
// Structural features in brace expressions — buildChipRequests
// =============================================================================

func TestBuildChipRequests_NilExpr(t *testing.T) {
	reqs := buildChipRequests(nil, 1)
	assert.Nil(t, reqs)
}

func TestBuildChipRequests_RegularURL(t *testing.T) {
	expr := &braceExpr{URL: "https://example.com"}
	reqs := buildChipRequests(expr, 1)
	assert.Nil(t, reqs)
}

func TestBuildChipRequests_PersonChip(t *testing.T) {
	expr := &braceExpr{URL: "chip://person/user@example.com"}
	reqs := buildChipRequests(expr, 1)
	assert.NotNil(t, reqs)
}

func TestBuildChipRequests_DateChip(t *testing.T) {
	expr := &braceExpr{URL: "chip://date/2024-01-15"}
	reqs := buildChipRequests(expr, 1)
	// Date chips return nil (API limitation)
	assert.Nil(t, reqs)
}

func TestBuildChipRequests_FileChip(t *testing.T) {
	expr := &braceExpr{URL: "chip://file/doc123"}
	reqs := buildChipRequests(expr, 1)
	assert.Nil(t, reqs) // RichLink is read-only
}

func TestBuildChipRequests_BookmarkChip(t *testing.T) {
	expr := &braceExpr{URL: "chip://bookmark/section1"}
	reqs := buildChipRequests(expr, 1)
	assert.Nil(t, reqs)
}

func TestBuildChipRequests_PlaceChip(t *testing.T) {
	expr := &braceExpr{URL: "chip://place/New York"}
	reqs := buildChipRequests(expr, 1)
	assert.Nil(t, reqs)
}

func TestBuildChipRequests_DropdownChip(t *testing.T) {
	expr := &braceExpr{URL: "chip://dropdown/Option A"}
	reqs := buildChipRequests(expr, 1)
	assert.Nil(t, reqs)
}

func TestResolveChipURL_Variations(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"person", "chip://person/user@example.com"},
		{"file", "chip://file/doc123"},
		{"date", "chip://date/2024-01-15"},
		{"place", "chip://place/New York"},
		{"dropdown", "chip://dropdown/Option A"},
		{"bookmark", "chip://bookmark/sec1"},
		{"chart", "chip://chart/sheet123"},
		{"non-chip", "https://example.com"},
		{"invalid", "chip://"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, fb := resolveChipURL(tt.url)
			_ = u
			_ = fb
		})
	}
}

// =============================================================================
// Brace pattern — edge cases
// =============================================================================

func TestBraceTableToSedExpr_Defaults(t *testing.T) {
	ref := &braceTableRef{
		TableIndex: 1,
		Row:        1,
		Col:        1,
	}
	expr := &sedExpr{replacement: "test"}
	braceTableToSedExpr(ref, expr)
	assert.NotNil(t, expr.cellRef)
	assert.Equal(t, 1, expr.cellRef.row)
}

func TestBraceTableToSedExpr_Nil(t *testing.T) {
	expr := &sedExpr{}
	braceTableToSedExpr(nil, expr)
	assert.Nil(t, expr.cellRef)
}

func TestBraceTableToSedExpr_BareTableRef(t *testing.T) {
	ref := &braceTableRef{TableIndex: 2}
	expr := &sedExpr{}
	braceTableToSedExpr(ref, expr)
	assert.Equal(t, 2, expr.tableRef)
}

func TestBraceTableToSedExpr_AllTables(t *testing.T) {
	ref := &braceTableRef{TableIndex: 0}
	expr := &sedExpr{}
	braceTableToSedExpr(ref, expr)
	assert.Equal(t, -2147483648, expr.tableRef) // math.MinInt32
}

func TestBraceTableToSedExpr_RowOp(t *testing.T) {
	ref := &braceTableRef{TableIndex: 1, RowOp: "+2"}
	expr := &sedExpr{}
	braceTableToSedExpr(ref, expr)
	assert.NotNil(t, expr.cellRef)
}

func TestBraceTableToSedExpr_ColOp(t *testing.T) {
	ref := &braceTableRef{TableIndex: 1, ColOp: "-1"}
	expr := &sedExpr{}
	braceTableToSedExpr(ref, expr)
	assert.NotNil(t, expr.cellRef)
}

func TestBraceTableToSedExpr_Create(t *testing.T) {
	ref := &braceTableRef{TableIndex: 1, IsCreate: true}
	expr := &sedExpr{}
	braceTableToSedExpr(ref, expr)
	// IsCreate doesn't set cellRef
}

func TestParseBraceExpr_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"just spaces", "   "},
		{"bold only", "bold"},
		{"heading", "h1"},
		{"color", "color=#ff0000"},
		{"font", "font=Arial"},
		{"columns", "cols=2"},
		{"checkbox", "checkbox"},
		{"toc", "toc"},
		{"comment", "comment=review this"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _ = parseBraceExpr(tt.input)
		})
	}
}

// =============================================================================
// parseSedExpr — more coverage
// =============================================================================

func TestParseSedExpr_IFlag(t *testing.T) {
	expr, err := parseFullExpr("s/Hello/world/i")
	require.NoError(t, err)
	// i flag is applied to the pattern via (?i) prefix
	assert.Contains(t, expr.pattern, "(?i)")
}

func TestParseSedExpr_GIFlags(t *testing.T) {
	expr, err := parseFullExpr("s/Hello/world/gi")
	require.NoError(t, err)
	assert.True(t, expr.global)
	assert.Contains(t, expr.pattern, "(?i)")
}

func TestParseSedExpr_EscapedNewline(t *testing.T) {
	expr, err := parseFullExpr(`s/foo/bar\nbaz/`)
	require.NoError(t, err)
	assert.Contains(t, expr.replacement, `\n`)
}

func TestParseSedExpr_HashDelimiter(t *testing.T) {
	expr, err := parseFullExpr("s#/path#/newpath#g")
	require.NoError(t, err)
	assert.Equal(t, "/path", expr.pattern)
	assert.Equal(t, "/newpath", expr.replacement)
}

// =============================================================================
// Run — dryRun mode
// =============================================================================

func TestSedIntegration_DryRun(t *testing.T) {
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
		DocID:      "test-doc-id",
		Expression: "s/hello/world/",
	}
	flags := &RootFlags{Account: "test@example.com", DryRun: true}
	err := cmd.Run(ctx, flags)
	assert.NoError(t, err)
}

// =============================================================================
// helpers — canUseNativeReplace edge cases
// =============================================================================

func TestCanUseNativeReplace_EdgeCases(t *testing.T) {
	// Just verify it doesn't panic and returns consistent results
	tests := []struct {
		name string
		repl string
	}{
		{"plain", "hello"},
		{"bold", "**bold**"},
		{"italic", "*italic*"},
		{"code", "`code`"},
		{"link", "[text](url)"},
		{"heading", "# heading"},
		{"image", "!(url)"},
		{"strikethrough", "~~strike~~"},
		{"underline", "__under__"},
		{"table", "|3x4|"},
		{"brace", "{bold}"},
		{"empty", ""},
		{"numbered ref", "$1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = canUseNativeReplace(tt.repl)
		})
	}
	// Verify known cases
	assert.True(t, canUseNativeReplace("hello"))
	assert.True(t, canUseNativeReplace(""))
	assert.False(t, canUseNativeReplace("**bold**"))
	assert.False(t, canUseNativeReplace("# heading"))
}

// =============================================================================
// Ensure no test breakage with unused helpers
// =============================================================================

func TestIsMergeOp_MoreCases(t *testing.T) {
	assert.True(t, isMergeOp("merge"))
	assert.True(t, isMergeOp("unmerge"))
	assert.True(t, isMergeOp("split"))
	assert.True(t, isMergeOp(" MERGE "))
	assert.False(t, isMergeOp("text"))
	assert.False(t, isMergeOp(""))
}

func TestLiteralReplacement_MoreCases(t *testing.T) {
	// Basic: no escapes
	assert.Equal(t, "no escapes", literalReplacement("no escapes"))
	// Verify it doesn't panic on various inputs
	_ = literalReplacement(`hello\nworld`)
	_ = literalReplacement(`tab\there`)
	_ = literalReplacement(`double\\slash`)
	_ = literalReplacement("")
	_ = literalReplacement("$1 $2")
}

// =============================================================================
// runManualInner — footnote, image, hrule, break paths
// =============================================================================

func TestRunManualInner_Footnote(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "text with note\n"}, StartIndex: 1, EndIndex: 16},
				},
			}, StartIndex: 1, EndIndex: 16},
		}},
	}

	// Need footnote response
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
			for _, rr := range req.Requests {
				reply := &docs.Response{}
				if rr.CreateFootnote != nil {
					reply.CreateFootnote = &docs.CreateFootnoteResponse{FootnoteId: "fn1"}
				}
				resp.Replies = append(resp.Replies, reply)
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	require.NoError(t, err)

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "note", replacement: "[^footnote text]"}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_HRule(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "DIVIDER\n"}, StartIndex: 1, EndIndex: 9},
				},
			}, StartIndex: 1, EndIndex: 9},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	expr := sedExpr{pattern: "DIVIDER", replacement: "---"}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_BraceExpr(t *testing.T) {
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
	brace, _ := parseBraceExpr("b")
	expr := sedExpr{pattern: "hello", replacement: "hello", brace: brace}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_BraceWithHeading(t *testing.T) {
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
	brace, _ := parseBraceExpr("h1")
	expr := sedExpr{pattern: "Title", replacement: "Title", brace: brace}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_BraceWithStructural(t *testing.T) {
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
	brace, _ := parseBraceExpr("cols=2")
	expr := sedExpr{pattern: "item", replacement: "item", brace: brace}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunManualInner_BraceBreak(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "content\n"}, StartIndex: 1, EndIndex: 9},
				},
			}, StartIndex: 1, EndIndex: 9},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()

	cmd := &DocsSedCmd{}
	brace, _ := parseBraceExpr("+=page")
	expr := sedExpr{pattern: "content", replacement: "content", brace: brace}
	count, _, err := cmd.runManualInner(context.Background(), svc, "test-doc", expr)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}
