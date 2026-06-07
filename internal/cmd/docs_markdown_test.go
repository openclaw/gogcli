package cmd

import "testing"

func TestParseMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []MarkdownElementType
	}{
		{
			name:     "heading 1",
			input:    "# Hello World",
			expected: []MarkdownElementType{MDHeading1},
		},
		{
			name:     "heading 2",
			input:    "## Hello World",
			expected: []MarkdownElementType{MDHeading2},
		},
		{
			name:     "paragraph",
			input:    "This is a paragraph",
			expected: []MarkdownElementType{MDParagraph},
		},
		{
			name:     "bullet list",
			input:    "- Item 1\n- Item 2",
			expected: []MarkdownElementType{MDListItem, MDListItem},
		},
		{
			name:     "numbered list",
			input:    "1. First\n2. Second",
			expected: []MarkdownElementType{MDNumberedList, MDNumberedList},
		},
		{
			name:     "code block",
			input:    "```\ncode here\n```",
			expected: []MarkdownElementType{MDCodeBlock},
		},
		{
			name:     "blockquote",
			input:    "> This is a quote",
			expected: []MarkdownElementType{MDBlockquote},
		},
		{
			name:     "mixed content",
			input:    "# Title\n\nParagraph here\n\n- List item",
			expected: []MarkdownElementType{MDHeading1, MDEmptyLine, MDParagraph, MDEmptyLine, MDListItem},
		},
		{
			name:     "consecutive blank lines collapse",
			input:    "Paragraph A\n\n\nParagraph B\n",
			expected: []MarkdownElementType{MDParagraph, MDEmptyLine, MDParagraph},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseMarkdown(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("ParseMarkdown() got %d elements, want %d", len(result), len(tt.expected))
				return
			}
			for i, el := range result {
				if el.Type != tt.expected[i] {
					t.Errorf("ParseMarkdown()[%d] = %v, want %v", i, el.Type, tt.expected[i])
				}
			}
		})
	}
}

func TestParseMarkdown_NestedLists(t *testing.T) {
	result := ParseMarkdown("- Parent\n  - Child\n    - Grandchild\n\t- Tab sibling\n1. One\n  1. Nested one")
	if len(result) != 6 {
		t.Fatalf("ParseMarkdown() got %d elements, want 6: %#v", len(result), result)
	}

	want := []struct {
		typ     MarkdownElementType
		content string
		level   int
	}{
		{MDListItem, "Parent", 0},
		{MDListItem, "Child", 1},
		{MDListItem, "Grandchild", 2},
		{MDListItem, "Tab sibling", 2},
		{MDNumberedList, "One", 0},
		{MDNumberedList, "Nested one", 1},
	}
	for i, w := range want {
		if got := result[i]; got.Type != w.typ || got.Content != w.content || got.Level != w.level {
			t.Fatalf("element %d = {type:%v content:%q level:%d}, want {type:%v content:%q level:%d}",
				i, got.Type, got.Content, got.Level, w.typ, w.content, w.level)
		}
	}
}

func TestParseMarkdown_NestedListsFourSpaceBlock(t *testing.T) {
	result := ParseMarkdown("- Two-space parent\n  - Two-space child\n\n- Four-space parent\n    - Four-space child\n        - Four-space grandchild")
	if len(result) != 6 {
		t.Fatalf("ParseMarkdown() got %d elements, want 6: %#v", len(result), result)
	}

	want := []struct {
		typ     MarkdownElementType
		content string
		level   int
	}{
		{MDListItem, "Two-space parent", 0},
		{MDListItem, "Two-space child", 1},
		{MDEmptyLine, "", 0},
		{MDListItem, "Four-space parent", 0},
		{MDListItem, "Four-space child", 1},
		{MDListItem, "Four-space grandchild", 2},
	}
	for i, w := range want {
		if got := result[i]; got.Type != w.typ || got.Content != w.content || got.Level != w.level {
			t.Fatalf("element %d = {type:%v content:%q level:%d}, want {type:%v content:%q level:%d}",
				i, got.Type, got.Content, got.Level, w.typ, w.content, w.level)
		}
	}
}

func TestParseMarkdown_IndentedListMarkerWithoutParent(t *testing.T) {
	result := ParseMarkdown("    - keep literal")
	if len(result) != 1 {
		t.Fatalf("ParseMarkdown() got %d elements, want 1: %#v", len(result), result)
	}
	got := result[0]
	if got.Type != MDParagraph || got.Content != "    - keep literal" {
		t.Fatalf("element = {type:%v content:%q}, want paragraph with literal text", got.Type, got.Content)
	}
}

