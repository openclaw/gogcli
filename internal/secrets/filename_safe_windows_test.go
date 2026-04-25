//go:build windows

package secrets

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"
)

func TestDefaultIsInvalidFilenameError_OnWindows(t *testing.T) {
	if defaultIsInvalidFilenameError(nil) {
		t.Errorf("defaultIsInvalidFilenameError(nil) = true, want false")
	}

	pathErr := &fs.PathError{Op: "open", Path: `C:\foo:bar`, Err: errorInvalidName}
	if !defaultIsInvalidFilenameError(pathErr) {
		t.Errorf("defaultIsInvalidFilenameError(ERROR_INVALID_NAME PathError) = false, want true")
	}

	wrapped := errors.New("wrap: " + pathErr.Error())
	if defaultIsInvalidFilenameError(wrapped) {
		t.Errorf("defaultIsInvalidFilenameError(non-PathError) = true, want false")
	}

	other := &fs.PathError{Op: "open", Path: "C:\\foo", Err: syscall.Errno(2)}
	if defaultIsInvalidFilenameError(other) {
		t.Errorf("defaultIsInvalidFilenameError(unrelated errno PathError) = true, want false")
	}
}
