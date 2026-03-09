package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/docs/v1"
)

func TestBuildBraceTextStyleRequests_BooleanFlags(t *testing.T) {
	tests := []struct {
		name       string
		expr       *braceExpr
		wantFields []string
		checkStyle func(t *testing.T, style *docs.TextStyle)
	}{
		{
			name:       "bold",
			expr:       &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"bold"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.True(t, style.Bold)
			},
		},
		{
			name:       "italic",
			expr:       &braceExpr{NoReset: true, Italic: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"italic"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.True(t, style.Italic)
			},
		},
		{
			name:       "underline",
			expr:       &braceExpr{NoReset: true, Underline: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"underline"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.True(t, style.Underline)
			},
		},
		{
			name:       "strikethrough",
			expr:       &braceExpr{NoReset: true, Strike: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"strikethrough"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.True(t, style.Strikethrough)
			},
		},
		{
			name:       "smallcaps",
			expr:       &braceExpr{NoReset: true, SmallCaps: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"smallCaps"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.True(t, style.SmallCaps)
			},
		},
		{
			name:       "superscript",
			expr:       &braceExpr{NoReset: true, Sup: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"baselineOffset"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.Equal(t, "SUPERSCRIPT", style.BaselineOffset)
			},
		},
		{
			name:       "subscript",
			expr:       &braceExpr{NoReset: true, Sub: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"baselineOffset"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.Equal(t, "SUBSCRIPT", style.BaselineOffset)
			},
		},
		{
			name:       "code (monospace + background)",
			expr:       &braceExpr{NoReset: true, Code: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"weightedFontFamily", "backgroundColor"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.NotNil(t, style.WeightedFontFamily)
				assert.Equal(t, "Courier New", style.WeightedFontFamily.FontFamily)
				assert.NotNil(t, style.BackgroundColor)
			},
		},
		{
			name:       "multiple flags",
			expr:       &braceExpr{NoReset: true, Bold: boolPtr(true), Italic: boolPtr(true), Underline: boolPtr(true), Indent: indentNotSet},
			wantFields: []string{"bold", "italic", "underline"},
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.True(t, style.Bold)
				assert.True(t, style.Italic)
				assert.True(t, style.Underline)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildBraceTextStyleRequests(tt.expr, 10, 20)
			require.Len(t, reqs, 1)
			require.NotNil(t, reqs[0].UpdateTextStyle)

			uts := reqs[0].UpdateTextStyle
			assert.Equal(t, int64(10), uts.Range.StartIndex)
			assert.Equal(t, int64(20), uts.Range.EndIndex)

			// Check that expected fields are present
			for _, field := range tt.wantFields {
				assert.Contains(t, uts.Fields, field)
			}

			tt.checkStyle(t, uts.TextStyle)
		})
	}
}

func TestBuildBraceTextStyleRequests_Negation(t *testing.T) {
	tests := []struct {
		name       string
		expr       *braceExpr
		wantFields string
		checkStyle func(t *testing.T, style *docs.TextStyle)
	}{
		{
			name:       "negate bold",
			expr:       &braceExpr{NoReset: true, Bold: boolPtr(false), Indent: indentNotSet},
			wantFields: "bold",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.False(t, style.Bold)
			},
		},
		{
			name:       "negate italic",
			expr:       &braceExpr{NoReset: true, Italic: boolPtr(false), Indent: indentNotSet},
			wantFields: "italic",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.False(t, style.Italic)
			},
		},
		{
			name:       "negate underline",
			expr:       &braceExpr{NoReset: true, Underline: boolPtr(false), Indent: indentNotSet},
			wantFields: "underline",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				assert.False(t, style.Underline)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildBraceTextStyleRequests(tt.expr, 0, 10)
			require.Len(t, reqs, 1)
			assert.Contains(t, reqs[0].UpdateTextStyle.Fields, tt.wantFields)
			tt.checkStyle(t, reqs[0].UpdateTextStyle.TextStyle)
		})
	}
}