func TestParseMarkdown_TopLevelListResetsNestedIndentStack(t *testing.T) {
	result := ParseMarkdown("- A\n  - B\n    - C\n1. D\n    1. E")
	if len(result) != 5 {
		t.Fatalf("ParseMarkdown() got %d elements, want 5: %#v", len(result), result)
	}

	wantLevels := []int{0, 1, 2, 0, 1}
	for i, want := range wantLevels {
		if got := result[i].Level; got != want {
			t.Fatalf("element %d level = %d, want %d (%#v)", i, got, want, result[i])
		}
	}
}

func TestParseInlineFormatting(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedText  string
		expectedCount int
	}{
		{
			name:          "bold text",
			input:         "This is **bold** text",
			expectedText:  "This is bold text",
			expectedCount: 1,
		},
		{
			name:          "italic text",
			input:         "This is *italic* text",
			expectedText:  "This is italic text",
			expectedCount: 1,
		},
		{
			name:          "underscore italic text",
			input:         "This is _italic_ text",
			expectedText:  "This is italic text",
			expectedCount: 1,
		},
		{
			name:          "underscore bold text",
			input:         "This is __bold__ text",
			expectedText:  "This is bold text",
			expectedCount: 1,
		},
		{
			name:          "code text",
			input:         "This is `code` text",
			expectedText:  "This is code text",
			expectedCount: 1,
		},
		{
			name:          "link",
			input:         "Check [this link](https://example.com)",
			expectedText:  "Check this link",
			expectedCount: 1,
		},
		{
			name:          "no formatting",
			input:         "Just plain text",
			expectedText:  "Just plain text",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styles, text := ParseInlineFormatting(tt.input)
			if text != tt.expectedText {
				t.Errorf("ParseInlineFormatting() text = %q, want %q", text, tt.expectedText)
			}
			if len(styles) != tt.expectedCount {
				t.Errorf("ParseInlineFormatting() got %d styles, want %d", len(styles), tt.expectedCount)
			}
		})
	}
}

func TestParseInlineFormatting_NestedAndUnderscoreStyles(t *testing.T) {
	styles, text := ParseInlineFormatting("**bold _and italic_** plus foo_bar_baz and ___both___")
	if text != "bold and italic plus foo_bar_baz and both" {
		t.Fatalf("text = %q", text)
	}

	assertInlineStyle(t, text, styles, "bold and italic", true, false, false)
	assertInlineStyle(t, text, styles, "and italic", false, true, false)
	assertInlineStyle(t, text, styles, "both", true, true, false)
}

func TestParseInlineFormatting_Strikethrough(t *testing.T) {
	styles, text := ParseInlineFormatting("~~struck out~~ vs **bold**")
	if text != "struck out vs bold" {
		t.Fatalf("text = %q", text)
	}

	assertInlineStrikethrough(t, text, styles, "struck out")
	assertInlineStyle(t, text, styles, "bold", true, false, false)
}

func TestParseInlineFormatting_LongTildeRunsAreLiteral(t *testing.T) {
	styles, text := ParseInlineFormatting("~~ok~~ and ~~~not~~~ and ~~~~also not~~~~")
	if text != "ok and ~~~not~~~ and ~~~~also not~~~~" {
		t.Fatalf("text = %q", text)
	}
	if len(styles) != 1 {
		t.Fatalf("expected only the exact two-tilde span to format, got %#v", styles)
	}
	assertInlineStrikethrough(t, text, styles, "ok")
}

func TestParseInlineFormatting_ClosingMarkerIgnoresCodeSpan(t *testing.T) {
	styles, text := ParseInlineFormatting("**Use `**` marker** and _keep `_` literal_")
	if text != "Use ** marker and keep _ literal" {
		t.Fatalf("text = %q", text)
	}

	assertInlineStyle(t, text, styles, "Use ** marker", true, false, false)
	assertInlineStyle(t, text, styles, "**", false, false, true)
	assertInlineStyle(t, text, styles, "keep _ literal", false, true, false)
	assertInlineStyle(t, text, styles, "_", false, false, true)
}

func TestParseInlineFormatting_PreservesAdjacentLiteralBackticks(t *testing.T) {
	for _, input := range []string{"``", "a``b"} {
		styles, text := ParseInlineFormatting(input)
		if text != input {
			t.Fatalf("ParseInlineFormatting(%q) text = %q", input, text)
		}
		if len(styles) != 0 {
			t.Fatalf("ParseInlineFormatting(%q) styles = %#v", input, styles)
		}
	}

	styles, text := ParseInlineFormatting("``code``")
	if text != "code" {
		t.Fatalf("text = %q", text)
	}
	assertInlineStyle(t, text, styles, "code", false, false, true)
}

