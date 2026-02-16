package tracking

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"strconv"

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
	trackingKeySecretSuffix          = "tracking_key"
	adminKeySecretSuffix             = "admin_key"
	trackingKeysCurrentVersionSuffix  = "tracking_key_current_version"
	trackingKeyVersionSecretPrefix    = "tracking_key_v"
)

func LoadSecrets(account string) (trackingKey, adminKey string, err error) {
	account = normalizeAccount(account)
	if account == "" {
		return "", "", errMissingAccount
	}

	keys, currentVersion, err := LoadTrackingKeys(account, nil, 0)
	if err != nil {
		return "", "", fmt.Errorf("read tracking keys: %w", err)
	}
	if currentVersion > 0 {
		if key := strings.TrimSpace(keys[currentVersion]); key != "" {
			trackingKey = key
		}
	}

	if trackingKey == "" {
		trackingKey, err = readSecretWithFallback(scopedSecretKey(account, trackingKeySecretSuffix), legacyTrackingKeySecretKey)
		if err != nil {
			return "", "", fmt.Errorf("read tracking key: %w", err)
		}
	}

	adminKey, err = readSecretWithFallback(scopedSecretKey(account, adminKeySecretSuffix), legacyAdminKeySecretKey)
	if err != nil {
		return "", "", fmt.Errorf("read admin key: %w", err)
	}

	return trackingKey, adminKey, nil
}

func LoadTrackingKeys(account string, versions []int, currentVersion int) (map[int]string, int, error) {
	account = normalizeAccount(account)
	if account == "" {
		return nil, 0, errMissingAccount
	}

	if currentVersion == 0 {
		currentVersion = currentTrackingKeyVersion(account)
	}

	if currentVersion == 0 {
		currentVersion = 1
	}

	keys := map[int]string{}
	versionsToLoad := append([]int{}, versions...)
	if len(versionsToLoad) == 0 {
		versionsToLoad = []int{currentVersion}
	}

	for _, version := range versionsToLoad {
		if version <= 0 {
			continue
		}

		raw, keyErr := secrets.GetSecret(scopedSecretKey(account, keyVersionSecret(version)))
		if keyErr != nil {
			if !errors.Is(keyErr, keyring.ErrKeyNotFound) {
				return nil, 0, fmt.Errorf("read tracking key v%d: %w", version, keyErr)
			}
			continue
		}
		key := strings.TrimSpace(string(raw))
		if key == "" {
			continue
		}
		keys[version] = key
	}

	if len(keys) == 0 {
		legacyKey, legacyErr := readSecretWithFallback(scopedSecretKey(account, trackingKeySecretSuffix), legacyTrackingKeySecretKey)
		if legacyErr != nil {
			return nil, 0, fmt.Errorf("read tracking key: %w", legacyErr)
		}
		keys[1] = legacyKey
		currentVersion = 1
	}

	return keys, currentVersion, nil
}

func SaveTrackingKeys(account string, trackingKeys map[int]string, currentVersion int, adminKey string) error {
	account = normalizeAccount(account)
	if account == "" {
		return errMissingAccount
	}

	currentKey := strings.TrimSpace(trackingKeys[currentVersion])
	if currentVersion <= 0 || currentVersion > 255 {
		return fmt.Errorf("invalid tracking key version: %d", currentVersion)
	}
	if currentKey == "" {
		return errMissingTrackingKey
	}
	if adminKey == "" {
		return errMissingAdminKey
	}

	for _, version := range sortedKeyVersions(trackingKeys) {
		key := strings.TrimSpace(trackingKeys[version])
		if key == "" {
			continue
		}
		if err := secrets.SetSecret(scopedSecretKey(account, keyVersionSecret(version)), []byte(key)); err != nil {
			return fmt.Errorf("store tracking key v%d: %w", version, err)
		}
	}

	if err := secrets.SetSecret(scopedSecretKey(account, trackingKeysCurrentVersionSuffix), []byte(strconv.Itoa(currentVersion))); err != nil {
		return fmt.Errorf("store tracking current key version: %w", err)
	}

	// Keep legacy key name available for compatibility with older worker deployments.
	if err := secrets.SetSecret(scopedSecretKey(account, trackingKeySecretSuffix), []byte(currentKey)); err != nil {
		return fmt.Errorf("store tracking key: %w", err)
	}

	if err := secrets.SetSecret(scopedSecretKey(account, adminKeySecretSuffix), []byte(adminKey)); err != nil {
		return fmt.Errorf("store admin key: %w", err)
	}

	return nil
}

func SaveSecrets(account, trackingKey, adminKey string) error {
	account = normalizeAccount(account)
	if account == "" {
		return errMissingAccount
	}

	return SaveTrackingKeys(account, map[int]string{1: trackingKey}, 1, adminKey)
}

func currentTrackingKeyVersion(account string) int {
	raw, err := secrets.GetSecret(scopedSecretKey(account, trackingKeysCurrentVersionSuffix))
	if err != nil {
		return 0
	}

	parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if parseErr != nil {
		return 0
	}

	if parsed <= 0 || parsed > 255 {
		return 0
	}

	return parsed
}

func keyVersionSecret(version int) string {
	return trackingKeyVersionSecretPrefix + strconv.Itoa(version)
}

func sortedKeyVersions(keys map[int]string) []int {
	versions := make([]int, 0, len(keys))
	for version := range keys {
		versions = append(versions, version)
	}
	sort.Ints(versions)
	return versions
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
