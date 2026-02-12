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
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	srv := httptestServerForAttachment(t, base64.RawURLEncoding.EncodeToString([]byte("fresh")))

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	gotPath, cached, bytes, err := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", path, -1)
	if err != nil {
		t.Fatalf("downloadAttachmentToPath: %v", err)
	}
	if gotPath != path || cached || bytes != 5 {
		t.Fatalf("unexpected result: path=%q cached=%v bytes=%d", gotPath, cached, bytes)
	}
	if data, err := os.ReadFile(path); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if string(data) != "fresh" {
		t.Fatalf("unexpected data: %q", string(data))
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

func TestDownloadAttachmentToPath_DirectoryNotCacheHit(t *testing.T) {
	dir := t.TempDir()
	srv := httptestServerForAttachment(t, base64.RawURLEncoding.EncodeToString([]byte("x")))

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, _, _, err := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", dir, -1); err == nil {
		t.Fatalf("expected error for directory output path")
	}
}

func TestSanitizeAttachmentFilename(t *testing.T) {
	tests := []struct {
		name     string
		fallback string
		want     string
	}{
		{"report.pdf", "attachment.bin", "report.pdf"},
		{"", "attachment.bin", "attachment.bin"},
		{"   ", "attachment.bin", "attachment.bin"},
		{".", "attachment.bin", "attachment.bin"},
		{"..", "attachment.bin", "attachment.bin"},
		{"../../etc/passwd", "attachment.bin", "passwd"},
		{"../../../secret.txt", "attachment.bin", "secret.txt"},
		{"/absolute/path/file.txt", "attachment.bin", "file.txt"},
		{"dir/subdir/file.txt", "attachment.bin", "file.txt"},
		{"normal.txt", "fallback.dat", "normal.txt"},
	}
	for _, tt := range tests {
		got := sanitizeAttachmentFilename(tt.name, tt.fallback)
		if got != tt.want {
			t.Errorf("sanitizeAttachmentFilename(%q, %q) = %q, want %q", tt.name, tt.fallback, got, tt.want)
		}
	}
}

func TestResolveAttachmentOutputPath(t *testing.T) {
	t.Run("explicit file path", func(t *testing.T) {
		path, err := resolveAttachmentOutputPath("m1", "a1", "/tmp/out.bin", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/tmp/out.bin" {
			t.Fatalf("got %q, want /tmp/out.bin", path)
		}
	})

	t.Run("directory target appends filename", func(t *testing.T) {
		dir := t.TempDir()
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", dir, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dir, "m1_abcdefgh_attachment.bin")
		if path != want {
			t.Fatalf("got %q, want %q", path, want)
		}
	})

	t.Run("directory target with custom name", func(t *testing.T) {
		dir := t.TempDir()
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", dir, "report.pdf")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dir, "report.pdf")
		if path != want {
			t.Fatalf("got %q, want %q", path, want)
		}
	})

	t.Run("traversal in name is stripped", func(t *testing.T) {
		dir := t.TempDir()
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", dir, "../../etc/passwd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dir, "passwd")
		if path != want {
			t.Fatalf("got %q, want %q", path, want)
		}
	})

	t.Run("trailing separator treated as directory", func(t *testing.T) {
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", "/tmp/newdir/", "report.pdf")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join("/tmp/newdir", "report.pdf")
		if path != want {
			t.Fatalf("got %q, want %q", path, want)
		}
	})
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
