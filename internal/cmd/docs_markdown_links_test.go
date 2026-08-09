package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/docsmarkdown"
)

func TestRewriteMarkdownHeadingLinks_RewritesTableCellLinks(t *testing.T) {
	var batchReq docs.BatchUpdateDocumentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/documents/doc1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&docs.Document{
				DocumentId: "doc1",
				Body: &docs.Body{Content: []*docs.StructuralElement{
					{
						StartIndex: 1,
						EndIndex:   7,
						Paragraph: &docs.Paragraph{
							ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1", HeadingId: "h.files"},
							Elements: []*docs.ParagraphElement{{
								StartIndex: 1,
								EndIndex:   7,
								TextRun:    &docs.TextRun{Content: "Files\n"},
							}},
						},
					},
					{
						StartIndex: 7,
						EndIndex:   20,
						Table: &docs.Table{TableRows: []*docs.TableRow{{
							TableCells: []*docs.TableCell{{
								Content: []*docs.StructuralElement{{
									StartIndex: 10,
									EndIndex:   15,
									Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
										StartIndex: 10,
										EndIndex:   14,
										TextRun: &docs.TextRun{
											Content:   "Jump",
											TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "#attachments"}},
										},
									}}},
								}},
							}},
						}}},
					},
				}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/documents/doc1:batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&batchReq); err != nil {
				t.Fatalf("decode batch update: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	count, err := rewriteMarkdownHeadingLinks(context.Background(), svc, "doc1", "", []docsmarkdown.ExplicitHeadingAnchor{{
		Anchor:     "attachments",
		Text:       "Files",
		Occurrence: 1,
	}})
	if err != nil {
		t.Fatalf("rewriteMarkdownHeadingLinks: %v", err)
	}
	if count != 1 {
		t.Fatalf("rewrite count = %d, want 1", count)
	}
	if len(batchReq.Requests) != 1 || batchReq.Requests[0].UpdateTextStyle == nil {
		t.Fatalf("expected one UpdateTextStyle request, got %#v", batchReq.Requests)
	}
	styleReq := batchReq.Requests[0].UpdateTextStyle
	if styleReq.Range.StartIndex != 10 || styleReq.Range.EndIndex != 14 {
		t.Fatalf("unexpected rewrite range: %#v", styleReq.Range)
	}
	if styleReq.TextStyle == nil || styleReq.TextStyle.Link == nil || styleReq.TextStyle.Link.HeadingId != "h.files" {
		t.Fatalf("unexpected link rewrite request: %#v", styleReq)
	}
}

func TestRewriteMarkdownHeadingLinks_MatchesExplicitAnchorByHeadingText(t *testing.T) {
	svc, capture := newDocsBatchUpdateTestService(t, docsHeadingLinkTestDocument(
		docsHeadingTestParagraph(1, "h.intro", "Intro\n"),
		docsHeadingTestParagraph(7, "h.files", "Files\n"),
		docsLinkTestParagraph(13, "#attachments", "Jump"),
	))

	count, err := rewriteMarkdownHeadingLinks(context.Background(), svc, "doc1", "", []docsmarkdown.ExplicitHeadingAnchor{{
		Anchor:     "attachments",
		Text:       "Files",
		Occurrence: 1,
	}})
	if err != nil {
		t.Fatalf("rewriteMarkdownHeadingLinks: %v", err)
	}
	if count != 1 {
		t.Fatalf("rewrite count = %d, want 1", count)
	}
	styleReq := capture.Requests[0][0].UpdateTextStyle
	if styleReq.TextStyle == nil || styleReq.TextStyle.Link == nil || styleReq.TextStyle.Link.HeadingId != "h.files" {
		t.Fatalf("link target = %#v, want h.files", styleReq)
	}
}

func TestRewriteMarkdownHeadingLinks_ExplicitAnchorReservesAutoSlug(t *testing.T) {
	svc, capture := newDocsBatchUpdateTestService(t, docsHeadingLinkTestDocument(
		docsHeadingTestParagraph(1, "h.files1", "Files\n"),
		docsHeadingTestParagraph(7, "h.files2", "Files\n"),
		docsLinkTestParagraph(13, "#files-1", "Next"),
	))

	count, err := rewriteMarkdownHeadingLinks(context.Background(), svc, "doc1", "", []docsmarkdown.ExplicitHeadingAnchor{{
		Anchor:     "files",
		Text:       "Files",
		Occurrence: 1,
	}})
	if err != nil {
		t.Fatalf("rewriteMarkdownHeadingLinks: %v", err)
	}
	if count != 1 {
		t.Fatalf("rewrite count = %d, want 1", count)
	}
	styleReq := capture.Requests[0][0].UpdateTextStyle
	if styleReq.TextStyle == nil || styleReq.TextStyle.Link == nil || styleReq.TextStyle.Link.HeadingId != "h.files2" {
		t.Fatalf("link target = %#v, want h.files2", styleReq)
	}
}

