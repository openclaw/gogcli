package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsFormatFlagsBuildRequests(t *testing.T) {
	reqs, err := (DocsFormatFlags{
		FontFamily:  "Georgia",
		FontSize:    14,
		TextColor:   "#3366cc",
		BgColor:     "#fff",
		NoBold:      true,
		Italic:      true,
		Alignment:   "center",
		LineSpacing: 150,
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
	}
	if paraReq.ParagraphStyle.Alignment != "CENTER" || paraReq.ParagraphStyle.LineSpacing != 150 {
		t.Fatalf("unexpected paragraph style: %#v", paraReq.ParagraphStyle)
	}
	if got := paraReq.Range; got.TabId != "t.second" {
		t.Fatalf("paragraph range lost tab id: %#v", got)
	}
}

func TestDocsFormatFlagsValidation(t *testing.T) {
	if _, err := (DocsFormatFlags{TextColor: "oops"}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected invalid color error")
	}
	if _, err := (DocsFormatFlags{Bold: true, NoBold: true}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected conflicting bold flags error")
	}
	if _, err := (DocsFormatFlags{Alignment: "sideways"}).buildRequests(1, 2, ""); err == nil {
		t.Fatalf("expected invalid alignment error")
	}
}

func TestDocsWriteFormatsInsertedRangeOnly(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

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
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	ctx := newDocsJSONContext(t)
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
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

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
			_ = json.NewEncoder(w).Encode(docBodyWithText("Alpha Beta Alpha\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	ctx := newDocsJSONContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := runKong(t, &DocsFormatCmd{}, []string{"doc1", "--match", "Alpha", "--match-all", "--underline", "--bg-color", "#fff"}, ctx, flags); err != nil {
		t.Fatalf("format: %v", err)
	}
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
