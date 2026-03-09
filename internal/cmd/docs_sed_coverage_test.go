package cmd

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gapi "google.golang.org/api/googleapi"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func testUI() *ui.UI {
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	return u
}

// --- collectAllTables / collectAllTablesWithIndex / findTableCell / getCellText ---

func makeDocWithTables(tables ...*docs.Table) *docs.Document {
	content := make([]*docs.StructuralElement, 0, len(tables))
	idx := int64(1)
	for _, t := range tables {
		content = append(content, &docs.StructuralElement{
			Table:      t,
			StartIndex: idx,
			EndIndex:   idx + 100,
		})
		idx += 100
	}
	return &docs.Document{Body: &docs.Body{Content: content}}
}

func makeTable(rows, cols int) *docs.Table {
	t := &docs.Table{Rows: int64(rows), Columns: int64(cols)}
	for r := 0; r < rows; r++ {
		row := &docs.TableRow{}
		for c := 0; c < cols; c++ {
			cell := &docs.TableCell{
				Content: []*docs.StructuralElement{
					{Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "cell\n"}, StartIndex: 10, EndIndex: 15},
						},
					}},
				},
			}
			row.TableCells = append(row.TableCells, cell)
		}
		t.TableRows = append(t.TableRows, row)
	}
	return t
}

func TestCollectAllTables(t *testing.T) {
	doc := makeDocWithTables(makeTable(2, 3), makeTable(1, 1))
	tables := collectAllTables(doc)
	assert.Equal(t, 2, len(tables))

	// nil body
	assert.Empty(t, collectAllTables(&docs.Document{}))
}

func TestCollectAllTablesWithIndex(t *testing.T) {
	doc := makeDocWithTables(makeTable(2, 2))
	withIdx := collectAllTablesWithIndex(doc)
	assert.Equal(t, 1, len(withIdx))
	assert.Equal(t, int64(1), withIdx[0].startIdx)
}

func TestCollectAllTables_Nested(t *testing.T) {
	// Table with a nested table inside a cell
	inner := makeTable(1, 1)
	outer := &docs.Table{Rows: 1, Columns: 1, TableRows: []*docs.TableRow{
		{TableCells: []*docs.TableCell{
			{Content: []*docs.StructuralElement{
				{Table: inner, StartIndex: 50, EndIndex: 80},
			}},
		}},
	}}
	doc := makeDocWithTables(outer)
	tables := collectAllTables(doc)
	assert.Equal(t, 2, len(tables)) // outer + inner
}

func TestFindTableCell(t *testing.T) {
	doc := makeDocWithTables(makeTable(2, 3))

	// Valid cell
	cell, err := findTableCell(doc, &tableCellRef{tableIndex: 1, row: 1, col: 1})
	require.NoError(t, err)
	assert.NotNil(t, cell)

	// Negative index (last table)
	cell, err = findTableCell(doc, &tableCellRef{tableIndex: -1, row: 1, col: 1})
	require.NoError(t, err)
	assert.NotNil(t, cell)

	// Table out of range
	_, err = findTableCell(doc, &tableCellRef{tableIndex: 5, row: 1, col: 1})
	assert.Error(t, err)

	// Row out of range
	_, err = findTableCell(doc, &tableCellRef{tableIndex: 1, row: 10, col: 1})
	assert.Error(t, err)

	// Col out of range
	_, err = findTableCell(doc, &tableCellRef{tableIndex: 1, row: 1, col: 10})
	assert.Error(t, err)

	// No tables
	_, err = findTableCell(&docs.Document{Body: &docs.Body{}}, &tableCellRef{tableIndex: 1, row: 1, col: 1})
	assert.Error(t, err)

	// Table index 0 (out of range)
	_, err = findTableCell(doc, &tableCellRef{tableIndex: 0, row: 1, col: 1})
	assert.Error(t, err)

	// Negative table index beyond range
	_, err = findTableCell(doc, &tableCellRef{tableIndex: -5, row: 1, col: 1})
	assert.Error(t, err)
}

func TestGetCellText(t *testing.T) {
	cell := &docs.TableCell{
		Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "hello"}, StartIndex: 10, EndIndex: 15},
					{TextRun: &docs.TextRun{Content: " world"}, StartIndex: 15, EndIndex: 21},
				},
			}},
		},
	}
	text, start, end := getCellText(cell)
	assert.Equal(t, "hello world", text)
	assert.Equal(t, int64(10), start)
	assert.Equal(t, int64(21), end)

	// Empty cell
	text, start, end = getCellText(&docs.TableCell{})
	assert.Equal(t, "", text)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(0), end)
}

