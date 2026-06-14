package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/docsmarkdown"
)

func TestDocsWrite_MarkdownReplaceUsesDriveUpdate(t *testing.T) {
	var sawDriveUpdate bool
	var uploadBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/doc1"):
			sawDriveUpdate = true
			if got := r.URL.Query().Get("supportsAllDrives"); got != "true" {
				t.Fatalf("drive update query: missing supportsAllDrives=true, got %q", got)
			}
			if got := r.Header.Get("Content-Type"); !strings.Contains(got, "text/markdown") && !strings.Contains(got, "multipart/related") {
				t.Fatalf("unexpected content type: %s", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			uploadBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc1",
				"name":        "Doc",
				"webViewLink": "https://docs.google.com/document/d/doc1/edit",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/drive/v3/"),
	)
	if err != nil {
		t.Fatalf("NewDriveService: %v", err)
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestServiceFactory(
		newDocsJSONContextWithDrive(t, driveSvc),
		func(context.Context, string) (*docs.Service, error) {
			t.Fatal("markdown replace should not use Docs batchUpdate service")
			return nil, errors.New("unexpected Docs service call")
		},
	)

	tmpDir := t.TempDir()
	mdFile := filepath.Join(tmpDir, "test.md")
	markdown := "# Hello\n\n- item\n\n| label | content |\n|---|---|\n| code | `doThing()` |\n"
	if err := os.WriteFile(mdFile, []byte(markdown), 0o600); err != nil {
		t.Fatalf("write markdown temp file: %v", err)
	}

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--file", mdFile, "--replace", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown replace write: %v", err)
	}
	if !sawDriveUpdate {
		t.Fatal("expected markdown replace path to call Drive update")
	}
	if !strings.Contains(uploadBody, "# Hello") {
		t.Fatalf("expected upload body to contain markdown content, got: %q", uploadBody)
	}
	if !strings.Contains(uploadBody, "| code | `doThing()` |") {
		t.Fatalf("expected upload body to preserve table-cell inline code, got: %q", uploadBody)
	}
}

func TestDocsWrite_MarkdownReplaceNormalizesEmptyTableHeaderForDrive(t *testing.T) {
	var uploadBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/doc1"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			uploadBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "doc1", "name": "Doc"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/drive/v3/"),
	)
	if err != nil {
		t.Fatalf("NewDriveService: %v", err)
	}

	markdown := "|     |     |\n|-----|-----|\n| Label A | Value A |\n| Label B | Value B |\n"
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestServiceFactory(
		newDocsJSONContextWithDrive(t, driveSvc),
		func(context.Context, string) (*docs.Service, error) {
			t.Fatal("empty-header normalization should not require Docs service")
			return nil, errors.New("unexpected Docs service call")
		},
	)
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", markdown, "--replace", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown replace write: %v", err)
	}
	if strings.Contains(uploadBody, "|     |     |") {
		t.Fatalf("expected blank header row to be removed, got: %q", uploadBody)
	}
	if !strings.Contains(uploadBody, "| Label A | Value A |\n|-----|-----|") {
		t.Fatalf("expected first data row promoted to markdown header, got: %q", uploadBody)
	}
}

