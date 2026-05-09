package tracking

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/secrets"
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

	if err := SaveTrackingKeys(account, map[int]string{1: trackingKey}, 1, adminKey); err != nil {
		return err
	}

	return nil
}

func SaveTrackingKeys(account string, trackingKeys map[int]string, currentVersion int, adminKey string) error {
	account = normalizeAccount(account)
	if account == "" {
		return errMissingAccount
	}

	if len(trackingKeys) == 0 {
		return errMissingTrackingKey
	}

	if adminKey == "" {
		return errMissingAdminKey
	}

	for version, trackingKey := range trackingKeys {
		if version < 1 || version > 255 {
			return fmt.Errorf("%w: %d", errInvalidTrackingKeyVersion, version)
		}

		if trackingKey == "" {
			return errMissingTrackingKey
		}

		if err := secrets.SetSecret(scopedSecretKey(account, versionedTrackingKeySecretSuffix(version)), []byte(trackingKey)); err != nil {
			return fmt.Errorf("store tracking key v%d: %w", version, err)
		}
	}

	currentKey := trackingKeys[currentVersion]
	if currentKey == "" {
		return fmt.Errorf("%w: %d", errMissingCurrentTrackingKeyValue, currentVersion)
	}

	if err := secrets.SetSecret(scopedSecretKey(account, trackingKeySecretSuffix), []byte(currentKey)); err != nil {
		return fmt.Errorf("store tracking key: %w", err)
	}

	if err := secrets.SetSecret(scopedSecretKey(account, adminKeySecretSuffix), []byte(adminKey)); err != nil {
		return fmt.Errorf("store admin key: %w", err)
	}

	return nil
}

func LoadTrackingKeys(account string, knownVersions []int, currentVersion int) (map[int]string, int, error) {
	account = normalizeAccount(account)
	if account == "" {
		return nil, 0, errMissingAccount
	}

	versions := NormalizeTrackingKeyVersions(knownVersions, currentVersion)
	if len(versions) == 0 {
		versions = []int{1}
	}

	keys := map[int]string{}

	for _, version := range versions {
		key, err := readSecretWithFallback(scopedSecretKey(account, versionedTrackingKeySecretSuffix(version)), "")
		if err != nil {
			return nil, 0, fmt.Errorf("read tracking key v%d: %w", version, err)
		}

		if key != "" {
			keys[version] = key
		}
	}

	if keys[1] == "" {
		legacyKey, err := readSecretWithFallback(scopedSecretKey(account, trackingKeySecretSuffix), legacyTrackingKeySecretKey)
		if err != nil {
			return nil, 0, fmt.Errorf("read tracking key: %w", err)
		}

		if legacyKey != "" {
			keys[1] = legacyKey
		}
	}

	if currentVersion <= 0 {
		currentVersion = 1
	}

	if keys[currentVersion] == "" {
		currentVersion = 0
		for version := range keys {
			if version > currentVersion {
				currentVersion = version
			}
		}
	}

	return keys, currentVersion, nil
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

	if legacy == "" {
		return "", nil
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

func versionedTrackingKeySecretSuffix(version int) string {
	return trackingKeySecretSuffix + "_v" + strconv.Itoa(version)
}
