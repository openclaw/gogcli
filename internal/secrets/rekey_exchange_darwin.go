package secrets

import "golang.org/x/sys/unix"

func exchangeRekeyDirs(a, b string) error {
	return unix.RenamexNp(a, b, unix.RENAME_SWAP)
}
