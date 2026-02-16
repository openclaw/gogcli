package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestExecute_DriveGet_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// google.golang.org/api/drive sometimes uses basepaths with or without /drive/v3.
		// For this test we accept any GET and return the metadata payload.
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "id1",
			"name":         "Doc",
			"mimeType":     "application/pdf",
			"size":         "1024",
			"modifiedTime": "2025-12-12T14:37:47Z",
			"createdTime":  "2025-12-11T00:00:00Z",
			"starred":      true,
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "get", "id1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		File struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Starred bool   `json:"starred"`
		} `json:"file"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.File.ID != "id1" || parsed.File.Name != "Doc" || !parsed.File.Starred {
		t.Fatalf("unexpected file: %#v", parsed.File)
	}
}

func TestExecute_DriveDownload_JSON(t *testing.T) {
	origNew := newDriveService
	origDownload := driveDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveDownload = origDownload
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Metadata fetch (Do()).
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "file.bin",
			"mimeType": "application/pdf",
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "out.bin")
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "download", "id1", "--out", dest}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Path != dest || parsed.Size != 3 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if b, err := os.ReadFile(dest); err != nil || string(b) != "abc" {
		t.Fatalf("file mismatch: err=%v body=%q", err, string(b))
	}

	// Sanity: downloads dir is still creatable (but we passed dest explicitly).
	if _, err := config.EnsureDriveDownloadsDir(); err != nil {
		t.Fatalf("EnsureDriveDownloadsDir: %v", err)
	}
}

func TestDriveDownloadCmd_FileHasNoName(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "",
			"mimeType": "application/pdf",
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	flags := &RootFlags{Account: "a@b.com"}
	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	if execErr := runKong(t, &DriveDownloadCmd{}, []string{"id1", "--out", filepath.Join(t.TempDir(), "out.bin")}, ctx, flags); execErr == nil || !strings.Contains(execErr.Error(), "file has no name") {
		t.Fatalf("expected file has no name error, got: %v", execErr)
	}
}

func TestExecute_DriveDownload_GoogleSheet_PDF(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Metadata fetch (Do()).
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "sheet1",
			"name":     "Sheet Name",
			"mimeType": "application/vnd.google-apps.spreadsheet",
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	var gotMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, fileID string, mimeType string) (*http.Response, error) {
		if fileID != "sheet1" {
			t.Fatalf("fileID=%q", fileID)
		}
		gotMime = mimeType
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("%PDF-FAKE")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "out")
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "download", "sheet1", "--format", "pdf", "--out", dest}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotMime != "application/pdf" {
		t.Fatalf("mimeType=%q", gotMime)
	}

	var parsed struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !strings.HasSuffix(parsed.Path, ".pdf") {
		t.Fatalf("expected .pdf path, got %q", parsed.Path)
	}
	if parsed.Size != int64(len("%PDF-FAKE")) {
		t.Fatalf("size=%d", parsed.Size)
	}
	if b, err := os.ReadFile(parsed.Path); err != nil || string(b) != "%PDF-FAKE" {
		t.Fatalf("file mismatch: err=%v body=%q", err, string(b))
	}
}

func TestExecute_DriveDownload_GoogleDoc_DOCX(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "doc1",
			"name":     "My Doc",
			"mimeType": "application/vnd.google-apps.document",
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	var gotMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, fileID string, mimeType string) (*http.Response, error) {
		if fileID != "doc1" {
			t.Fatalf("fileID=%q", fileID)
		}
		gotMime = mimeType
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("DOCX-FAKE")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "out")
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "download", "doc1", "--format", "docx", "--out", dest}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotMime != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("mimeType=%q", gotMime)
	}

	var parsed struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !strings.HasSuffix(parsed.Path, ".docx") {
		t.Fatalf("expected .docx path, got %q", parsed.Path)
	}
	if b, err := os.ReadFile(parsed.Path); err != nil || string(b) != "DOCX-FAKE" {
		t.Fatalf("file mismatch: err=%v body=%q", err, string(b))
	}
}

func TestExecute_DriveDownload_GoogleSlides_PPTX(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "slides1",
			"name":     "My Slides",
			"mimeType": "application/vnd.google-apps.presentation",
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	var gotMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, fileID string, mimeType string) (*http.Response, error) {
		if fileID != "slides1" {
			t.Fatalf("fileID=%q", fileID)
		}
		gotMime = mimeType
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("PPTX-FAKE")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "out")
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "download", "slides1", "--format", "pptx", "--out", dest}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotMime != "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		t.Fatalf("mimeType=%q", gotMime)
	}

	var parsed struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !strings.HasSuffix(parsed.Path, ".pptx") {
		t.Fatalf("expected .pptx path, got %q", parsed.Path)
	}
	if b, err := os.ReadFile(parsed.Path); err != nil || string(b) != "PPTX-FAKE" {
		t.Fatalf("file mismatch: err=%v body=%q", err, string(b))
	}
}