func TestDocsWrite_MarkdownReplaceRewritesHeadingSlugLinks(t *testing.T) {
	var sawDocsGet bool
	var batchReq docs.BatchUpdateDocumentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/doc1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "doc1", "name": "Doc"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/documents/doc1"):
			sawDocsGet = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&docs.Document{
				DocumentId: "doc1",
				Body: &docs.Body{Content: []*docs.StructuralElement{
					{
						StartIndex: 1,
						EndIndex:   20,
						Paragraph: &docs.Paragraph{
							ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1", HeadingId: "h.heading1"},
							Elements: []*docs.ParagraphElement{{
								StartIndex: 1,
								EndIndex:   19,
								TextRun:    &docs.TextRun{Content: "Executive Summary\n"},
							}},
						},
					},
					{
						StartIndex: 20,
						EndIndex:   25,
						Paragraph: &docs.Paragraph{
							Elements: []*docs.ParagraphElement{{
								StartIndex: 20,
								EndIndex:   24,
								TextRun: &docs.TextRun{
									Content:   "Jump",
									TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "#executive-summary"}},
								},
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

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/drive/v3/"),
	)
	if err != nil {
		t.Fatalf("NewDriveService: %v", err)
	}
	docsSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	markdown := "# Executive Summary\n\n[Jump](#executive-summary)\n"
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newDocsJSONContextWithDrive(t, driveSvc), docsSvc)
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", markdown, "--replace", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown replace write: %v", err)
	}
	if !sawDocsGet {
		t.Fatal("expected Docs get after Drive markdown import")
	}
	if len(batchReq.Requests) != 1 || batchReq.Requests[0].UpdateTextStyle == nil {
		t.Fatalf("expected one UpdateTextStyle request, got %#v", batchReq.Requests)
	}
	styleReq := batchReq.Requests[0].UpdateTextStyle
	if styleReq.Fields != "link" || styleReq.TextStyle.Link == nil || styleReq.TextStyle.Link.HeadingId != "h.heading1" {
		t.Fatalf("unexpected link rewrite request: %#v", styleReq)
	}
}

func TestDocsWrite_MarkdownReplaceStripsExplicitHeadingAnchors(t *testing.T) {
	var uploadBody string
	var sawDocsGet bool
	var batchReq docs.BatchUpdateDocumentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/doc1"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			uploadBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "doc1", "name": "Doc"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/documents/doc1"):
			sawDocsGet = true
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
						EndIndex:   12,
						Paragraph: &docs.Paragraph{
							Elements: []*docs.ParagraphElement{{
								StartIndex: 7,
								EndIndex:   11,
								TextRun: &docs.TextRun{
									Content:   "Jump",
									TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "#attachments"}},
								},
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

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/drive/v3/"),
	)
	if err != nil {
		t.Fatalf("NewDriveService: %v", err)
	}
	docsSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	markdown := "# Files {#attachments}\n\n[Jump](#attachments)\n"
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newDocsJSONContextWithDrive(t, driveSvc), docsSvc)
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", markdown, "--replace", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown replace write: %v", err)
	}
	if strings.Contains(uploadBody, "{#attachments}") {
		t.Fatalf("upload body still contains explicit anchor: %q", uploadBody)
	}
	if !sawDocsGet {
		t.Fatal("expected Docs get after Drive markdown import")
	}
	if len(batchReq.Requests) != 1 || batchReq.Requests[0].UpdateTextStyle == nil {
		t.Fatalf("expected one UpdateTextStyle request, got %#v", batchReq.Requests)
	}
	styleReq := batchReq.Requests[0].UpdateTextStyle
	if styleReq.TextStyle == nil || styleReq.TextStyle.Link == nil || styleReq.TextStyle.Link.HeadingId != "h.files" {
		t.Fatalf("unexpected link rewrite request: %#v", styleReq)
	}
}

func TestDocsWrite_MarkdownImagesInsertedAfterDriveUpdate(t *testing.T) {
	origRetryDelays := docsImageInsertRetryDelays
	t.Cleanup(func() {
		docsImageInsertRetryDelays = origRetryDelays
	})
	docsImageInsertRetryDelays = []time.Duration{0}

	var uploadBody string
	var sawDocsGet bool
	var imageInsertAttempts int
	var batchReq docs.BatchUpdateDocumentRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/doc1"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			uploadBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc1",
				"name":        "Doc",
				"webViewLink": "https://docs.google.com/document/d/doc1/edit",
			})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/documents/doc1"):
			sawDocsGet = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithText(uploadBody))
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/documents/doc1:batchUpdate"):
			imageInsertAttempts++
			if imageInsertAttempts == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    http.StatusInternalServerError,
						"message": "Internal Error",
						"status":  "INTERNAL",
					},
				})
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&batchReq); err != nil {
				t.Fatalf("decode batch update: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/drive/v3/"),
	)
	if err != nil {
		t.Fatalf("NewDriveService: %v", err)
	}
	docsSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	markdown := strings.Join([]string{
		"# Images",
		"![default](https://example.com/default.png)",
		"![wide](https://example.com/wide.png){width=200}",
		"![sized](https://example.com/sized.png){width=200 height=150}",
		"",
	}, "\n")

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newDocsJSONContextWithDrive(t, driveSvc), docsSvc)
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", markdown, "--replace", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown replace write: %v", err)
	}

	if strings.Contains(uploadBody, "![default]") || strings.Contains(uploadBody, "![wide]") || strings.Contains(uploadBody, "![sized]") {
		t.Fatalf("expected drive update body to use placeholders, got: %q", uploadBody)
	}
	if count := strings.Count(uploadBody, "<<IMG_"); count != 3 {
		t.Fatalf("expected 3 image placeholders in drive update body, got %d in %q", count, uploadBody)
	}
	if !sawDocsGet {
		t.Fatal("expected image insertion path to read the document")
	}
	if imageInsertAttempts != 2 {
		t.Fatalf("expected image insert retry, got %d attempts", imageInsertAttempts)
	}

	inserts := map[string]*docs.InsertInlineImageRequest{}
	for _, req := range batchReq.Requests {
		if req.InsertInlineImage != nil {
			inserts[req.InsertInlineImage.Uri] = req.InsertInlineImage
		}
	}
	if len(inserts) != 3 {
		t.Fatalf("expected 3 inserted images, got %d", len(inserts))
	}

	assertImageSize(t, inserts["https://example.com/default.png"], defaultImageMaxWidthPt, 0)
	assertImageSize(t, inserts["https://example.com/wide.png"], 200, 0)
	assertImageSize(t, inserts["https://example.com/sized.png"], 200, 150)
}

