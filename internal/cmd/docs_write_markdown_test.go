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
