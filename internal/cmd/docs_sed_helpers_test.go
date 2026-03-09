package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/docs/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/ui"
)

func TestIsMergeOp(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"merge", true},
		{"MERGE", true},
		{" Merge ", true},
		{"unmerge", true},
		{"split", true},
		{"replace", false},
		{"", false},
		{"merged", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isMergeOp(tt.input), "isMergeOp(%q)", tt.input)
	}
}

func TestCanBatchCell(t *testing.T) {
	// Simple whole-cell replacement — can batch
	ie := indexedExpr{
		index: 0,
		expr: sedExpr{
			replacement: "hello",
			cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1},
		},
	}
	assert.True(t, canBatchCell(ie))

	// No cellRef — cannot batch
	ie2 := indexedExpr{expr: sedExpr{replacement: "hello"}}
	assert.False(t, canBatchCell(ie2))

	// Has pattern — cannot batch
	ie3 := indexedExpr{
		expr: sedExpr{
			pattern:     "foo",
			replacement: "bar",
			cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1},
		},
	}
	assert.False(t, canBatchCell(ie3))

	// Merge op — cannot batch
	ie4 := indexedExpr{
		expr: sedExpr{
			replacement: "merge",
			cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1},
		},
	}
	assert.False(t, canBatchCell(ie4))

	// Row op — cannot batch
	ie5 := indexedExpr{
		expr: sedExpr{
			replacement: "hello",
			cellRef:     &tableCellRef{tableIndex: 1, row: 1, col: 1, rowOp: "append"},
		},
	}
	assert.False(t, canBatchCell(ie5))

	// Row=0 — cannot batch
	ie6 := indexedExpr{
		expr: sedExpr{
			replacement: "hello",
			cellRef:     &tableCellRef{tableIndex: 1, row: 0, col: 1},
		},
	}
	assert.False(t, canBatchCell(ie6))
}

func TestTruncateSed(t *testing.T) {
	assert.Equal(t, "hello", truncateSed("hello", 10))
	assert.Equal(t, "hello", truncateSed("hello", 5))
	assert.Equal(t, "he...", truncateSed("hello world", 5))
	assert.Equal(t, "hel...", truncateSed("hello world", 6))
}

func TestFormatBraceFlags(t *testing.T) {
	assert.Equal(t, "", formatBraceFlags(nil))

	b := true
	f := false
	be := &braceExpr{Bold: &b}
	result := formatBraceFlags(be)
	assert.Contains(t, result, "b")

	be2 := &braceExpr{Bold: &f}
	result2 := formatBraceFlags(be2)
	assert.Contains(t, result2, "!b")

	be3 := &braceExpr{Reset: true, Italic: &b, Underline: &b}
	result3 := formatBraceFlags(be3)
	assert.Contains(t, result3, "0")
	assert.Contains(t, result3, "i")
	assert.Contains(t, result3, "_")
}

func TestIsRetryableError(t *testing.T) {
	assert.False(t, isRetryableError(nil))
	assert.False(t, isRetryableError(errors.New("some error")))

	// 429 rate limit
	apiErr429 := &gapi.Error{Code: 429}
	assert.True(t, isRetryableError(apiErr429))

	// 500 server error
	apiErr500 := &gapi.Error{Code: 500}
	assert.True(t, isRetryableError(apiErr500))

	// 502 bad gateway
	apiErr502 := &gapi.Error{Code: 502}
	assert.True(t, isRetryableError(apiErr502))

	// 503 service unavailable
	apiErr503 := &gapi.Error{Code: 503}
	assert.True(t, isRetryableError(apiErr503))

	// 404 not found — not retryable
	apiErr404 := &gapi.Error{Code: 404}
	assert.False(t, isRetryableError(apiErr404))

	// String-based match
	assert.True(t, isRetryableError(errors.New("rateLimitExceeded")))
	assert.True(t, isRetryableError(errors.New("error 429 too many requests")))
}

func TestCompilePattern(t *testing.T) {
	e := sedExpr{pattern: `\d+`}
	re, err := e.compilePattern()
	assert.NoError(t, err)
	assert.NotNil(t, re)
	assert.True(t, re.MatchString("123"))

	e2 := sedExpr{pattern: `[invalid`}
	_, err2 := e2.compilePattern()
	assert.Error(t, err2)
}

func TestRunDryRun(t *testing.T) {
	var buf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	require.NoError(t, err)

	cmd := &DocsSedCmd{}
	exprs := []sedExpr{
		{pattern: "hello", replacement: "world", global: true},
		{pattern: "[invalid", replacement: "x"},
		{command: 'd', pattern: "delete-me"},
	}

	err = cmd.runDryRun(context.Background(), u, exprs)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "native") // first expr is native (global, plain replace)
	assert.Contains(t, output, "ERROR")  // second expr has invalid regex
	assert.Contains(t, output, "delete") // third is delete command
	assert.Contains(t, output, "dry-run: 3 expressions parsed")
}