func TestDocsWrite_MarkdownLocalImagesReturnActionableError(t *testing.T) {
	tmpDir := t.TempDir()
	imgDir := filepath.Join(tmpDir, "assets")
	if err := os.Mkdir(imgDir, 0o700); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	imagePath := filepath.Join(imgDir, "local.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	mdFile := filepath.Join(tmpDir, "source.md")
	if err := os.WriteFile(mdFile, []byte("![local](assets/local.png)\n"), 0o600); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	var uploadBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/doc1"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read markdown upload body: %v", err)
			}
			uploadBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "doc1", "name": "Doc"})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/documents/doc1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithText(uploadBody))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/drive/v3/"),
	)
	if err != nil {
		t.Fatalf("NewDriveService: %v", err)
	}
	docsSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newDocsJSONContextWithDrive(t, driveSvc), docsSvc)
	err = runKong(t, &DocsWriteCmd{}, []string{"doc1", "--file", mdFile, "--replace", "--markdown"}, ctx, flags)
	if err == nil {
		t.Fatal("expected local markdown image error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
	if !strings.Contains(err.Error(), "local markdown image") || !strings.Contains(err.Error(), "public HTTPS image URL") {
		t.Fatalf("expected actionable local-image error, got %v", err)
	}
}

func TestDocsWrite_MarkdownAppendUsesDocsFormatting(t *testing.T) {
	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithText("Existing\n"))
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch request: %v", err)
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

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(
		newDocsJSONContextWithoutDrive(t, "markdown append should not use Drive update"),
		docSvc,
	)

	markdown := "# Title\n\n**bold**\n"
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text=" + markdown, "--append", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown append write: %v", err)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 3 {
		t.Fatalf("expected insert plus 2 formatting requests, got %#v", reqs)
	}
	if reqs[0].InsertText == nil {
		t.Fatalf("expected first request to insert text, got %#v", reqs[0])
	}
	if got := reqs[0].InsertText; got.Location.Index != 9 || got.Text != "\nTitle\nbold\n" {
		t.Fatalf("unexpected markdown insert: %#v", got)
	}
	if reqs[1].UpdateParagraphStyle == nil {
		t.Fatalf("expected heading paragraph style request, got %#v", reqs[1])
	}
	if got := reqs[1].UpdateParagraphStyle.Range; got.StartIndex != 10 || got.EndIndex != 16 {
		t.Fatalf("unexpected heading range: %#v", got)
	}
	if reqs[2].UpdateTextStyle == nil {
		t.Fatalf("expected bold text style request, got %#v", reqs[2])
	}
	if got := reqs[2].UpdateTextStyle.Range; got.StartIndex != 16 || got.EndIndex != 20 {
		t.Fatalf("unexpected bold range: %#v", got)
	}
}

func TestInsertPreparedDocsMarkdownAtReturnsContentBounds(t *testing.T) {
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, ":batchUpdate") {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
	}))
	defer cleanup()

	result, err := insertPreparedDocsMarkdownAt(
		context.Background(),
		docSvc,
		"doc1",
		9,
		prepareMarkdown("# Title"),
		"",
		true,
	)
	if err != nil {
		t.Fatalf("insertPreparedDocsMarkdownAt: %v", err)
	}
	if result.RequestCount != 2 || result.Inserted != len("\nTitle\n") {
		t.Fatalf("result = %#v", result)
	}
	if result.ContentStart != 10 || result.ContentEnd != 16 {
		t.Fatalf("content bounds = [%d,%d), want [10,16)", result.ContentStart, result.ContentEnd)
	}
}

