package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBraceExpr_BooleanFlags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantBold *bool
		wantItal *bool
		wantUndr *bool
		wantStrk *bool
		wantCode *bool
		wantSup  *bool
		wantSub  *bool
		wantSC   *bool
	}{
		{"bold short", "b", boolPtr(true), nil, nil, nil, nil, nil, nil, nil},
		{"bold long", "bold", boolPtr(true), nil, nil, nil, nil, nil, nil, nil},
		{"italic short", "i", nil, boolPtr(true), nil, nil, nil, nil, nil, nil},
		{"italic long", "italic", nil, boolPtr(true), nil, nil, nil, nil, nil, nil},
		{"underline short", "_", nil, nil, boolPtr(true), nil, nil, nil, nil, nil},
		{"underline long", "underline", nil, nil, boolPtr(true), nil, nil, nil, nil, nil},
		{"strike short", "-", nil, nil, nil, boolPtr(true), nil, nil, nil, nil},
		{"strike long", "strike", nil, nil, nil, boolPtr(true), nil, nil, nil, nil},
		{"code short", "#", nil, nil, nil, nil, boolPtr(true), nil, nil, nil},
		{"code long", "code", nil, nil, nil, nil, boolPtr(true), nil, nil, nil},
		{"sup short", "^", nil, nil, nil, nil, nil, boolPtr(true), nil, nil},
		{"sup long", "sup", nil, nil, nil, nil, nil, boolPtr(true), nil, nil},
		{"sub short", ",", nil, nil, nil, nil, nil, nil, boolPtr(true), nil},
		{"sub long", "sub", nil, nil, nil, nil, nil, nil, boolPtr(true), nil},
		{"smallcaps short", "w", nil, nil, nil, nil, nil, nil, nil, boolPtr(true)},
		{"smallcaps long", "smallcaps", nil, nil, nil, nil, nil, nil, nil, boolPtr(true)},
		{"multiple flags", "b i _", boolPtr(true), boolPtr(true), boolPtr(true), nil, nil, nil, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBold, expr.Bold)
			assert.Equal(t, tt.wantItal, expr.Italic)
			assert.Equal(t, tt.wantUndr, expr.Underline)
			assert.Equal(t, tt.wantStrk, expr.Strike)
			assert.Equal(t, tt.wantCode, expr.Code)
			assert.Equal(t, tt.wantSup, expr.Sup)
			assert.Equal(t, tt.wantSub, expr.Sub)
			assert.Equal(t, tt.wantSC, expr.SmallCaps)
		})
	}
}

func TestParseBraceExpr_Negation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantBold *bool
		wantItal *bool
	}{
		{"negate bold short", "!b", boolPtr(false), nil},
		{"negate bold long", "!bold", boolPtr(false), nil},
		{"negate italic short", "!i", nil, boolPtr(false)},
		{"negate multiple", "!b !i", boolPtr(false), boolPtr(false)},
		{"mixed enable disable", "b !i", boolPtr(true), boolPtr(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBold, expr.Bold)
			assert.Equal(t, tt.wantItal, expr.Italic)
		})
	}
}

