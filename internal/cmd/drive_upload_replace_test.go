package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDriveUpload_Replace_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	var sawPatch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drive service is configured with endpoint srv.URL+"/", so API calls are rooted at /drive/v3.
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")

		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(path, "/files/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          id,
				"name":        "Existing.pdf",
				"mimeType":    "application/pdf",
				"webViewLink": "https://example.com/" + id,
			})
			return
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/") && r.Method == http.MethodPatch:
			sawPatch = true
			id := strings.TrimPrefix(r.URL.Path, "/upload/drive/v3/files/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          id,
				"name":        "Existing.pdf",
				"mimeType":    "application/pdf",
				"webViewLink": "https://example.com/" + id,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		cmd := &DriveUploadCmd{}
		if err := runKong(t, cmd, []string{local, "--replace", "file1"}, ctx, flags); err != nil {
			t.Fatalf("replace: %v", err)
		}
	})
	if !sawPatch {
		t.Fatalf("expected PATCH upload")
	}

	var got struct {
		File            *drive.File `json:"file"`
		Replaced        bool        `json:"replaced"`
		PreservedFileID bool        `json:"preservedFileId"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, out)
	}
	if got.File == nil || got.File.Id != "file1" {
		t.Fatalf("unexpected file: %#v", got.File)
	}
	if !got.Replaced {
		t.Fatalf("expected replaced=true")
	}
	if !got.PreservedFileID {
		t.Fatalf("expected preservedFileId=true")
	}
}

func TestDriveUpload_Replace_Text(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")

		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "file1",
				"name":        "Existing.pdf",
				"mimeType":    "application/pdf",
				"webViewLink": "https://example.com/file1",
			})
			return
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/") && r.Method == http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "file1",
				"name":        "Renamed.pdf",
				"mimeType":    "application/pdf",
				"webViewLink": "https://example.com/file1",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	var outBuf bytes.Buffer
	u, uiErr := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--replace", "file1", "--name", "Renamed.pdf"}, ctx, flags); err != nil {
		t.Fatalf("replace: %v", err)
	}

	out := outBuf.String()
	if !strings.Contains(out, "replaced\ttrue") {
		t.Fatalf("expected replaced=true in output, got: %q", out)
	}
	if !strings.Contains(out, "name\tRenamed.pdf") {
		t.Fatalf("expected updated name in output, got: %q", out)
	}
}

func TestDriveUpload_Replace_ParentValidation(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	tmp := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(tmp, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := (&DriveUploadCmd{
		LocalPath:     tmp,
		Parent:        "p1",
		ReplaceFileID: "file1",
	}).Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %#v", err)
	}
}

func TestDriveUpload_Replace_GoogleWorkspaceUnsupported(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "doc1",
				"name":     "Doc",
				"mimeType": "application/vnd.google-apps.document",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--replace", "doc1"}, ctx, flags); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), "Google Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveUpload_Replace_ConvertValidation(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	tmp := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(tmp, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := (&DriveUploadCmd{
		LocalPath:     tmp,
		ReplaceFileID: "file1",
		Convert:       true,
	}).Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %#v", err)
	}
}

func TestDriveUpload_Replace_KeepRevisionForeverAndMimeType(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	const customMimeType = "application/x-custom-pdf"
	var sawKeepRevisionForever bool
	var sawMimeType bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")

		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"mimeType": "application/pdf",
			})
			return
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/") && r.Method == http.MethodPatch:
			parsedKeepRevisionForever, parseBoolErr := strconv.ParseBool(r.URL.Query().Get("keepRevisionForever"))
			if parseBoolErr != nil {
				t.Fatalf("ParseBool: %v", parseBoolErr)
			}
			sawKeepRevisionForever = parsedKeepRevisionForever
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("ReadAll: %v", readErr)
			}
			sawMimeType = strings.Contains(string(body), customMimeType)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"name":     "Existing.pdf",
				"mimeType": "application/pdf",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--replace", "file1", "--keep-revision-forever", "--mime-type", customMimeType}, ctx, flags); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if !sawKeepRevisionForever {
		t.Fatalf("expected keepRevisionForever query param set")
	}
	if !sawMimeType {
		t.Fatalf("expected upload body to include custom mime type %q", customMimeType)
	}
}

func TestDriveUpload_Create_KeepRevisionForever(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	var sawKeepRevisionForever bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/upload/drive/v3/files" && r.Method == http.MethodPost:
			parsedKeepRevisionForever, parseBoolErr := strconv.ParseBool(r.URL.Query().Get("keepRevisionForever"))
			if parseBoolErr != nil {
				t.Fatalf("ParseBool: %v", parseBoolErr)
			}
			sawKeepRevisionForever = parsedKeepRevisionForever
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "new1",
				"name": "upload.pdf",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--keep-revision-forever"}, ctx, flags); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !sawKeepRevisionForever {
		t.Fatalf("expected keepRevisionForever query param set")
	}
}
