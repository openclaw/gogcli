package cmd

import (
	"testing"

	"google.golang.org/api/docs/v1"
)

// Regression for #608: inline markdown markers inside table cells previously
// rendered as literal characters because the inserter passed the cell content
// straight through to InsertText without running the same inline-formatting
// pass used by paragraphs/headings.

func TestBuildTableCellRequests_AppliesInlineBold(t *testing.T) {
	reqs, inserted, err := buildTableCellRequests("**Alice**", 100, false, "")
	if err != nil {
		t.Fatal(err)
	}

	if inserted != utf16Len("Alice") {
		t.Fatalf("expected inserted len = utf16Len(\"Alice\") = %d, got %d", utf16Len("Alice"), inserted)
	}

	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (InsertText + UpdateTextStyle), got %d: %#v", len(reqs), reqs)
	}

	insert := reqs[0].InsertText
	if insert == nil || insert.Text != "Alice" {
		t.Fatalf("expected InsertText with stripped text \"Alice\", got %#v", reqs[0])
	}
	if insert.Location == nil || insert.Location.Index != 100 {
		t.Fatalf("expected insert at index 100, got %#v", insert.Location)
	}

	style := reqs[1].UpdateTextStyle
	if style == nil || style.TextStyle == nil || !style.TextStyle.Bold {
		t.Fatalf("expected UpdateTextStyle with Bold:true, got %#v", reqs[1])
	}
	if style.Range == nil || style.Range.StartIndex != 100 || style.Range.EndIndex != 100+utf16Len("Alice") {
		t.Fatalf("expected style range [100,105], got %#v", style.Range)
	}
}

func TestBuildTableCellRequests_AppliesInlineItalicAndCode(t *testing.T) {
	cases := []struct {
		name       string
		cell       string
		wantText   string
		wantBold   bool
		wantItalic bool
		wantCode   bool
	}{
		{"italic", "*important*", "important", false, true, false},
		{"code", "`xyz123`", "xyz123", false, false, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			reqs, inserted, err := buildTableCellRequests(tt.cell, 50, false, "")
			if err != nil {
				t.Fatal(err)
			}
			if inserted != utf16Len(tt.wantText) {
				t.Fatalf("inserted = %d, want %d", inserted, utf16Len(tt.wantText))
			}
			if len(reqs) < 2 {
				t.Fatalf("expected >=2 requests, got %d: %#v", len(reqs), reqs)
			}
			if got := reqs[0].InsertText; got == nil || got.Text != tt.wantText {
				t.Fatalf("InsertText.Text = %q, want %q", textOf(reqs[0]), tt.wantText)
			}
			style := reqs[1].UpdateTextStyle
			if style == nil || style.TextStyle == nil {
				t.Fatalf("expected UpdateTextStyle, got %#v", reqs[1])
			}
			if tt.wantItalic && !style.TextStyle.Italic {
				t.Fatalf("expected Italic, got %#v", style.TextStyle)
			}
			if tt.wantCode {
				font := style.TextStyle.WeightedFontFamily
				if font == nil || font.FontFamily != "Courier New" || font.Weight != 400 {
					t.Fatalf("expected Courier New 400 code styling, got %#v", style.TextStyle)
				}
			}
		})
	}
}

func TestBuildTableCellRequests_TableBreaksPreserveInlineStyleRanges(t *testing.T) {
	rows := parseTableRow("| Alice<br>**Bob** and `<br>` |")
	if len(rows) != 1 {
		t.Fatalf("parseTableRow() = %#v, want one cell", rows)
	}

	reqs, inserted, err := buildTableCellRequests(rows[0], 100, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if inserted != utf16Len("Alice\nBob and <br>") {
		t.Fatalf("inserted = %d, want %d", inserted, utf16Len("Alice\nBob and <br>"))
	}
	if len(reqs) != 3 {
		t.Fatalf("expected insert, bold, and code requests, got %d: %#v", len(reqs), reqs)
	}
	if got := reqs[0].InsertText; got == nil || got.Text != "Alice\nBob and <br>" {
		t.Fatalf("InsertText = %#v, want text %q", got, "Alice\nBob and <br>")
	}
	bold := reqs[1].UpdateTextStyle
	if bold == nil || bold.TextStyle == nil || !bold.TextStyle.Bold {
		t.Fatalf("expected bold UpdateTextStyle, got %#v", reqs[1])
	}
	if bold.Range == nil || bold.Range.StartIndex != 106 || bold.Range.EndIndex != 109 {
		t.Fatalf("bold range = %#v, want [106,109]", bold.Range)
	}
	code := reqs[2].UpdateTextStyle
	if code == nil || code.TextStyle == nil || code.TextStyle.WeightedFontFamily == nil {
		t.Fatalf("expected code UpdateTextStyle, got %#v", reqs[2])
	}
	if code.Range == nil || code.Range.StartIndex != 114 || code.Range.EndIndex != 118 {
		t.Fatalf("code range = %#v, want [114,118]", code.Range)
	}
}

func TestBuildTableCellRequests_HeaderRowAppliesBoldOverWholeCell(t *testing.T) {
	reqs, inserted, err := buildTableCellRequests("Field", 10, true, "")
	if err != nil {
		t.Fatal(err)
	}

	if inserted != utf16Len("Field") {
		t.Fatalf("inserted = %d, want %d", inserted, utf16Len("Field"))
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (InsertText + header bold), got %d: %#v", len(reqs), reqs)
	}
	style := reqs[1].UpdateTextStyle
	if style == nil || style.TextStyle == nil || !style.TextStyle.Bold {
		t.Fatalf("expected header-row UpdateTextStyle with Bold:true, got %#v", reqs[1])
	}
	if style.Range == nil || style.Range.StartIndex != 10 || style.Range.EndIndex != 10+utf16Len("Field") {
		t.Fatalf("expected style range [10,15], got %#v", style.Range)
	}
}

func TestBuildTableCellRequests_IncludesTabID(t *testing.T) {
	reqs, inserted, err := buildTableCellRequests("**Field**", 10, true, "t.second")
	if err != nil {
		t.Fatal(err)
	}
	if inserted != utf16Len("Field") {
		t.Fatalf("inserted = %d, want %d", inserted, utf16Len("Field"))
	}
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests (insert + header bold + inline bold), got %d: %#v", len(reqs), reqs)
	}
	if got := reqs[0].InsertText.Location; got == nil || got.TabId != "t.second" {
		t.Fatalf("expected insert tab ID, got %#v", got)
	}
	for i, req := range reqs[1:] {
		if req.UpdateTextStyle == nil || req.UpdateTextStyle.Range == nil {
			t.Fatalf("request %d: expected UpdateTextStyle range, got %#v", i+1, req)
		}
		if req.UpdateTextStyle.Range.TabId != "t.second" {
			t.Fatalf("request %d: expected tab ID t.second, got %#v", i+1, req.UpdateTextStyle.Range)
		}
	}
}

