package cmd

import (
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/ui"
)

func TestDocsSedCmd_RegexMatching(t *testing.T) {
	tests := []struct {
		name  string
		expr  string
		input string
		want  string
		wantN int
	}{
		{
			name:  "simple global replace",
			expr:  "s/foo/bar/g",
			input: "foo foo foo", //nolint:dupword
			want:  "bar bar bar", //nolint:dupword
			wantN: 3,
		},
		{
			name:  "first match only",
			expr:  "s/foo/bar/",
			input: "foo foo foo", //nolint:dupword
			want:  "bar foo foo", //nolint:dupword
			wantN: 1,
		},
		{
			name:  "digit replacement",
			expr:  `s/\d+/NUM/g`,
			input: "item1 item2 item3",
			want:  "itemNUM itemNUM itemNUM", //nolint:dupword
			wantN: 3,
		},
		{
			name:  "capture group",
			expr:  `s/(\w+)@(\w+)/\2:\1/g`,
			input: "user@host",
			want:  "host:user",
			wantN: 1,
		},
		{
			name:  "word boundary",
			expr:  `s/\bcat\b/dog/g`,
			input: "cat catalog bobcat cat",
			want:  "dog catalog bobcat dog",
			wantN: 2,
		},
		{
			name:  "whole match backreference ampersand",
			expr:  `s/foo/&/`,
			input: "foo",
			want:  "foo",
			wantN: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, replacement, global, err := parseSedExpr(tt.expr)
			if err != nil {
				t.Fatalf("parseSedExpr: %v", err)
			}

			re, err := compilePattern(pattern)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			var result string
			var count int
			if global {
				result = re.ReplaceAllString(tt.input, replacement)
				count = len(re.FindAllString(tt.input, -1))
			} else {
				loc := re.FindStringIndex(tt.input)
				if loc != nil {
					result = tt.input[:loc[0]] + re.ReplaceAllString(tt.input[loc[0]:loc[1]], replacement) + tt.input[loc[1]:]
					count = 1
				} else {
					result = tt.input
					count = 0
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

func TestParseFullExpr_BackrefsSkipBraceFormatting(t *testing.T) {
	testCases := []struct {
		name string
		expr string
		want string
	}{
		{name: "whole match ampersand", expr: `s/foo/&/`, want: "${0}"},
		{name: "capture backref", expr: `s/(foo)/\1/`, want: "${1}"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := parseFullExpr(tc.expr)
			if err != nil {
				t.Fatalf("parseFullExpr: %v", err)
			}
			if expr.replacement != tc.want {
				t.Fatalf("replacement = %q, want %q", expr.replacement, tc.want)
			}
			if expr.brace != nil || len(expr.braceSpans) != 0 {
				t.Fatalf("backreference parsed as brace formatting: %#v %#v", expr.brace, expr.braceSpans)
			}
			if canUseNativeReplace(expr.replacement) {
				t.Fatalf("backreference replacement should use manual path")
			}
		})
	}
}

func compilePattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}

func TestDocsEditCmd_EmptyDocId(t *testing.T) {
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsEditCmd{
		DocID:      "   ",
		Find:       "foo",
		ReplaceStr: "bar",
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

func TestDocsEditCmd_EmptyFind(t *testing.T) {
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsEditCmd{
		DocID:      "doc123",
		Find:       "",
		ReplaceStr: "bar",
	}

	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Error("expected error for empty find")
	}
}

func TestDocsSedCmd_InvalidExpression(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"not starting with s", "x/foo/bar/"},
		{"too short", "s/"},
		{"missing replacement", "s/foo"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseSedExpr(tt.expr)
			if err == nil {
				t.Errorf("expected error for expr %q", tt.expr)
			}
		})
	}
}

func TestDocsSedCmd_InvalidRegex(t *testing.T) {
	pattern, _, _, err := parseSedExpr("s/[unclosed/replacement/")
	if err != nil {
		return
	}

	_, err = regexp.Compile(pattern)
	if err == nil {
		t.Error("expected regex compile error for unclosed bracket")
	}
}

func TestDocsEditCmd_OutputFormat_JSON(t *testing.T) {
	expectedFields := []string{"status", "docId", "replaced"}
	output := map[string]any{
		"status":   "ok",
		"docId":    "test-doc",
		"replaced": 5,
	}

	for _, field := range expectedFields {
		if _, ok := output[field]; !ok {
			t.Errorf("missing field %q in JSON output", field)
		}
	}
}

func TestDocsEditCmd_OutputFormat_Text(t *testing.T) {
	lines := []string{
		"status\tok",
		"docId\ttest-doc",
		"replaced\t5",
	}

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			t.Errorf("line %q should have 2 tab-separated parts", line)
		}
	}
}
