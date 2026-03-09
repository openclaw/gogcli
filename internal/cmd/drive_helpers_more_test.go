package cmd

import "testing"

func TestReplaceExt(t *testing.T) {
	if got := replaceExt("/tmp/a.txt", ".pdf"); got != "/tmp/a.pdf" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := replaceExt("a", ".pdf"); got != "a.pdf" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestDriveExportMimeType(t *testing.T) {
	if got := driveExportMimeType("application/vnd.google-apps.document"); got != "application/pdf" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportMimeType("application/vnd.google-apps.spreadsheet"); got != "text/csv" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportMimeType("application/vnd.google-apps.presentation"); got != "application/pdf" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportMimeType("application/vnd.google-apps.drawing"); got != "image/png" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportMimeType("application/vnd.google-apps.unknown"); got != "application/pdf" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestDriveExportExtension(t *testing.T) {
	if got := driveExportExtension("application/pdf"); got != ".pdf" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("text/csv"); got != ".csv" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); got != ".xlsx" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("application/vnd.openxmlformats-officedocument.presentationml.presentation"); got != ".pptx" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("application/vnd.openxmlformats-officedocument.wordprocessingml.document"); got != ".docx" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("image/png"); got != ".png" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("text/plain"); got != ".txt" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("text/markdown"); got != ".md" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("text/html"); got != ".html" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveExportExtension("nope"); got != ".pdf" {
		t.Fatalf("unexpected: %q", got)
	}
}
