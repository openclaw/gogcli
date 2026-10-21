//go:build !windows

package secrets

import (
	"errors"
	"fmt"
	"os"
)

func syncRekeyDir(path string) error {
	dir, err := os.Open(path) //nolint:gosec // deliberately sync the selected output directory
	if err != nil {
		return fmt.Errorf("open output directory for sync: %w", err)
	}

	return errors.Join(dir.Sync(), dir.Close())
}

func renameRekeyDir(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("rename keyring directory: %w", err)
	}

	return nil
}
