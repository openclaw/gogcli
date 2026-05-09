package cmd

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
)

func TestExecute_DriveDownload_WithOutStdout(t *testing.T) {
	origDownload := driveDownload
	t.Cleanup(func() { driveDownload = origDownload })
	t.Chdir(t.TempDir())

	svc, closeSvc := newDriveMetadataTestService(t, "text/plain")
	t.Cleanup(closeSvc)
	stubDriveServiceForTest(t, svc)

	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--account", "a@b.com",
				"drive", "download", "id1",
				"--out", "-",
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	if stdout != "abc" {
		t.Fatalf("stdout=%q, want raw bytes", stdout)
	}
	if _, statErr := os.Stat("-"); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file named -, stat=%v", statErr)
	}
}

func TestExecute_DriveDownload_WithOutStdout_JSONRejected(t *testing.T) {
	origDownload := driveDownload
	t.Cleanup(func() { driveDownload = origDownload })

	svc, closeSvc := newDriveMetadataTestService(t, "text/plain")
	t.Cleanup(closeSvc)
	stubDriveServiceForTest(t, svc)

	called := false
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	var execErr error
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			execErr = Execute([]string{
				"--json",
				"--account", "a@b.com",
				"drive", "download", "id1",
				"--out", "-",
			})
		})
	})

	if execErr == nil || !strings.Contains(execErr.Error(), "can't combine --json with --out -") {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if stdout != "" {
		t.Fatalf("stdout=%q, want empty", stdout)
	}
	if called {
		t.Fatalf("download should not be called")
	}
}

func TestExecute_DocsExport_WithOutStdout(t *testing.T) {
	origExport := driveExportDownload
	t.Cleanup(func() { driveExportDownload = origExport })
	t.Chdir(t.TempDir())

	svc, closeSvc := newDriveMetadataTestService(t, "application/vnd.google-apps.document")
	t.Cleanup(closeSvc)
	stubDriveServiceForTest(t, svc)

	var gotExportMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, _ string, mimeType string) (*http.Response, error) {
		gotExportMime = mimeType
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("plain text\n")),
		}, nil
	}

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--account", "a@b.com",
				"docs", "export", "id1",
				"--out", "-",
				"--format", "txt",
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	if stdout != "plain text\n" {
		t.Fatalf("stdout=%q, want raw export bytes", stdout)
	}
	if gotExportMime != "text/plain" {
		t.Fatalf("unexpected export mime type: %q", gotExportMime)
	}
	if _, statErr := os.Stat("-.txt"); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file named -.txt, stat=%v", statErr)
	}
}

func TestExecute_DocsExport_WithOutStdout_JSONRejected(t *testing.T) {
	origExport := driveExportDownload
	t.Cleanup(func() { driveExportDownload = origExport })

	svc, closeSvc := newDriveMetadataTestService(t, "application/vnd.google-apps.document")
	t.Cleanup(closeSvc)
	stubDriveServiceForTest(t, svc)

	called := false
	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("plain text\n")),
		}, nil
	}

	var execErr error
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			execErr = Execute([]string{
				"--json",
				"--account", "a@b.com",
				"docs", "export", "id1",
				"--out", "-",
				"--format", "txt",
			})
		})
	})

	if execErr == nil || !strings.Contains(execErr.Error(), "can't combine --json with --out -") {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if stdout != "" {
		t.Fatalf("stdout=%q, want empty", stdout)
	}
	if called {
		t.Fatalf("export should not be called")
	}
}
