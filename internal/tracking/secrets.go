package tracking

import (
	"errors"
	"fmt"
	"strings"

	"github.com/99designs/keyring"

	"github.com/degree-analytics/ratatosk/internal/secrets"
)

var (
	errMissingTrackingKey = errors.New("missing tracking key")
	errMissingAdminKey    = errors.New("missing admin key")
)

const (
	legacyTrackingKeySecretKey = "tracking/tracking_key"
	legacyAdminKeySecretKey    = "tracking/admin_key"
	trackingKeySecretSuffix    = "tracking_key"
	adminKeySecretSuffix       = "admin_key"
)

func SaveSecrets(account, trackingKey, adminKey string) error {
	account = normalizeAccount(account)
	if account == "" {
		return errMissingAccount
	}

	if trackingKey == "" {
		return errMissingTrackingKey
	}

	if adminKey == "" {
		return errMissingAdminKey
	}

	if err := secrets.SetSecret(scopedSecretKey(account, trackingKeySecretSuffix), []byte(trackingKey)); err != nil {
		return fmt.Errorf("store tracking key: %w", err)
	}

	if err := secrets.SetSecret(scopedSecretKey(account, adminKeySecretSuffix), []byte(adminKey)); err != nil {
		return fmt.Errorf("store admin key: %w", err)
	}

	return nil
}

func LoadSecrets(account string) (trackingKey, adminKey string, err error) {
	account = normalizeAccount(account)
	if account == "" {
		return "", "", errMissingAccount
	}

	trackingKey, err = readSecretWithFallback(scopedSecretKey(account, trackingKeySecretSuffix), legacyTrackingKeySecretKey)
	if err != nil {
		return "", "", fmt.Errorf("read tracking key: %w", err)
	}

	adminKey, err = readSecretWithFallback(scopedSecretKey(account, adminKeySecretSuffix), legacyAdminKeySecretKey)
	if err != nil {
		return "", "", fmt.Errorf("read admin key: %w", err)
	}

	return trackingKey, adminKey, nil
}

func readSecretWithFallback(primary, legacy string) (string, error) {
	val, err := secrets.GetSecret(primary)
	if err == nil {
		return string(val), nil
	}

	if !errors.Is(err, keyring.ErrKeyNotFound) {
		return "", fmt.Errorf("read secret: %w", err)
	}

	legacyVal, legacyErr := secrets.GetSecret(legacy)
	if legacyErr == nil {
		return string(legacyVal), nil
	}

	if errors.Is(legacyErr, keyring.ErrKeyNotFound) {
		return "", nil
	}

	return "", fmt.Errorf("read legacy secret: %w", legacyErr)
}

func scopedSecretKey(account, suffix string) string {
	account = strings.ReplaceAll(account, " ", "")
	return fmt.Sprintf("tracking/%s/%s", account, suffix)
}
