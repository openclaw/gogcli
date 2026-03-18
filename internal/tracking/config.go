package tracking

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

var errMissingAccount = errors.New("missing account")

const trackingConfigVersion = 1

// Config holds tracking configuration for a single account.
type Config struct {
	Enabled          bool   `json:"enabled"`
	WorkerURL        string `json:"worker_url"`
	WorkerName       string `json:"worker_name,omitempty"`
	DatabaseName     string `json:"database_name,omitempty"`
	DatabaseID       string `json:"database_id,omitempty"`
	SecretsInKeyring bool   `json:"secrets_in_keyring,omitempty"`
	TrackingKey      string `json:"tracking_key,omitempty"`
	AdminKey         string `json:"admin_key,omitempty"`
}

type fileConfig struct {
	Version   int                `json:"version,omitempty"`
	UpdatedAt string             `json:"updated_at,omitempty"`
	Accounts  map[string]*Config `json:"accounts,omitempty"`
}

// ConfigPath returns the path to the tracking config file.
func ConfigPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}

	return filepath.Join(dir, "tracking.json"), nil
}

func legacyConfigPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "gog", "tracking.json"), nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}

	return filepath.Join(configDir, "gog", "tracking.json"), nil
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

	legacyPath, legacyErr := legacyConfigPath()
	if legacyErr != nil {
		return nil, false, fmt.Errorf("legacy config path: %w", legacyErr)
	}

	// #nosec G304 -- path is derived from user config dir
	legacyData, legacyReadErr := os.ReadFile(legacyPath)
	if legacyReadErr == nil {
		return legacyData, true, nil
	}

	if os.IsNotExist(legacyReadErr) {
		return nil, false, nil
	}

	return nil, false, fmt.Errorf("read legacy tracking config: %w", legacyReadErr)
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

	fileCfg := fileConfig{Accounts: map[string]*Config{}}
	if data, ok, readErr := readConfigBytes(path); readErr == nil && ok {
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
	if _, mkErr := config.EnsureDir(); mkErr != nil {
		return fmt.Errorf("ensure config dir: %w", mkErr)
	}

	data, err := json.MarshalIndent(fileCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tracking config: %w", err)
	}

	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("write tracking config: %w", writeErr)
	}

	return nil
}

// IsConfigured returns true if tracking is set up.
func (c *Config) IsConfigured() bool {
	return c.Enabled && c.WorkerURL != "" && c.TrackingKey != ""
}

func hydrateConfig(account string, cfg *Config) (*Config, error) {
	if strings.TrimSpace(cfg.TrackingKey) == "" || strings.TrimSpace(cfg.AdminKey) == "" || cfg.SecretsInKeyring {
		trackingKey, adminKey, secretErr := LoadSecrets(account)
		if secretErr != nil {
			return nil, secretErr
		}

		if strings.TrimSpace(cfg.TrackingKey) == "" {
			cfg.TrackingKey = trackingKey
		}

		if strings.TrimSpace(cfg.AdminKey) == "" {
			cfg.AdminKey = adminKey
		}
	}

	return cfg, nil
}

func normalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}
