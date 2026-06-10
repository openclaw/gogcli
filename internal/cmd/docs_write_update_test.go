package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsWriteUpdate_JSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/documents/"), ":batchUpdate")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": id})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"body": map[string]any{
					"content": []any{
						map[string]any{"startIndex": 1, "endIndex": 12},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello"}, ctx, flags); err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	if got := batchRequests[0]; len(got) != 2 || got[0].DeleteContentRange == nil || got[1].InsertText == nil {
		t.Fatalf("unexpected write requests: %#v", got)
	}
	if got := batchRequests[0][0].DeleteContentRange.Range; got.StartIndex != 1 || got.EndIndex != 11 {
		t.Fatalf("unexpected delete range: %#v", got)
	}
	if got := batchRequests[0][1].InsertText; got.Location.Index != 1 || got.Text != "hello" {
		t.Fatalf("unexpected insert: %#v", got)
	}

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "world", "--append"}, ctx, flags); err != nil {
		t.Fatalf("write append: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected 2 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[1]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected append requests: %#v", got)
	}
	if got := batchRequests[1][0].InsertText; got.Location.Index != 11 || got.Text != "world" {
		t.Fatalf("unexpected append insert: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "!"}, ctx, flags); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(batchRequests) != 3 {
		t.Fatalf("expected 3 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[2]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected update requests: %#v", got)
	}
	if got := batchRequests[2][0].InsertText; got.Location.Index != 11 || got.Text != "!" {
		t.Fatalf("unexpected update insert: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "?", "--index", "5"}, ctx, flags); err != nil {
		t.Fatalf("update index: %v", err)
	}
	if len(batchRequests) != 4 {
		t.Fatalf("expected 4 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[3]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected update index requests: %#v", got)
	}
	if got := batchRequests[3][0].InsertText; got.Location.Index != 5 || got.Text != "?" {
		t.Fatalf("unexpected update index insert: %#v", got)
	}
}

func TestDocsUpdate_MarkdownWithTab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request
	var includeTabsCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			if strings.Contains(r.URL.RawQuery, "includeTabsContent=true") {
				includeTabsCalls++
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	markdown := "## Heading\n\n**bold** and [link](https://example.com)\n"
	if err := runKong(t, &DocsUpdateCmd{}, []string{
		"doc1", "--text", markdown, "--markdown", "--tab", "Second",
	}, ctx, flags); err != nil {
		t.Fatalf("update markdown with tab: %v", err)
	}

	if includeTabsCalls != 1 {
		t.Fatalf("expected 1 tab-aware GET, got %d", includeTabsCalls)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) < 2 || reqs[0].InsertText == nil {
		t.Fatalf("expected markdown insert + formatting requests, got %#v", reqs)
	}
	insert := reqs[0].InsertText
	if insert.Location.TabId != "t.second" || insert.Location.Index != 19 {
		t.Fatalf("insert location = %+v, want tab t.second index 19", insert.Location)
	}
	if got := insert.Text; got != "\nHeading\nbold and link\n" {
		t.Fatalf("inserted text = %q, want markdown-rendered text", got)
	}
	for i, req := range reqs[1:] {
		var r *docs.Range
		switch {
		case req.UpdateTextStyle != nil:
			r = req.UpdateTextStyle.Range
		case req.UpdateParagraphStyle != nil:
			r = req.UpdateParagraphStyle.Range
		case req.CreateParagraphBullets != nil:
			r = req.CreateParagraphBullets.Range
		case req.DeleteParagraphBullets != nil:
			r = req.DeleteParagraphBullets.Range
		}
		if r == nil {
			continue
		}
		if r.TabId != "t.second" {
			t.Fatalf("formatting request %d range tab = %q, want t.second", i+1, r.TabId)
		}
	}
}

func TestDocsUpdate_MarkdownRewritesExplicitHeadingAnchorLinks(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request
	gets := 0
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			gets++
			w.Header().Set("Content-Type", "application/json")
			if gets == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"documentId": "doc1",
					"body": map[string]any{"content": []any{
						map[string]any{"startIndex": 1, "endIndex": 2},
					}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(markdownAnchorRewriteDoc("doc1", "h.files", "#attachments"))
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)
	markdown := "# Files {#attachments}\n\n[Jump](#attachments)\n"
	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", markdown, "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("update markdown: %v", err)
	}

	if len(batchRequests) != 2 {
		t.Fatalf("expected insert and rewrite batch requests, got %d", len(batchRequests))
	}
	insert := batchRequests[0][0].InsertText
	if insert == nil || strings.Contains(insert.Text, "{#attachments}") {
		t.Fatalf("insert text should strip explicit anchor, got %#v", insert)
	}
	rewrite := batchRequests[1][0].UpdateTextStyle
	if rewrite == nil || rewrite.TextStyle == nil || rewrite.TextStyle.Link == nil || rewrite.TextStyle.Link.HeadingId != "h.files" {
		t.Fatalf("expected native heading rewrite to h.files, got %#v", batchRequests[1])
	}
}