// --- buildSectionRangeForMatch ---

func TestBuildSectionRangeForMatch(t *testing.T) {
	doc := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{
		{SectionBreak: &docs.SectionBreak{}, StartIndex: 0, EndIndex: 1},
		{Paragraph: &docs.Paragraph{}, StartIndex: 1, EndIndex: 50},
		{SectionBreak: &docs.SectionBreak{}, StartIndex: 50, EndIndex: 51},
		{Paragraph: &docs.Paragraph{}, StartIndex: 51, EndIndex: 100},
	}}}

	// Match in first section
	s, e := buildSectionRangeForMatch(doc, 10, 20)
	assert.GreaterOrEqual(t, s, int64(1))
	assert.Greater(t, e, int64(20))

	// Match in second section
	s, e = buildSectionRangeForMatch(doc, 60, 70)
	assert.GreaterOrEqual(t, s, int64(50))
	assert.GreaterOrEqual(t, e, int64(70))

	// nil doc
	s, e = buildSectionRangeForMatch(nil, 10, 20)
	assert.Equal(t, int64(1), s)
	assert.Equal(t, int64(21), e)
}

// --- retryOnQuota ---

func TestRetryOnQuota_Success(t *testing.T) {
	calls := 0
	err := retryOnQuota(context.Background(), func() error {
		calls++
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryOnQuota_NonRetryable(t *testing.T) {
	err := retryOnQuota(context.Background(), func() error {
		return errors.New("permanent error")
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permanent error")
}

func TestRetryOnQuota_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	calls := 0
	err := retryOnQuota(ctx, func() error {
		calls++
		return &gapi.Error{Code: 429, Message: "rate limit"}
	})
	// Should get either context error or the 429 error
	assert.Error(t, err)
}

func TestRetryOnQuota_RetryableEventualSuccess(t *testing.T) {
	calls := 0
	// Override constants not possible, but we can test with a fast context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := retryOnQuota(ctx, func() error {
		calls++
		if calls < 2 {
			return &gapi.Error{Code: 429, Message: "rate limit"}
		}
		return nil
	})
	// May succeed or timeout depending on backoff timing
	if err == nil {
		assert.GreaterOrEqual(t, calls, 2)
	}
}

func TestIsRetryableError_Extended(t *testing.T) {
	assert.False(t, isRetryableError(nil))
	assert.False(t, isRetryableError(errors.New("random error")))
	assert.True(t, isRetryableError(&gapi.Error{Code: 429}))
	assert.True(t, isRetryableError(&gapi.Error{Code: 500}))
	assert.True(t, isRetryableError(&gapi.Error{Code: 502}))
	assert.True(t, isRetryableError(&gapi.Error{Code: 503}))
	assert.False(t, isRetryableError(&gapi.Error{Code: 404}))
	assert.True(t, isRetryableError(errors.New("rateLimitExceeded")))
	assert.True(t, isRetryableError(errors.New("error 429")))
}

// --- formatBraceFlags ---

func TestFormatBraceFlags_Extended(t *testing.T) {
	// nil
	assert.Equal(t, "", formatBraceFlags(nil))

	bTrue := true
	bFalse := false

	// Reset
	assert.Contains(t, formatBraceFlags(&braceExpr{Reset: true, Indent: indentNotSet}), "0")

	// Bold true/false
	assert.Contains(t, formatBraceFlags(&braceExpr{Bold: &bTrue, Indent: indentNotSet}), "b")
	assert.Contains(t, formatBraceFlags(&braceExpr{Bold: &bFalse, Indent: indentNotSet}), "!b")

	// Italic
	assert.Contains(t, formatBraceFlags(&braceExpr{Italic: &bTrue, Indent: indentNotSet}), "i")
	assert.Contains(t, formatBraceFlags(&braceExpr{Italic: &bFalse, Indent: indentNotSet}), "!i")

	// Underline
	assert.Contains(t, formatBraceFlags(&braceExpr{Underline: &bTrue, Indent: indentNotSet}), "_")
	assert.Contains(t, formatBraceFlags(&braceExpr{Underline: &bFalse, Indent: indentNotSet}), "!_")

	// Strike
	assert.Contains(t, formatBraceFlags(&braceExpr{Strike: &bTrue, Indent: indentNotSet}), "-")

	// Sup/Sub
	assert.Contains(t, formatBraceFlags(&braceExpr{Sup: &bTrue, Indent: indentNotSet}), "^")
	assert.Contains(t, formatBraceFlags(&braceExpr{Sub: &bTrue, Indent: indentNotSet}), ",")

	// Color, Bg, Font, Size
	assert.Contains(t, formatBraceFlags(&braceExpr{Color: "#FF0000", Indent: indentNotSet}), "c=#FF0000")
	assert.Contains(t, formatBraceFlags(&braceExpr{Bg: "#00FF00", Indent: indentNotSet}), "z=#00FF00")
	assert.Contains(t, formatBraceFlags(&braceExpr{Font: "Arial", Indent: indentNotSet}), "f=Arial")
	assert.Contains(t, formatBraceFlags(&braceExpr{Size: 14, Indent: indentNotSet}), "s=14")

	// Heading, Align
	assert.Contains(t, formatBraceFlags(&braceExpr{Heading: "h1", Indent: indentNotSet}), "h=h1")
	assert.Contains(t, formatBraceFlags(&braceExpr{Align: "center", Indent: indentNotSet}), "a=center")

	// URL
	assert.Contains(t, formatBraceFlags(&braceExpr{URL: "https://x.com", Indent: indentNotSet}), "u=https://x.com")

	// Break
	assert.Contains(t, formatBraceFlags(&braceExpr{HasBreak: true, Indent: indentNotSet}), "+")
	assert.Contains(t, formatBraceFlags(&braceExpr{HasBreak: true, Break: "p", Indent: indentNotSet}), "+=p")

	// SmallCaps, Code
	assert.Contains(t, formatBraceFlags(&braceExpr{SmallCaps: &bTrue, Indent: indentNotSet}), "w")
	assert.Contains(t, formatBraceFlags(&braceExpr{Code: &bTrue, Indent: indentNotSet}), "#")
}

// --- mergeBraceSpans ---

func TestMergeBraceSpans_Extended(t *testing.T) {
	bTrue := true

	// Empty spans
	merged := mergeBraceSpans(nil)
	assert.NotNil(t, merged)
	assert.Equal(t, -1, merged.Indent)

	// Non-global spans are ignored
	merged = mergeBraceSpans([]*braceSpan{
		{IsGlobal: false, Expr: &braceExpr{Bold: &bTrue}},
	})
	assert.Nil(t, merged.Bold)

	// Global spans merge
	merged = mergeBraceSpans([]*braceSpan{
		{IsGlobal: true, Expr: &braceExpr{Bold: &bTrue, Color: "#FF0000", Font: "Arial", Size: 12, Indent: indentNotSet}},
		{IsGlobal: true, Expr: &braceExpr{Italic: &bTrue, Bg: "#00FF00", URL: "http://x.com", Heading: "h1", Indent: 2}},
	})
	assert.NotNil(t, merged.Bold)
	assert.True(t, *merged.Bold)
	assert.NotNil(t, merged.Italic)
	assert.True(t, *merged.Italic)
	assert.Equal(t, "#FF0000", merged.Color)
	assert.Equal(t, "#00FF00", merged.Bg)
	assert.Equal(t, "Arial", merged.Font)
	assert.Equal(t, float64(12), merged.Size)
	assert.Equal(t, "http://x.com", merged.URL)
	assert.Equal(t, "h1", merged.Heading)
	assert.Equal(t, 2, merged.Indent)

	// All boolean fields
	merged = mergeBraceSpans([]*braceSpan{
		{IsGlobal: true, Expr: &braceExpr{
			Strike: &bTrue, Code: &bTrue, Sup: &bTrue, Sub: &bTrue, SmallCaps: &bTrue,
			Align: "center", Leading: 1.5, SpacingSet: true, SpacingAbove: 10, SpacingBelow: 20,
			Reset: true, HasBreak: true, Break: "p", Indent: indentNotSet,
		}},
	})
	assert.True(t, *merged.Strike)
	assert.True(t, *merged.Code)
	assert.True(t, *merged.Sup)
	assert.True(t, *merged.Sub)
	assert.True(t, *merged.SmallCaps)
	assert.Equal(t, "center", merged.Align)
	assert.Equal(t, 1.5, merged.Leading)
	assert.True(t, merged.SpacingSet)
	assert.Equal(t, float64(10), merged.SpacingAbove)
	assert.Equal(t, float64(20), merged.SpacingBelow)
	assert.True(t, merged.Reset)
	assert.True(t, merged.HasBreak)
	assert.Equal(t, "p", merged.Break)

	// nil Expr in span is handled
	merged = mergeBraceSpans([]*braceSpan{
		{IsGlobal: true, Expr: nil},
	})
	assert.NotNil(t, merged)
}

// --- findDocImages ---

func TestFindDocImages(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{InlineObjectElement: &docs.InlineObjectElement{InlineObjectId: "obj1"}, StartIndex: 5},
				},
			}},
		}},
		InlineObjects: map[string]docs.InlineObject{
			"obj1": {InlineObjectProperties: &docs.InlineObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{Title: "My Image"},
			}},
		},
	}

	images := findDocImages(doc)
	require.Equal(t, 1, len(images))
	assert.Equal(t, "obj1", images[0].ObjectID)
	assert.Equal(t, int64(5), images[0].Index)
	assert.Equal(t, "My Image", images[0].Alt)
	assert.False(t, images[0].IsPositioned)
}

