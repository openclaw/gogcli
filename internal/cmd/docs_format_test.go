package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/docsformat"
	"github.com/steipete/gogcli/internal/docssed"
)

func TestDocsFormatFlagsBuildRequests(t *testing.T) {
	t.Parallel()

	reqs, err := (DocsFormatFlags{
		FontFamily:  "Georgia",
		FontSize:    14,
		TextColor:   "#3366cc",
		BgColor:     "#fff",
		NoBold:      true,
		Italic:      true,
		Alignment:   "center",
		LineSpacing: 150,
		SpacingMode: "collapse_lists",
	}).buildRequests(3, 9, "t.second")
	if err != nil {
		t.Fatalf("buildRequests: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected text and paragraph requests, got %d", len(reqs))
	}

	textReq := reqs[0].UpdateTextStyle
	if textReq == nil {
		t.Fatalf("missing text request: %#v", reqs[0])
		return
	}
	if got := textReq.Range; got.StartIndex != 3 || got.EndIndex != 9 || got.TabId != "t.second" {
		t.Fatalf("unexpected text range: %#v", got)
	}
	if textReq.Fields != "weightedFontFamily,fontSize,foregroundColor,backgroundColor,bold,italic" {
		t.Fatalf("unexpected text fields: %q", textReq.Fields)
	}
	if textReq.TextStyle.WeightedFontFamily.FontFamily != "Georgia" {
		t.Fatalf("unexpected font: %#v", textReq.TextStyle.WeightedFontFamily)
	}
	if textReq.TextStyle.Bold {
		t.Fatalf("bold should be false")
	}

	encoded, err := json.Marshal(textReq.TextStyle)
	if err != nil {
		t.Fatalf("marshal style: %v", err)
	}
	if !strings.Contains(string(encoded), `"bold":false`) {
		t.Fatalf("clearing bold must force-send false, got %s", encoded)
	}

	paraReq := reqs[1].UpdateParagraphStyle
	if paraReq == nil {
		t.Fatalf("missing paragraph request: %#v", reqs[1])
		return
	}
	if paraReq.ParagraphStyle.Alignment != "CENTER" || paraReq.ParagraphStyle.LineSpacing != 150 ||
		paraReq.ParagraphStyle.SpacingMode != docsformat.SpacingModeCollapseLists {
		t.Fatalf("unexpected paragraph style: %#v", paraReq.ParagraphStyle)
	}
	if got := paraReq.Range; got.TabId != "t.second" {
		t.Fatalf("paragraph range lost tab id: %#v", got)
	}
}

func TestDocsFormatFlagsBuildRequestsCode(t *testing.T) {
	t.Parallel()

	reqs, err := (DocsFormatFlags{Code: true}).buildRequests(3, 9, "t.second")
	if err != nil {
		t.Fatalf("buildRequests: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected text request, got %d", len(reqs))
	}
	textReq := reqs[0].UpdateTextStyle
	if textReq == nil {
		t.Fatalf("missing text request: %#v", reqs[0])
	}
	if textReq.Fields != "weightedFontFamily,backgroundColor" {
		t.Fatalf("unexpected text fields: %q", textReq.Fields)
	}
	style := textReq.TextStyle
	if style.WeightedFontFamily == nil || style.WeightedFontFamily.FontFamily != "Courier New" {
		t.Fatalf("unexpected code font: %#v", style.WeightedFontFamily)
	}
	rgb := style.BackgroundColor.Color.RgbColor
	if rgb.Red != codeBackgroundGrey || rgb.Green != codeBackgroundGrey || rgb.Blue != codeBackgroundGrey {
		t.Fatalf("unexpected code background: %#v", rgb)
	}
	if got := textReq.Range; got.StartIndex != 3 || got.EndIndex != 9 || got.TabId != "t.second" {
		t.Fatalf("unexpected text range: %#v", got)
	}
}