func TestBuildTableCellRequests_PlainTextNoStyleRequest(t *testing.T) {
	reqs, inserted, err := buildTableCellRequests("plain text", 1, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if inserted != utf16Len("plain text") {
		t.Fatalf("inserted = %d, want %d", inserted, utf16Len("plain text"))
	}
	if len(reqs) != 1 {
		t.Fatalf("expected just InsertText, got %d: %#v", len(reqs), reqs)
	}
	if got := reqs[0].InsertText; got == nil || got.Text != "plain text" {
		t.Fatalf("InsertText.Text = %q, want %q", textOf(reqs[0]), "plain text")
	}
}

func TestBuildTableCellRequests_PreservesSeparatorShapedData(t *testing.T) {
	for _, content := range []string{"---", ":---", "---:", ":---:"} {
		t.Run(content, func(t *testing.T) {
			reqs, inserted, err := buildTableCellRequests(content, 1, false, "")
			if err != nil {
				t.Fatal(err)
			}
			if inserted != utf16Len(content) {
				t.Fatalf("inserted = %d, want %d", inserted, utf16Len(content))
			}
			if len(reqs) != 1 {
				t.Fatalf("expected just InsertText, got %d: %#v", len(reqs), reqs)
			}
			if got := reqs[0].InsertText; got == nil || got.Text != content {
				t.Fatalf("InsertText.Text = %q, want %q", textOf(reqs[0]), content)
			}
		})
	}
}

func TestBuildTableCellRequests_EmptyAfterStrippingReturnsNothing(t *testing.T) {
	// A cell whose entire content is markers (e.g. "**") would strip to "".
	reqs, inserted, err := buildTableCellRequests("", 1, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 0 || inserted != 0 {
		t.Fatalf("expected (nil, 0) for empty cell, got reqs=%#v inserted=%d", reqs, inserted)
	}
}

func TestBuildTableCellRequests_PreservesNestedMarkdownLists(t *testing.T) {
	rows := parseTableRow("| nested | - a<br>  - a1<br>  - a2<br>- b |")
	if len(rows) != 2 {
		t.Fatalf("parseTableRow() = %#v, want two cells", rows)
	}

	reqs, inserted, err := buildTableCellRequests(rows[1], 100, false, "t.second")
	if err != nil {
		t.Fatal(err)
	}
	if inserted != utf16Len("a\na1\na2\nb") {
		t.Fatalf("inserted = %d, want final nested-list length %d", inserted, utf16Len("a\na1\na2\nb"))
	}
	if got := reqs[0].InsertText; got == nil || got.Text != "a\n\ta1\n\ta2\nb" {
		t.Fatalf("InsertText = %#v, want nested list text", got)
	}
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want insert+bullets: %#v", len(reqs), reqs)
	}
	bullets := reqs[1].CreateParagraphBullets
	if bullets == nil || bullets.BulletPreset != bulletPresetDisc {
		t.Fatalf("missing native bullets: %#v", reqs[1])
	}
	if bullets.Range == nil || bullets.Range.StartIndex != 100 || bullets.Range.EndIndex != 111 || bullets.Range.TabId != "t.second" {
		t.Fatalf("bullet range = %#v, want [100,111] in t.second", bullets.Range)
	}
}

func textOf(r *docs.Request) string {
	if r == nil || r.InsertText == nil {
		return ""
	}
	return r.InsertText.Text
}
