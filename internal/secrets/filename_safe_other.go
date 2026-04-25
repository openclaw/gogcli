//go:build !windows

package secrets

// defaultIsInvalidFilenameError on non-Windows platforms always returns false:
// no syscall.Errno equivalent of ERROR_INVALID_NAME exists, and the filesystem
// rules that produce it (NTFS reserved characters) don't apply here.
func defaultIsInvalidFilenameError(_ error) bool {
	return false
}