func TestParseBraceExpr_ValueFlags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, expr *braceExpr)
	}{
		{
			name:  "text flag",
			input: "t=hello",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "hello", expr.Text)
			},
		},
		{
			name:  "text flag long",
			input: "text=world",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "world", expr.Text)
			},
		},
		{
			name:  "color named",
			input: "c=red",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "#FF0000", expr.Color)
			},
		},
		{
			name:  "color hex",
			input: "c=#00FF00",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "#00FF00", expr.Color)
			},
		},
		{
			name:  "background named",
			input: "z=yellow",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "#FFFF00", expr.Bg)
			},
		},
		{
			name:  "font",
			input: "f=Georgia",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "Georgia", expr.Font)
			},
		},
		{
			name:  "size",
			input: "s=14",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 14.0, expr.Size)
			},
		},
		{
			name:  "size float",
			input: "s=10.5",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 10.5, expr.Size)
			},
		},
		{
			name:  "url",
			input: "u=https://example.com",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "https://example.com", expr.URL)
			},
		},
		{
			name:  "heading title",
			input: "h=t",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "t", expr.Heading)
			},
		},
		{
			name:  "heading subtitle",
			input: "h=s",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "s", expr.Heading)
			},
		},
		{
			name:  "heading level",
			input: "h=2",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "2", expr.Heading)
			},
		},
		{
			name:  "heading reset",
			input: "h=0",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "0", expr.Heading)
			},
		},
		{
			name:  "leading",
			input: "l=1.5",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 1.5, expr.Leading)
			},
		},
		{
			name:  "align left",
			input: "a=left",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "left", expr.Align)
			},
		},
		{
			name:  "align center",
			input: "a=center",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "center", expr.Align)
			},
		},
		{
			name:  "opacity",
			input: "o=50",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 50, expr.Opacity)
			},
		},
		{
			name:  "indent",
			input: "n=2",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 2, expr.Indent)
			},
		},
		{
			name:  "kerning",
			input: "k=0.5",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 0.5, expr.Kerning)
			},
		},
		{
			name:  "width",
			input: "x=600",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 600, expr.Width)
			},
		},
		{
			name:  "height",
			input: "y=400",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 400, expr.Height)
			},
		},
		{
			name:  "effect",
			input: "e=shadow",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "shadow", expr.Effect)
			},
		},
		{
			name:  "cols",
			input: "cols=2",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, 2, expr.Cols)
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

func TestParseBraceExpr_InlineScoping(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSpans  int
		wantText   string
		wantFlags  []string
		checkBools func(t *testing.T, expr *braceExpr)
	}{
		{
			name:      "bold inline",
			input:     "b=Warning",
			wantSpans: 1,
			wantText:  "Warning",
			wantFlags: []string{"b"},
		},
		{
			name:      "superscript inline",
			input:     "^=2",
			wantSpans: 1,
			wantText:  "2",
			wantFlags: []string{"^"},
		},
		{
			name:      "subscript inline",
			input:     ",=2",
			wantSpans: 1,
			wantText:  "2",
			wantFlags: []string{","},
		},
		{
			name:      "italic inline",
			input:     "i=emphasis",
			wantSpans: 1,
			wantText:  "emphasis",
			wantFlags: []string{"i"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			require.Len(t, expr.InlineSpans, tt.wantSpans)
			if tt.wantSpans > 0 {
				assert.Equal(t, tt.wantText, expr.InlineSpans[0].Text)
				assert.Equal(t, tt.wantFlags, expr.InlineSpans[0].Flags)
			}
		})
	}
}

func TestParseBraceExpr_Reset(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantReset bool
		wantBold  *bool
	}{
		{"reset only", "0", true, nil},
		{"reset then bold", "0 b", true, boolPtr(true)},
		{"reset with color", "0 c=red", true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantReset, expr.Reset)
			assert.Equal(t, tt.wantBold, expr.Bold)
		})
	}
}

func TestParseBraceExpr_NoReset(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantNoReset bool
		wantBold    *bool
	}{
		{"no-reset only", "!0", true, nil},
		{"no-reset then bold", "!0 b", true, boolPtr(true)},
		{"no-reset with color", "!0 c=red", true, nil},
		{"plain bold has no NoReset", "b", false, boolPtr(true)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantNoReset, expr.NoReset)
			assert.Equal(t, tt.wantBold, expr.Bold)
		})
	}
}

func TestBuildBraceTextStyleRequests_ImplicitReset(t *testing.T) {
	// Without NoReset, a simple {b} should produce 2 requests: reset + bold
	expr := &braceExpr{Bold: boolPtr(true), Indent: indentNotSet}
	reqs := buildBraceTextStyleRequests(expr, 1, 10)
	require.Len(t, reqs, 2, "implicit reset should produce reset + style requests")
	// First request is the reset
	assert.Contains(t, reqs[0].UpdateTextStyle.Fields, "bold")
	assert.Contains(t, reqs[0].UpdateTextStyle.Fields, "baselineOffset")
	// Second request is the bold
	assert.True(t, reqs[1].UpdateTextStyle.TextStyle.Bold)

	// With NoReset, only 1 request (additive)
	exprAdditive := &braceExpr{Bold: boolPtr(true), NoReset: true, Indent: indentNotSet}
	reqs2 := buildBraceTextStyleRequests(exprAdditive, 1, 10)
	require.Len(t, reqs2, 1, "additive mode should produce only style request")
	assert.True(t, reqs2[0].UpdateTextStyle.TextStyle.Bold)
}

