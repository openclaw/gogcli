//go:build !windows

package filelock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(file *os.File) error {
	// File descriptors are OS-assigned small integers; unix.Flock requires int.
	//nolint:gosec
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("flock: %w", err)
	}

	return nil
}

func unlockFile(file *os.File) error {
	// File descriptors are OS-assigned small integers; unix.Flock requires int.
	//nolint:gosec
	if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlock flock: %w", err)
	}

	return nil
}

func wouldBlock(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
