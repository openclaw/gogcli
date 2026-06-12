package cmd

import "testing"

func TestDriveType(t *testing.T) {
	if got := driveType("application/vnd.google-apps.folder"); got != "folder" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveType("application/pdf"); got != "file" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := driveType(driveMimeShortcut); got != "shortcut" {
		t.Fatalf("unexpected shortcut type: %q", got)
	}
}

func TestFormatDateTime(t *testing.T) {
	if got := formatDateTime(""); got != "-" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := formatDateTime("2025-12-12T14:37:47Z"); got != "2025-12-12 14:37" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := formatDateTime("short"); got != "short" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestGuessMimeType(t *testing.T) {
	cases := map[string]string{
		"a.PDF":     "application/pdf",
		"a.docx":    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"a.xlsx":    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"a.ppt":     "application/vnd.ms-powerpoint",
		"a.jpeg":    "image/jpeg",
		"a.json":    "application/json",
		"a.csv":     "text/csv",
		"a.unknown": "application/octet-stream",
	}
	for in, want := range cases {
		if got := guessMimeType(in); got != want {
			t.Fatalf("guessMimeType(%q)=%q want %q", in, got, want)
		}
	}
}