func TestParseBraceExpr_Break(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantHasBreak bool
		wantBreak    string
	}{
		{"horizontal rule", "+", true, ""},
		{"page break", "+=p", true, "p"},
		{"column break", "+=c", true, "c"},
		{"section break", "+=s", true, "s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHasBreak, expr.HasBreak)
			assert.Equal(t, tt.wantBreak, expr.Break)
		})
	}
}

func TestParseBraceExpr_Comment(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantComment string
		wantBold    *bool
	}{
		{"comment only", `"=needs review`, "needs review", nil},
		{"comment with spaces", `"=this is a long comment`, "this is a long comment", nil},
		{"comment with bold", `b "=needs review`, "needs review", boolPtr(true)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantComment, expr.Comment)
			assert.Equal(t, tt.wantBold, expr.Bold)
		})
	}
}

func TestParseBraceExpr_Bookmark(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantBookmark string
	}{
		{"simple bookmark", "@=ch1", "ch1"},
		{"bookmark with heading", "@=section-1 h=1", "section-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBookmark, expr.Bookmark)
		})
	}
}

func TestParseBraceExpr_Combined(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, expr *braceExpr)
	}{
		{
			name:  "bold red yellow",
			input: "b c=red z=yellow s=14",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, boolPtr(true), expr.Bold)
				assert.Equal(t, "#FF0000", expr.Color)
				assert.Equal(t, "#FFFF00", expr.Bg)
				assert.Equal(t, 14.0, expr.Size)
			},
		},
		{
			name:  "heading with alignment",
			input: "h=1 a=center b",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "1", expr.Heading)
				assert.Equal(t, "center", expr.Align)
				assert.Equal(t, boolPtr(true), expr.Bold)
			},
		},
		{
			name:  "link with color",
			input: "u=https://example.com c=blue _",
			check: func(t *testing.T, expr *braceExpr) {
				t.Helper()
				assert.Equal(t, "https://example.com", expr.URL)
				assert.Equal(t, "#0000FF", expr.Color)
				assert.Equal(t, boolPtr(true), expr.Underline)
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

func TestParseBraceExpr_NamedColors(t *testing.T) {
	tests := []struct {
		name      string
		colorName string
		wantHex   string
	}{
		{"black", "black", "#000000"},
		{"white", "white", "#FFFFFF"},
		{"red", "red", "#FF0000"},
		{"green", "green", "#00FF00"},
		{"blue", "blue", "#0000FF"},
		{"yellow", "yellow", "#FFFF00"},
		{"cyan", "cyan", "#00FFFF"},
		{"magenta", "magenta", "#FF00FF"},
		{"orange", "orange", "#FF8C00"},
		{"purple", "purple", "#800080"},
		{"pink", "pink", "#FF69B4"},
		{"brown", "brown", "#8B4513"},
		{"gray", "gray", "#808080"},
		{"grey", "grey", "#808080"},
		{"lightgray", "lightgray", "#D3D3D3"},
		{"darkgray", "darkgray", "#404040"},
		{"navy", "navy", "#000080"},
		{"teal", "teal", "#008080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr("c=" + tt.colorName)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHex, expr.Color)
		})
	}
}

func TestParseBraceExpr_ParagraphSpacing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSet   bool
		wantAbove float64
		wantBelow float64
	}{
		{"single value", "p=12", true, 12, 12},
		{"above and below", "p=12,6", true, 12, 6},
		{"zero", "p=0", true, 0, 0},
		{"zero above", "p=0,12", true, 0, 12},
		{"bare reset", "p", true, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSet, expr.SpacingSet)
			assert.Equal(t, tt.wantAbove, expr.SpacingAbove)
			assert.Equal(t, tt.wantBelow, expr.SpacingBelow)
		})
	}
}

func TestParseBraceExpr_Checkboxes(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCheck *bool
	}{
		{"bare check", "check", boolPtr(false)},
		{"check yes", "check=y", boolPtr(true)},
		{"check no", "check=n", boolPtr(false)},
		{"check true", "check=true", boolPtr(true)},
		{"check false", "check=false", boolPtr(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCheck, expr.Check)
		})
	}
}

