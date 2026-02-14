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
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func TestDriveCommands_MissingAccount(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{}

	cases := []struct {
		name string
		run  func() error
	}{
		{"ls", func() error { return (&DriveLsCmd{}).Run(ctx, flags) }},
		{"search", func() error { return (&DriveSearchCmd{}).Run(ctx, flags) }},
		{"get", func() error { return (&DriveGetCmd{}).Run(ctx, flags) }},
		{"download", func() error { return (&DriveDownloadCmd{}).Run(ctx, flags) }},
		{"upload", func() error { return (&DriveUploadCmd{}).Run(ctx, flags) }},
		{"mkdir", func() error { return (&DriveMkdirCmd{}).Run(ctx, flags) }},
		{"delete", func() error { return (&DriveDeleteCmd{}).Run(ctx, flags) }},
		{"move", func() error { return (&DriveMoveCmd{}).Run(ctx, flags) }},
		{"rename", func() error { return (&DriveRenameCmd{}).Run(ctx, flags) }},
		{"share", func() error { return (&DriveShareCmd{}).Run(ctx, flags) }},
		{"unshare", func() error { return (&DriveUnshareCmd{}).Run(ctx, flags) }},
		{"permissions", func() error { return (&DrivePermissionsCmd{}).Run(ctx, flags) }},
		{"url", func() error { return (&DriveURLCmd{}).Run(ctx, flags) }},
	}

	for _, tc := range cases {
		if err := tc.run(); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestDriveCommands_UsageErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	cases := []struct {
		name string
		run  func() error
	}{
		{"search missing query", func() error { return (&DriveSearchCmd{}).Run(ctx, flags) }},
		{"get missing file", func() error { return (&DriveGetCmd{}).Run(ctx, flags) }},
		{"download missing file", func() error { return (&DriveDownloadCmd{}).Run(ctx, flags) }},
		{"upload missing path", func() error { return (&DriveUploadCmd{}).Run(ctx, flags) }},
		{"mkdir missing name", func() error { return (&DriveMkdirCmd{}).Run(ctx, flags) }},
		{"delete missing file", func() error { return (&DriveDeleteCmd{}).Run(ctx, flags) }},
		{"move missing file", func() error { return (&DriveMoveCmd{}).Run(ctx, flags) }},
		{"move missing parent", func() error { return (&DriveMoveCmd{FileID: "f1"}).Run(ctx, flags) }},
		{"rename missing file", func() error { return (&DriveRenameCmd{}).Run(ctx, flags) }},
		{"rename missing name", func() error { return (&DriveRenameCmd{FileID: "f1"}).Run(ctx, flags) }},
		{"share missing file", func() error { return (&DriveShareCmd{}).Run(ctx, flags) }},
		{"share missing target", func() error { return (&DriveShareCmd{FileID: "f1"}).Run(ctx, flags) }},
		{"share invalid role", func() error { return (&DriveShareCmd{FileID: "f1", Email: "x@y.com", Role: "nope"}).Run(ctx, flags) }},
		{"unshare missing file", func() error { return (&DriveUnshareCmd{}).Run(ctx, flags) }},
		{"unshare missing perm", func() error { return (&DriveUnshareCmd{FileID: "f1"}).Run(ctx, flags) }},
		{"permissions missing file", func() error { return (&DrivePermissionsCmd{}).Run(ctx, flags) }},
	}

	for _, tc := range cases {
		if err := tc.run(); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestDriveShare_DefaultRole(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		return nil, errors.New("no service")
	}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DriveShareCmd{FileID: "f1", Email: "x@y.com"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDriveDownload_TextOutput(t *testing.T) {
	origNew := newDriveService
	origDownload := driveDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveDownload = origDownload
	})

	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data")),
		}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/drive/v3/files/") && !strings.HasPrefix(r.URL.Path, "/files/") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "file1",
			"name":     "File",
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

	var outBuf bytes.Buffer
	u, uiErr := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	dest := filepath.Join(t.TempDir(), "out.txt")
	cmd := &DriveDownloadCmd{FileID: "file1", Output: OutputPathFlag{Path: dest}}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(outBuf.String(), "path") {
		t.Fatalf("unexpected output: %q", outBuf.String())
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected file: %v", err)
	}
}

func TestDownloadDriveFile_ErrorPaths(t *testing.T) {
	origDownload := driveDownload
	origExport := driveExportDownload
	t.Cleanup(func() {
		driveDownload = origDownload
		driveExportDownload = origExport
	})

	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return nil, errors.New("download boom")
	}
	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return nil, errors.New("export boom")
	}

	if _, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "x", MimeType: "text/plain"}, "out", ""); err == nil {
		t.Fatalf("expected download error")
	}
	if _, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "x", MimeType: driveMimeGoogleDoc}, "out", ""); err == nil {
		t.Fatalf("expected export error")
	}
}