func TestDocsFormatFlagsValidation(t *testing.T) {
	t.Parallel()

	if _, err := (DocsFormatFlags{TextColor: "oops"}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected invalid color error")
	}
	if _, err := (DocsFormatFlags{link: "https://example.com", noLink: true}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected conflicting link flags error")
	}
	if _, err := (DocsFormatFlags{Bold: true, NoBold: true}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected conflicting bold flags error")
	}
	if _, err := (DocsFormatFlags{Alignment: "sideways"}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected invalid alignment error")
	}
	if _, err := (DocsFormatFlags{SpacingMode: "sometimes"}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected invalid spacing-mode error")
	}
	if _, err := (DocsFormatFlags{Code: true, FontFamily: "Arial"}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected conflicting code/font-family flags error")
	}
	if _, err := (DocsFormatFlags{Code: true, BgColor: "#fff"}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected conflicting code/bg-color flags error")
	}
}

func TestDocsFormatFlagsLinkRequests(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		flags        DocsFormatFlags
		wantURL      string
		wantBookmark string
	}{
		{
			name:    "url",
			flags:   DocsFormatFlags{link: "https://example.com"},
			wantURL: "https://example.com",
		},
		{
			name:    "mailto",
			flags:   DocsFormatFlags{link: "mailto:foo@bar.com"},
			wantURL: "mailto:foo@bar.com",
		},
		{
			name:         "bookmark",
			flags:        DocsFormatFlags{link: "#bookmark-1"},
			wantBookmark: "bookmark-1",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reqs, err := tt.flags.buildRequests(3, 9, "")
			if err != nil {
				t.Fatalf("buildRequests: %v", err)
			}
			textReq := reqs[0].UpdateTextStyle
			if textReq == nil || textReq.Fields != "link" || textReq.TextStyle.Link == nil {
				t.Fatalf("unexpected link request: %#v", reqs)
			}
			if textReq.TextStyle.Link.Url != tt.wantURL {
				t.Fatalf("url = %q, want %q", textReq.TextStyle.Link.Url, tt.wantURL)
			}
			if textReq.TextStyle.Link.BookmarkId != tt.wantBookmark {
				t.Fatalf("bookmark = %q, want %q", textReq.TextStyle.Link.BookmarkId, tt.wantBookmark)
			}
		})
	}
}

func TestDocsFormatFlagsNoLinkClearsLink(t *testing.T) {
	t.Parallel()

	reqs, err := (DocsFormatFlags{noLink: true}).buildRequests(3, 9, "")
	if err != nil {
		t.Fatalf("buildRequests: %v", err)
	}
	textReq := reqs[0].UpdateTextStyle
	if textReq == nil || textReq.Fields != "link" {
		t.Fatalf("unexpected clear-link request: %#v", reqs)
	}
	encoded, err := json.Marshal(textReq.TextStyle)
	if err != nil {
		t.Fatalf("marshal style: %v", err)
	}
	if !strings.Contains(string(encoded), `"link":null`) {
		t.Fatalf("clearing link must send JSON null, got %s", encoded)
	}
}

func TestDocsFormatInternalLinkTreatsHeadingID(t *testing.T) {
	t.Parallel()

	link := docsFormatInternalLink(nil, "", "h.heading1")
	if link == nil || link.HeadingId != "h.heading1" {
		t.Fatalf("expected heading ID link, got %#v", link)
	}
}

