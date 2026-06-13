package tracking

import (
	"errors"
	"slices"
	"strings"
)

var errMissingAccount = errors.New("missing account")

// Config holds tracking configuration for a single account.
type Config struct {
	Enabled                   bool   `json:"enabled"`
	WorkerURL                 string `json:"worker_url"`
	WorkerName                string `json:"worker_name,omitempty"`
	DatabaseName              string `json:"database_name,omitempty"`
	DatabaseID                string `json:"database_id,omitempty"`
	SecretsInKeyring          bool   `json:"secrets_in_keyring,omitempty"`
	TrackingKey               string `json:"tracking_key,omitempty"`
	TrackingKeyVersions       []int  `json:"tracking_key_versions,omitempty"`
	TrackingCurrentKeyVersion int    `json:"tracking_current_key_version,omitempty"`
	AdminKey                  string `json:"admin_key,omitempty"`
}

type fileConfig struct {
	Version   int                `json:"version,omitempty"`
	UpdatedAt string             `json:"updated_at,omitempty"`
	Accounts  map[string]*Config `json:"accounts,omitempty"`
}

// IsConfigured returns true if tracking is set up.
func (c *Config) IsConfigured() bool {
	return c.Enabled && c.WorkerURL != "" && c.TrackingKey != ""
}

func (c *Config) NeedsSecretStore() bool {
	return shouldLoadTrackingSecrets(c)
}

func hydrateConfig(account string, cfg *Config, secretStore *SecretStore) (*Config, error) {
	if shouldLoadTrackingSecrets(cfg) {
		if secretStore == nil {
			return nil, errNilSecretStore
		}

		trackingKey, adminKey, secretErr := secretStore.LoadSecrets(account)
		if secretErr != nil {
			return nil, secretErr
		}

		if strings.TrimSpace(trackingKey) != "" {
			cfg.TrackingKey = trackingKey
		}

		if strings.TrimSpace(adminKey) != "" {
			cfg.AdminKey = adminKey
		}

		if cfg.TrackingCurrentKeyVersion > 0 || len(cfg.TrackingKeyVersions) > 0 {
			versions := NormalizeTrackingKeyVersions(cfg.TrackingKeyVersions, cfg.TrackingCurrentKeyVersion)

			keys, currentVersion, keyErr := secretStore.LoadTrackingKeys(account, versions, cfg.TrackingCurrentKeyVersion)
			if keyErr != nil {
				return nil, keyErr
			}

			if strings.TrimSpace(keys[currentVersion]) != "" {
				cfg.TrackingKey = keys[currentVersion]
				cfg.TrackingCurrentKeyVersion = currentVersion
				cfg.TrackingKeyVersions = NormalizeTrackingKeyVersions(versions, currentVersion)
			}
		}
	}

	return cfg, nil
}

func NormalizeTrackingKeyVersions(versions []int, currentVersion int) []int {
	normalized := make([]int, 0, len(versions)+1)
	for _, version := range versions {
		if version > 0 && version <= 255 {
			normalized = append(normalized, version)
		}
	}

	if currentVersion > 0 && currentVersion <= 255 {
		normalized = append(normalized, currentVersion)
	}

	slices.Sort(normalized)

	return slices.Compact(normalized)
}

func shouldLoadTrackingSecrets(cfg *Config) bool {
	if cfg == nil {
		return false
	}

	if cfg.SecretsInKeyring {
		return true
	}

	// Backward compat: if no SecretsInKeyring flag but keys are empty,
	// try keyring as fallback (legacy behavior).
	return strings.TrimSpace(cfg.TrackingKey) == "" && strings.TrimSpace(cfg.AdminKey) == ""
}

func normalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}
