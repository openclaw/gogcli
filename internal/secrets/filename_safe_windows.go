//go:build windows

package secrets

import (
	"errors"
	"io/fs"
	"syscall"
)

// errorInvalidName is Windows ERROR_INVALID_NAME (0x7B). The 99designs/keyring
// file backend surfaces it via *fs.PathError when os.OpenFile / os.Remove is
// called with a path containing characters NTFS rejects (":", "<", etc.).
const errorInvalidName = syscall.Errno(0x7B)

func defaultIsInvalidFilenameError(err error) bool {
	if err == nil {
		return false
	}

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		return false
	}

	var errno syscall.Errno

	return errors.As(pathErr.Err, &errno) && errno == errorInvalidName
}
