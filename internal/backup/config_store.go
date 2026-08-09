//nolint:wrapcheck,wsl_v5 // Backup config errors are surfaced directly to the CLI.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	appconfig "github.com/openclaw/gogcli/internal/config"
)

var (
	errConfigStoreRequired  = errors.New("backup config store is required")
	errConfigDirRequired    = errors.New("backup config directory is required")
	errHomeResolverRequired = errors.New("backup home resolver is required")
	errUserHomeRequired     = errors.New("backup user home is required")
)

type ConfigStore struct {
	defaultPath string
	allowLegacy bool
	resolveHome func() (string, error)
	homeOnce    sync.Once
	userHome    string
	homeErr     error
}

type configDocument struct {
	config         Config
	remoteExplicit bool
}

func NewConfigStore(layout appconfig.Layout, resolveHome func() (string, error)) (*ConfigStore, error) {
	configDir := strings.TrimSpace(layout.ConfigDir)
	if configDir == "" {
		return nil, errConfigDirRequired
	}
	if !filepath.IsAbs(configDir) {
		return nil, fmt.Errorf("%w: %s", errConfigDirRequired, configDir)
	}
	if resolveHome == nil {
		return nil, errHomeResolverRequired
	}

	return &ConfigStore{
		defaultPath: filepath.Join(configDir, "backup.json"),
		allowLegacy: !layout.ExplicitConfig,
		resolveHome: resolveHome,
	}, nil
}

func (s *ConfigStore) Path() string {
	if s == nil {
		return ""
	}
	return s.defaultPath
}

func (s *ConfigStore) Load(path string) (Config, error) {
	doc, err := s.loadDocument(path)
	if err != nil {
		return Config{}, err
	}
	return doc.config, nil
}

func (s *ConfigStore) Save(path string, cfg Config) error {
	resolved, _, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	if resolved == "" {
		return errConfigStoreRequired
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(resolved), 0o700); mkdirErr != nil {
		return mkdirErr
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return appconfig.WriteFileAtomic(resolved, append(data, '\n'), 0o600)
}

func (s *ConfigStore) ResolveOptions(opts Options) (Config, error) {
	doc, err := s.loadDocument(opts.ConfigPath)
	if err != nil {
		return Config{}, err
	}
	cfg := doc.config
	if strings.TrimSpace(opts.Repo) != "" {
		cfg.Repo = opts.Repo
	}
	if strings.TrimSpace(opts.Remote) != "" {
		cfg.Remote = opts.Remote
	}
	if opts.SuppressDefaultRemote && !doc.remoteExplicit && cfg.Remote == defaultRemote {
		cfg.Remote = ""
	}
	if strings.TrimSpace(opts.Identity) != "" {
		cfg.Identity = opts.Identity
	}
	if len(opts.Recipients) > 0 {
		cfg.Recipients = opts.Recipients
	}
	cfg.Repo, err = s.expandHome(cfg.Repo)
	if err != nil {
		return Config{}, err
	}
	cfg.Identity, err = s.expandHome(cfg.Identity)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ResolveOptions(opts Options) (Config, error) {
	if opts.ConfigStore == nil {
		return Config{}, errConfigStoreRequired
	}
	return opts.ConfigStore.ResolveOptions(opts)
}

func (s *ConfigStore) loadDocument(path string) (configDocument, error) {
	resolved, useDefault, err := s.resolvePath(path)
	if err != nil {
		return configDocument{}, err
	}
	if resolved == "" {
		return configDocument{}, errConfigStoreRequired
	}

	data, err := os.ReadFile(resolved) //nolint:gosec // caller-selected or injected config path.
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return configDocument{}, err
		}
		if !useDefault || !s.allowLegacy {
			return configDocument{config: DefaultConfig()}, nil
		}
		legacyPath, pathErr := s.legacyPath()
		if pathErr != nil {
			return configDocument{}, pathErr
		}
		data, err = os.ReadFile(legacyPath) //nolint:gosec // injected compatibility path.
		if errors.Is(err, os.ErrNotExist) {
			return configDocument{config: DefaultConfig()}, nil
		}
		if err != nil {
			return configDocument{}, err
		}
	}

	cfg := DefaultConfig()
	if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
		return configDocument{}, fmt.Errorf("read backup config: %w", unmarshalErr)
	}
	remoteExplicit, err := jsonObjectHasKey(data, "remote")
	if err != nil {
		return configDocument{}, err
	}
	return configDocument{config: cfg, remoteExplicit: remoteExplicit}, nil
}

func (s *ConfigStore) resolvePath(path string) (string, bool, error) {
	if s == nil {
		return "", false, nil
	}
	if strings.TrimSpace(path) == "" {
		return s.defaultPath, true, nil
	}
	resolved, err := s.expandHome(path)
	return resolved, false, err
}

func (s *ConfigStore) expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return path, nil
	}
	home, err := s.home()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimLeft(path[2:], `/\`)), nil
}

func (s *ConfigStore) legacyPath() (string, error) {
	home, err := s.home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gog", "backup.json"), nil
}

func (s *ConfigStore) home() (string, error) {
	if s == nil {
		return "", errConfigStoreRequired
	}
	s.homeOnce.Do(func() {
		s.userHome, s.homeErr = s.resolveHome()
		s.userHome = strings.TrimSpace(s.userHome)
		if s.homeErr == nil && s.userHome == "" {
			s.homeErr = errUserHomeRequired
		}
		if s.homeErr == nil && !filepath.IsAbs(s.userHome) {
			s.homeErr = fmt.Errorf("%w: %s", errUserHomeRequired, s.userHome)
		}
	})
	return s.userHome, s.homeErr
}

func jsonObjectHasKey(data []byte, key string) (bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false, fmt.Errorf("read backup config: %w", err)
	}
	_, ok := raw[key]
	return ok, nil
}
