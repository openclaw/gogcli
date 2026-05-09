//go:build windows

package secrets

import (
	"errors"
	"io/fs"
	"syscall"
)

func defaultIsInvalidFileKeyError(err error) bool {
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		return false
	}

	var errno syscall.Errno

	return errors.As(pathErr.Err, &errno) && errno == syscall.Errno(0x7B)
}