func TestDocsUpdate_ReplaceRangeMarkdownRewritesExplicitHeadingAnchorLinks(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request
	gets := 0
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			gets++
			w.Header().Set("Content-Type", "application/json")
			if gets == 1 {
				_ = json.NewEncoder(w).Encode(&docs.Document{
					DocumentId: "doc1",
					RevisionId: "rev1",
					Body: &docs.Body{Content: []*docs.StructuralElement{{
						StartIndex: 1,
						EndIndex:   8,
						Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
							StartIndex: 1,
							EndIndex:   7,
							TextRun:    &docs.TextRun{Content: "target"},
						}}},
					}}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(markdownAnchorRewriteDoc("doc1", "h.files", "#attachments"))
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)
	markdown := "# Files {#attachments}\n\n[Jump](#attachments)\n"
	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", markdown, "--markdown", "--replace-range", "1:7"}, ctx, flags); err != nil {
		t.Fatalf("update replace markdown: %v", err)
	}

	if len(batchRequests) != 2 {
		t.Fatalf("expected replace and rewrite batch requests, got %d", len(batchRequests))
	}
	insert := batchRequests[0][1].InsertText
	if insert == nil || strings.Contains(insert.Text, "{#attachments}") {
		t.Fatalf("insert text should strip explicit anchor, got %#v", insert)
	}
	rewrite := batchRequests[1][0].UpdateTextStyle
	if rewrite == nil || rewrite.TextStyle == nil || rewrite.TextStyle.Link == nil || rewrite.TextStyle.Link.HeadingId != "h.files" {
		t.Fatalf("expected native heading rewrite to h.files, got %#v", batchRequests[1])
	}
}

func markdownAnchorRewriteDoc(docID, headingID, linkURL string) *docs.Document {
	return &docs.Document{
		DocumentId: docID,
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{
				StartIndex: 1,
				EndIndex:   7,
				Paragraph: &docs.Paragraph{
					ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1", HeadingId: headingID},
					Elements: []*docs.ParagraphElement{{
						StartIndex: 1,
						EndIndex:   7,
						TextRun:    &docs.TextRun{Content: "Files\n"},
					}},
				},
			},
			{
				StartIndex: 8,
				EndIndex:   13,
				Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
					StartIndex: 8,
					EndIndex:   12,
					TextRun: &docs.TextRun{
						Content:   "Jump",
						TextStyle: &docs.TextStyle{Link: &docs.Link{Url: linkURL}},
					},
				}}},
			},
		}},
	}
}

func TestDocsUpdate_ReplaceRangePlainWithTab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	if err := runKong(t, &DocsUpdateCmd{}, []string{
		"doc1", "--text", "replacement", "--replace-range", "7:12", "--tab", "Second",
	}, ctx, flags); err != nil {
		t.Fatalf("update replace-range plain with tab: %v", err)
	}

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 2 || reqs[0].DeleteContentRange == nil || reqs[1].InsertText == nil {
		t.Fatalf("unexpected replace requests: %#v", reqs)
	}
	if got := reqs[0].DeleteContentRange.Range; got.StartIndex != 7 || got.EndIndex != 12 || got.TabId != "t.second" {
		t.Fatalf("delete range = %+v, want 7:12 in t.second", got)
	}
	if got := reqs[1].InsertText; got.Location.Index != 7 || got.Location.TabId != "t.second" || got.Text != "replacement" {
		t.Fatalf("insert text = %+v, want replacement at 7 in t.second", got)
	}
}

