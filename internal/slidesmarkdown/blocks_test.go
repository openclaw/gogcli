package slidesmarkdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBlocks_Paragraph(t *testing.T) {
	got := parseBlocks("Hello world.\n")
	assert.Equal(t, []Block{
		ParagraphBlock{Inlines: []Inline{TextRun{Text: "Hello world."}}},
	}, got)
}

func TestParseBlocks_BulletList(t *testing.T) {
	got := parseBlocks("- one\n- two **bold**\n- three\n")
	assert.Equal(t, []Block{
		BulletsBlock{Items: []BulletItem{
			{Indent: 0, Inlines: []Inline{TextRun{Text: "one"}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "two "}, TextRun{Text: "bold", Bold: true}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "three"}}},
		}},
	}, got)
}

func TestParseBlocks_OrderedList(t *testing.T) {
	got := parseBlocks("1. first\n2. second\n")
	assert.Equal(t, []Block{
		BulletsBlock{Ordered: true, Items: []BulletItem{
			{Indent: 0, Inlines: []Inline{TextRun{Text: "first"}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "second"}}},
		}},
	}, got)
}

func TestParseBlocks_CodeBlock(t *testing.T) {
	input := "```go\nfunc main() {}\n```\n"
	got := parseBlocks(input)
	assert.Equal(t, []Block{
		CodeBlock{Lang: "go", Source: "func main() {}"},
	}, got)
}

func TestParseBlocks_Heading(t *testing.T) {
	got := parseBlocks("### Subsection\n")
	assert.Equal(t, []Block{
		HeadingBlock{Level: 3, Inlines: []Inline{TextRun{Text: "Subsection"}}},
	}, got)
}

func TestParseBlocks_BareHeading(t *testing.T) {
	got := parseBlocks("##\n")
	assert.Equal(t, []Block{
		HeadingBlock{Level: 2},
	}, got)
}

func TestParseBlocks_Mixed(t *testing.T) {
	input := "## Topic\n\nIntro paragraph.\n\n- bullet 1\n- bullet 2\n\nFollowup.\n"
	got := parseBlocks(input)
	assert.Equal(t, []Block{
		HeadingBlock{Level: 2, Inlines: []Inline{TextRun{Text: "Topic"}}},
		ParagraphBlock{Inlines: []Inline{TextRun{Text: "Intro paragraph."}}},
		BulletsBlock{Items: []BulletItem{
			{Inlines: []Inline{TextRun{Text: "bullet 1"}}},
			{Inlines: []Inline{TextRun{Text: "bullet 2"}}},
		}},
		ParagraphBlock{Inlines: []Inline{TextRun{Text: "Followup."}}},
	}, got)
}

func TestParseBlocks_TwoColumns(t *testing.T) {
	input := "::cols::\n\nleft side text\n\n::col2::\n\nright side text\n\n::/cols::\n"
	got := parseBlocks(input)
	assert.Equal(t, []Block{
		ColumnsBlock{Columns: [][]Block{
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "left side text"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "right side text"}}}},
		}},
	}, got)
}

func TestParseBlocks_ThreeColumns(t *testing.T) {
	input := "::cols::\n\nA\n\n::col2::\n\nB\n\n::col3::\n\nC\n\n::/cols::\n"
	got := parseBlocks(input)
	assert.Equal(t, []Block{
		ColumnsBlock{Columns: [][]Block{
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "A"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "B"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "C"}}}},
		}},
	}, got)
}

func TestParseBlocks_MermaidBlock(t *testing.T) {
	input := "```mermaid\nflowchart LR\n  A --> B\n```\n"
	got := parseBlocks(input)
	require.Equal(t, 1, len(got))
	d, ok := got[0].(DiagramBlock)
	require.True(t, ok)
	assert.Equal(t, "mermaid", d.Kind)
	assert.Equal(t, "flowchart LR\n  A --> B", d.Source)
	assert.NotEmpty(t, d.ID)
}

func TestParseBlocks_RightSynonymForCol2(t *testing.T) {
	input := "::cols::\n\nA\n\n::right::\n\nB\n\n::/cols::\n"
	got := parseBlocks(input)
	require.Equal(t, 1, len(got))
	col, ok := got[0].(ColumnsBlock)
	assert.True(t, ok)
	assert.Equal(t, 2, len(col.Columns))
}

func TestParseBlocks_ColumnMarkersInsideFenceStayCode(t *testing.T) {
	input := "::cols::\n\n```md\n::right::\n```\n\n::right::\n\nright side\n\n::/cols::\n"
	got := parseBlocks(input)
	require.Equal(t, 1, len(got))
	col, ok := got[0].(ColumnsBlock)
	require.True(t, ok)
	require.Equal(t, 2, len(col.Columns))
	require.Equal(t, 1, len(col.Columns[0]))
	code, ok := col.Columns[0][0].(CodeBlock)
	require.True(t, ok)
	assert.Equal(t, "::right::", code.Source)
	assert.Equal(t, "right side", blocksToPlainText(col.Columns[1]))
}

func TestParseBlocks_ColumnMarkersInsideTildeFenceStayCode(t *testing.T) {
	input := "::cols::\n\n~~~md\n::right::\n~~~\n\n::right::\n\nright side\n\n::/cols::\n"
	got := parseBlocks(input)
	require.Equal(t, 1, len(got))
	col, ok := got[0].(ColumnsBlock)
	require.True(t, ok)
	require.Equal(t, 2, len(col.Columns))
	require.Equal(t, 1, len(col.Columns[0]))
	code, ok := col.Columns[0][0].(CodeBlock)
	require.True(t, ok)
	assert.Equal(t, "::right::", code.Source)
	assert.Equal(t, "right side", blocksToPlainText(col.Columns[1]))
}

func TestParseBlocks_MismatchedFenceMarkersStayCode(t *testing.T) {
	input := "::cols::\n\n````md\n~~~\n::right::\n```\n::boxes::\n````\n\n::right::\n\nright side\n\n::/cols::\n"
	got := parseBlocks(input)
	require.Equal(t, 1, len(got))
	col, ok := got[0].(ColumnsBlock)
	require.True(t, ok)
	require.Equal(t, 2, len(col.Columns))
	require.Equal(t, 1, len(col.Columns[0]))
	code, ok := col.Columns[0][0].(CodeBlock)
	require.True(t, ok)
	assert.Equal(t, "~~~\n::right::\n```\n::boxes::", code.Source)
	assert.Equal(t, "right side", blocksToPlainText(col.Columns[1]))
}

func TestParseBlocks_IconRows(t *testing.T) {
	input := "::boxes::\n:fa-headset: Support Tickets\n:fab-github: GitHub\n::/boxes::\n"
	got := parseBlocks(input)
	assert.Equal(t, []Block{
		IconRowsBlock{Kind: "boxes", Rows: []IconRow{
			{Icon: &IconRef{Style: "solid", Name: "headset"}, Text: "Support Tickets"},
			{Icon: &IconRef{Style: "brands", Name: "github"}, Text: "GitHub"},
		}},
	}, got)
}

func TestParseBlocks_ArrowRowsStripHeadingMarkers(t *testing.T) {
	input := "::arrows::\n### Screen-scrape legacy systems.\n### Pray nothing leaks.\n::/arrows::\n"
	got := parseBlocks(input)
	assert.Equal(t, []Block{
		IconRowsBlock{Kind: "arrows", Rows: []IconRow{
			{Text: "Screen-scrape legacy systems."},
			{Text: "Pray nothing leaks."},
		}},
	}, got)
}