func TestRunDryRun_WithBrace(t *testing.T) {
	var buf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	require.NoError(t, err)

	b := true
	cmd := &DocsSedCmd{}
	exprs := []sedExpr{
		{pattern: "hello", replacement: "world", brace: &braceExpr{Bold: &b}},
	}

	err = cmd.runDryRun(context.Background(), u, exprs)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "brace")
	assert.Contains(t, buf.String(), "b")
}

func TestRunDryRun_CellExpr(t *testing.T) {
	var buf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	require.NoError(t, err)

	cmd := &DocsSedCmd{}
	exprs := []sedExpr{
		{
			pattern:     "",
			replacement: "new value",
			cellRef:     &tableCellRef{tableIndex: 1, row: 2, col: 3},
		},
	}

	err = cmd.runDryRun(context.Background(), u, exprs)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "cell")
}

func TestInferBulletPreset(t *testing.T) {
	// nil doc.Lists
	doc := &docs.Document{}
	assert.Equal(t, bulletPresetDisc, inferBulletPreset(doc, "list1"))

	// Unknown list ID
	doc.Lists = map[string]docs.List{}
	assert.Equal(t, bulletPresetDisc, inferBulletPreset(doc, "unknown"))

	// Unordered list (default)
	doc.Lists["list1"] = docs.List{
		ListProperties: &docs.ListProperties{
			NestingLevels: []*docs.NestingLevel{
				{GlyphType: "GLYPH_TYPE_UNSPECIFIED"},
			},
		},
	}
	assert.Equal(t, bulletPresetDisc, inferBulletPreset(doc, "list1"))

	// Ordered list (DECIMAL)
	doc.Lists["list2"] = docs.List{
		ListProperties: &docs.ListProperties{
			NestingLevels: []*docs.NestingLevel{
				{GlyphType: "DECIMAL"},
			},
		},
	}
	assert.Equal(t, "NUMBERED_DECIMAL_NESTED", inferBulletPreset(doc, "list2"))

	// Ordered list (UPPER_ALPHA)
	doc.Lists["list3"] = docs.List{
		ListProperties: &docs.ListProperties{
			NestingLevels: []*docs.NestingLevel{
				{GlyphType: "UPPER_ALPHA"},
			},
		},
	}
	assert.Equal(t, "NUMBERED_DECIMAL_NESTED", inferBulletPreset(doc, "list3"))
}

func TestBuildTextStyleRequests_Superscript(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"superscript"}, 1, 5)
	require.Len(t, reqs, 1)
	assert.Equal(t, "SUPERSCRIPT", reqs[0].UpdateTextStyle.TextStyle.BaselineOffset)
}

func TestBuildTextStyleRequests_Subscript(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"subscript"}, 1, 5)
	require.Len(t, reqs, 1)
	assert.Equal(t, "SUBSCRIPT", reqs[0].UpdateTextStyle.TextStyle.BaselineOffset)
}

func TestBuildTextStyleRequests_SmallCaps(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"smallcaps"}, 1, 5)
	require.Len(t, reqs, 1)
	assert.True(t, reqs[0].UpdateTextStyle.TextStyle.SmallCaps)
}

func TestBuildTextStyleRequests_Font(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"font:Arial"}, 1, 5)
	require.Len(t, reqs, 1)
	assert.Equal(t, "Arial", reqs[0].UpdateTextStyle.TextStyle.WeightedFontFamily.FontFamily)
}

func TestBuildTextStyleRequests_Size(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"size:14"}, 1, 5)
	require.Len(t, reqs, 1)
	assert.Equal(t, float64(14), reqs[0].UpdateTextStyle.TextStyle.FontSize.Magnitude)
}

func TestBuildTextStyleRequests_Color(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"color:#ff0000"}, 1, 5)
	require.Len(t, reqs, 1)
	assert.InDelta(t, 1.0, reqs[0].UpdateTextStyle.TextStyle.ForegroundColor.Color.RgbColor.Red, 0.01)
}

func TestBuildTextStyleRequests_BgColor(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"bg:#00ff00"}, 1, 5)
	require.Len(t, reqs, 1)
	assert.InDelta(t, 1.0, reqs[0].UpdateTextStyle.TextStyle.BackgroundColor.Color.RgbColor.Green, 0.01)
}

func TestBuildTextStyleRequests_BookmarkLink(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"link:#bookmark1"}, 1, 5)
	require.Len(t, reqs, 1)
	assert.Equal(t, "bookmark1", reqs[0].UpdateTextStyle.TextStyle.Link.BookmarkId)
}

func TestBuildTextStyleRequests_Empty(t *testing.T) {
	reqs := buildTextStyleRequests([]string{}, 1, 5)
	assert.Nil(t, reqs)
}

func TestBuildTextStyleRequests_UnknownFormat(t *testing.T) {
	reqs := buildTextStyleRequests([]string{"unknownformat"}, 1, 5)
	assert.Nil(t, reqs)
}
