package cmd

import (
	"testing"
)

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
			expected: []MarkdownElementType{MDHeading1, MDParagraph, MDListItem},
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