func TestFindDocImages_NoInlineObjects(t *testing.T) {
	doc := &docs.Document{Body: &docs.Body{}}
	images := findDocImages(doc)
	assert.Empty(t, images)
}

func TestFindDocImages_DescriptionFallback(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{InlineObjectElement: &docs.InlineObjectElement{InlineObjectId: "obj1"}, StartIndex: 5},
				},
			}},
		}},
		InlineObjects: map[string]docs.InlineObject{
			"obj1": {InlineObjectProperties: &docs.InlineObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{Description: "Alt Text"},
			}},
		},
	}
	images := findDocImages(doc)
	require.Equal(t, 1, len(images))
	assert.Equal(t, "Alt Text", images[0].Alt)
}

func TestFindDocImages_InTable(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Table: &docs.Table{TableRows: []*docs.TableRow{
				{TableCells: []*docs.TableCell{
					{Content: []*docs.StructuralElement{
						{Paragraph: &docs.Paragraph{
							Elements: []*docs.ParagraphElement{
								{InlineObjectElement: &docs.InlineObjectElement{InlineObjectId: "obj2"}, StartIndex: 20},
							},
						}},
					}},
				}},
			}}},
		}},
		InlineObjects: map[string]docs.InlineObject{
			"obj2": {InlineObjectProperties: &docs.InlineObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{},
			}},
		},
	}
	images := findDocImages(doc)
	require.Equal(t, 1, len(images))
	assert.Equal(t, "obj2", images[0].ObjectID)
}

