package cmd

import (
	"testing"
)

func TestStripYAMLFrontmatter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "standard",
			in:   "---\ntitle: T\n---\n\n# Hi\n",
			want: "\n# Hi\n",
		},
		{
			name: "no_trailing_newline_after_body",
			in:   "---\nx: 1\n---\nbody",
			want: "body",
		},
		{
			name: "closing_delimiter_at_eof",
			in:   "---\nx: 1\n---",
			want: "",
		},
		{
			name: "crlf",
			in:   "---\r\nx: 1\r\n---\r\nbody\r\n",
			want: "body\r\n",
		},
		{
			name: "empty_body",
			in:   "---\nx: 1\n---\n",
			want: "",
		},
		{
			name: "bom_then_frontmatter",
			in:   string(utf8BOM) + "---\na: b\n---\nText",
			want: "Text",
		},
		{
			name: "no_frontmatter",
			in:   "# Just markdown\n",
			want: "# Just markdown\n",
		},
		{
			name: "only_opening_delimiter",
			in:   "---\ntitle: no close\n",
			want: "---\ntitle: no close\n",
		},
		{
			name: "four_dashes_not_frontmatter",
			in:   "----\nnot fm\n",
			want: "----\nnot fm\n",
		},
		{
			name: "blank_lines_inside_fm",
			in:   "---\n\nx: 1\n\n---\n\nHello",
			want: "\nHello",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := string(stripYAMLFrontmatter([]byte(tc.in)))
			if got != tc.want {
				t.Fatalf("stripYAMLFrontmatter() = %q, want %q", got, tc.want)
			}
		})
	}
}
