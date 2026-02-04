package cmd

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func TestDriveUploadConvertMetadata(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	var (
		mu        sync.Mutex
		gotMime   string
		gotParsed bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/upload/drive/v3/files") {
			http.NotFound(w, r)
			return
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse content-type: %v", err)
		}
		if !strings.HasPrefix(mediaType, "multipart/") {
			t.Fatalf("expected multipart upload, got %q", mediaType)
		}
		boundary := params["boundary"]
		if boundary == "" {
			t.Fatalf("missing multipart boundary")
		}

		reader := multipart.NewReader(r.Body, boundary)
		found := false
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("read multipart: %v", err)
			}
			if strings.Contains(part.Header.Get("Content-Type"), "application/json") {
				var meta drive.File
				if err := json.NewDecoder(part).Decode(&meta); err != nil {
					t.Fatalf("decode metadata: %v", err)
				}
				mu.Lock()
				gotMime = meta.MimeType
				gotParsed = true
				mu.Unlock()
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("metadata part not found")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "up1",
			"name":     "upload.docx",
			"mimeType": driveMimeGoogleDoc,
		})
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	tmpFile := filepath.Join(t.TempDir(), "upload.docx")
	if err := os.WriteFile(tmpFile, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &DriveUploadCmd{LocalPath: tmpFile, Convert: true}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("upload: %v", err)
	}

	mu.Lock()
	got := gotMime
	parsed := gotParsed
	mu.Unlock()

	if !parsed {
		t.Fatalf("expected metadata to be parsed")
	}
	if got != driveMimeGoogleDoc {
		t.Fatalf("mimeType = %q, want %q", got, driveMimeGoogleDoc)
	}
}
