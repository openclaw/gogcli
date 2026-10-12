package secrets

import "golang.org/x/sys/unix"

func exchangeRekeyDirs(a, b string) error {
	return unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
}