func TestDocsUpdate_ReplaceRangeMarkdownWithTab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request
	var includeTabsCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			if strings.Contains(r.URL.RawQuery, "includeTabsContent=true") {
				includeTabsCalls++
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	markdown := "## New Heading\n\n**bold** and [link](https://example.com)\n"
	var runErr error
	out := captureStdout(t, func() {
		runErr = runKong(t, &DocsUpdateCmd{}, []string{
			"doc1", "--text", markdown, "--markdown", "--replace-range", "7:12", "--tab", "Second",
		}, ctx, flags)
	})
	if runErr != nil {
		t.Fatalf("update replace-range markdown with tab: %v", runErr)
	}

	if includeTabsCalls == 0 {
		t.Fatalf("expected tab-aware GET")
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) < 3 || reqs[0].DeleteContentRange == nil || reqs[1].InsertText == nil {
		t.Fatalf("expected delete + markdown insert + formatting, got %#v", reqs)
	}
	if got := reqs[0].DeleteContentRange.Range; got.StartIndex != 7 || got.EndIndex != 12 || got.TabId != "t.second" {
		t.Fatalf("delete range = %+v, want 7:12 in t.second", got)
	}
	if got := reqs[1].InsertText; got.Location.Index != 7 || got.Location.TabId != "t.second" || got.Text != "\nNew Heading\nbold and link\n" {
		t.Fatalf("insert text = %+v, want rendered markdown at 7 in t.second", got)
	}
	var payload struct {
		Requests int `json:"requests"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal output: %v\noutput=%q", err, out)
	}
	if payload.Requests != len(reqs) {
		t.Fatalf("reported requests = %d, want actual batch request count %d", payload.Requests, len(reqs))
	}
	for i, req := range reqs[2:] {
		var r *docs.Range
		switch {
		case req.UpdateTextStyle != nil:
			r = req.UpdateTextStyle.Range
		case req.UpdateParagraphStyle != nil:
			r = req.UpdateParagraphStyle.Range
		case req.CreateParagraphBullets != nil:
			r = req.CreateParagraphBullets.Range
		case req.DeleteParagraphBullets != nil:
			r = req.DeleteParagraphBullets.Range
		}
		if r == nil {
			continue
		}
		if r.TabId != "t.second" {
			t.Fatalf("formatting request %d range tab = %q, want t.second", i+2, r.TabId)
		}
	}
}

func TestDocsWriteUpdate_Pageless(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/documents/"), ":batchUpdate")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": id})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"body": map[string]any{
					"content": []any{
						map[string]any{"startIndex": 1, "endIndex": 12},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--pageless"}, ctx, flags); err != nil {
		t.Fatalf("write pageless: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected 2 batch requests after write, got %d", len(batchRequests))
	}
	if got := batchRequests[1]; len(got) != 1 || got[0].UpdateDocumentStyle == nil {
		t.Fatalf("unexpected pageless write request: %#v", got)
	}
	if got := batchRequests[1][0].UpdateDocumentStyle; got.Fields != "documentFormat" || got.DocumentStyle.DocumentFormat.DocumentMode != "PAGELESS" {
		t.Fatalf("unexpected pageless write style request: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "!", "--pageless"}, ctx, flags); err != nil {
		t.Fatalf("update pageless: %v", err)
	}
	if len(batchRequests) != 4 {
		t.Fatalf("expected 4 batch requests after update, got %d", len(batchRequests))
	}
	if got := batchRequests[3]; len(got) != 1 || got[0].UpdateDocumentStyle == nil {
		t.Fatalf("unexpected pageless update request: %#v", got)
	}
	if got := batchRequests[3][0].UpdateDocumentStyle; got.Fields != "documentFormat" || got.DocumentStyle.DocumentFormat.DocumentMode != "PAGELESS" {
		t.Fatalf("unexpected pageless update style request: %#v", got)
	}
}

func TestDocsWrite_PageSizeAndMargins(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"body": map[string]any{
					"content": []any{map[string]any{"startIndex": 1, "endIndex": 2}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	args := []string{"doc1", "--text", "hello", "--page-width=8.5in", "--margin-left=0.5in", "--margin-right=0.5in"}
	if err := runKong(t, &DocsWriteCmd{}, args, ctx, flags); err != nil {
		t.Fatalf("write margins: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected write and style batch requests, got %d", len(batchRequests))
	}
	upd := batchRequests[1][0].UpdateDocumentStyle
	if upd == nil {
		t.Fatalf("expected style update, got %#v", batchRequests[1])
	}
	if upd.Fields != "pageSize.width,marginLeft,marginRight" {
		t.Fatalf("fields = %q", upd.Fields)
	}
	if upd.DocumentStyle.PageSize.Width.Magnitude != 612 {
		t.Fatalf("page width = %#v", upd.DocumentStyle.PageSize.Width)
	}
	if upd.DocumentStyle.MarginLeft.Magnitude != 36 || upd.DocumentStyle.MarginRight.Magnitude != 36 {
		t.Fatalf("margins = left %#v right %#v", upd.DocumentStyle.MarginLeft, upd.DocumentStyle.MarginRight)
	}
}

func TestDocsWrite_InvalidLayoutValueFailsBeforeMutation(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })
	newDocsService = func(context.Context, string) (*docs.Service, error) {
		t.Fatal("invalid layout value should fail before creating Docs service")
		return nil, errors.New("unexpected Docs service creation")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--page-width=bogus"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "invalid --page-width") {
		t.Fatalf("expected invalid page-width error, got %v", err)
	}

	err = runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--page-width=NaN"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "invalid --page-width") {
		t.Fatalf("expected invalid page-width NaN error, got %v", err)
	}
}
