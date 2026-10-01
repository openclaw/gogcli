//go:build !windows

package cmd

import "os"

func makeMCPStoragePrivate(path string, directory bool) error {
	mode := os.FileMode(0o600)
	if directory {
		mode = 0o700
	}
	return os.Chmod(path, mode)
}
