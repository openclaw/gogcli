package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
)

func TestResolveDriveDownloadDestPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	if _, err := resolveDriveDownloadDestPath(nil, ""); err == nil {
		t.Fatalf("expected error for nil meta")
	}

	if _, err := resolveDriveDownloadDestPath(&drive.File{Name: "x"}, ""); err == nil {
		t.Fatalf("expected error for missing id")
	}

	if _, err := resolveDriveDownloadDestPath(&drive.File{Id: "id"}, ""); err == nil {
		t.Fatalf("expected error for missing name")
	}

	meta := &drive.File{Id: "id1", Name: "../file.txt"}
	path, err := resolveDriveDownloadDestPath(meta, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if filepath.Base(path) != "id1_file.txt" {
		t.Fatalf("unexpected path: %q", path)
	}

	meta.Name = ".."
	path, err = resolveDriveDownloadDestPath(meta, "")
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}

	if filepath.Base(path) != "id1_download" {
		t.Fatalf("unexpected default path: %q", path)
	}

	dir := t.TempDir()
	path, err = resolveDriveDownloadDestPath(meta, dir)
	if err != nil {
		t.Fatalf("resolve dir: %v", err)
	}

	if !strings.HasPrefix(path, dir+string(os.PathSeparator)) {
		t.Fatalf("expected path under dir, got %q", path)
	}

	outFile := filepath.Join(t.TempDir(), "custom.bin")
	path, err = resolveDriveDownloadDestPath(meta, outFile)
	if err != nil {
		t.Fatalf("resolve file: %v", err)
	}

	if path != outFile {
		t.Fatalf("expected custom path, got %q", path)
	}
}

func TestGuessMimeTypeMore(t *testing.T) {
	tests := map[string]string{
		"file.pdf":  mimePDF,
		"file.doc":  "application/msword",
		"file.docx": mimeDocx,
		"file.xls":  "application/vnd.ms-excel",
		"file.xlsx": mimeXlsx,
		"file.ppt":  "application/vnd.ms-powerpoint",
		"file.pptx": mimePptx,
		"file.png":  mimePNG,
		"file.jpg":  "image/jpeg",
		"file.gif":  "image/gif",
		"file.txt":  mimeTextPlain,
		"file.html": "text/html",
		"file.css":  "text/css",
		"file.js":   "application/javascript",
		"file.json": "application/json",
		"file.zip":  "application/zip",
		"file.csv":  "text/csv",
		"file.md":   "text/markdown",
		"file.bin":  "application/octet-stream",
	}

	for name, expected := range tests {
		if got := guessMimeType(name); got != expected {
			t.Fatalf("guessMimeType(%q) = %q, want %q", name, got, expected)
		}
	}
}

func TestDriveUploadConvertMimeType(t *testing.T) {
	tests := map[string]string{
		"file.doc":  driveMimeGoogleDoc,
		"file.docx": driveMimeGoogleDoc,
		"file.xls":  driveMimeGoogleSheet,
		"file.xlsx": driveMimeGoogleSheet,
		"file.csv":  driveMimeGoogleSheet,
		"file.ppt":  driveMimeGoogleSlides,
		"file.pptx": driveMimeGoogleSlides,
	}

	for name, expected := range tests {
		got, err := driveUploadConvertMimeType(name)
		if err != nil {
			t.Fatalf("driveUploadConvertMimeType(%q) error: %v", name, err)
		}
		if got != expected {
			t.Fatalf("driveUploadConvertMimeType(%q) = %q, want %q", name, got, expected)
		}
	}

	if _, err := driveUploadConvertMimeType("file.pdf"); err == nil {
		t.Fatalf("expected error for unsupported extension")
	}
	if _, err := driveUploadConvertMimeType("file"); err == nil {
		t.Fatalf("expected error for missing extension")
	}
}
