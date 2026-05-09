//go:build !windows

package secrets

func defaultIsInvalidFileKeyError(error) bool {
	return false
}
