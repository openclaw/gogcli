//go:build !windows

package secrets

import (
	"testing"
)

func TestDefaultIsInvalidFilenameError_OnNonWindowsAlwaysFalse(t *testing.T) {
	if defaultIsInvalidFilenameError(nil) {
		t.Errorf("defaultIsInvalidFilenameError(nil) = true, want false")
	}

	if defaultIsInvalidFilenameError(errFakeInvalidName) {
		t.Errorf("defaultIsInvalidFilenameError(errFakeInvalidName) = true, want false on non-Windows host")
	}
}
