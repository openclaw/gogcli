//go:build !windows

package secrets

import (
	"errors"
	"fmt"
	"os"
)

func syncRekeyDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open output directory for sync: %w", err)
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func renameRekeyDir(from, to string) error { return os.Rename(from, to) }