func TestDocsFormatCmdLinkResolvesHeadingSlug(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithHeadingAndText("Target Heading\n", "see this\n", "h.target"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), docSvc)
	flags := &RootFlags{Account: "a@b.com"}
	if err := runKong(t, &DocsFormatCmd{}, []string{"doc1", "--match", "see", "--link", "#target-heading"}, ctx, flags); err != nil {
		t.Fatalf("format: %v", err)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected one batch, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 1 || reqs[0].UpdateTextStyle == nil {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
	textReq := reqs[0].UpdateTextStyle
	if got := textReq.Range; got.StartIndex != 16 || got.EndIndex != 19 {
		t.Fatalf("unexpected match range: %#v", got)
	}
	if textReq.TextStyle.Link == nil || textReq.TextStyle.Link.HeadingId != "h.target" {
		t.Fatalf("expected heading link, got %#v", textReq.TextStyle.Link)
	}
}

func TestDocsWriteFormatsInsertedRangeOnly(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"body": map[string]any{"content": []any{
					map[string]any{"startIndex": 1, "endIndex": 12},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), docSvc)
	flags := &RootFlags{Account: "a@b.com"}
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "world", "--append", "--bold", "--font-size", "12"}, ctx, flags); err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected one batch, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 2 || reqs[0].InsertText == nil || reqs[1].UpdateTextStyle == nil {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
	if got := reqs[0].InsertText.Location.Index; got != 11 {
		t.Fatalf("unexpected insert index: %d", got)
	}
	if got := reqs[1].UpdateTextStyle.Range; got.StartIndex != 11 || got.EndIndex != 16 {
		t.Fatalf("format should cover inserted text only, got %#v", got)
	}
}

func TestDocsFormatCmdMatchAll(t *testing.T) {
	t.Parallel()

	docSvc, capture := newDocsBatchUpdateTestService(t, docBodyWithText("Alpha Beta Alpha\n"))

	ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), docSvc)
	flags := &RootFlags{Account: "a@b.com"}
	if err := runKong(t, &DocsFormatCmd{}, []string{"doc1", "--match", "Alpha", "--match-all", "--underline", "--bg-color", "#fff"}, ctx, flags); err != nil {
		t.Fatalf("format: %v", err)
	}
	batchRequests := capture.Requests
	if len(batchRequests) != 1 {
		t.Fatalf("expected one batch, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 2 {
		t.Fatalf("expected two match requests, got %#v", reqs)
	}
	if got := reqs[0].UpdateTextStyle.Range; got.StartIndex != 1 || got.EndIndex != 6 {
		t.Fatalf("unexpected first match range: %#v", got)
	}
	if got := reqs[1].UpdateTextStyle.Range; got.StartIndex != 12 || got.EndIndex != 17 {
		t.Fatalf("unexpected second match range: %#v", got)
	}
}

func TestDocsFormatCmdParagraphControls(t *testing.T) {
	t.Parallel()

	docSvc, capture := newDocsBatchUpdateTestService(t, docBodyWithText("Alpha Beta Alpha\n"))

	ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), docSvc)
	flags := &RootFlags{Account: "a@b.com"}
	err := runKong(t, &DocsFormatCmd{}, []string{
		"doc1", "--match", "Alpha", "--match-all", "--bold", "--ordered",
		"--indent-start", "24", "--space-below", "6",
		"--spacing-mode", "never_collapse",
		"--keep-with-next", "--no-keep-lines-together",
	}, ctx, flags)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	batchRequests := capture.Requests
	if len(batchRequests) != 1 || len(batchRequests[0]) != 4 {
		t.Fatalf("unexpected requests: %#v", batchRequests)
	}

	if first, second := batchRequests[0][0].UpdateTextStyle, batchRequests[0][1].UpdateTextStyle; first == nil || second == nil || first.Range.StartIndex != 1 || first.Range.EndIndex != 6 ||
		second.Range.StartIndex != 12 || second.Range.EndIndex != 17 {
		t.Fatalf("exact text requests must precede bullets: %#v", batchRequests[0][:2])
	}
	bullets := batchRequests[0][2].CreateParagraphBullets
	paragraph := batchRequests[0][3].UpdateParagraphStyle
	if paragraph == nil || bullets == nil || paragraph.Range.StartIndex != 1 || paragraph.Range.EndIndex != 18 ||
		bullets.Range.StartIndex != 1 || bullets.Range.EndIndex != 18 {
		t.Fatalf("grouped paragraph requests: %#v", batchRequests[0])
	}
	if paragraph.ParagraphStyle.IndentStart.Magnitude != 24 || paragraph.ParagraphStyle.SpaceBelow.Magnitude != 6 ||
		paragraph.ParagraphStyle.SpacingMode != docsformat.SpacingModeNeverCollapse ||
		!paragraph.ParagraphStyle.KeepWithNext || paragraph.ParagraphStyle.KeepLinesTogether {
		t.Fatalf("unexpected paragraph style: %#v", paragraph.ParagraphStyle)
	}
	if bullets.BulletPreset != docsformat.BulletPresetNumbered {
		t.Fatalf("preset = %q", bullets.BulletPreset)
	}
}

