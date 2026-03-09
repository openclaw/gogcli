package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveColor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"red", "#FF0000"},
		{"RED", "#FF0000"},
		{"Red", "#FF0000"},
		{"#ABCDEF", "#ABCDEF"},
		{"unknown", "unknown"},
		{"navy", "#000080"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveColor(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveHeading(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"t", "TITLE"},
		{"s", "SUBTITLE"},
		{"1", "HEADING_1"},
		{"6", "HEADING_6"},
		{"0", "NORMAL_TEXT"},
		{"2", "HEADING_2"},
		{"3", "HEADING_3"},
		{"4", "HEADING_4"},
		{"5", "HEADING_5"},
		{"unknown", "unknown"},     // passthrough
		{"HEADING_1", "HEADING_1"}, // already resolved
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveHeading(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveAlign(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"left", "START"},
		{"center", "CENTER"},
		{"right", "END"},
		{"justify", "JUSTIFIED"},
		{"LEFT", "START"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveAlign(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveBreak(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "horizontal_rule"},
		{"p", "page_break"},
		{"c", "column_break"},
		{"s", "section_break"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveBreak(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasBraceFormatting(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"plain text", false},
		{"{b}", true},
		{"{b c=red}", true},
		{`\{escaped\}`, false},
		{"H{,=2}O", true},
		{"{}", false},
		{"{unknown}", false},
		{"{b t=hello}", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := hasBraceFormatting(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBraceExprHasAnyFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"bold", "b", true},
		{"color", "c=red", true},
		{"empty", "", false},
		{"reset", "0", true},
		{"break", "+", true},
		{"dimensions", "x=600 y=400", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			got := braceExprHasAnyFormat(expr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsHexColor(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"#AABBCC", true},
		{"#abc", true},
		{"#ABC", true},
		{"#123456", true},
		{"123456", false},
		{"#GGGGGG", false},
		{"#12345", false},
		{"red", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isHexColor(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeHexColor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"#abc", "#AABBCC"},
		{"#ABC", "#AABBCC"},
		{"#AABBCC", "#AABBCC"},
		{"red", "red"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeHexColor(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseBraceExpr_BareValueFlags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, expr *braceExpr)
	}{
		{
			name:  "bare text",
			input: "t",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "$0", expr.Text)
			},
		},
		{
			name:  "bare color",
			input: "c",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "#000000", expr.Color)
			},
		},
		{
			name:  "bare font",
			input: "f",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "Arial", expr.Font)
			},
		},
		{
			name:  "bare size",
			input: "s",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 11.0, expr.Size)
			},
		},
		{
			name:  "bare heading",
			input: "h",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "1", expr.Heading)
			},
		},
		{
			name:  "bare cols",
			input: "cols",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 1, expr.Cols)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			tt.check(t, expr)
		})
	}
}

func TestLooksLikeBraceExpr(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"b", true},
		{"bold", true},
		{"!b", true},
		{"0", true},
		{"0 b", true},
		{"+", true},
		{"+=p", true},
		{"c=red", true},
		{"unknown", false},
		{"", false},
		{"12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := looksLikeBraceExpr(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseBraceExpr_Empty(t *testing.T) {
	expr, err := parseBraceExpr("")
	require.NoError(t, err)
	assert.False(t, braceExprHasAnyFormat(expr))
}

func TestParseBraceExpr_ComplexURL(t *testing.T) {
	input := "u=chip://person/user@example.com"
	expr, err := parseBraceExpr(input)
	require.NoError(t, err)
	assert.Equal(t, "chip://person/user@example.com", expr.URL)
}

func TestParseBraceExpr_BookmarkLink(t *testing.T) {
	input := "u=#ch1 c=blue _"
	expr, err := parseBraceExpr(input)
	require.NoError(t, err)
	assert.Equal(t, "#ch1", expr.URL)
	assert.Equal(t, "#0000FF", expr.Color)
	assert.Equal(t, boolPtr(true), expr.Underline)
}

// boolPtr is a helper to create *bool for test cases.
func boolPtr(b bool) *bool {
	return &b
}
