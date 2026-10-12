//go:build !linux && !darwin

package secrets

import "errors"

func exchangeRekeyDirs(_, _ string) error {
	return errors.New("atomic replacement is unavailable on this platform; choose a fresh output directory")
}