func TestBuildBraceTextStyleRequests_ValueFlags(t *testing.T) {
	tests := []struct {
		name       string
		expr       *braceExpr
		wantField  string
		checkStyle func(t *testing.T, style *docs.TextStyle)
	}{
		{
			name:      "font",
			expr:      &braceExpr{NoReset: true, Font: "Georgia", Indent: indentNotSet},
			wantField: "weightedFontFamily",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				require.NotNil(t, style.WeightedFontFamily)
				assert.Equal(t, "Georgia", style.WeightedFontFamily.FontFamily)
			},
		},
		{
			name:      "size",
			expr:      &braceExpr{NoReset: true, Size: 14, Indent: indentNotSet},
			wantField: "fontSize",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				require.NotNil(t, style.FontSize)
				assert.Equal(t, 14.0, style.FontSize.Magnitude)
				assert.Equal(t, "PT", style.FontSize.Unit)
			},
		},
		{
			name:      "color hex",
			expr:      &braceExpr{NoReset: true, Color: "#FF0000", Indent: indentNotSet},
			wantField: "foregroundColor",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				require.NotNil(t, style.ForegroundColor)
				require.NotNil(t, style.ForegroundColor.Color)
				require.NotNil(t, style.ForegroundColor.Color.RgbColor)
				assert.Equal(t, 1.0, style.ForegroundColor.Color.RgbColor.Red)
				assert.Equal(t, 0.0, style.ForegroundColor.Color.RgbColor.Green)
				assert.Equal(t, 0.0, style.ForegroundColor.Color.RgbColor.Blue)
			},
		},
		{
			name:      "background hex",
			expr:      &braceExpr{NoReset: true, Bg: "#FFFF00", Indent: indentNotSet},
			wantField: "backgroundColor",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				require.NotNil(t, style.BackgroundColor)
				require.NotNil(t, style.BackgroundColor.Color)
				require.NotNil(t, style.BackgroundColor.Color.RgbColor)
				assert.Equal(t, 1.0, style.BackgroundColor.Color.RgbColor.Red)
				assert.Equal(t, 1.0, style.BackgroundColor.Color.RgbColor.Green)
				assert.Equal(t, 0.0, style.BackgroundColor.Color.RgbColor.Blue)
			},
		},
		{
			name:      "url link",
			expr:      &braceExpr{NoReset: true, URL: "https://example.com", Indent: indentNotSet},
			wantField: "link",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				require.NotNil(t, style.Link)
				assert.Equal(t, "https://example.com", style.Link.Url)
			},
		},
		{
			name:      "bookmark link",
			expr:      &braceExpr{NoReset: true, URL: "#section1", Indent: indentNotSet},
			wantField: "link",
			checkStyle: func(t *testing.T, style *docs.TextStyle) {
				t.Helper()
				require.NotNil(t, style.Link)
				assert.Equal(t, "section1", style.Link.BookmarkId)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildBraceTextStyleRequests(tt.expr, 0, 10)
			require.Len(t, reqs, 1)
			assert.Contains(t, reqs[0].UpdateTextStyle.Fields, tt.wantField)
			tt.checkStyle(t, reqs[0].UpdateTextStyle.TextStyle)
		})
	}
}

func TestBuildBraceTextStyleRequests_Reset(t *testing.T) {
	tests := []struct {
		name      string
		expr      *braceExpr
		wantReqs  int
		checkReqs func(t *testing.T, reqs []*docs.Request)
	}{
		{
			name:     "reset only",
			expr:     &braceExpr{Reset: true, Indent: indentNotSet},
			wantReqs: 1,
			checkReqs: func(t *testing.T, reqs []*docs.Request) {
				t.Helper()
				// Should have reset fields
				uts := reqs[0].UpdateTextStyle
				assert.Contains(t, uts.Fields, "bold")
				assert.Contains(t, uts.Fields, "italic")
				assert.Contains(t, uts.Fields, "foregroundColor")
			},
		},
		{
			name:     "reset then bold",
			expr:     &braceExpr{Reset: true, Bold: boolPtr(true), Indent: indentNotSet},
			wantReqs: 2,
			checkReqs: func(t *testing.T, reqs []*docs.Request) {
				t.Helper()
				// First request resets
				assert.NotNil(t, reqs[0].UpdateTextStyle)
				// Second request applies bold
				require.NotNil(t, reqs[1].UpdateTextStyle)
				assert.True(t, reqs[1].UpdateTextStyle.TextStyle.Bold)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildBraceTextStyleRequests(tt.expr, 0, 10)
			require.Len(t, reqs, tt.wantReqs)
			tt.checkReqs(t, reqs)
		})
	}
}