func TestDocsFormatBulletTargetsAdjustLeadingTabsInForwardOrder(t *testing.T) {
	t.Parallel()

	paragraphs := []docssed.DocumentParagraph{
		{Text: "\tFirst\n", StartIndex: 1, EndIndex: 8},
		{Text: "\t\tSecond\n", StartIndex: 8, EndIndex: 17},
		{Text: "Third\n", StartIndex: 17, EndIndex: 23},
	}
	targets := []docsFormatTargetRange{
		{TextRange: docsedit.TextRange{StartIndex: 2, EndIndex: 7}, BulletParagraphs: docsFormatBulletParagraphs(paragraphs, 2, 7)},
		{TextRange: docsedit.TextRange{StartIndex: 10, EndIndex: 16}, BulletParagraphs: docsFormatBulletParagraphs(paragraphs, 10, 16)},
	}
	adjustDocsFormatBulletTargets(targets, true)
	if got := targets[0]; got.StartIndex != 2 || got.EndIndex != 7 || got.PostBulletStart != 1 || got.PostBulletEnd != 7 {
		t.Fatalf("first adjusted target: %#v", got)
	}
	if got := targets[1]; got.StartIndex != 9 || got.EndIndex != 15 || got.PostBulletStart != 7 || got.PostBulletEnd != 14 {
		t.Fatalf("second adjusted target: %#v", got)
	}
}

func TestGroupDocsFormatBulletTargets(t *testing.T) {
	t.Parallel()

	p1 := docsFormatBulletParagraph{StartIndex: 1, EndIndex: 8}
	p2 := docsFormatBulletParagraph{StartIndex: 8, EndIndex: 17}
	p3 := docsFormatBulletParagraph{StartIndex: 20, EndIndex: 27}
	targets := []docsFormatTargetRange{
		{BulletParagraphs: []docsFormatBulletParagraph{p1}},
		{BulletParagraphs: []docsFormatBulletParagraph{p1}},
		{BulletParagraphs: []docsFormatBulletParagraph{p2}},
		{BulletParagraphs: []docsFormatBulletParagraph{p3}},
	}
	groups := groupDocsFormatBulletTargets(targets)
	if len(groups) != 2 || groups[0].StartIndex != 1 || groups[0].EndIndex != 17 ||
		len(groups[0].BulletParagraphs) != 2 || groups[1].StartIndex != 20 || groups[1].EndIndex != 27 {
		t.Fatalf("groups = %#v", groups)
	}
}

func docBodyWithHeadingAndText(heading, body, headingID string) map[string]any {
	bodyStart := 1 + len(heading)
	bodyEnd := bodyStart + len(body)
	return map[string]any{
		"documentId": "doc1",
		"body": map[string]any{
			"content": []any{
				map[string]any{
					"startIndex":   0,
					"endIndex":     1,
					"sectionBreak": map[string]any{"sectionStyle": map[string]any{}},
				},
				map[string]any{
					"startIndex": 1,
					"endIndex":   bodyStart,
					"paragraph": map[string]any{
						"paragraphStyle": map[string]any{
							"namedStyleType": "HEADING_1",
							"headingId":      headingID,
						},
						"elements": []any{
							map[string]any{
								"startIndex": 1,
								"endIndex":   bodyStart,
								"textRun":    map[string]any{"content": heading},
							},
						},
					},
				},
				map[string]any{
					"startIndex": bodyStart,
					"endIndex":   bodyEnd,
					"paragraph": map[string]any{
						"elements": []any{
							map[string]any{
								"startIndex": bodyStart,
								"endIndex":   bodyEnd,
								"textRun":    map[string]any{"content": body},
							},
						},
					},
				},
			},
		},
	}
}