func TestParseInlineFormatting_SingleEmphasisContainsStrong(t *testing.T) {
	styles, text := ParseInlineFormatting("*italic **bold** text* and _slant __strong__ text_")
	if text != "italic bold text and slant strong text" {
		t.Fatalf("text = %q", text)
	}

	assertInlineStyle(t, text, styles, "italic bold text", false, true, false)
	assertInlineStyle(t, text, styles, "bold", true, false, false)
	assertInlineStyle(t, text, styles, "slant strong text", false, true, false)
	assertInlineStyle(t, text, styles, "strong", true, false, false)
}

func TestParseInlineFormatting_SplitsAdjacentDelimiterRuns(t *testing.T) {
	styles, text := ParseInlineFormatting("**one****two** and *italic **bold*** and **bold *em***")
	if text != "onetwo and italic bold and bold em" {
		t.Fatalf("text = %q", text)
	}

	assertInlineStyle(t, text, styles, "one", true, false, false)
	assertInlineStyle(t, text, styles, "two", true, false, false)
	assertInlineStyle(t, text, styles, "italic bold", false, true, false)
	assertInlineStyle(t, text, styles, "bold", true, false, false)
	assertInlineStyle(t, text, styles, "bold em", true, false, false)
	assertInlineStyle(t, text, styles, "em", false, true, false)
}

func TestParseInlineFormatting_StrongThenLiteralStar(t *testing.T) {
	styles, text := ParseInlineFormatting("**foo***")
	if text != "foo*" {
		t.Fatalf("text = %q", text)
	}

	assertInlineStyle(t, text, styles, "foo", true, false, false)
}

func TestParseInlineFormatting_ItalicThenLiteralStar(t *testing.T) {
	styles, text := ParseInlineFormatting("*foo**")
	if text != "foo*" {
		t.Fatalf("text = %q", text)
	}

	assertInlineStyle(t, text, styles, "foo", false, true, false)
}

func TestParseInlineFormatting_UnderscoreWhitespaceIsLiteral(t *testing.T) {
	styles, text := ParseInlineFormatting("before _ foo _ after and a _ b _ c")
	if text != "before _ foo _ after and a _ b _ c" {
		t.Fatalf("text = %q", text)
	}
	if len(styles) != 0 {
		t.Fatalf("styles = %#v", styles)
	}
}

func TestParseInlineFormatting_LiteralBracketBeforeLink(t *testing.T) {
	styles, text := ParseInlineFormatting("Use arr[0] and [link](https://x)")
	if text != "Use arr[0] and link" {
		t.Fatalf("text = %q", text)
	}

	assertInlineLink(t, text, styles, "link", "https://x")
}

func TestParseMarkdown_StripsReporterBlockMarkers(t *testing.T) {
	input := "> quoted text\n```go\nfmt.Println(\"hi\")\n```"
	got := ParseMarkdown(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(got))
	}
	if got[0].Type != MDBlockquote || got[0].Content != "quoted text" {
		t.Fatalf("blockquote = %#v", got[0])
	}
	if got[1].Type != MDCodeBlock || got[1].Content != "fmt.Println(\"hi\")" {
		t.Fatalf("code block = %#v", got[1])
	}
}

func assertInlineStyle(t *testing.T, text string, styles []TextStyle, wantText string, bold, italic, code bool) {
	t.Helper()
	for _, style := range styles {
		if int(style.End) > len(text) {
			continue
		}
		if text[style.Start:style.End] == wantText && style.Bold == bold && style.Italic == italic && style.Code == code && !style.Strikethrough {
			return
		}
	}
	t.Fatalf("missing style text=%q bold=%v italic=%v code=%v in %#v", wantText, bold, italic, code, styles)
}

func assertInlineStrikethrough(t *testing.T, text string, styles []TextStyle, wantText string) {
	t.Helper()
	for _, style := range styles {
		if int(style.End) > len(text) {
			continue
		}
		if text[style.Start:style.End] == wantText && style.Strikethrough {
			return
		}
	}
	t.Fatalf("missing strikethrough text=%q in %#v", wantText, styles)
}

func assertInlineLink(t *testing.T, text string, styles []TextStyle, wantText string, wantURL string) {
	t.Helper()
	for _, style := range styles {
		if int(style.End) > len(text) {
			continue
		}
		if text[style.Start:style.End] == wantText && style.Link == wantURL {
			return
		}
	}
	t.Fatalf("missing link text=%q url=%q in %#v", wantText, wantURL, styles)
}

func TestParseHeading(t *testing.T) {
	tests := []struct {
		line            string
		expectedLevel   int
		expectedContent string
	}{
		{"# Title", 1, "Title"},
		{"## Subtitle", 2, "Subtitle"},
		{"### Section", 3, "Section"},
		{"#### Subsection", 4, "Subsection"},
		{"Not a heading", 0, ""},
		{"#No space", 0, ""},
	}

	for _, tt := range tests {
		level, content := parseHeading(tt.line)
		if level != tt.expectedLevel {
			t.Errorf("parseHeading(%q) level = %d, want %d", tt.line, level, tt.expectedLevel)
		}
		if content != tt.expectedContent {
			t.Errorf("parseHeading(%q) content = %q, want %q", tt.line, content, tt.expectedContent)
		}
	}
}