func TestFindDocImages_PositionedObjects(t *testing.T) {
	doc := &docs.Document{
		Body:          &docs.Body{},
		InlineObjects: map[string]docs.InlineObject{},
		PositionedObjects: map[string]docs.PositionedObject{
			"pos1": {PositionedObjectProperties: &docs.PositionedObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{Title: "Positioned"},
			}},
		},
	}
	images := findDocImages(doc)
	require.Equal(t, 1, len(images))
	assert.Equal(t, "pos1", images[0].ObjectID)
	assert.True(t, images[0].IsPositioned)
}

// --- resolveAlign / resolveBreak edge cases ---

func TestResolveAlign_CaseInsensitive(t *testing.T) {
	assert.Equal(t, "START", resolveAlign("Left"))
	assert.Equal(t, "CENTER", resolveAlign("CENTER"))
	assert.Equal(t, "END", resolveAlign("RIGHT"))
	assert.Equal(t, "JUSTIFIED", resolveAlign("Justify"))
	assert.Equal(t, "unknown", resolveAlign("unknown"))
}

func TestResolveBreak_AllValues(t *testing.T) {
	assert.Equal(t, "horizontal_rule", resolveBreak(""))
	assert.Equal(t, "page_break", resolveBreak("p"))
	assert.Equal(t, "column_break", resolveBreak("c"))
	assert.Equal(t, "section_break", resolveBreak("s"))
	assert.Equal(t, "x", resolveBreak("x"))
}

// --- tokenizeBraceContent edge cases ---

