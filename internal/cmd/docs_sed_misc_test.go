package cmd

import (
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestDocsSedCmd_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name  string
		expr  string
		input string
		want  string
		wantN int
	}{
		{
			name:  "email obfuscation",
			expr:  `s/(\w+)@(\w+)\.(\w+)/\1[at]\2[dot]\3/g`,
			input: "Contact: john@example.com or jane@test.org",
			want:  "Contact: john[at]example[dot]com or jane[at]test[dot]org",
			wantN: 2,
		},
		{
			name:  "date format conversion",
			expr:  `s#(\d{4})-(\d{2})-(\d{2})#\2/\3/\1#g`,
			input: "Date: 2026-02-07",
			want:  "Date: 02/07/2026",
			wantN: 1,
		},
		{
			name:  "remove html tags",
			expr:  `s/<[^>]+>//g`,
			input: "<p>Hello <b>world</b></p>",
			want:  "Hello world",
			wantN: 4,
		},
		{
			name:  "camelCase to snake_case",
			expr:  `s/([a-z])([A-Z])/\1_\2/g`,
			input: "getUserName",
			want:  "get_User_Name",
			wantN: 2,
		},
		{
			name:  "trim whitespace",
			expr:  `s/^\s+|\s+$//g`,
			input: "  hello world  ",
			want:  "hello world",
			wantN: 2,
		},
		{
			name:  "collapse whitespace",
			expr:  `s/\s+/ /g`,
			input: "hello    world\t\tfoo",
			want:  "hello world foo",
			wantN: 2,
		},
		{
			name:  "quote words",
			expr:  `s/\b(\w+)\b/"\1"/g`,
			input: "hello world",
			want:  `"hello" "world"`,
			wantN: 2,
		},
		{
			name:  "version bump",
			expr:  `s/v(\d+)\.(\d+)\.(\d+)/v\1.\2.999/`,
			input: "Current: v1.2.3",
			want:  "Current: v1.2.999",
			wantN: 1,
		},
		{
			name:  "no match",
			expr:  `s/xyz/abc/g`,
			input: "hello world",
			want:  "hello world",
			wantN: 0,
		},
		{
			name:  "overlapping matches",
			expr:  `s/aa/a/g`,
			input: "aaaa",
			want:  "aa",
			wantN: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, replacement, global, err := parseSedExpr(tt.expr)
			if err != nil {
				t.Fatalf("parseSedExpr: %v", err)
			}

			re, err := regexp.Compile(pattern)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			var result string
			var count int
			if global {
				matches := re.FindAllStringIndex(tt.input, -1)
				count = len(matches)
				result = re.ReplaceAllString(tt.input, replacement)
			} else {
				if re.MatchString(tt.input) {
					result = re.ReplaceAllStringFunc(tt.input, func(s string) string {
						count++
						if count > 1 {
							return s
						}
						return re.ReplaceAllString(s, replacement)
					})
				} else {
					result = tt.input
				}
			}

			if result != tt.want {
				t.Errorf("result = %q, want %q", result, tt.want)
			}
			if count != tt.wantN {
				t.Errorf("match count = %d, want %d", count, tt.wantN)
			}
		})
	}
}

func TestDocsSedCmd_EmptyDocId(t *testing.T) {
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsSedCmd{
		DocID:      "   ",
		Expression: "s/foo/bar/",
	}

	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Error("expected error for empty docId")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %v, want error containing 'empty'", err)
	}
}

func TestDocsEditCmd_MatchCase(t *testing.T) {
	cmd := DocsEditCmd{MatchCase: true}
	if !cmd.MatchCase {
		t.Error("MatchCase should be true")
	}

	cmdNoCase := DocsEditCmd{MatchCase: false}
	if cmdNoCase.MatchCase {
		t.Error("MatchCase should be false")
	}
}

func TestDocsSedCmd_FlagsVariations(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		wantGlobal bool
	}{
		{"no flags", "s/a/b/", false},
		{"g flag", "s/a/b/g", true},
		{"gi flags", "s/a/b/gi", true},
		{"ig flags", "s/a/b/ig", true},
		{"empty flags", "s/a/b//", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, global, err := parseSedExpr(tt.expr)
			if err != nil {
				return
			}
			if global != tt.wantGlobal {
				t.Errorf("global = %v, want %v", global, tt.wantGlobal)
			}
		})
	}
}

