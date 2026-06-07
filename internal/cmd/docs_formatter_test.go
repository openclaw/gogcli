package cmd

import (
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestMarkdownToDocsRequests_BaseIndex(t *testing.T) {
	elements := []MarkdownElement{{Type: MDParagraph, Content: "**bold**"}}
	requests, text, tables := MarkdownToDocsRequests(elements, 42, "")

	if text != "bold\n" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(tables) != 0 {
		t.Fatalf("unexpected tables: %d", len(tables))
	}
	if len(requests) != 1 || requests[0].UpdateTextStyle == nil {
		t.Fatalf("expected one text-style request, got %#v", requests)
	}

	rng := requests[0].UpdateTextStyle.Range
	if rng.StartIndex != 42 || rng.EndIndex != 46 {
		t.Fatalf("unexpected range: [%d,%d]", rng.StartIndex, rng.EndIndex)
	}
}

func TestMarkdownToDocsRequests_TableStartIndexUsesBase(t *testing.T) {
	elements := []MarkdownElement{
		{Type: MDParagraph, Content: "A"},
		{Type: MDTable, TableCells: [][]string{{"h1", "h2"}, {"v1", "v2"}}},
	}
	_, text, tables := MarkdownToDocsRequests(elements, 10, "")

	if text != "A\n\n" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].StartIndex != 12 {
		t.Fatalf("unexpected table start index: %d", tables[0].StartIndex)
	}
}

func TestMarkdownToDocsRequests_Strikethrough(t *testing.T) {
	elements := []MarkdownElement{{Type: MDParagraph, Content: "~~struck out~~ vs **bold**"}}
	requests, text, tables := MarkdownToDocsRequests(elements, 10, "t.second")

	if text != "struck out vs bold\n" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(tables) != 0 {
		t.Fatalf("unexpected tables: %d", len(tables))
	}

	var sawStrike bool
	for _, req := range requests {
		if req.UpdateTextStyle == nil || req.UpdateTextStyle.TextStyle == nil {
			continue
		}
		if !req.UpdateTextStyle.TextStyle.Strikethrough {
			continue
		}
		sawStrike = true
		if req.UpdateTextStyle.Fields != "strikethrough" {
			t.Fatalf("unexpected strikethrough fields: %q", req.UpdateTextStyle.Fields)
		}
		if got := req.UpdateTextStyle.Range; got.StartIndex != 10 || got.EndIndex != 20 || got.TabId != "t.second" {
			t.Fatalf("unexpected strikethrough range: %#v", got)
		}
	}
	if !sawStrike {
		t.Fatalf("missing strikethrough request: %#v", requests)
	}
}

func TestMarkdownToDocsRequests_NestedLists(t *testing.T) {
	elements := ParseMarkdown("- Parent\n  - **Child**\n    - Grandchild\n\n1. One\n  1. Nested one")
	requests, text, tables := MarkdownToDocsRequests(elements, 10, "t.second")

	wantText := "Parent\n\tChild\n\t\tGrandchild\n\nOne\n\tNested one\n"
	if text != wantText {
		t.Fatalf("text = %q, want %q", text, wantText)
	}
	if len(tables) != 0 {
		t.Fatalf("unexpected tables: %d", len(tables))
	}

	wantBullets := []struct {
		start  int64
		end    int64
		preset string
	}{
		{10, 37, bulletPresetDisc},
		{35, 51, bulletPresetNumbered},
	}
	var gotBullets []struct {
		start  int64
		end    int64
		preset string
	}
	var boldRange *docs.Range
	for _, req := range requests {
		if req.CreateParagraphBullets != nil {
			got := req.CreateParagraphBullets
			gotBullets = append(gotBullets, struct {
				start  int64
				end    int64
				preset string
			}{got.Range.StartIndex, got.Range.EndIndex, got.BulletPreset})
		}
		if req.UpdateTextStyle != nil && req.UpdateTextStyle.TextStyle != nil && req.UpdateTextStyle.TextStyle.Bold {
			boldRange = req.UpdateTextStyle.Range
		}
	}
	if len(gotBullets) != len(wantBullets) {
		t.Fatalf("bullet requests = %#v, want %#v", gotBullets, wantBullets)
	}
	for i, want := range wantBullets {
		if got := gotBullets[i]; got != want {
			t.Fatalf("bullet %d = %#v, want %#v", i, got, want)
		}
	}
	if boldRange == nil || boldRange.StartIndex != 17 || boldRange.EndIndex != 22 || boldRange.TabId != "t.second" {
		t.Fatalf("unexpected bold range after nested bullet tab removal: %#v", boldRange)
	}
}

