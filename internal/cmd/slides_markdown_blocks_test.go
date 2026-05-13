package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
