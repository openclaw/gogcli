package secrets

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func exchangeRekeyDirs(a, b string) error {
	if err := unix.RenamexNp(a, b, unix.RENAME_SWAP); err != nil {
		return fmt.Errorf("exchange keyring directories: %w", err)
	}

	return nil
}