func TestMarkdownToDocsRequests_MixedListChildrenStayNested(t *testing.T) {
	elements := ParseMarkdown("1. Parent\n  - Bullet child\n  1. Number child\n2. Sibling")
	requests, text, tables := MarkdownToDocsRequests(elements, 1, "t.second")

	wantText := "Parent\n\tBullet child\n\tNumber child\nSibling\n"
	if text != wantText {
		t.Fatalf("text = %q, want %q", text, wantText)
	}
	if len(tables) != 0 {
		t.Fatalf("unexpected tables: %d", len(tables))
	}

	wantBullets := []struct {
		start  int64
		end    int64
		preset string
	}{
		{1, 44, bulletPresetNumbered},
		{8, 21, bulletPresetDisc},
	}
	var gotBullets []struct {
		start  int64
		end    int64
		preset string
	}
	for _, req := range requests {
		if req.CreateParagraphBullets != nil {
			got := req.CreateParagraphBullets
			gotBullets = append(gotBullets, struct {
				start  int64
				end    int64
				preset string
			}{got.Range.StartIndex, got.Range.EndIndex, got.BulletPreset})
		}
	}
	if len(gotBullets) != len(wantBullets) {
		t.Fatalf("bullet requests = %#v, want %#v", gotBullets, wantBullets)
	}
	for i, want := range wantBullets {
		if got := gotBullets[i]; got != want {
			t.Fatalf("bullet %d = %#v, want %#v", i, got, want)
		}
	}
}

// TestMarkdownToDocsRequests_AppendBulletsAndCode is a regression test for
// #594. The append path used to inline literal "• " glyphs for bullet lists
// (leaving paragraphs as NORMAL_TEXT) and split fenced code blocks into one
// Courier-styled NORMAL_TEXT paragraph per source line with no contiguous
// shading. The fix routes bullets through CreateParagraphBullets and joins
// code-block lines with vertical-tab soft breaks so the whole block is one
// shaded paragraph.
func TestMarkdownToDocsRequests_AppendBulletsAndCode(t *testing.T) {
	input := strings.Join([]string{
		"- **First** — bullet one.",
		"- Second item.",
		"1. step one",
		"```",
		"line 1",
		"line 2",
		"line 3",
		"```",
	}, "\n")

	elements := ParseMarkdown(input)
	requests, text, _ := MarkdownToDocsRequests(elements, 1, "")

	// The plain text fed to InsertText must NOT contain the literal "• "
	// glyph or the "1. " numeric prefix — those have to come from the
	// paragraph style, not the text run, otherwise the resulting paragraph
	// is NORMAL_TEXT with a glyph baked in (the #594 symptom).
	if strings.Contains(text, "• ") {
		t.Fatalf("text run still contains literal bullet glyph: %q", text)
	}
	if strings.Contains(text, "1. step one") {
		t.Fatalf("text run still contains literal numbered prefix: %q", text)
	}
	if !strings.Contains(text, "First — bullet one.\n") {
		t.Fatalf("expected bullet text content stripped of glyph, got %q", text)
	}
	if !strings.Contains(text, "step one\n") {
		t.Fatalf("expected numbered list content stripped of prefix, got %q", text)
	}

	// The fenced code block lines must end up inside a SINGLE paragraph,
	// joined by vertical-tab soft line breaks (Docs treats \v as a
	// line-break-within-paragraph), so a single paragraph-level shading
	// covers the whole block.
	if !strings.Contains(text, "line 1"+docsSoftLineBreak+"line 2"+docsSoftLineBreak+"line 3\n") {
		t.Fatalf("expected code block lines joined by Docs soft breaks, got %q", text)
	}
	if strings.Contains(text, "line 1\nline 2") {
		t.Fatalf("code block was split into separate paragraphs: %q", text)
	}

	// We expect at least:
	//   - 1 CreateParagraphBullets request for the contiguous bullet block
	//   - 1 CreateParagraphBullets for the numbered item
	//   - 1 UpdateParagraphStyle with paragraph-level shading covering the
	//     code block
	//   - 1 UpdateTextStyle for Courier on the code block range
	var (
		bulletDisc       int
		bulletNumbered   int
		codeShading      int
		codeMonospace    int
		bulletPrefixText bool
	)
	for _, r := range requests {
		if r.CreateParagraphBullets != nil {
			switch r.CreateParagraphBullets.BulletPreset {
			case "BULLET_DISC_CIRCLE_SQUARE":
				bulletDisc++
			case bulletPresetNumbered:
				bulletNumbered++
			}
		}
		if r.UpdateParagraphStyle != nil &&
			r.UpdateParagraphStyle.ParagraphStyle != nil &&
			r.UpdateParagraphStyle.ParagraphStyle.Shading != nil &&
			r.UpdateParagraphStyle.ParagraphStyle.Shading.BackgroundColor != nil {
			codeShading++
		}
		if r.UpdateTextStyle != nil &&
			r.UpdateTextStyle.TextStyle != nil &&
			r.UpdateTextStyle.TextStyle.WeightedFontFamily != nil &&
			r.UpdateTextStyle.TextStyle.WeightedFontFamily.FontFamily == "Courier New" {
			codeMonospace++
		}
		if r.InsertText != nil && strings.Contains(r.InsertText.Text, "• ") {
			bulletPrefixText = true
		}
	}

	if bulletDisc < 1 {
		t.Errorf("expected at least 1 BULLET_DISC_CIRCLE_SQUARE CreateParagraphBullets, got %d", bulletDisc)
	}
	if bulletNumbered < 1 {
		t.Errorf("expected at least 1 %s CreateParagraphBullets, got %d", bulletPresetNumbered, bulletNumbered)
	}
	if codeShading != 1 {
		t.Errorf("expected exactly 1 paragraph shading request for the code block, got %d", codeShading)
	}
	if codeMonospace < 1 {
		t.Errorf("expected at least 1 Courier New text style request for the code block, got %d", codeMonospace)
	}
	if bulletPrefixText {
		t.Errorf("unexpected literal bullet glyph inside an InsertText request")
	}
}
