package cmd

import (
	"context"
	"errors"
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

func TestDownloadDriveFile_RequiresOverwrite(t *testing.T) {
	download := func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("replacement")),
		}, nil
	}
	ctx := withDriveTestOperations(context.Background(), &drive.Service{}, download, nil)
	dest := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, _, err := downloadDriveFile(ctx, &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "", false); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected existing-file error, got %v", err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "original" {
		t.Fatalf("existing file changed: data=%q err=%v", got, err)
	}

	outPath, size, err := downloadDriveFile(ctx, &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "", true)
	if err != nil {
		t.Fatalf("downloadDriveFile overwrite: %v", err)
	}
	if outPath != dest || size != int64(len("replacement")) {
		t.Fatalf("outPath=%q size=%d", outPath, size)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "replacement" {
		t.Fatalf("overwrite failed: data=%q err=%v", got, err)
	}
}

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
	ctx := withDriveTestOperations(context.Background(), svc, driveDownload, driveExportDownload)
	outPath, n, err := downloadDriveFile(ctx, svc, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "", false)
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
	called := false
	download := func(context.Context, *drive.Service, string) (*http.Response, error) {
		called = true
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "file.html")
	ctx := withDriveTestOperations(context.Background(), &drive.Service{}, download, nil)
	_, _, err := downloadDriveFile(ctx, &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "html", false)
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
	ctx := withDriveTestOperations(context.Background(), svc, driveDownload, driveExportDownload)
	outPath, n, err := downloadDriveFile(ctx, svc, &drive.File{Id: "id1", MimeType: "application/vnd.google-apps.document"}, dest, "", false)
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
	download := func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "403 Forbidden",
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("nope\n")),
		}, nil
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "file.bin")
	ctx := withDriveTestOperations(context.Background(), &drive.Service{}, download, nil)
	_, _, err := downloadDriveFile(ctx, &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "", false)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "download failed") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadDriveFile_CreatesMissingParentDirs(t *testing.T) {
	download := func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("x")),
		}, nil
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "no-such-dir", "file.bin")
	ctx := withDriveTestOperations(context.Background(), &drive.Service{}, download, nil)
	outPath, size, err := downloadDriveFile(ctx, &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "", false)
	if err != nil {
		t.Fatalf("downloadDriveFile: %v", err)
	}
	if outPath != dest {
		t.Fatalf("outPath=%q, want %q", outPath, dest)
	}
	if size != 1 {
		t.Fatalf("size=%d, want 1", size)
	}
	data, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if string(data) != "x" {
		t.Fatalf("data=%q, want %q", string(data), "x")
	}
}
