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

func TestDriveConvertToMimeType(t *testing.T) {
	tests := map[string]string{
		"":       "",
		"sheet":  driveMimeGoogleSheet,
		"SHEET":  driveMimeGoogleSheet,
		"doc":    driveMimeGoogleDoc,
		"DOC":    driveMimeGoogleDoc,
		"slides": driveMimeGoogleSlides,
		"SLIDES": driveMimeGoogleSlides,
	}

	for in, want := range tests {
		got, err := driveConvertToMimeType(in)
		if err != nil {
			t.Fatalf("driveConvertToMimeType(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("driveConvertToMimeType(%q)=%q want %q", in, got, want)
		}
	}

	_, err := driveConvertToMimeType("nope")
	if err == nil {
		t.Fatalf("expected invalid convert-to error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", ExitCode(err), err)
	}
	if !strings.Contains(err.Error(), "invalid --convert-to") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "sheet|doc|slides") {
		t.Fatalf("unexpected error details: %v", err)
	}
}
