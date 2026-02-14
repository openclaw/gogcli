package cmd

import (
	"context"
	"encoding/json"
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

func TestExecute_DriveDownload_WithOutFile_JSON(t *testing.T) {
	origNew := newDriveService
	origDownload := driveDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveDownload = origDownload
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/files/id1") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Doc",
			"mimeType": "text/plain",
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

	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	outPath := filepath.Join(t.TempDir(), "out.bin")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"drive", "download", "id1",
				"--out", outPath,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	var parsed struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if unmarshalErr := json.Unmarshal([]byte(stdout), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, stdout)
	}
	if parsed.Path != outPath || parsed.Size != 3 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "abc" {
		t.Fatalf("unexpected file contents: %q", string(b))
	}
}

func TestExecute_DriveDownload_WithOutDir_JSON(t *testing.T) {
	origNew := newDriveService
	origDownload := driveDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveDownload = origDownload
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/files/id1") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Doc",
			"mimeType": "text/plain",
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

	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	outDir := t.TempDir()
	wantPath := filepath.Join(outDir, "id1_Doc")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"drive", "download", "id1",
				"--out", outDir,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	var parsed struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if unmarshalErr := json.Unmarshal([]byte(stdout), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, stdout)
	}
	if parsed.Path != wantPath || parsed.Size != 3 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if _, statErr := os.Stat(wantPath); statErr != nil {
		t.Fatalf("expected file at %s: %v", wantPath, statErr)
	}
}

func TestExecute_DriveDownload_FormatRejected_NonGoogle(t *testing.T) {
	origNew := newDriveService
	origDownload := driveDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveDownload = origDownload
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/files/id1") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Doc",
			"mimeType": "text/plain",
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

	called := false
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	outPath := filepath.Join(t.TempDir(), "out.pdf")
	var execErr error
	_ = captureStderr(t, func() {
		execErr = Execute([]string{
			"--account", "a@b.com",
			"drive", "download", "id1",
			"--format", "pdf",
			"--out", outPath,
		})
	})
	if execErr == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(execErr.Error(), "non-Google Workspace") {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if called {
		t.Fatalf("download should not be called on format error")
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file written, stat=%v", statErr)
	}
}
