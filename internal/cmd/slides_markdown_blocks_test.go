package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBlocks_Paragraph(t *testing.T) {
	got := parseBlocks("Hello world.\n", "solid")
	assert.Equal(t, []Block{
		ParagraphBlock{Inlines: []Inline{TextRun{Text: "Hello world."}}},
	}, got)
}

func TestParseBlocks_BulletList(t *testing.T) {
	got := parseBlocks("- one\n- two **bold**\n- three\n", "solid")
	assert.Equal(t, []Block{
		BulletsBlock{Items: []BulletItem{
			{Indent: 0, Inlines: []Inline{TextRun{Text: "one"}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "two "}, TextRun{Text: "bold", Bold: true}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "three"}}},
		}},
	}, got)
}

func TestParseBlocks_OrderedList(t *testing.T) {
	got := parseBlocks("1. first\n2. second\n", "solid")
	assert.Equal(t, []Block{
		BulletsBlock{Ordered: true, Items: []BulletItem{
			{Indent: 0, Inlines: []Inline{TextRun{Text: "first"}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "second"}}},
		}},
	}, got)
}

func TestParseBlocks_CodeBlock(t *testing.T) {
	input := "```go\nfunc main() {}\n```\n"
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		CodeBlock{Lang: "go", Source: "func main() {}"},
	}, got)
}

func TestParseBlocks_Heading(t *testing.T) {
	got := parseBlocks("### Subsection\n", "solid")
	assert.Equal(t, []Block{
		HeadingBlock{Level: 3, Inlines: []Inline{TextRun{Text: "Subsection"}}},
	}, got)
}

func TestParseBlocks_Mixed(t *testing.T) {
	input := "## Topic\n\nIntro paragraph.\n\n- bullet 1\n- bullet 2\n\nFollowup.\n"
	got := parseBlocks(input, "solid")
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
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		ColumnsBlock{Columns: [][]Block{
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "left side text"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "right side text"}}}},
		}},
	}, got)
}

func TestParseBlocks_ThreeColumns(t *testing.T) {
	input := "::cols::\n\nA\n\n::col2::\n\nB\n\n::col3::\n\nC\n\n::/cols::\n"
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		ColumnsBlock{Columns: [][]Block{
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "A"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "B"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "C"}}}},
		}},
	}, got)
}

func TestParseBlocks_RightSynonymForCol2(t *testing.T) {
	input := "::cols::\n\nA\n\n::right::\n\nB\n\n::/cols::\n"
	got := parseBlocks(input, "solid")
	require.Equal(t, 1, len(got))
	col, ok := got[0].(ColumnsBlock)
	assert.True(t, ok)
	assert.Equal(t, 2, len(col.Columns))
}
