package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestDownloadAttachmentToPath_MissingOutPath(t *testing.T) {
	if _, _, _, err := downloadAttachmentToPath(context.Background(), nil, "m1", "a1", " ", 0); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadAttachmentToPath_CachedBySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gotPath, cached, bytes, err := downloadAttachmentToPath(context.Background(), nil, "m1", "a1", path, 3)
	if err != nil {
		t.Fatalf("downloadAttachmentToPath: %v", err)
	}
	if gotPath != path || !cached || bytes != 3 {
		t.Fatalf("unexpected result: path=%q cached=%v bytes=%d", gotPath, cached, bytes)
	}
}

func TestDownloadAttachmentToPath_CachedByAnySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.bin")
	if err := os.WriteFile(path, []byte("abcd"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gotPath, cached, bytes, err := downloadAttachmentToPath(context.Background(), nil, "m1", "a1", path, -1)
	if err != nil {
		t.Fatalf("downloadAttachmentToPath: %v", err)
	}
	if gotPath != path || !cached || bytes != 4 {
		t.Fatalf("unexpected result: path=%q cached=%v bytes=%d", gotPath, cached, bytes)
	}
}

func TestDownloadAttachmentToPath_Base64Fallback(t *testing.T) {
	srv := httptestServerForAttachment(t, base64.URLEncoding.EncodeToString([]byte("hello")))

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	path := filepath.Join(t.TempDir(), "c.bin")
	gotPath, cached, bytes, err := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", path, 0)
	if err != nil {
		t.Fatalf("downloadAttachmentToPath: %v", err)
	}
	if gotPath != path || cached || bytes != 5 {
		t.Fatalf("unexpected result: path=%q cached=%v bytes=%d", gotPath, cached, bytes)
	}
	if data, err := os.ReadFile(path); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if string(data) != "hello" {
		t.Fatalf("unexpected data: %q", string(data))
	}
}

func TestDownloadAttachmentToPath_EmptyData(t *testing.T) {
	srv := httptestServerForAttachment(t, "")

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	path := filepath.Join(t.TempDir(), "d.bin")
	if _, _, _, err := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", path, 0); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadAttachmentToPath_DirectoryNotCached(t *testing.T) {
	dir := t.TempDir()
	// A directory should not be treated as a cached attachment even though
	// os.Stat succeeds and Size() > 0 on directories.
	srv := httptestServerForAttachment(t, base64.RawURLEncoding.EncodeToString([]byte("data")))
	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	outPath := filepath.Join(dir, "subdir")
	if err := os.Mkdir(outPath, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	// With expectedSize == -1, a directory should NOT be returned as cached.
	// Instead it should attempt download and fail trying to write to the dir path.
	_, _, _, dlErr := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", outPath, -1)
	// We expect an error because outPath is a directory and WriteFile to a dir fails.
	if dlErr == nil {
		t.Fatalf("expected error when outPath is a directory")
	}
}

func TestDownloadAttachmentToPath_DirectoryNotCachedBySize(t *testing.T) {
	dir := t.TempDir()
	// Even with a positive expectedSize matching the directory's metadata
	// size, a directory should not be treated as a cached file.
	srv := httptestServerForAttachment(t, base64.RawURLEncoding.EncodeToString([]byte("data")))
	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Pass the directory size as expectedSize — should NOT match because
	// the path is a directory, not a regular file.
	info, _ := os.Stat(dir)
	_, cached, _, dlErr := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", dir, info.Size())
	// It should not return cached=true; it will try to download and fail
	// because WriteFile to a directory fails.
	if dlErr == nil && cached {
		t.Fatalf("directory should not be treated as cached file")
	}
}

func httptestServerForAttachment(t *testing.T, data string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data,
		})
	}))
}
