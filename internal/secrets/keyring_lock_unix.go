//go:build !windows

package secrets

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockKeyringFile(file *os.File, exclusive bool) error {
	op := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		op = unix.LOCK_EX | unix.LOCK_NB
	}

	// File descriptors are OS-assigned small integers; unix.Flock requires int.
	if err := unix.Flock(int(file.Fd()), op); err != nil {
		return fmt.Errorf("flock: %w", err)
	}

	return nil
}

func unlockKeyringFile(file *os.File) error {
	// File descriptors are OS-assigned small integers; unix.Flock requires int.
	if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlock flock: %w", err)
	}

	return nil
}

func keyringLockWouldBlock(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
