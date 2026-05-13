package cmd

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

func TestSplitMarkdownIntoSlideBlocks_UnclosedFrontmatter(t *testing.T) {
	_, err := splitMarkdownIntoSlideBlocks("---\nlayout: hero\n\n# never closed\n")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "frontmatter")
}
