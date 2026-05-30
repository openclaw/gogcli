package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsPageSizePresetBuildsDimensions(t *testing.T) {
	req, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{
		DocsLayoutFlags: DocsLayoutFlags{PageSize: "A4"},
	})
	if err != nil {
		t.Fatalf("buildUpdateDocumentStyleRequest: %v", err)
	}
	if req.Fields != "pageSize.width,pageSize.height" {
		t.Fatalf("fields = %q", req.Fields)
	}
	if got := req.DocumentStyle.PageSize.Width.Magnitude; got != 595.275 {
		t.Fatalf("width = %v", got)
	}
	if got := req.DocumentStyle.PageSize.Height.Magnitude; got != 841.890 {
		t.Fatalf("height = %v", got)
	}
}

func TestDocsPageSizeRejectsExplicitDimensions(t *testing.T) {
	_, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{
		DocsLayoutFlags: DocsLayoutFlags{PageSize: "Letter", PageWidth: "1in"},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestDocsCellStyleBuildsTableAndTextRequests(t *testing.T) {
	cmd := &DocsCellStyleCmd{
		Row:             0,
		Col:             1,
		RowSpan:         1,
		ColSpan:         2,
		BackgroundColor: "#abc",
		TextColor:       "#123456",
		Bold:            true,
	}
	cell := &docs.TableCell{Content: []*docs.StructuralElement{{
		Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
			StartIndex: 10,
			EndIndex:   16,
			TextRun:    &docs.TextRun{Content: "Badge\n"},
		}}},
	}}}
	reqs, err := cmd.buildRequests(5, cell, "tab-1")
	if err != nil {
		t.Fatalf("buildRequests: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	cellReq := reqs[0].UpdateTableCellStyle
	if cellReq == nil || cellReq.Fields != "backgroundColor" {
		t.Fatalf("unexpected cell style request: %#v", reqs[0])
	}
	loc := cellReq.TableRange.TableCellLocation
	if loc.RowIndex != 0 || loc.ColumnIndex != 1 || loc.TableStartLocation.Index != 5 || loc.TableStartLocation.TabId != "tab-1" {
		t.Fatalf("unexpected cell location: %#v", loc)
	}
	textReq := reqs[1].UpdateTextStyle
	if textReq == nil || textReq.Range.StartIndex != 10 || textReq.Range.EndIndex != 15 || textReq.Range.TabId != "tab-1" {
		t.Fatalf("unexpected text style request: %#v", reqs[1])
	}
	if !textReq.TextStyle.Bold || textReq.Fields != "foregroundColor,bold" {
		t.Fatalf("unexpected text style: %#v fields=%q", textReq.TextStyle, textReq.Fields)
	}
}

func TestDocsSmartChipCommandsBuildRequests(t *testing.T) {
	person := &docs.Request{InsertPerson: &docs.InsertPersonRequest{PersonProperties: &docs.PersonProperties{Email: "a@example.com"}}}
	setDocsInsertRequestLocation(person, 7, "tab-1")
	if person.InsertPerson.Location.Index != 7 || person.InsertPerson.Location.TabId != "tab-1" {
		t.Fatalf("person location = %#v", person.InsertPerson.Location)
	}

	dateFormat, err := normalizeDocsDateChipFormat("iso")
	if err != nil {
		t.Fatalf("normalize date: %v", err)
	}
	if dateFormat != "DATE_FORMAT_ISO8601" {
		t.Fatalf("date format = %q", dateFormat)
	}
}

func TestDocsInsertImageBuildsPlaceholderReplacement(t *testing.T) {
	cmd := &DocsInsertImageCmd{At: "IMG_HERE", Width: 320}
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/documents/doc1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(docBodyWithText("before IMG_HERE after\n"))
	}))
	defer cleanup()

	reqs, index, tabID, err := cmd.buildInsertRequests(context.Background(), docSvc, "doc1", "IMG_HERE", "https://example.com/i.png")
	if err != nil {
		t.Fatalf("buildInsertRequests: %v", err)
	}
	if tabID != "" || index != 8 {
		t.Fatalf("index=%d tab=%q", index, tabID)
	}
	if len(reqs) != 2 || reqs[0].DeleteContentRange == nil || reqs[1].InsertInlineImage == nil {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
	if reqs[1].InsertInlineImage.Uri != "https://example.com/i.png" || reqs[1].InsertInlineImage.ObjectSize.Width.Magnitude != 320 {
		t.Fatalf("unexpected image request: %#v", reqs[1].InsertInlineImage)
	}
}