func TestParseBraceExpr_TOC(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantHas   bool
		wantDepth int
	}{
		{"bare toc", "toc", true, -1},
		{"toc depth 2", "toc=2", true, 2},
		{"toc depth 3", "toc=3", true, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHas, expr.HasTOC)
			assert.Equal(t, tt.wantDepth, expr.TOC)
		})
	}
}

func TestParseBraceExpr_ImageAndTable(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantImgRef   string
		wantTableRef string
	}{
		{"image ref", "img=1", "1", ""},
		{"image last", "img=-1", "-1", ""},
		{"image all", "img=*", "*", ""},
		{"table ref", "T=1!A1", "", "1!A1"},
		{"table create", "T=3x4", "", "3x4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseBraceExpr(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantImgRef, expr.ImgRef)
			assert.Equal(t, tt.wantTableRef, expr.TableRef)
		})
	}
}

func TestFindBraceExprs_Basic(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantText  string
		wantSpans int
		checkSpan func(t *testing.T, spans []*braceSpan)
	}{
		{
			name:      "simple bold",
			input:     "{b}",
			wantText:  "",
			wantSpans: 1,
			checkSpan: func(t *testing.T, spans []*braceSpan) {
				t.Helper()
				assert.True(t, spans[0].IsGlobal)
				assert.Equal(t, boolPtr(true), spans[0].Expr.Bold)
			},
		},
		{
			name:      "bold with text",
			input:     "{b t=hello}",
			wantText:  "hello",
			wantSpans: 1,
			checkSpan: func(t *testing.T, spans []*braceSpan) {
				t.Helper()
				assert.Equal(t, 0, spans[0].Start)
				assert.Equal(t, 5, spans[0].End)
			},
		},
		{
			name:      "no braces",
			input:     "plain text",
			wantText:  "plain text",
			wantSpans: 0,
		},
		{
			name:      "escaped braces",
			input:     `\{not a brace\}`,
			wantText:  "{not a brace}",
			wantSpans: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, spans := findBraceExprs(tt.input)
			assert.Equal(t, tt.wantText, text)
			require.Len(t, spans, tt.wantSpans)
			if tt.checkSpan != nil && len(spans) > 0 {
				tt.checkSpan(t, spans)
			}
		})
	}
}

func TestFindBraceExprs_InlineScoping(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantText  string
		wantSpans int
		check     func(t *testing.T, spans []*braceSpan)
	}{
		{
			name:      "H2O subscript",
			input:     "H{,=2}O",
			wantText:  "H2O",
			wantSpans: 1,
			check: func(t *testing.T, spans []*braceSpan) {
				t.Helper()
				assert.Equal(t, 1, spans[0].Start)
				assert.Equal(t, 2, spans[0].End)
				assert.Equal(t, boolPtr(true), spans[0].Expr.Sub)
			},
		},
		{
			name:      "E=mc2 superscript",
			input:     "E=mc{^=2}",
			wantText:  "E=mc2",
			wantSpans: 1,
			check: func(t *testing.T, spans []*braceSpan) {
				t.Helper()
				assert.Equal(t, 4, spans[0].Start)
				assert.Equal(t, 5, spans[0].End)
				assert.Equal(t, boolPtr(true), spans[0].Expr.Sup)
			},
		},
		{
			name:      "bold warning",
			input:     "{b=Warning}: please read",
			wantText:  "Warning: please read",
			wantSpans: 1,
			check: func(t *testing.T, spans []*braceSpan) {
				t.Helper()
				assert.Equal(t, 0, spans[0].Start)
				assert.Equal(t, 7, spans[0].End)
				assert.Equal(t, boolPtr(true), spans[0].Expr.Bold)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, spans := findBraceExprs(tt.input)
			assert.Equal(t, tt.wantText, text)
			require.Len(t, spans, tt.wantSpans)
			if tt.check != nil {
				tt.check(t, spans)
			}
		})
	}
}

