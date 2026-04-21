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
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

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

func TestDocsWrite_MarkdownLocalImagesResolveRelativeToSourceFile(t *testing.T) {
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
	var uploadedImageName string

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
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/upload/drive/v3/files"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read image upload body: %v", err)
			}
			if !strings.Contains(string(body), "png") {
				t.Fatalf("expected local image file contents in upload body, got %q", string(body))
			}
			uploadedImageName = "local.png"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "img1",
				"webContentLink": "https://drive.google.com/uc?id=img1",
			})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drive/v3/files/img1/permissions"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/documents/doc1:batchUpdate"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/drive/v3/files/img1"):
			w.WriteHeader(http.StatusNoContent)
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
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--file", mdFile, "--replace", "--markdown"}, ctx, flags); err != nil {
		t.Fatalf("markdown replace write: %v", err)
	}
	if uploadedImageName != "local.png" {
		t.Fatalf("expected local image upload from markdown directory, got %q", uploadedImageName)
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