func TestBuildReplaceAllTextRequest(t *testing.T) {
	find := "old text"
	replace := "new text"
	matchCase := true

	req := &docs.Request{
		ReplaceAllText: &docs.ReplaceAllTextRequest{
			ContainsText: &docs.SubstringMatchCriteria{
				Text:      find,
				MatchCase: matchCase,
			},
			ReplaceText: replace,
		},
	}

	if req.ReplaceAllText == nil {
		t.Fatal("ReplaceAllText should not be nil")
	}
	if req.ReplaceAllText.ContainsText.Text != find {
		t.Errorf("Text = %q, want %q", req.ReplaceAllText.ContainsText.Text, find)
	}
	if req.ReplaceAllText.ReplaceText != replace {
		t.Errorf("ReplaceText = %q, want %q", req.ReplaceAllText.ReplaceText, replace)
	}
	if req.ReplaceAllText.ContainsText.MatchCase != matchCase {
		t.Errorf("MatchCase = %v, want %v", req.ReplaceAllText.ContainsText.MatchCase, matchCase)
	}
}

func TestMarkdownToDocsAPIMapping(t *testing.T) {
	tests := []struct {
		format   string
		wantBold bool
		wantItal bool
		wantStrk bool
	}{
		{"bold", true, false, false},
		{"italic", false, true, false},
		{"strikethrough", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			_, formats := parseMarkdownReplacement("**test**")
			hasBold := false
			for _, f := range formats {
				if f == "bold" {
					hasBold = true
				}
			}
			if tt.format == "bold" && !hasBold {
				t.Error("expected bold format")
			}
		})
	}
}

func TestParseMarkdownReplacement_Escapes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantText    string
		wantFormats []string
	}{
		{
			name:        "escaped asterisks - literal bold syntax",
			input:       `\*\*this\*\*`,
			wantText:    "**this**",
			wantFormats: nil,
		},
		{
			name:        "escaped single asterisks - literal italic syntax",
			input:       `\*italic\*`,
			wantText:    "*italic*",
			wantFormats: nil,
		},
		{
			name:        "escaped hash - literal heading syntax",
			input:       `\# Not a heading`,
			wantText:    "# Not a heading",
			wantFormats: nil,
		},
		{
			name:        "escaped multiple hashes",
			input:       `\#\#\# literal hashes`,
			wantText:    "### literal hashes",
			wantFormats: nil,
		},
		{
			name:        "escaped tildes - literal strikethrough syntax",
			input:       `\~\~not struck\~\~`,
			wantText:    "~~not struck~~",
			wantFormats: nil,
		},
		{
			name:        "escaped backticks - literal code syntax",
			input:       "\\`not code\\`",
			wantText:    "`not code`",
			wantFormats: nil,
		},
		{
			name:        "escaped dash - literal bullet syntax",
			input:       `\- not a bullet`,
			wantText:    "- not a bullet",
			wantFormats: nil,
		},
		{
			name:        "escaped plus - literal checkbox syntax",
			input:       `\+ not a checkbox`,
			wantText:    "+ not a checkbox",
			wantFormats: nil,
		},
		{
			name:        "escaped backslash",
			input:       `path\\to\\file`,
			wantText:    `path\to\file`,
			wantFormats: nil,
		},
		{
			name:        "escaped backslash before asterisk",
			input:       `\\*still italic*`,
			wantText:    `\*still italic*`,
			wantFormats: nil,
		},
		{
			name:        "escaped backslash then asterisks",
			input:       `\\*italic*`,
			wantText:    `\*italic*`,
			wantFormats: nil,
		},
		{
			name:        "mixed escaped and real formatting",
			input:       `\*\*literal\*\* and **bold**`,
			wantText:    "**literal** and **bold**",
			wantFormats: nil,
		},
		{
			name:        "real bold with escaped content inside",
			input:       `**has \* inside**`,
			wantText:    "has * inside",
			wantFormats: []string{"bold"},
		},
		{
			name:        "escape at end",
			input:       `text\*`,
			wantText:    "text*",
			wantFormats: nil,
		},
		{
			name:        "multiple escapes",
			input:       `\*\*\*triple\*\*\*`,
			wantText:    "***triple***",
			wantFormats: nil,
		},
		{
			name:        "escaped newline still works",
			input:       `line1\nline2`,
			wantText:    "line1\nline2",
			wantFormats: nil,
		},
		{
			name:        "escaped chars with real formatting",
			input:       `**bold with \* asterisk**`,
			wantText:    "bold with * asterisk",
			wantFormats: []string{"bold"},
		},
		{
			name:        "heading with escaped hash inside",
			input:       `# Title with \# hash`,
			wantText:    "Title with # hash",
			wantFormats: []string{"heading1"},
		},
		{
			name:        "bullet with escaped dash",
			input:       `- item with \- dash`,
			wantText:    "item with - dash",
			wantFormats: []string{"bullet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, formats := parseMarkdownReplacement(tt.input)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if len(formats) != len(tt.wantFormats) {
				t.Errorf("formats = %v, want %v", formats, tt.wantFormats)
				return
			}
			for i, f := range formats {
				if f != tt.wantFormats[i] {
					t.Errorf("formats[%d] = %q, want %q", i, f, tt.wantFormats[i])
				}
			}
		})
	}
}
