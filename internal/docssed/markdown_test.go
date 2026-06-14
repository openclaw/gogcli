//nolint:wsl_v5 // Table-driven parser tests stay compact around assertions.
package docssed

import (
	"reflect"
	"testing"
)

func TestParseMarkdownReplacement(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		text    string
		formats []string
	}{
		{name: "plain", input: "hello", text: "hello"},
		{name: "bold", input: "**hello**", text: "hello", formats: []string{"bold"}},
		{name: "italic", input: "*hello*", text: "hello", formats: []string{"italic"}},
		{name: "bold italic", input: "***hello***", text: "hello", formats: []string{"bold", "italic"}},
		{name: "strike", input: "~~hello~~", text: "hello", formats: []string{"strikethrough"}},
		{name: "code", input: "`hello`", text: "hello", formats: []string{"code"}},
		{name: "heading", input: "### Sub", text: "Sub", formats: []string{"heading3"}},
		{name: "bullet", input: "- Item", text: "Item", formats: []string{"bullet"}},
		{name: "numbered", input: "1. Item", text: "Item", formats: []string{"numbered"}},
		{name: "nested bullet", input: "    - Item", text: "\t\tItem", formats: []string{"bullet"}},
		{name: "horizontal rule", input: "---", text: "\n", formats: []string{"hrule"}},
		{name: "blockquote", input: "> Quote", text: "Quote", formats: []string{"blockquote"}},
		{name: "code block", input: "```go\nfoo\n```", text: "foo\n", formats: []string{"codeblock"}},
		{name: "footnote", input: "[^note]", text: "note", formats: []string{"footnote"}},
		{name: "link", input: "[text](https:\\/\\/example.com)", text: "text", formats: []string{"link:https://example.com"}},
		{name: "escaped formatting", input: `\*not bold\*`, text: "*not bold*"},
		{name: "escaped inside bold", input: `**bold with \* asterisk**`, text: "bold with * asterisk", formats: []string{"bold"}},
		{name: "escaped newline", input: `line1\nline2`, text: "line1\nline2"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := ParseMarkdownReplacement(test.input)
			if got.Text != test.text || !reflect.DeepEqual(got.Formats, test.formats) {
				t.Fatalf("ParseMarkdownReplacement(%q) = %#v, want text=%q formats=%v", test.input, got, test.text, test.formats)
			}
		})
	}
}

func TestCanUseNativeReplacement(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  bool
	}{
		{input: "plain", want: true},
		{input: "", want: true},
		{input: "**bold**"},
		{input: "# heading"},
		{input: "- bullet"},
		{input: "{b}text"},
		{input: "![alt](url)"},
		{input: "!(https://example.com/image.png)"},
		{input: "---"},
		{input: `line1\nline2`},
		{input: "$1"},
		{input: "${1}"},
		{input: "[text](url)"},
		{input: "{not a valid brace expr}", want: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			if got := CanUseNativeReplacement(test.input); got != test.want {
				t.Fatalf("CanUseNativeReplacement(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

func FuzzParseMarkdownReplacement(f *testing.F) {
	for _, seed := range []string{
		"plain",
		"**bold**",
		"*italic*",
		"```go\nfunc main() {}\n```",
		"[text](url)",
		"  1. nested",
		`line1\nline2`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		ParseMarkdownReplacement(input)
		CanUseNativeReplacement(input)
	})
}
