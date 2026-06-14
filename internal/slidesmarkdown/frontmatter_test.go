package slidesmarkdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitMarkdownIntoSlideBlocks(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected []slideBlock
	}{
		{
			name:  "single slide no frontmatter",
			input: "# Hello\n\nbody\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# Hello\n\nbody\n"},
			},
		},
		{
			name:  "two slides separated by ---",
			input: "# A\n\n---\n\n# B\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# B\n"},
			},
		},
		{
			name:  "leading frontmatter then content",
			input: "---\nlayout: hero\n---\n\n# Title\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Layout: "hero", Raw: map[string]string{"layout": "hero"}}, Body: "# Title\n"},
			},
		},
		{
			name:  "frontmatter on second slide",
			input: "# A\n\n---\nlayout: center\n---\n\n# B\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
				{Frontmatter: SlideFrontmatter{Layout: "center", Raw: map[string]string{"layout": "center"}}, Body: "# B\n"},
			},
		},
		{
			name:  "frontmatter with content key",
			input: "---\nlayout: center\ncontent: wide\n---\n\nbody\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{
					Layout:  "center",
					Content: "wide",
					Raw:     map[string]string{"layout": "center", "content": "wide"},
				}, Body: "body\n"},
			},
		},
		{
			name:  "bare --- at slide start is separator not frontmatter",
			input: "# A\n\n---\n\nplain text body\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "plain text body\n"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitMarkdownIntoSlideBlocks(tc.input)
			require.NoError(t, err)
			require.Equal(t, len(tc.expected), len(got))

			for i := range tc.expected {
				assert.Equal(t, tc.expected[i].Frontmatter, got[i].Frontmatter, "slide %d frontmatter", i)
				assert.Equal(t, tc.expected[i].Body, got[i].Body, "slide %d body", i)
			}
		})
	}
}

func TestSplitMarkdownIntoSlideBlocks_UnclosedFrontmatterStaysBody(t *testing.T) {
	got, err := splitMarkdownIntoSlideBlocks("---\nlayout: hero\n\n# never closed\n")
	require.NoError(t, err)
	require.Equal(t, []slideBlock{
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "layout: hero\n\n# never closed\n"},
	}, got)
}

func TestSplitMarkdownIntoSlideBlocks_MalformedClosedFrontmatter(t *testing.T) {
	_, err := splitMarkdownIntoSlideBlocks("---\nlayout: [\n---\n\n# broken\n")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "frontmatter")
}

func TestSplitMarkdownIntoSlideBlocks_SkipsEmptySeparatorChunks(t *testing.T) {
	got, err := splitMarkdownIntoSlideBlocks("# A\n\n---\n\n")
	require.NoError(t, err)
	require.Equal(t, []slideBlock{
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
	}, got)
}

func TestSplitMarkdownIntoSlideBlocks_EmptyInput(t *testing.T) {
	got, err := splitMarkdownIntoSlideBlocks("")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSplitMarkdownIntoSlideBlocks_SkipsMetadataOnlyFrontmatter(t *testing.T) {
	got, err := splitMarkdownIntoSlideBlocks("---\ntitle: Deck\n---\n\n---\n\n# First\n")
	require.NoError(t, err)
	require.Equal(t, []slideBlock{
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# First\n"},
	}, got)
}

func TestSplitMarkdownIntoSlideBlocks_KeyValueBodyAfterSeparator(t *testing.T) {
	got, err := splitMarkdownIntoSlideBlocks("# A\n\n---\n\nProblem: hard\n\n## B\nbody\n")
	require.NoError(t, err)
	require.Equal(t, []slideBlock{
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "Problem: hard\n\n## B\nbody\n"},
	}, got)
}

func TestSplitMarkdownIntoSlideBlocks_KeyValueBodyBeforeNextSeparator(t *testing.T) {
	got, err := splitMarkdownIntoSlideBlocks("# A\n\n---\n\nlayout: responsive\n\nbody\n\n---\n\n# C\n")
	require.NoError(t, err)
	require.Equal(t, []slideBlock{
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "layout: responsive\n\nbody\n"},
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# C\n"},
	}, got)
}

func TestSplitMarkdownIntoSlideBlocks_FrontmatterMayStartWithUnknownKey(t *testing.T) {
	got, err := splitMarkdownIntoSlideBlocks("---\nbackground: dark\nlayout: hero\n---\n\n# B\n")
	require.NoError(t, err)
	require.Equal(t, []slideBlock{
		{Frontmatter: SlideFrontmatter{
			Layout: "hero",
			Raw: map[string]string{
				"background": "dark",
				"layout":     "hero",
			},
		}, Body: "# B\n"},
	}, got)
}

func TestSplitMarkdownIntoSlideBlocks_DelimiterInsideFenceStaysBody(t *testing.T) {
	input := "# A\n\n```markdown\n---\nlayout: hero\n---\n```\n\n---\n\n# B\n"
	got, err := splitMarkdownIntoSlideBlocks(input)
	require.NoError(t, err)
	require.Equal(t, []slideBlock{
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n\n```markdown\n---\nlayout: hero\n---\n```\n"},
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# B\n"},
	}, got)
}

func TestSplitMarkdownIntoSlideBlocks_DelimiterInsideTildeFenceStaysBody(t *testing.T) {
	input := "# A\n\n~~~yaml\n---\nlayout: hero\n---\n~~~\n\n---\n\n# B\n"
	got, err := splitMarkdownIntoSlideBlocks(input)
	require.NoError(t, err)
	require.Equal(t, []slideBlock{
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n\n~~~yaml\n---\nlayout: hero\n---\n~~~\n"},
		{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# B\n"},
	}, got)
}
