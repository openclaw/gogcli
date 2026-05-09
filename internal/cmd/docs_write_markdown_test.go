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
)

func TestDocsWrite_MarkdownReplaceUsesDriveUpdate(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

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
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newDocsService = func(context.Context, string) (*docs.Service, error) {
		t.Fatal("markdown replace should not use Docs batchUpdate service")
		return nil, errors.New("unexpected Docs service call")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	tmpDir := t.TempDir()
	mdFile := filepath.Join(tmpDir, "test.md")
	markdown := "# Hello\n\n- item\n"
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
}

func TestDocsWrite_MarkdownImagesInsertedAfterDriveUpdate(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	origRetryDelays := docsImageInsertRetryDelays
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
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
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docsSvc, nil }

	markdown := strings.Join([]string{
		"# Images",
		"![default](https://example.com/default.png)",
		"![wide](https://example.com/wide.png){width=200}",
		"![sized](https://example.com/sized.png){width=200 height=150}",
		"",
	}, "\n")

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)
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
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

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
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docsSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)
	err = runKong(t, &DocsWriteCmd{}, []string{"doc1", "--file", mdFile, "--replace", "--markdown"}, ctx, flags)
	if err == nil {
		t.Fatal("expected local markdown image error")
	}
	if !strings.Contains(err.Error(), "local markdown image") || !strings.Contains(err.Error(), "public HTTPS image URL") {
		t.Fatalf("expected actionable local-image error, got %v", err)
	}
}

func TestDocsWrite_MarkdownAppendUsesDocsFormatting(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

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

	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("markdown append should not use Drive update")
		return nil, errors.New("unexpected Drive service call")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	markdown := "# Title\n\n**bold**\n"
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", markdown, "--append", "--markdown"}, ctx, flags); err != nil {
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
	if got := reqs[0].InsertText; got.Location.Index != 9 || got.Text != "Title\nbold\n" {
		t.Fatalf("unexpected markdown insert: %#v", got)
	}
	if reqs[1].UpdateParagraphStyle == nil {
		t.Fatalf("expected heading paragraph style request, got %#v", reqs[1])
	}
	if got := reqs[1].UpdateParagraphStyle.Range; got.StartIndex != 9 || got.EndIndex != 15 {
		t.Fatalf("unexpected heading range: %#v", got)
	}
	if reqs[2].UpdateTextStyle == nil {
		t.Fatalf("expected bold text style request, got %#v", reqs[2])
	}
	if got := reqs[2].UpdateTextStyle.Range; got.StartIndex != 15 || got.EndIndex != 19 {
		t.Fatalf("unexpected bold range: %#v", got)
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
