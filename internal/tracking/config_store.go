package tracking

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/filelock"
)

const (
	trackingConfigVersion = 1
	trackingLockTimeout   = 2 * time.Second
)

var (
	errInvalidTrackingConfigPath = errors.New("invalid tracking config path")
	errMissingTrackingConfigPath = errors.New("missing tracking config path")
)

type ConfigStore struct {
	path         string
	previousPath string
	legacyPath   string
	allowLegacy  bool
	lock         *filelock.Lock
	secrets      *SecretStore
}

func NewConfigStore(layout config.Layout, legacyConfigBase string, secretStore *SecretStore) (*ConfigStore, error) {
	if !filepath.IsAbs(layout.StateDir) {
		return nil, fmt.Errorf("%w: state=%s", errInvalidTrackingConfigPath, layout.StateDir)
	}

	if !layout.ExplicitState && !filepath.IsAbs(layout.ConfigDir) {
		return nil, fmt.Errorf("%w: config=%s", errInvalidTrackingConfigPath, layout.ConfigDir)
	}

	store := &ConfigStore{
		path:        filepath.Join(layout.StateDir, "tracking.json"),
		allowLegacy: !layout.ExplicitState,
		lock:        filelock.Shared(filepath.Join(layout.StateDir, "tracking.lock"), trackingLockTimeout),
		secrets:     secretStore,
	}
	if store.allowLegacy {
		if !filepath.IsAbs(legacyConfigBase) {
			return nil, fmt.Errorf("%w: legacy_config_base=%s", errInvalidTrackingConfigPath, legacyConfigBase)
		}
		store.previousPath = filepath.Join(layout.ConfigDir, "tracking.json")
		store.legacyPath = filepath.Join(legacyConfigBase, "gog", "tracking.json")
	}

	return store, nil
}

func (s *ConfigStore) Path() string {
	if s == nil {
		return ""
	}

	return s.path
}

func (s *ConfigStore) Load(account string) (*Config, error) {
	return s.load(account, true)
}

func (s *ConfigStore) LoadMetadata(account string) (*Config, error) {
	return s.load(account, false)
}

func (s *ConfigStore) load(account string, hydrate bool) (*Config, error) {
	account = normalizeAccount(account)
	if account == "" {
		return nil, errMissingAccount
	}

	if s == nil || s.path == "" {
		return nil, errMissingTrackingConfigPath
	}

	data, ok, err := s.readConfigBytes()
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

		if !hydrate {
			return cfg, nil
		}

		return hydrateConfig(account, cfg, s.secrets)
	}

	var legacy Config
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse tracking config: %w", err)
	}

	if !hydrate {
		return &legacy, nil
	}

	return hydrateConfig(account, &legacy, s.secrets)
}

func (s *ConfigStore) Save(account string, cfg *Config) error {
	account = normalizeAccount(account)
	if account == "" {
		return errMissingAccount
	}

	if s == nil || s.path == "" {
		return errMissingTrackingConfigPath
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("ensure state dir: %w", err)
	}

	lockErr := s.lock.WithExclusive(func() error {
		fileCfg := fileConfig{Accounts: map[string]*Config{}}

		data, ok, readErr := s.readConfigBytes()
		if readErr != nil {
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

		data, err := json.MarshalIndent(fileCfg, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal tracking config: %w", err)
		}

		if err := config.WriteFileAtomic(s.path, append(data, '\n'), 0o600); err != nil {
			return fmt.Errorf("write tracking config: %w", err)
		}

		return nil
	})
	if lockErr != nil {
		return fmt.Errorf("lock tracking config: %w", lockErr)
	}

	return nil
}

func (s *ConfigStore) readConfigBytes() ([]byte, bool, error) {
	paths := []string{s.path}
	if s.allowLegacy {
		paths = append(paths, s.previousPath, s.legacyPath)
	}

	seen := make(map[string]struct{}, len(paths))
	for index, path := range paths {
		if path == "" {
			continue
		}

		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		data, err := os.ReadFile(path)
		if err == nil {
			return data, true, nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			label := "tracking config"
			if index > 0 {
				label = "legacy tracking config"
			}

			return nil, false, fmt.Errorf("read %s: %w", label, err)
		}
	}

	return nil, false, nil
}
