package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
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

func TestDownloadAttachmentToPath_ExpectedSizeUnknown_Redownloads(t *testing.T) {
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

func mustDryRunAttachmentPath(t *testing.T, args ...string) string {
	t.Helper()

	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		err := runKong(t, &GmailAttachmentCmd{}, args, ctx, &RootFlags{DryRun: true})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("expected exit code 0, got: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput=%q", err, out)
	}
	req, ok := got["request"].(map[string]any)
	if !ok {
		t.Fatalf("expected request object, got=%T", got["request"])
	}
	path, ok := req["path"].(string)
	if !ok {
		t.Fatalf("expected request.path string, got=%T", req["path"])
	}
	return path
}

func TestGmailAttachmentCmd_DryRun_OutDir_UsesName(t *testing.T) {
	outDir := t.TempDir()
	got := mustDryRunAttachmentPath(t, "m1", "a1", "--out", outDir, "--name", "invoice.pdf")
	want := filepath.Join(outDir, "invoice.pdf")
	if got != want {
		t.Fatalf("unexpected path: got=%q want=%q", got, want)
	}
}

func TestGmailAttachmentCmd_DryRun_OutDirTrailingSlash_UsesNameEvenIfMissing(t *testing.T) {
	base := t.TempDir()
	outDir := filepath.Join(base, "newdir") + string(os.PathSeparator)
	got := mustDryRunAttachmentPath(t, "m1", "a1", "--out", outDir, "--name", "invoice.pdf")
	want := filepath.Join(filepath.Join(base, "newdir"), "invoice.pdf")
	if got != want {
		t.Fatalf("unexpected path: got=%q want=%q", got, want)
	}
}

func TestGmailAttachmentCmd_DryRun_OutDirTrailingSlash_NoName_UsesStableDefault(t *testing.T) {
	base := t.TempDir()
	outDir := filepath.Join(base, "newdir") + string(os.PathSeparator)
	got := mustDryRunAttachmentPath(t, "m1", "a1", "--out", outDir)
	want := filepath.Join(filepath.Join(base, "newdir"), "m1_a1_attachment.bin")
	if got != want {
		t.Fatalf("unexpected path: got=%q want=%q", got, want)
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
		{"..\\..\\etc\\passwd", "attachment.bin", "passwd"},
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
		path, err := resolveAttachmentOutputPath("m1", "a1", "/tmp/out.bin", "", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/tmp/out.bin" {
			t.Fatalf("got %q, want /tmp/out.bin", path)
		}
	})

	t.Run("directory target appends filename", func(t *testing.T) {
		dir := t.TempDir()
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", dir, "", false)
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
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", dir, "report.pdf", false)
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
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", dir, "../../etc/passwd", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dir, "passwd")
		if path != want {
			t.Fatalf("got %q, want %q", path, want)
		}
	})

	t.Run("trailing separator treated as directory", func(t *testing.T) {
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", "/tmp/newdir/", "report.pdf", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join("/tmp/newdir", "report.pdf")
		if path != want {
			t.Fatalf("got %q, want %q", path, want)
		}
	})

	t.Run("trailing separator with no name uses stable default", func(t *testing.T) {
		path, err := resolveAttachmentOutputPath("m1", "abcdefghij", "/tmp/newdir/", "", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join("/tmp/newdir", "m1_abcdefgh_attachment.bin")
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
