package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestDownloadDriveFile_NonGoogleDoc(t *testing.T) {
	body := "hello"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Files.Get(...).Download hits /drive/v3/files/{id}?alt=media
		if !(strings.Contains(r.URL.Path, "/files/") && r.URL.Query().Get("alt") == "media") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
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

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "file.bin")
	outPath, n, err := downloadDriveFile(context.Background(), svc, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "")
	if err != nil {
		t.Fatalf("downloadDriveFile: %v", err)
	}
	if outPath != dest {
		t.Fatalf("unexpected outPath: %q", outPath)
	}
	if n != int64(len(body)) {
		t.Fatalf("unexpected n: %d", n)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != body {
		t.Fatalf("unexpected body: %q", string(b))
	}
}

func TestDownloadDriveFile_NonGoogleDocFormatRejected(t *testing.T) {
	origDownload := driveDownload
	t.Cleanup(func() { driveDownload = origDownload })

	called := false
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		called = true
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "file.html")
	_, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "html")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "non-Google Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatalf("download should not be called on format error")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file written, stat=%v", statErr)
	}
}

func TestDownloadDriveFile_GoogleDocExport(t *testing.T) {
	body := "exported"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Files.Export(...).Download hits /drive/v3/files/{id}/export?mimeType=...
		if !(strings.Contains(r.URL.Path, "/export") && strings.Contains(r.URL.Path, "/files/")) {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
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

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "doc.txt")
	outPath, n, err := downloadDriveFile(context.Background(), svc, &drive.File{Id: "id1", MimeType: "application/vnd.google-apps.document"}, dest, "")
	if err != nil {
		t.Fatalf("downloadDriveFile: %v", err)
	}
	if !strings.HasSuffix(outPath, ".pdf") {
		t.Fatalf("expected pdf outPath, got: %q", outPath)
	}
	if n != int64(len(body)) {
		t.Fatalf("unexpected n: %d", n)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != body {
		t.Fatalf("unexpected body: %q", string(b))
	}
}

func TestDownloadDriveFile_HTTPError(t *testing.T) {
	orig := driveDownload
	t.Cleanup(func() { driveDownload = orig })
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "403 Forbidden",
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("nope\n")),
		}, nil
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "file.bin")
	_, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "download failed") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadDriveFile_CreateError(t *testing.T) {
	orig := driveDownload
	t.Cleanup(func() { driveDownload = orig })
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("x")),
		}, nil
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "no-such-dir", "file.bin")
	_, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "")
	if err == nil {
		t.Fatalf("expected error")
	}
}