func TestBuildBraceParagraphStyleRequests_Heading(t *testing.T) {
	tests := []struct {
		name          string
		heading       string
		wantNamedType string
	}{
		{"title", "t", "TITLE"},
		{"subtitle", "s", "SUBTITLE"},
		{"heading 1", "1", "HEADING_1"},
		{"heading 2", "2", "HEADING_2"},
		{"heading 6", "6", "HEADING_6"},
		{"normal", "0", "NORMAL_TEXT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &braceExpr{NoReset: true, Heading: tt.heading, Indent: indentNotSet}
			reqs := buildBraceParagraphStyleRequests(expr, 0, 10)
			require.Len(t, reqs, 1)
			require.NotNil(t, reqs[0].UpdateParagraphStyle)
			assert.Equal(t, tt.wantNamedType, reqs[0].UpdateParagraphStyle.ParagraphStyle.NamedStyleType)
		})
	}
}

func TestBuildBraceParagraphStyleRequests_Alignment(t *testing.T) {
	tests := []struct {
		name          string
		align         string
		wantAlignment string
	}{
		{"left", "left", "START"},
		{"center", "center", "CENTER"},
		{"right", "right", "END"},
		{"justify", "justify", "JUSTIFIED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &braceExpr{NoReset: true, Align: tt.align, Indent: indentNotSet}
			reqs := buildBraceParagraphStyleRequests(expr, 0, 10)
			require.Len(t, reqs, 1)
			require.NotNil(t, reqs[0].UpdateParagraphStyle)
			assert.Equal(t, tt.wantAlignment, reqs[0].UpdateParagraphStyle.ParagraphStyle.Alignment)
		})
	}
}

func TestBuildBraceParagraphStyleRequests_Spacing(t *testing.T) {
	tests := []struct {
		name      string
		above     float64
		below     float64
		wantAbove float64
		wantBelow float64
	}{
		{"symmetric", 12, 12, 12, 12},
		{"asymmetric", 24, 6, 24, 6},
		{"zero", 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &braceExpr{NoReset: true, SpacingSet: true, SpacingAbove: tt.above, SpacingBelow: tt.below, Indent: indentNotSet}
			reqs := buildBraceParagraphStyleRequests(expr, 0, 10)
			require.Len(t, reqs, 1)
			require.NotNil(t, reqs[0].UpdateParagraphStyle)
			style := reqs[0].UpdateParagraphStyle.ParagraphStyle
			require.NotNil(t, style.SpaceAbove)
			require.NotNil(t, style.SpaceBelow)
			assert.Equal(t, tt.wantAbove, style.SpaceAbove.Magnitude)
			assert.Equal(t, tt.wantBelow, style.SpaceBelow.Magnitude)
		})
	}
}

func TestBuildBraceParagraphStyleRequests_Indent(t *testing.T) {
	tests := []struct {
		name       string
		indent     int
		wantIndent float64
	}{
		{"level 0", 0, 0},
		{"level 1", 1, 36},
		{"level 2", 2, 72},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &braceExpr{NoReset: true, Indent: tt.indent}
			reqs := buildBraceParagraphStyleRequests(expr, 0, 10)
			require.Len(t, reqs, 1)
			require.NotNil(t, reqs[0].UpdateParagraphStyle)
			style := reqs[0].UpdateParagraphStyle.ParagraphStyle
			require.NotNil(t, style.IndentStart)
			assert.Equal(t, tt.wantIndent, style.IndentStart.Magnitude)
		})
	}
}

func TestBuildBraceParagraphStyleRequests_Leading(t *testing.T) {
	expr := &braceExpr{NoReset: true, Leading: 1.5, Indent: indentNotSet}
	reqs := buildBraceParagraphStyleRequests(expr, 0, 10)
	require.Len(t, reqs, 1)
	require.NotNil(t, reqs[0].UpdateParagraphStyle)
	// 1.5 * 100 = 150
	assert.Equal(t, 150.0, reqs[0].UpdateParagraphStyle.ParagraphStyle.LineSpacing)
}

func TestBuildBraceInlineRequests(t *testing.T) {
	tests := []struct {
		name      string
		spans     []*braceSpan
		baseIndex int64
		wantReqs  int
	}{
		{
			name:      "empty spans",
			spans:     nil,
			baseIndex: 0,
			wantReqs:  0,
		},
		{
			name: "global span only (skipped)",
			spans: []*braceSpan{
				{Expr: &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet}, Start: 0, End: 10, IsGlobal: true},
			},
			baseIndex: 0,
			wantReqs:  0,
		},
		{
			name: "inline bold span",
			spans: []*braceSpan{
				{Expr: &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet}, Start: 0, End: 5, IsGlobal: false},
			},
			baseIndex: 10,
			wantReqs:  1,
		},
		{
			name: "multiple inline spans",
			spans: []*braceSpan{
				{Expr: &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet}, Start: 0, End: 3, IsGlobal: false},
				{Expr: &braceExpr{NoReset: true, Italic: boolPtr(true), Indent: indentNotSet}, Start: 5, End: 8, IsGlobal: false},
			},
			baseIndex: 10,
			wantReqs:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildBraceInlineRequests(tt.spans, tt.baseIndex)
			assert.Len(t, reqs, tt.wantReqs)
		})
	}
}

