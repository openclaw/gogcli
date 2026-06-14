package slidesmarkdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_TitleHoistFromH1(t *testing.T) {
	input := "# Hello\n\nbody text\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "Hello", got[0].Title)
	require.Equal(t, 1, len(got[0].Body))
	assert.IsType(t, ParagraphBlock{}, got[0].Body[0])
}

func TestParse_TitleFallbackToH2(t *testing.T) {
	input := "## Topic Heading\n\n- a\n- b\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "Topic Heading", got[0].Title)
}

func TestParse_HeroLayoutKeepsH1InBody(t *testing.T) {
	input := "---\nlayout: hero\n---\n\n# Big Wordmark\n\nsubline\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "", got[0].Title, "title should not be hoisted on hero")
	require.GreaterOrEqual(t, len(got[0].Body), 1)
	first, ok := got[0].Body[0].(HeadingBlock)
	require.True(t, ok)
	assert.Equal(t, 1, first.Level)
}

func TestParse_NotesExtraction(t *testing.T) {
	input := "## Topic\n\nbody\n\n## Notes\n\n- speaker note one\n- speaker note two\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Contains(t, got[0].Notes, "speaker note one")
	assert.Contains(t, got[0].Notes, "speaker note two")

	for _, b := range got[0].Body {
		if h, ok := b.(HeadingBlock); ok && len(h.Inlines) > 0 {
			if tr, ok := h.Inlines[0].(TextRun); ok {
				assert.NotEqual(t, "Notes", tr.Text, "Notes heading should be removed from body")
			}
		}
	}
}

func TestParse_NotesStripsFAShortcodes(t *testing.T) {
	input := "## Topic\n\nbody\n\n## Notes\n\n:fa-truck-fast: Orders matter\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.NotContains(t, got[0].Notes, ":fa-truck-fast:")
	assert.Contains(t, got[0].Notes, "Orders matter")
}

func TestParse_NotesHeadingInsideFenceStaysBody(t *testing.T) {
	input := "## Topic\n\n```md\n## Notes\nkeep as code\n```\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Empty(t, got[0].Notes)
	require.Equal(t, 1, len(got[0].Body))
	code, ok := got[0].Body[0].(CodeBlock)
	require.True(t, ok)
	assert.Equal(t, "## Notes\nkeep as code", code.Source)
}

func TestParse_NotesHeadingInsideTildeFenceStaysBody(t *testing.T) {
	input := "## Topic\n\n~~~md\n## Notes\nkeep as code\n~~~\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Empty(t, got[0].Notes)
	require.Equal(t, 1, len(got[0].Body))
	code, ok := got[0].Body[0].(CodeBlock)
	require.True(t, ok)
	assert.Equal(t, "## Notes\nkeep as code", code.Source)
}

func TestParse_NotesHeadingInsideLongFenceStaysBody(t *testing.T) {
	input := "## Topic\n\n````md\n```\n## Notes\nkeep as code\n````\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Empty(t, got[0].Notes)
	require.Equal(t, 1, len(got[0].Body))
	code, ok := got[0].Body[0].(CodeBlock)
	require.True(t, ok)
	assert.Equal(t, "```\n## Notes\nkeep as code", code.Source)
}

func TestParse_DiagramIDsAreUniqueAndDeterministic(t *testing.T) {
	input := "## One\n\n```mermaid\ngraph TD\nA-->B\n```\n\n---\n\n## Two\n\n```mermaid\ngraph TD\nC-->D\n```\n"
	first, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	second, err := Parse(input, ParseOptions{})
	require.NoError(t, err)

	firstDiagrams := collectDiagrams(first)
	secondDiagrams := collectDiagrams(second)

	assert.Equal(t, map[string]string{
		"block-1": "graph TD\nA-->B",
		"block-2": "graph TD\nC-->D",
	}, firstDiagrams)
	assert.Equal(t, firstDiagrams, secondDiagrams)
}

func TestParse_ShorthandColumns(t *testing.T) {
	input := "---\nlayout: two-cols\n---\n\n## Topic\n\nleft\n\n::right::\n\nright\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "Topic", got[0].Title)
	require.Equal(t, 1, len(got[0].Body))
	cols, ok := got[0].Body[0].(ColumnsBlock)
	require.True(t, ok)
	require.Equal(t, 2, len(cols.Columns))
	assert.Equal(t, "left", blocksToPlainText(cols.Columns[0]))
	assert.Equal(t, "right", blocksToPlainText(cols.Columns[1]))
}

func TestParse_ShorthandMarkerInsideFenceStaysCode(t *testing.T) {
	input := "---\nlayout: two-cols\n---\n\n## Syntax\n\n```md\n::right::\n```\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, 1, len(got[0].Body))
	code, ok := got[0].Body[0].(CodeBlock)
	require.True(t, ok)
	assert.Equal(t, "::right::", code.Source)
}

func TestParse_ShorthandMarkerInsideTildeFenceStaysCode(t *testing.T) {
	input := "---\nlayout: two-cols\n---\n\n## Syntax\n\n~~~md\n::right::\n~~~\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, 1, len(got[0].Body))
	code, ok := got[0].Body[0].(CodeBlock)
	require.True(t, ok)
	assert.Equal(t, "::right::", code.Source)
}

func TestParse_ShorthandColumnsWithColsMentionInsideFence(t *testing.T) {
	input := "---\nlayout: two-cols\n---\n\n## Syntax\n\n```md\n::cols::\n```\n\nleft\n\n::right::\n\nright\n"
	got, err := Parse(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, 1, len(got[0].Body))
	cols, ok := got[0].Body[0].(ColumnsBlock)
	require.True(t, ok)
	require.Equal(t, 2, len(cols.Columns))
	assert.Equal(t, "::cols::\n\nleft", blocksToPlainText(cols.Columns[0]))
	assert.Equal(t, "right", blocksToPlainText(cols.Columns[1]))
}
