package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

// TestDocsInsertCmd_MarkdownAtIndex verifies that `docs insert --markdown
// --index N` converts the markdown and places the converted block at the
// resolved index: a body InsertText at N plus the converter's heading
// paragraph-style request, rather than a single plain InsertText.
func TestDocsInsertCmd_MarkdownAtIndex(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request
	var getCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithEndIndex(42))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	markdown := "## Heading\n\n```\ncode\n```"
	if err := runKong(t, &DocsInsertCmd{}, []string{"doc1", markdown, "--markdown", "--index", "7"}, ctx, flags); err != nil {
		t.Fatalf("insert --markdown: %v", err)
	}

	// Explicit --index means no GET is needed to resolve placement, and the
	// markdown has no heading links so no rewrite GET is issued either.
	if getCalls != 0 {
		t.Fatalf("explicit --index with no heading links should not GET the doc, got %d", getCalls)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected one batchUpdate, got %d", len(batchRequests))
	}

	var insertAtIndex *docs.InsertTextRequest
	var sawHeadingStyle bool
	var fencedCodeStyle *docs.UpdateTextStyleRequest
	for _, req := range batchRequests[0] {
		if req.InsertText != nil && req.InsertText.Location != nil && req.InsertText.Location.Index == 7 {
			insertAtIndex = req.InsertText
		}
		if req.UpdateParagraphStyle != nil &&
			req.UpdateParagraphStyle.ParagraphStyle != nil &&
			strings.HasPrefix(req.UpdateParagraphStyle.ParagraphStyle.NamedStyleType, "HEADING") {
			sawHeadingStyle = true
		}
		if req.UpdateTextStyle != nil && strings.Contains(req.UpdateTextStyle.Fields, "weightedFontFamily") {
			fencedCodeStyle = req.UpdateTextStyle
		}
	}
	if insertAtIndex == nil {
		t.Fatalf("expected an InsertText at index 7, requests: %#v", batchRequests[0])
	}
	if !strings.Contains(insertAtIndex.Text, "Heading") {
		t.Fatalf("expected inserted text to contain the heading text, got %q", insertAtIndex.Text)
	}
	if !sawHeadingStyle {
		t.Fatalf("expected a HEADING paragraph-style request from the markdown converter, requests: %#v", batchRequests[0])
	}
	assertFencedCodeTextStyle(t, fencedCodeStyle)
}

func TestDocsInsertCmd_MarkdownAtAnchorUsesRevisionControl(t *testing.T) {
	t.Parallel()

	rec := &docsAtAnchorRecorder{}
	doc := docsFindRangeDoc(docsFindRangeParagraph(1, "Before anchor after\n"))
	doc.RevisionId = "rev-markdown-anchor"
	svc := setupDocsAtAnchorTestService(t, doc, rec)

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &DocsInsertCmd{}, []string{"doc1", "## Heading", "--markdown", "--at", "anchor"}, ctx, flags); err != nil {
		t.Fatalf("insert --markdown --at: %v", err)
	}
	if len(rec.batchRequests) != 1 {
		t.Fatalf("batch calls = %d, want 1", len(rec.batchRequests))
	}
	assertDocsAtAnchorWriteControl(t, rec, 0, "rev-markdown-anchor")
}

// TestDocsInsertCmd_MarkdownRejectsBatch verifies the --markdown/--batch combo
// is rejected up front (the markdown path issues its own batchUpdate calls and
// cannot be queued into a persisted batch).
func TestDocsInsertCmd_MarkdownRejectsBatch(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	err := runKong(t, &DocsInsertCmd{}, []string{"doc1", "## H", "--markdown", "--batch", "b1"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--markdown cannot be combined with --batch") {
		t.Fatalf("expected --markdown/--batch rejection, got %v", err)
	}
}
