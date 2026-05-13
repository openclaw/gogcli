package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarkdownToSlides_TitleHoistFromH1(t *testing.T) {
	input := "# Hello\n\nbody text\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "Hello", got[0].Title)
	require.Equal(t, 1, len(got[0].Body))
	assert.IsType(t, ParagraphBlock{}, got[0].Body[0])
}

func TestParseMarkdownToSlides_TitleFallbackToH2(t *testing.T) {
	input := "## Topic Heading\n\n- a\n- b\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "Topic Heading", got[0].Title)
}

func TestParseMarkdownToSlides_HeroLayoutKeepsH1InBody(t *testing.T) {
	input := "---\nlayout: hero\n---\n\n# Big Wordmark\n\nsubline\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "", got[0].Title, "title should not be hoisted on hero")
	require.GreaterOrEqual(t, len(got[0].Body), 1)
	first, ok := got[0].Body[0].(HeadingBlock)
	require.True(t, ok)
	assert.Equal(t, 1, first.Level)
}

func TestParseMarkdownToSlides_NotesExtraction(t *testing.T) {
	input := "## Topic\n\nbody\n\n## Notes\n\n- speaker note one\n- speaker note two\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
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

func TestParseMarkdownToSlides_NotesStripsFAShortcodes(t *testing.T) {
	input := "## Topic\n\nbody\n\n## Notes\n\n:fa-truck-fast: Orders matter\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.NotContains(t, got[0].Notes, ":fa-truck-fast:")
	assert.Contains(t, got[0].Notes, "Orders matter")
}