func TestBuildBraceBreakRequests(t *testing.T) {
	tests := []struct {
		name     string
		expr     *braceExpr
		wantReqs int
		checkReq func(t *testing.T, reqs []*docs.Request)
	}{
		{
			name:     "no break",
			expr:     &braceExpr{NoReset: true, Indent: indentNotSet},
			wantReqs: 0,
		},
		{
			name:     "page break",
			expr:     &braceExpr{NoReset: true, HasBreak: true, Break: "p", Indent: indentNotSet},
			wantReqs: 1,
			checkReq: func(t *testing.T, reqs []*docs.Request) {
				t.Helper()
				assert.NotNil(t, reqs[0].InsertPageBreak)
			},
		},
		{
			name:     "column break",
			expr:     &braceExpr{NoReset: true, HasBreak: true, Break: "c", Indent: indentNotSet},
			wantReqs: 1,
			checkReq: func(t *testing.T, reqs []*docs.Request) {
				t.Helper()
				assert.NotNil(t, reqs[0].InsertText)
				assert.Equal(t, "\v", reqs[0].InsertText.Text)
			},
		},
		{
			name:     "section break",
			expr:     &braceExpr{NoReset: true, HasBreak: true, Break: "s", Indent: indentNotSet},
			wantReqs: 1,
			checkReq: func(t *testing.T, reqs []*docs.Request) {
				t.Helper()
				assert.NotNil(t, reqs[0].InsertSectionBreak)
			},
		},
		{
			name:     "horizontal rule",
			expr:     &braceExpr{NoReset: true, HasBreak: true, Break: "", Indent: indentNotSet},
			wantReqs: 2,
			checkReq: func(t *testing.T, reqs []*docs.Request) {
				t.Helper()
				// Insert newline + style with border
				assert.NotNil(t, reqs[0].InsertText)
				assert.NotNil(t, reqs[1].UpdateParagraphStyle)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildBraceBreakRequests(tt.expr, 10)
			assert.Len(t, reqs, tt.wantReqs)
			if tt.checkReq != nil && len(reqs) > 0 {
				tt.checkReq(t, reqs)
			}
		})
	}
}