func TestGoogleConvertMimeType(t *testing.T) {
	cases := []struct {
		path     string
		wantMime string
		wantOK   bool
	}{
		{"report.docx", driveMimeGoogleDoc, true},
		{"report.DOCX", driveMimeGoogleDoc, true},
		{"old.doc", driveMimeGoogleDoc, true},
		{"budget.xlsx", driveMimeGoogleSheet, true},
		{"budget.xls", driveMimeGoogleSheet, true},
		{"data.csv", driveMimeGoogleSheet, true},
		{"deck.pptx", driveMimeGoogleSlides, true},
		{"deck.ppt", driveMimeGoogleSlides, true},
		{"notes.txt", driveMimeGoogleDoc, true},
		{"page.html", driveMimeGoogleDoc, true},
		{"photo.png", "", false},
		{"archive.zip", "", false},
		{"binary.exe", "", false},
	}
	for _, tc := range cases {
		mime, ok := googleConvertMimeType(tc.path)
		if ok != tc.wantOK || mime != tc.wantMime {
			t.Errorf("googleConvertMimeType(%q) = (%q, %v), want (%q, %v)", tc.path, mime, ok, tc.wantMime, tc.wantOK)
		}
	}
}

func TestGoogleConvertTargetMimeType(t *testing.T) {
	cases := []struct {
		target   string
		wantMime string
		wantOK   bool
	}{
		{"doc", driveMimeGoogleDoc, true},
		{"sheet", driveMimeGoogleSheet, true},
		{"slides", driveMimeGoogleSlides, true},
		{"DOC", driveMimeGoogleDoc, true},
		{"unknown", "", false},
	}
	for _, tc := range cases {
		mime, ok := googleConvertTargetMimeType(tc.target)
		if ok != tc.wantOK || mime != tc.wantMime {
			t.Errorf("googleConvertTargetMimeType(%q) = (%q, %v), want (%q, %v)", tc.target, mime, ok, tc.wantMime, tc.wantOK)
		}
	}
}

func TestDriveUploadConvertMimeType(t *testing.T) {
	mimeType, convert, err := driveUploadConvertMimeType("report.docx", true, "")
	if err != nil {
		t.Fatalf("auto convert: %v", err)
	}
	if !convert || mimeType != driveMimeGoogleDoc {
		t.Fatalf("auto convert = (%q, %v), want (%q, true)", mimeType, convert, driveMimeGoogleDoc)
	}

	mimeType, convert, err = driveUploadConvertMimeType("photo.png", false, "sheet")
	if err != nil {
		t.Fatalf("explicit convert: %v", err)
	}
	if !convert || mimeType != driveMimeGoogleSheet {
		t.Fatalf("explicit convert = (%q, %v), want (%q, true)", mimeType, convert, driveMimeGoogleSheet)
	}

	mimeType, convert, err = driveUploadConvertMimeType("photo.png", false, "")
	if err != nil {
		t.Fatalf("no convert: %v", err)
	}
	if convert || mimeType != "" {
		t.Fatalf("no convert = (%q, %v), want empty/false", mimeType, convert)
	}

	if _, _, err = driveUploadConvertMimeType("photo.png", false, "not-a-target"); err == nil {
		t.Fatalf("expected error for invalid --convert-to target")
	}
}

func TestStripOfficeExt(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"report.docx", "report"},
		{"report.doc", "report"},
		{"budget.xlsx", "budget"},
		{"budget.xls", "budget"},
		{"deck.pptx", "deck"},
		{"deck.ppt", "deck"},
		{"notes.txt", "notes.txt"},
		{"photo.png", "photo.png"},
		{"no-ext", "no-ext"},
	}
	for _, tc := range cases {
		got := stripOfficeExt(tc.name)
		if got != tc.want {
			t.Errorf("stripOfficeExt(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDriveUpload_ConvertUnsupported(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	tmp := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(tmp, []byte("png-data"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })
	newServiceCalled := false
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		newServiceCalled = true
		return &drive.Service{}, nil
	}

	cmd := &DriveUploadCmd{LocalPath: tmp, Convert: true}
	if err := cmd.Run(ctx, flags); err == nil {
		t.Fatalf("expected error for unsupported --convert type")
	} else if !strings.Contains(err.Error(), "--convert: unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
	if newServiceCalled {
		t.Fatalf("newDriveService should not be called when --convert validation fails")
	}
}

func TestDriveWebLink_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
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
	if _, err := driveWebLink(context.Background(), svc, "file1"); err == nil {
		t.Fatalf("expected error")
	}
}