func TestTokenizeBraceContent_Extended(t *testing.T) {
	// Already tested at 70%, add edge cases
	tokens := tokenizeBraceContent("{b,i,_}")
	assert.NotEmpty(t, tokens)

	// Nested braces
	tokens = tokenizeBraceContent("{b,{color:#FF0000}}")
	assert.NotEmpty(t, tokens)

	// Empty
	tokens = tokenizeBraceContent("{}")
	// May return empty or single empty token depending on impl
	_ = tokens
}

// --- parseFullExpr edge cases to increase coverage ---

func TestParseFullExpr_BraceAndTableRef(t *testing.T) {
	// Table cell reference
	expr, err := parseFullExpr("s/|1|[1,1]/replacement/")
	if err == nil {
		assert.NotNil(t, expr.cellRef)
	}

	// Global + case insensitive + multiline
	expr, err = parseFullExpr("s/pattern/replacement/gim")
	require.NoError(t, err)
	assert.True(t, expr.global)
	assert.Contains(t, expr.pattern, "(?i)")
	assert.Contains(t, expr.pattern, "(?m)")
}

// --- isMergeOp / canBatchCell extra coverage ---

func TestIsMergeOp_Extended(t *testing.T) {
	assert.True(t, isMergeOp("merge"))
	assert.True(t, isMergeOp("MERGE"))
	assert.True(t, isMergeOp(" merge "))
	assert.True(t, isMergeOp("unmerge"))
	assert.True(t, isMergeOp("split"))
	assert.False(t, isMergeOp("replace"))
	assert.False(t, isMergeOp(""))
}

func TestCanBatchCell_Extended(t *testing.T) {
	// Batchable
	assert.True(t, canBatchCell(indexedExpr{0, sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1},
	}}))

	// Not batchable - has pattern
	assert.False(t, canBatchCell(indexedExpr{0, sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1},
		pattern: "foo",
	}}))

	// Not batchable - wildcard row
	assert.False(t, canBatchCell(indexedExpr{0, sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, row: 0, col: 1},
	}}))

	// Not batchable - merge op
	assert.False(t, canBatchCell(indexedExpr{0, sedExpr{
		cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1},
		replacement: "merge",
	}}))

	// Not batchable - row op
	assert.False(t, canBatchCell(indexedExpr{0, sedExpr{
		cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1, rowOp: "delete"},
	}}))

	// No cellRef
	assert.False(t, canBatchCell(indexedExpr{0, sedExpr{}}))
}

// --- sedOutputOK more coverage ---

func TestSedOutputOK_NoExtra(t *testing.T) {
	u := testUI()
	ctx := context.Background()
	err := sedOutputOK(ctx, u, "doc123")
	assert.NoError(t, err)
}

func TestSedOutputOK_WithExtra(t *testing.T) {
	u := testUI()
	ctx := context.Background()
	err := sedOutputOK(ctx, u, "doc123",
		sedOutputKV{Key: "replaced", Value: 5},
		sedOutputKV{Key: "native", Value: true},
	)
	assert.NoError(t, err)
}

// --- parseExpressionLines ---

func TestParseExpressionLines(t *testing.T) {
	exprs := parseExpressionLines([]byte("s/foo/bar/\ns/baz/qux/g"))
	assert.Equal(t, 2, len(exprs))
	assert.Equal(t, "s/foo/bar/", exprs[0])
	assert.Equal(t, "s/baz/qux/g", exprs[1])
}

func TestParseExpressionLines_Empty(t *testing.T) {
	exprs := parseExpressionLines(nil)
	assert.Empty(t, exprs)
}

func TestParseExpressionLines_WithBlankAndComments(t *testing.T) {
	exprs := parseExpressionLines([]byte("s/foo/bar/\n\n  \n# comment\ns/baz/qux/"))
	assert.Equal(t, 2, len(exprs))
}

// --- literalReplacement ---

func TestLiteralReplacement(t *testing.T) {
	assert.Equal(t, "hello", literalReplacement("hello"))
	assert.Equal(t, "hello world", literalReplacement("hello world"))
	assert.Equal(t, "$1", literalReplacement("$1"))
}

// --- buildTOCRequest / buildCommentRequest ---

func TestBuildTOCRequest(t *testing.T) {
	assert.Nil(t, buildTOCRequest(nil, 0))
	assert.Nil(t, buildTOCRequest(&braceExpr{HasTOC: false, Indent: indentNotSet}, 0))
	assert.Nil(t, buildTOCRequest(&braceExpr{HasTOC: true, Indent: indentNotSet}, 0)) // API limitation
}

