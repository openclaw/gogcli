//go:build !linux && !darwin

package secrets

import "errors"

var errRekeyExchangeUnavailable = errors.New("atomic replacement is unavailable on this platform; choose a fresh output directory")

func exchangeRekeyDirs(_, _ string) error {
	return errRekeyExchangeUnavailable
}