func TestFindBraceExprs_MultipleBraces(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantText  string
		wantSpans int
	}{
		{
			name:      "glucose formula",
			input:     "C{,=6}H{,=12}O{,=6}",
			wantText:  "C6H12O6",
			wantSpans: 3,
		},
		{
			name:      "pythagorean",
			input:     "x{^=2}+y{^=2}=z{^=2}",
			wantText:  "x2+y2=z2",
			wantSpans: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, spans := findBraceExprs(tt.input)
			assert.Equal(t, tt.wantText, text)
			assert.Len(t, spans, tt.wantSpans)
		})
	}
}

func TestFindMatchingBrace_EdgeCases(t *testing.T) {
	// Out of bounds
	assert.Equal(t, -1, findMatchingBrace("", 0))
	assert.Equal(t, -1, findMatchingBrace("abc", 5))
	// Not a brace
	assert.Equal(t, -1, findMatchingBrace("abc", 0))
	// Unmatched
	assert.Equal(t, -1, findMatchingBrace("{abc", 0))
	// Escaped closing brace
	assert.Equal(t, 5, findMatchingBrace("{ab\\}}", 0))
	// Nested braces
	assert.Equal(t, 5, findMatchingBrace("{{ab}}", 0))
	assert.Equal(t, 3, findMatchingBrace("{{a}}", 1))
	// Simple
	assert.Equal(t, 1, findMatchingBrace("{}", 0))
}

func TestTokenizeBraceContent_Quotes(t *testing.T) {
	// Single-quoted value
	tokens := tokenizeBraceContent("f='Times New Roman' s=14")
	assert.Equal(t, []string{"f='Times New Roman'", "s=14"}, tokens)
	// Double-quoted value
	tokens = tokenizeBraceContent(`f="Courier New" b`)
	assert.Equal(t, []string{`f="Courier New"`, "b"}, tokens)
	// Tab separator
	tokens = tokenizeBraceContent("b\ti")
	assert.Equal(t, []string{"b", "i"}, tokens)
	// Empty
	tokens = tokenizeBraceContent("")
	assert.Nil(t, tokens)
}

func TestResolveHeading_AllValues(t *testing.T) {
	assert.Equal(t, "TITLE", resolveHeading("t"))
	assert.Equal(t, "SUBTITLE", resolveHeading("s"))
	assert.Equal(t, "HEADING_1", resolveHeading("1"))
	assert.Equal(t, "HEADING_6", resolveHeading("6"))
	assert.Equal(t, "NORMAL_TEXT", resolveHeading("0"))
	// Numeric string fallthrough
	assert.Equal(t, "HEADING_3", resolveHeading("3"))
	// Unknown passthrough
	assert.Equal(t, "CUSTOM", resolveHeading("CUSTOM"))
}

func TestClassifyMatch_BraceImage(t *testing.T) {
	expr := sedExpr{
		brace: &braceExpr{ImgRef: "https://example.com/img.png", Width: 100, Height: 50, Indent: indentNotSet},
	}
	m := classifyMatch(10, []int{0, 5}, "hello", "world", expr)
	assert.NotNil(t, m.image)
	assert.Equal(t, "https://example.com/img.png", m.image.URL)
	assert.Equal(t, 100, m.image.Width)
	assert.Equal(t, 50, m.image.Height)
	assert.Equal(t, int64(10), m.start)
	assert.Equal(t, int64(15), m.end)
}

func TestClassifyMatch_PlainText(t *testing.T) {
	expr := sedExpr{}
	m := classifyMatch(0, []int{0, 3}, "foo", "bar", expr)
	assert.Nil(t, m.image)
	assert.Equal(t, "bar", m.newText)
}

func TestBraceExprHasAnyFormat_Comprehensive(t *testing.T) {
	assert.False(t, braceExprHasAnyFormat(nil))
	assert.False(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet}))
	// Check each individual flag
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, Effect: "glow"}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, Cols: 2}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, HasTOC: true}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, ImgRef: "url"}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, TableRef: "1"}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, Bookmark: "x"}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, Kerning: 1.5}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, Opacity: 50}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, Leading: 1.5}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: 0})) // 0 means explicitly set to 0
	tr := true
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, Check: &tr}))
	assert.True(t, braceExprHasAnyFormat(&braceExpr{Indent: indentNotSet, InlineSpans: []inlineSpan{{Text: "x"}}}))
}
