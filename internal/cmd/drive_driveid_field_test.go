package cmd

import (
	"strings"
	"testing"
)

// TestDriveFileListFields_IncludesDriveId guards against future regressions
// where driveId gets removed from the shared field mask. Without driveId
// in the mask, the Drive API will not return it in responses, making it
// impossible for callers to distinguish My Drive files from Shared Drive
// files in gog JSON output.
func TestDriveFileListFields_IncludesDriveId(t *testing.T) {
	if !strings.Contains(driveFileListFields, "driveId") {
		t.Fatalf("driveFileListFields must include driveId; got %q", driveFileListFields)
	}
}