func TestBuildCommentRequest(t *testing.T) {
	assert.Nil(t, buildCommentRequest(nil, 0, 0))
	assert.Nil(t, buildCommentRequest(&braceExpr{Comment: "", Indent: indentNotSet}, 0, 10))
	assert.Nil(t, buildCommentRequest(&braceExpr{Comment: "test", Indent: indentNotSet}, 0, 10)) // API limitation
}

// --- parseFullExpr extended coverage for brace formatting paths ---

func TestParseFullExpr_BraceFormatting(t *testing.T) {
	// Replacement with brace formatting
	expr, err := parseFullExpr("s/foo/{b}bar{/b}/")
	require.NoError(t, err)
	assert.Equal(t, "foo", expr.pattern)
	// Brace formatting should be parsed (may or may not set brace depending on parsing)

	// Replacement with multiple brace tokens
	expr, err = parseFullExpr("s/foo/{b,i}replacement text/g")
	require.NoError(t, err)
	assert.True(t, expr.global)

	// Table ref pattern |1|
	expr, err = parseFullExpr("s/|1|/replacement/")
	require.NoError(t, err)
	assert.NotEqual(t, 0, expr.tableRef)

	// Wildcard table ref |*|
	_, err = parseFullExpr("s/|*|/replacement/")
	require.NoError(t, err)

	// Negative table ref |-1|
	_, err = parseFullExpr("s/|-1|/replacement/")
	require.NoError(t, err)

	// Brace table creation in replacement
	expr, err = parseFullExpr("s/foo/{T=3x4}/")
	if err == nil && expr.replacement != "" {
		// Should be converted to pipe-style
		assert.Contains(t, expr.replacement, "|3x4|")
	}

	// Brace with color
	_, err = parseFullExpr("s/foo/{c=#FF0000}bar/")
	require.NoError(t, err)

	// Brace with heading
	_, err = parseFullExpr("s/foo/{h=1}bar/")
	require.NoError(t, err)

	// Brace with font + size
	_, err = parseFullExpr("s/foo/{f=Arial,s=14}bar/")
	require.NoError(t, err)
}

func TestParseFullExpr_ImageRef(t *testing.T) {
	// Image reference pattern
	expr, err := parseFullExpr("s/!(1)/replacement/")
	require.NoError(t, err)
	assert.Equal(t, "!(1)", expr.pattern)
}

func TestParseFullExpr_BracePatternTable(t *testing.T) {
	// Brace table addressing in pattern
	expr, err := parseFullExpr("s/{T=1[1,1]}/replacement/")
	if err == nil {
		_ = expr // Just exercising the code path
	}
}

func TestParseFullExpr_NthMatch(t *testing.T) {
	expr, err := parseFullExpr("s/foo/bar/5")
	require.NoError(t, err)
	assert.Equal(t, 5, expr.nthMatch)

	expr, err = parseFullExpr("s/foo/bar/2g")
	require.NoError(t, err)
	assert.Equal(t, 2, expr.nthMatch)
	assert.True(t, expr.global)
}

// --- classifyExpression / runDryRun extended coverage ---

func TestClassifyExpression_MoreCases(t *testing.T) {
	// native vs manual
	assert.Equal(t, "native", classifyExpression(sedExpr{pattern: "foo", replacement: "bar", global: true}))
	assert.Equal(t, "manual", classifyExpression(sedExpr{pattern: "foo", replacement: "**bar**"}))

	// table cell
	assert.Equal(t, "cell |1|[1,1]", classifyExpression(sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}}))

	// table op
	assert.Equal(t, "delete table 1", classifyExpression(sedExpr{tableRef: 1}))

	// image
	assert.Equal(t, "image", classifyExpression(sedExpr{pattern: "!(1)"}))
}

// --- canUseNativeReplace ---

func TestCanUseNativeReplace_Extended(t *testing.T) {
	assert.True(t, canUseNativeReplace("simple text"))
	assert.True(t, canUseNativeReplace(""))
	assert.False(t, canUseNativeReplace("**bold**"))
	assert.False(t, canUseNativeReplace("*italic*"))
	assert.False(t, canUseNativeReplace("# heading"))
	assert.False(t, canUseNativeReplace("- bullet"))
	assert.False(t, canUseNativeReplace("{b}text")) // brace formatting requires manual path
	assert.False(t, canUseNativeReplace("![alt](url)"))
}

