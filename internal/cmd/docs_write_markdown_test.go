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
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"body": map[string]any{
					"content": []any{
						map[string]any{"startIndex": 1, "endIndex": 20},
					},
				},
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
	if got := reqs[0].InsertText; got.Location.Index != 19 || got.Text != "Title\nbold\n" {
		t.Fatalf("unexpected markdown insert: %#v", got)
	}
	if reqs[1].UpdateParagraphStyle == nil {
		t.Fatalf("expected heading paragraph style request, got %#v", reqs[1])
	}
	if got := reqs[1].UpdateParagraphStyle.Range; got.StartIndex != 19 || got.EndIndex != 25 {
		t.Fatalf("unexpected heading range: %#v", got)
	}
	if reqs[2].UpdateTextStyle == nil {
		t.Fatalf("expected bold text style request, got %#v", reqs[2])
	}
	if got := reqs[2].UpdateTextStyle.Range; got.StartIndex != 25 || got.EndIndex != 29 {
		t.Fatalf("unexpected bold range: %#v", got)
	}
}
