package cmd

import (
	"strings"
	"testing"
)

func TestDriveFileListFieldsIncludesDriveID(t *testing.T) {
	if !strings.Contains(driveFileListFields, "driveId") {
		t.Fatalf("driveFileListFields must include driveId; got %q", driveFileListFields)
	}
	if !strings.Contains(driveFileListFields, "hasThumbnail") || !strings.Contains(driveFileListFields, "thumbnailLink") {
		t.Fatalf("driveFileListFields must include thumbnail fields; got %q", driveFileListFields)
	}
	if !strings.Contains(driveFileListFields, "shortcutDetails(targetId,targetMimeType,targetResourceKey)") {
		t.Fatalf("driveFileListFields must include shortcut details; got %q", driveFileListFields)
	}
}

func TestDriveFileGetFieldsIncludesDriveID(t *testing.T) {
	if !strings.Contains(driveFileGetFields, "driveId") {
		t.Fatalf("driveFileGetFields must include driveId; got %q", driveFileGetFields)
	}
	if !strings.Contains(driveFileGetFields, "hasThumbnail") || !strings.Contains(driveFileGetFields, "thumbnailLink") {
		t.Fatalf("driveFileGetFields must include thumbnail fields; got %q", driveFileGetFields)
	}
	if !strings.Contains(driveFileGetFields, "shortcutDetails(targetId,targetMimeType,targetResourceKey)") {
		t.Fatalf("driveFileGetFields must include shortcut details; got %q", driveFileGetFields)
	}
}
