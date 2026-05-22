package tracking

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

var (
	errMissingAccount            = errors.New("missing account")
	errTrackingConfigLockTimeout = errors.New("acquire tracking config lock timeout")
)

const trackingConfigVersion = 1

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

// ConfigPath returns the path to the tracking config file.
func ConfigPath() (string, error) {
	dir, err := config.StateDir()
	if err != nil {
		return "", fmt.Errorf("state dir: %w", err)
	}

	return filepath.Join(dir, "tracking.json"), nil
}

func previousConfigPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}

	return filepath.Join(dir, "tracking.json"), nil
}

func legacyConfigPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		if filepath.IsAbs(xdg) {
			return filepath.Join(xdg, "gog", "tracking.json"), nil
		}

		if usesXDGConfigDefault() {
			configDir, err := homeConfigDir()
			if err != nil {
				return "", err
			}

			return filepath.Join(configDir, "gog", "tracking.json"), nil
		}
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}

	if !filepath.IsAbs(configDir) {
		var homeErr error

		configDir, homeErr = homeConfigDir()
		if homeErr != nil {
			return "", homeErr
		}
	}

	return filepath.Join(configDir, "gog", "tracking.json"), nil
}

func usesXDGConfigDefault() bool {
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		return true
	default:
		return false
	}
}

func homeConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home config dir: %w", err)
	}

	return filepath.Join(home, ".config"), nil
}

func readConfigBytes(path string) ([]byte, bool, error) {
	// #nosec G304 -- path is derived from user config dir
	data, readErr := os.ReadFile(path)
	if readErr == nil {
		return data, true, nil
	}

	if !os.IsNotExist(readErr) {
		return nil, false, fmt.Errorf("read tracking config: %w", readErr)
	}

	if config.HasExplicitStateOverride() {
		return nil, false, nil
	}

	for _, fallbackPathFn := range []func() (string, error){previousConfigPath, legacyConfigPath} {
		legacyPath, legacyErr := fallbackPathFn()
		if legacyErr != nil {
			return nil, false, fmt.Errorf("legacy config path: %w", legacyErr)
		}

		if legacyPath == path {
			continue
		}

		// #nosec G304 -- path is derived from user config dir
		legacyData, legacyReadErr := os.ReadFile(legacyPath)
		if legacyReadErr == nil {
			return legacyData, true, nil
		}

		if !os.IsNotExist(legacyReadErr) {
			return nil, false, fmt.Errorf("read legacy tracking config: %w", legacyReadErr)
		}
	}

	return nil, false, nil
}

func trackingConfigLockPath() (string, error) {
	dir, err := config.EnsureStateDir()
	if err != nil {
		return "", fmt.Errorf("ensure state dir: %w", err)
	}

	return filepath.Join(dir, "tracking.lock"), nil
}

func acquireTrackingConfigLock() (func(), error) {
	path, err := trackingConfigLockPath()
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(2 * time.Second)

	for {
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // lock path is computed inside config dir
		if openErr == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()

			return func() { _ = os.Remove(path) }, nil
		}

		if !os.IsExist(openErr) && !isTransientTrackingConfigLockError(openErr) {
			return nil, fmt.Errorf("acquire tracking config lock: %w", openErr)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s", errTrackingConfigLockTimeout, path)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// LoadConfig loads tracking configuration from disk for the specified account.
func LoadConfig(account string) (*Config, error) {
	account = normalizeAccount(account)
	if account == "" {
		return nil, errMissingAccount
	}

	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, ok, err := readConfigBytes(path)
	if err != nil {
		return nil, err
	}

	if !ok {
		return &Config{Enabled: false}, nil
	}

	var fileCfg fileConfig
	if err := json.Unmarshal(data, &fileCfg); err == nil && len(fileCfg.Accounts) > 0 {
		cfg := fileCfg.Accounts[account]
		if cfg == nil {
			return &Config{Enabled: false}, nil
		}

		return hydrateConfig(account, cfg)
	}

	var legacy Config
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse tracking config: %w", err)
	}

	return hydrateConfig(account, &legacy)
}

// SaveConfig saves tracking configuration to disk for the specified account.
func SaveConfig(account string, cfg *Config) error {
	account = normalizeAccount(account)
	if account == "" {
		return errMissingAccount
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	unlock, err := acquireTrackingConfigLock()
	if err != nil {
		return err
	}
	defer unlock()

	fileCfg := fileConfig{Accounts: map[string]*Config{}}

	if data, ok, readErr := readConfigBytes(path); readErr != nil {
		return readErr
	} else if ok {
		if unmarshalErr := json.Unmarshal(data, &fileCfg); unmarshalErr != nil {
			return fmt.Errorf("parse tracking config: %w", unmarshalErr)
		}

		if fileCfg.Accounts == nil {
			fileCfg.Accounts = map[string]*Config{}
		}
	}

	toSave := *cfg
	if cfg.SecretsInKeyring {
		toSave.TrackingKey = ""
		toSave.AdminKey = ""
	}

	fileCfg.Accounts[account] = &toSave
	fileCfg.Version = trackingConfigVersion
	fileCfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	// Ensure directory exists
	if _, mkErr := config.EnsureStateDir(); mkErr != nil {
		return fmt.Errorf("ensure state dir: %w", mkErr)
	}

	data, err := json.MarshalIndent(fileCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tracking config: %w", err)
	}

	tmp := path + ".tmp"
	if writeErr := os.WriteFile(tmp, data, 0o600); writeErr != nil {
		return fmt.Errorf("write tracking config: %w", writeErr)
	}

	if renameErr := os.Rename(tmp, path); renameErr != nil {
		return fmt.Errorf("commit tracking config: %w", renameErr)
	}

	return nil
}

// IsConfigured returns true if tracking is set up.
func (c *Config) IsConfigured() bool {
	return c.Enabled && c.WorkerURL != "" && c.TrackingKey != ""
}

func hydrateConfig(account string, cfg *Config) (*Config, error) {
	if shouldLoadTrackingSecrets(cfg) {
		trackingKey, adminKey, secretErr := LoadSecrets(account)
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

			keys, currentVersion, keyErr := LoadTrackingKeys(account, versions, cfg.TrackingCurrentKeyVersion)
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
