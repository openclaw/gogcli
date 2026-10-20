package secrets

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func exchangeRekeyDirs(a, b string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE); err != nil {
		return fmt.Errorf("exchange keyring directories: %w", err)
	}

	return nil
}
