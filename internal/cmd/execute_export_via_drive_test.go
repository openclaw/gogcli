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

func TestExecute_DocsExport_JSON(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Doc",
			"mimeType": "application/vnd.google-apps.document",
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

	var gotExportMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, _ string, mimeType string) (*http.Response, error) {
		gotExportMime = mimeType
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	outBase := filepath.Join(t.TempDir(), "out")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"docs", "export", "id1",
				"--out", outBase,
				"--format", "docx",
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
	if want := outBase + ".docx"; parsed.Path != want || parsed.Size != 3 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if gotExportMime != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("unexpected export mime type: %q", gotExportMime)
	}
	b, err := os.ReadFile(outBase + ".docx")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "abc" {
		t.Fatalf("unexpected file contents: %q", string(b))
	}
}

func TestExecute_DocsExport_Markdown(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Doc",
			"mimeType": "application/vnd.google-apps.document",
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

	var gotExportMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, _ string, mimeType string) (*http.Response, error) {
		gotExportMime = mimeType
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("# Doc\n")),
		}, nil
	}

	outBase := filepath.Join(t.TempDir(), "out")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"docs", "export", "id1",
				"--out", outBase,
				"--format", "md",
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
	if want := outBase + ".md"; parsed.Path != want || parsed.Size != 6 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if gotExportMime != "text/markdown" {
		t.Fatalf("unexpected export mime type: %q", gotExportMime)
	}
	b, err := os.ReadFile(outBase + ".md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "# Doc\n" {
		t.Fatalf("unexpected file contents: %q", string(b))
	}
}

func TestExecute_DocsExport_HTML(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Doc",
			"mimeType": "application/vnd.google-apps.document",
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

	var gotExportMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, _ string, mimeType string) (*http.Response, error) {
		gotExportMime = mimeType
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("<h1>Doc</h1>\n")),
		}, nil
	}

	outBase := filepath.Join(t.TempDir(), "out")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"docs", "export", "id1",
				"--out", outBase,
				"--format", "html",
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
	if want := outBase + ".html"; parsed.Path != want || parsed.Size != 13 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if gotExportMime != "text/html" {
		t.Fatalf("unexpected export mime type: %q", gotExportMime)
	}
	b, err := os.ReadFile(outBase + ".html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "<h1>Doc</h1>\n" {
		t.Fatalf("unexpected file contents: %q", string(b))
	}
}

func TestExecute_DocsExport_TypeMismatch(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "NotADoc",
			"mimeType": "application/pdf",
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

	errOut := captureStderr(t, func() {
		if err := Execute([]string{"--account", "a@b.com", "docs", "export", "id1", "--out", filepath.Join(t.TempDir(), "out")}); err == nil {
			t.Fatalf("expected error")
		}
	})
	if !strings.Contains(errOut, "file is not a Google Doc") {
		t.Fatalf("unexpected stderr=%q", errOut)
	}
}

func TestExecute_SheetsExport_DefaultFormat_XLSX(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Sheet",
			"mimeType": "application/vnd.google-apps.spreadsheet",
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

	var gotExportMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, _ string, mimeType string) (*http.Response, error) {
		gotExportMime = mimeType
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	outBase := filepath.Join(t.TempDir(), "out")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"sheets", "export", "id1",
				"--out", outBase,
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
	if want := outBase + ".xlsx"; parsed.Path != want || parsed.Size != 3 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if gotExportMime != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("unexpected export mime type: %q", gotExportMime)
	}
	if _, err := os.Stat(outBase + ".xlsx"); err != nil {
		t.Fatalf("expected file at %s: %v", outBase+".xlsx", err)
	}
}

func TestExecute_SheetsExport_PDF(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Sheet",
			"mimeType": "application/vnd.google-apps.spreadsheet",
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

	var gotExportMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, _ string, mimeType string) (*http.Response, error) {
		gotExportMime = mimeType
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	outBase := filepath.Join(t.TempDir(), "out")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"sheets", "export", "id1",
				"--out", outBase,
				"--format", "pdf",
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
	if want := outBase + ".pdf"; parsed.Path != want || parsed.Size != 3 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if gotExportMime != "application/pdf" {
		t.Fatalf("unexpected export mime type: %q", gotExportMime)
	}
}

func TestExecute_SlidesExport_DefaultFormat_PPTX(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Deck",
			"mimeType": "application/vnd.google-apps.presentation",
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

	var gotExportMime string
	driveExportDownload = func(_ context.Context, _ *drive.Service, _ string, mimeType string) (*http.Response, error) {
		gotExportMime = mimeType
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	outBase := filepath.Join(t.TempDir(), "out")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"slides", "export", "id1",
				"--out", outBase,
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
	if want := outBase + ".pptx"; parsed.Path != want || parsed.Size != 3 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if gotExportMime != "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		t.Fatalf("unexpected export mime type: %q", gotExportMime)
	}
}