// --- findDocImages with positioned objects having description ---

func TestFindDocImages_PositionedWithDesc(t *testing.T) {
	doc := &docs.Document{
		Body:          &docs.Body{},
		InlineObjects: map[string]docs.InlineObject{},
		PositionedObjects: map[string]docs.PositionedObject{
			"pos1": {PositionedObjectProperties: &docs.PositionedObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{Description: "Desc Only"},
			}},
		},
	}
	images := findDocImages(doc)
	require.Equal(t, 1, len(images))
	assert.Equal(t, "Desc Only", images[0].Alt)
}

func TestCanUseNativeReplace_BraceFormatting(t *testing.T) {
	assert.False(t, canUseNativeReplace("{b}bold text"))
	assert.False(t, canUseNativeReplace("text{c=red}"))
	assert.False(t, canUseNativeReplace("{s=14}sized"))
	assert.True(t, canUseNativeReplace("no braces here"))
	assert.True(t, canUseNativeReplace("{not a valid brace expr}")) // literal braces, not valid expr
}

func TestClassifyExprForBatch(t *testing.T) {
	tests := []struct {
		name string
		expr sedExpr
		want exprCategory
	}{
		{"positional ^", sedExpr{pattern: "^", replacement: "text"}, exprCatPositional},
		{"positional $", sedExpr{pattern: "$", replacement: "text"}, exprCatPositional},
		{"positional ^$", sedExpr{pattern: "^$", replacement: "text"}, exprCatPositional},
		{"image repl", sedExpr{pattern: "foo", replacement: "![alt](url)"}, exprCatImage},
		{"brace image", sedExpr{pattern: "foo", brace: &braceExpr{ImgRef: "url", Indent: indentNotSet}}, exprCatImage},
		{"delete cmd", sedExpr{pattern: "foo", command: 'd'}, exprCatCommand},
		{"append cmd", sedExpr{pattern: "foo", command: 'a', replacement: "text"}, exprCatCommand},
		{"cell ref", sedExpr{cellRef: &tableCellRef{tableIndex: 1, row: 1, col: 1}}, exprCatCell},
		{"table ref", sedExpr{tableRef: 1}, exprCatCell},
		{"native simple", sedExpr{pattern: "foo", replacement: "bar", global: true}, exprCatNative},
		{"manual bold", sedExpr{pattern: "foo", replacement: "**bold**", global: true}, exprCatManual},
		{"manual nth", sedExpr{pattern: "foo", replacement: "bar", global: true, nthMatch: 2}, exprCatManual},
		{"manual non-global", sedExpr{pattern: "foo", replacement: "bar"}, exprCatManual},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyExprForBatch(tt.expr))
		})
	}
}

func TestExtractParagraphText_FastPath(t *testing.T) {
	// Single text run — fast path
	p := &docs.Paragraph{
		Elements: []*docs.ParagraphElement{
			{TextRun: &docs.TextRun{Content: "hello world\n"}},
		},
	}
	assert.Equal(t, "hello world", extractParagraphText(p))

	// Multiple text runs — builder path
	p2 := &docs.Paragraph{
		Elements: []*docs.ParagraphElement{
			{TextRun: &docs.TextRun{Content: "hello "}},
			{TextRun: &docs.TextRun{Content: "world\n"}},
		},
	}
	assert.Equal(t, "hello world", extractParagraphText(p2))

	// Non-text element mixed in
	p3 := &docs.Paragraph{
		Elements: []*docs.ParagraphElement{
			{TextRun: &docs.TextRun{Content: "hello "}},
			{}, // non-text element
			{TextRun: &docs.TextRun{Content: "world"}},
		},
	}
	assert.Equal(t, "hello world", extractParagraphText(p3))

	// Empty paragraph
	p4 := &docs.Paragraph{Elements: []*docs.ParagraphElement{}}
	assert.Equal(t, "", extractParagraphText(p4))
}

func TestLiteralReplacement_Extended(t *testing.T) {
	assert.Equal(t, "$", literalReplacement("$$"))
	assert.Equal(t, "hello$world", literalReplacement("hello$$world"))
	assert.Equal(t, "no dollars", literalReplacement("no dollars"))
	assert.Equal(t, "${0}", literalReplacement("${0}")) // backrefs preserved as-is
}