func TestRewriteMarkdownHeadingLinksFromIndex_RewritesLinkInExistingParagraphTail(t *testing.T) {
	var batchReq docs.BatchUpdateDocumentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/documents/doc1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&docs.Document{
				DocumentId: "doc1",
				Body: &docs.Body{Content: []*docs.StructuralElement{
					{
						StartIndex: 1,
						EndIndex:   14,
						Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								EndIndex:   9,
								TextRun:    &docs.TextRun{Content: "Existing"},
							},
							{
								StartIndex: 9,
								EndIndex:   13,
								TextRun: &docs.TextRun{
									Content:   "Jump",
									TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "#attachments"}},
								},
							},
						}},
					},
					{
						StartIndex: 14,
						EndIndex:   20,
						Paragraph: &docs.Paragraph{
							ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1", HeadingId: "h.files"},
							Elements: []*docs.ParagraphElement{{
								StartIndex: 14,
								EndIndex:   20,
								TextRun:    &docs.TextRun{Content: "Files\n"},
							}},
						},
					},
				}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/documents/doc1:batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&batchReq); err != nil {
				t.Fatalf("decode batch update: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	count, err := rewriteMarkdownHeadingLinksFromIndex(context.Background(), svc, "doc1", "", []docsmarkdown.ExplicitHeadingAnchor{{
		Anchor:     "attachments",
		Text:       "Files",
		Occurrence: 1,
	}}, 9)
	if err != nil {
		t.Fatalf("rewriteMarkdownHeadingLinksFromIndex: %v", err)
	}
	if count != 1 {
		t.Fatalf("rewrite count = %d, want 1", count)
	}
	if len(batchReq.Requests) != 1 || batchReq.Requests[0].UpdateTextStyle == nil {
		t.Fatalf("expected one UpdateTextStyle request, got %#v", batchReq.Requests)
	}
	styleReq := batchReq.Requests[0].UpdateTextStyle
	if styleReq.Range.StartIndex != 9 || styleReq.Range.EndIndex != 13 {
		t.Fatalf("unexpected rewrite range: %#v", styleReq.Range)
	}
	if styleReq.TextStyle == nil || styleReq.TextStyle.Link == nil || styleReq.TextStyle.Link.HeadingId != "h.files" {
		t.Fatalf("unexpected link rewrite request: %#v", styleReq)
	}
}

func TestRewriteMarkdownHeadingLinksInRange_SkipsLaterExistingLinks(t *testing.T) {
	var batchReq docs.BatchUpdateDocumentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/documents/doc1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&docs.Document{
				DocumentId: "doc1",
				Body: &docs.Body{Content: []*docs.StructuralElement{
					{
						StartIndex: 10,
						EndIndex:   16,
						Paragraph: &docs.Paragraph{
							ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1", HeadingId: "h.files"},
							Elements: []*docs.ParagraphElement{{
								StartIndex: 10,
								EndIndex:   16,
								TextRun:    &docs.TextRun{Content: "Files\n"},
							}},
						},
					},
					{
						StartIndex: 16,
						EndIndex:   21,
						Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
							StartIndex: 16,
							EndIndex:   20,
							TextRun: &docs.TextRun{
								Content:   "Jump",
								TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "#attachments"}},
							},
						}}},
					},
					{
						StartIndex: 40,
						EndIndex:   46,
						Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
							StartIndex: 40,
							EndIndex:   45,
							TextRun: &docs.TextRun{
								Content:   "Later",
								TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "#attachments"}},
							},
						}}},
					},
				}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/documents/doc1:batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&batchReq); err != nil {
				t.Fatalf("decode batch update: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	count, err := rewriteMarkdownHeadingLinksInRange(context.Background(), svc, "doc1", "", []docsmarkdown.ExplicitHeadingAnchor{{
		Anchor:     "attachments",
		Text:       "Files",
		Occurrence: 1,
	}}, 10, 21)
	if err != nil {
		t.Fatalf("rewriteMarkdownHeadingLinksInRange: %v", err)
	}
	if count != 1 {
		t.Fatalf("rewrite count = %d, want 1", count)
	}
	if len(batchReq.Requests) != 1 || batchReq.Requests[0].UpdateTextStyle == nil {
		t.Fatalf("expected one rewrite request, got %#v", batchReq.Requests)
	}
	if got := batchReq.Requests[0].UpdateTextStyle.Range; got.StartIndex != 16 || got.EndIndex != 20 {
		t.Fatalf("rewrite range = %#v, want inserted link only", got)
	}
}