func TestBraceExprToFormats(t *testing.T) {
	tests := []struct {
		name        string
		expr        *braceExpr
		wantFormats []string
	}{
		{
			name:        "nil",
			expr:        nil,
			wantFormats: nil,
		},
		{
			name:        "bold",
			expr:        &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet},
			wantFormats: []string{"bold"},
		},
		{
			name:        "multiple boolean",
			expr:        &braceExpr{NoReset: true, Bold: boolPtr(true), Italic: boolPtr(true), Underline: boolPtr(true), Indent: indentNotSet},
			wantFormats: []string{"bold", "italic", "underline"},
		},
		{
			name:        "font and size",
			expr:        &braceExpr{NoReset: true, Font: "Georgia", Size: 14, Indent: indentNotSet},
			wantFormats: []string{"font:Georgia", "size:14"},
		},
		{
			name:        "color",
			expr:        &braceExpr{NoReset: true, Color: "#FF0000", Indent: indentNotSet},
			wantFormats: []string{"color:#FF0000"},
		},
		{
			name:        "link",
			expr:        &braceExpr{NoReset: true, URL: "https://example.com", Indent: indentNotSet},
			wantFormats: []string{"link:https://example.com"},
		},
		{
			name:        "heading title",
			expr:        &braceExpr{NoReset: true, Heading: "t", Indent: indentNotSet},
			wantFormats: []string{"title"},
		},
		{
			name:        "heading 1",
			expr:        &braceExpr{NoReset: true, Heading: "1", Indent: indentNotSet},
			wantFormats: []string{"heading1"},
		},
		{
			name:        "alignment",
			expr:        &braceExpr{NoReset: true, Align: "center", Indent: indentNotSet},
			wantFormats: []string{"align:center"},
		},
		{
			name:        "smallcaps",
			expr:        &braceExpr{NoReset: true, SmallCaps: boolPtr(true), Indent: indentNotSet},
			wantFormats: []string{"smallcaps"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formats := braceExprToFormats(tt.expr)
			assert.Equal(t, tt.wantFormats, formats)
		})
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{14, "14"},
		{14.0, "14"},
		{10.5, "10.5"},
		{1.25, "1.25"},
		{0, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatFloat(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasBraceTextFormat(t *testing.T) {
	tests := []struct {
		name string
		expr *braceExpr
		want bool
	}{
		{"nil", nil, false},
		{"empty", &braceExpr{NoReset: true, Indent: indentNotSet}, false},
		{"bold", &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet}, true},
		{"color", &braceExpr{NoReset: true, Color: "#FF0000", Indent: indentNotSet}, true},
		{"font", &braceExpr{NoReset: true, Font: "Arial", Indent: indentNotSet}, true},
		{"url", &braceExpr{NoReset: true, URL: "https://x.com", Indent: indentNotSet}, true},
		{"heading only", &braceExpr{NoReset: true, Heading: "1", Indent: indentNotSet}, false},
		{"align only", &braceExpr{NoReset: true, Align: "center", Indent: indentNotSet}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasBraceTextFormat(tt.expr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasBraceParagraphFormat(t *testing.T) {
	tests := []struct {
		name string
		expr *braceExpr
		want bool
	}{
		{"nil", nil, false},
		{"empty", &braceExpr{NoReset: true, Indent: indentNotSet}, false},
		{"heading", &braceExpr{NoReset: true, Heading: "1", Indent: indentNotSet}, true},
		{"align", &braceExpr{NoReset: true, Align: "center", Indent: indentNotSet}, true},
		{"indent", &braceExpr{NoReset: true, Indent: 1}, true},
		{"indent 0", &braceExpr{NoReset: true, Indent: 0}, true},
		{"leading", &braceExpr{NoReset: true, Leading: 1.5, Indent: indentNotSet}, true},
		{"spacing", &braceExpr{NoReset: true, SpacingSet: true, Indent: indentNotSet}, true},
		{"bold only", &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasBraceParagraphFormat(tt.expr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergeBraceSpans(t *testing.T) {
	tests := []struct {
		name  string
		spans []*braceSpan
		check func(t *testing.T, merged *braceExpr)
	}{
		{
			name:  "empty",
			spans: nil,
			check: func(t *testing.T, merged *braceExpr) {
				t.Helper()
				assert.Nil(t, merged.Bold)
			},
		},
		{
			name: "single global span",
			spans: []*braceSpan{
				{Expr: &braceExpr{NoReset: true, Bold: boolPtr(true), Italic: boolPtr(true), Indent: indentNotSet}, IsGlobal: true},
			},
			check: func(t *testing.T, merged *braceExpr) {
				t.Helper()
				require.NotNil(t, merged.Bold)
				assert.True(t, *merged.Bold)
				require.NotNil(t, merged.Italic)
				assert.True(t, *merged.Italic)
			},
		},
		{
			name: "non-global spans ignored",
			spans: []*braceSpan{
				{Expr: &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet}, IsGlobal: false},
			},
			check: func(t *testing.T, merged *braceExpr) {
				t.Helper()
				assert.Nil(t, merged.Bold)
			},
		},
		{
			name: "mixed global and non-global",
			spans: []*braceSpan{
				{Expr: &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet}, IsGlobal: true},
				{Expr: &braceExpr{NoReset: true, Italic: boolPtr(true), Indent: indentNotSet}, IsGlobal: false},
			},
			check: func(t *testing.T, merged *braceExpr) {
				t.Helper()
				require.NotNil(t, merged.Bold)
				assert.True(t, *merged.Bold)
				assert.Nil(t, merged.Italic) // Non-global not merged
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := mergeBraceSpans(tt.spans)
			tt.check(t, merged)
		})
	}
}

func TestBuildBraceTextStyleRequests_NilExpr(t *testing.T) {
	reqs := buildBraceTextStyleRequests(nil, 0, 10)
	assert.Nil(t, reqs)
}

func TestBuildBraceParagraphStyleRequests_NilExpr(t *testing.T) {
	reqs := buildBraceParagraphStyleRequests(nil, 0, 10)
	assert.Nil(t, reqs)
}

func TestBuildBraceParagraphStyleRequests_NoFormat(t *testing.T) {
	expr := &braceExpr{NoReset: true, Bold: boolPtr(true), Indent: indentNotSet} // No paragraph-level format
	reqs := buildBraceParagraphStyleRequests(expr, 0, 10)
	assert.Empty(t, reqs)
}
