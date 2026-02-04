package cmd

import (
	"testing"

	"github.com/steipete/gogcli/internal/googleapi"
)

func TestBuildDriveListQuery(t *testing.T) {
	t.Run("adds parent and trashed", func(t *testing.T) {
		got := buildDriveListQuery("root", "")
		if got != "'root' in parents and trashed = false" {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("combines with user query", func(t *testing.T) {
		got := buildDriveListQuery("abc", "mimeType='image/png'")
		if got != "mimeType='image/png' and 'abc' in parents and trashed = false" {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("does not force trashed when user sets it", func(t *testing.T) {
		got := buildDriveListQuery("abc", "trashed = true")
		if got != "trashed = true and 'abc' in parents" {
			t.Fatalf("unexpected: %q", got)
		}
	})
}

func TestBuildDriveSearchQuery(t *testing.T) {
	got := buildDriveSearchQuery("hello world")
	if got != "fullText contains 'hello world' and trashed = false" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestEscapeDriveQueryString(t *testing.T) {
	got := googleapi.EscapeDriveQueryValue("a'b")
	if got != "a\\'b" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormatDriveSize(t *testing.T) {
	if got := formatDriveSize(0); got != "-" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := formatDriveSize(1); got != "1 B" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := formatDriveSize(1024); got != "1.0 KB" {
		t.Fatalf("unexpected: %q", got)
	}
}