func TestDocsWrite_MarkdownAppendRewritesExplicitHeadingAnchorLinks(t *testing.T) {
	var batchRequests [][]*docs.Request
	var getCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			if getCalls == 1 {
				_ = json.NewEncoder(w).Encode(docBodyWithText("Existing\n"))
				return
			}
			_ = json.NewEncoder(w).Encode(&docs.Document{
				DocumentId: "doc1",
				Body: &docs.Body{Content: []*docs.StructuralElement{
					{
						StartIndex: 1,
						EndIndex:   10,
						Paragraph: &docs.Paragraph{
							ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1", HeadingId: "h.existing"},
							Elements: []*docs.ParagraphElement{{
								StartIndex: 1,
								EndIndex:   10,
								TextRun:    &docs.TextRun{Content: "Existing\n"},
							}},
						},
					},
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
				}},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch request: %v", err)
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

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(
		newDocsJSONContextWithoutDrive(t, "markdown append should not use Drive update"),
		docSvc,
	)

	markdown := "# Files {#attachments}\n\n[Jump](#attachments)\n"
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text=" + markdown, "--append", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown append write: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected insert + link rewrite batches, got %d", len(batchRequests))
	}
	insertReqs := batchRequests[0]
	if len(insertReqs) == 0 || insertReqs[0].InsertText == nil {
		t.Fatalf("expected first batch to insert text, got %#v", insertReqs)
	}
	if got := insertReqs[0].InsertText; got.Location.Index != 9 || got.Text != "\nFiles\nJump\n" {
		t.Fatalf("unexpected append insert: %#v", got)
	}
	rewriteReqs := batchRequests[1]
	if len(rewriteReqs) != 1 || rewriteReqs[0].UpdateTextStyle == nil {
		t.Fatalf("expected one link rewrite request, got %#v", rewriteReqs)
	}
	styleReq := rewriteReqs[0].UpdateTextStyle
	if styleReq.Range.StartIndex != 16 || styleReq.Range.EndIndex != 20 {
		t.Fatalf("unexpected rewrite range: %#v", styleReq.Range)
	}
	if styleReq.TextStyle == nil || styleReq.TextStyle.Link == nil || styleReq.TextStyle.Link.HeadingId != "h.files" {
		t.Fatalf("unexpected link rewrite request: %#v", styleReq)
	}
}

func TestDocsWrite_MarkdownAppendStartsStyledBlocksOnFreshParagraph(t *testing.T) {
	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithText("Existing\n"))
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch request: %v", err)
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

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(
		newDocsJSONContextWithoutDrive(t, "markdown append should not use Drive update"),
		docSvc,
	)

	markdown := "- Item\n\n```\nline 1\nline 2\n```\n"
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text=" + markdown, "--append", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown append write: %v", err)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 4 {
		t.Fatalf("expected insert, bullet, code text style, and code shading requests, got %#v", reqs)
	}
	if got := reqs[0].InsertText; got == nil || got.Location.Index != 9 || got.Text != "\nItem\n\nline 1"+docsmarkdown.SoftLineBreak+"line 2\n" {
		t.Fatalf("unexpected markdown insert: %#v", got)
	}
	if got := reqs[1].CreateParagraphBullets; got == nil || got.Range.StartIndex != 10 || got.Range.EndIndex != 15 {
		t.Fatalf("unexpected bullet request: %#v", got)
	}
	if got := reqs[2].UpdateTextStyle; got == nil || got.Range.StartIndex != 16 || got.Range.EndIndex != 30 {
		t.Fatalf("unexpected code text style request: %#v", got)
	} else {
		assertFencedCodeTextStyle(t, got)
	}
	if got := reqs[3].UpdateParagraphStyle; got == nil || got.Range.StartIndex != 16 || got.Range.EndIndex != 30 {
		t.Fatalf("unexpected code shading request: %#v", got)
	}
}

func assertImageSize(t *testing.T, ins *docs.InsertInlineImageRequest, wantWidth, wantHeight float64) {
	t.Helper()
	if ins == nil {
		t.Fatal("missing inserted image request")
	}
	if wantWidth == 0 {
		if ins.ObjectSize.Width != nil {
			t.Fatalf("expected no width, got %+v", ins.ObjectSize.Width)
		}
	} else if ins.ObjectSize.Width == nil || ins.ObjectSize.Width.Magnitude != wantWidth || ins.ObjectSize.Width.Unit != "PT" {
		t.Fatalf("expected width=%v PT, got %+v", wantWidth, ins.ObjectSize.Width)
	}
	if wantHeight == 0 {
		if ins.ObjectSize.Height != nil {
			t.Fatalf("expected no height, got %+v", ins.ObjectSize.Height)
		}
	} else if ins.ObjectSize.Height == nil || ins.ObjectSize.Height.Magnitude != wantHeight || ins.ObjectSize.Height.Unit != "PT" {
		t.Fatalf("expected height=%v PT, got %+v", wantHeight, ins.ObjectSize.Height)
	}
}