func TestIsHorizontalRule(t *testing.T) {
	tests := []struct {
		line     string
		expected bool
	}{
		{"---", true},
		{"***", true},
		{"___", true},
		{"- - -", true},
		{"* * *", true},
		{"--", false},
		{"---text", false},
		{"text---", false},
	}

	for _, tt := range tests {
		result := isHorizontalRule(tt.line)
		if result != tt.expected {
			t.Errorf("isHorizontalRule(%q) = %v, want %v", tt.line, result, tt.expected)
		}
	}
}

func TestParseMarkdown_TableDoesNotSkipFollowingLine(t *testing.T) {
	input := "| Name | Value |\n| --- | --- |\n| a | b |\nAfter table"
	got := ParseMarkdown(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(got))
	}
	if got[0].Type != MDTable {
		t.Fatalf("first element type = %v, want %v", got[0].Type, MDTable)
	}
	if got[1].Type != MDParagraph || got[1].Content != "After table" {
		t.Fatalf("second element = %#v, want paragraph 'After table'", got[1])
	}
}

func TestIsTableSeparator_EmptyPipeRowRejected(t *testing.T) {
	// Regression for #609: a row of empty pipe cells (e.g. an empty markdown
	// table header) must not be classified as a separator line. Otherwise the
	// outer parser drops a row from the table and re-parses the next data line
	// as a literal pipe paragraph.
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"empty cells", "|     |     |", false},
		{"empty cells tight", "||", false},
		{"empty cells three cols", "|   |   |   |", false},
		{"normal separator", "|---|---|", true},
		{"spaced separator", "| --- | --- |", true},
		{"left align", "|:---|---|", true},
		{"center align", "|:---:|---:|", true},
		{"mixed empty+dashes still valid", "|---|   |", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTableSeparator(tt.line); got != tt.want {
				t.Errorf("isTableSeparator(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestParseMarkdown_EmptyHeaderTableDropsBlankHeaderAndKeepsDataRows(t *testing.T) {
	// Regression for #609: an empty-header table previously had its last data
	// row re-parsed as a literal pipe paragraph (because the empty pipe row
	// matched isTableSeparator and the outer loop advanced too far). Regression
	// for #632: the blank header itself should not render as a visible row.
	input := "|     |     |\n|-----|-----|\n| Label A | Value A |\n| Label B | Value B |"
	got := ParseMarkdown(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 element (table only), got %d: %#v", len(got), got)
	}
	if got[0].Type != MDTable {
		t.Fatalf("element type = %v, want MDTable", got[0].Type)
	}
	if len(got[0].TableCells) != 2 {
		t.Fatalf("expected 2 data rows, got %d: %#v", len(got[0].TableCells), got[0].TableCells)
	}
	first := got[0].TableCells[0]
	if len(first) != 2 || first[0] != "Label A" || first[1] != "Value A" {
		t.Fatalf("first row = %#v, want [Label A, Value A]", first)
	}
	last := got[0].TableCells[1]
	if len(last) != 2 || last[0] != "Label B" || last[1] != "Value B" {
		t.Fatalf("last row = %#v, want [Label B, Value B]", last)
	}
}

func TestNormalizeMarkdownTablesForDriveImport_PromotesFirstDataRow(t *testing.T) {
	input := "|     |     |\n|-----|-----|\n| Label A | Value A |\n| Label B | Value B |\n\nAfter"
	got := normalizeMarkdownTablesForDriveImport(input)
	want := "| Label A | Value A |\n|-----|-----|\n| Label B | Value B |\n\nAfter"
	if got != want {
		t.Fatalf("normalized markdown = %q, want %q", got, want)
	}
}

func TestNormalizeMarkdownTablesForDriveImport_SkipsCodeBlocks(t *testing.T) {
	input := "```\n|     |     |\n|-----|-----|\n| A | B |\n```\n\n    |     |     |\n    |-----|-----|\n    | A | B |\n"
	got := normalizeMarkdownTablesForDriveImport(input)
	if got != input {
		t.Fatalf("code block markdown changed:\n got %q\nwant %q", got, input)
	}
}

func TestNormalizeMarkdownTablesForDriveImport_TracksFenceMarker(t *testing.T) {
	input := "```\n~~~\n|     |     |\n|-----|-----|\n| A | B |\n```\n"
	got := normalizeMarkdownTablesForDriveImport(input)
	if got != input {
		t.Fatalf("mixed-fence code block changed:\n got %q\nwant %q", got, input)
	}
}
