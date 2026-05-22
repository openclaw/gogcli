//nolint:wrapcheck,wsl_v5 // Backup config errors are surfaced directly to the CLI.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/steipete/gogcli/internal/config"
)

const (
	defaultRemote = "https://github.com/steipete/backup-gog.git"
)

type Config struct {
	Repo       string   `json:"repo"`
	Remote     string   `json:"remote"`
	Identity   string   `json:"identity"`
	Recipients []string `json:"recipients"`
}

type Options struct {
	ConfigPath     string
	Repo           string
	Remote         string
	Identity       string
	Recipients     []string
	Push           bool
	SkipPull       bool
	AsyncPush      bool
	PushQueueLimit int
	Progress       func(format string, args ...any)
}

func DefaultConfig() Config {
	return Config{
		Repo:     "~/Projects/backup-gog",
		Remote:   defaultRemote,
		Identity: "~/.gog/age.key",
	}
}

func DefaultConfigPath() (string, error) {
	dir, err := appconfig.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "backup.json"), nil
}

func legacyDefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "backup.json"
	}
	return filepath.Join(home, ".gog", "backup.json")
}

func LoadConfig(path string) (Config, error) {
	useDefault := strings.TrimSpace(path) == ""
	if strings.TrimSpace(path) == "" {
		defaultPath, pathErr := DefaultConfigPath()
		if pathErr != nil {
			return Config{}, fmt.Errorf("resolve backup config path: %w", pathErr)
		}
		path = defaultPath
	}
	cfg := DefaultConfig()
	data, err := os.ReadFile(expandHome(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if useDefault && !appconfig.HasExplicitConfigOverride() {
				if legacyData, legacyErr := os.ReadFile(expandHome(legacyDefaultConfigPath())); legacyErr == nil {
					if unmarshalErr := json.Unmarshal(legacyData, &cfg); unmarshalErr != nil {
						return Config{}, fmt.Errorf("read backup config: %w", unmarshalErr)
					}
					return cfg, nil
				} else if !errors.Is(legacyErr, os.ErrNotExist) {
					return Config{}, legacyErr
				}
			}
			return cfg, nil
		}
		return Config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("read backup config: %w", err)
	}
	return cfg, nil
}

func SaveConfig(path string, cfg Config) error {
	if strings.TrimSpace(path) == "" {
		defaultPath, pathErr := DefaultConfigPath()
		if pathErr != nil {
			return fmt.Errorf("resolve backup config path: %w", pathErr)
		}
		path = defaultPath
	}
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return appconfig.WriteFileAtomic(path, data, 0o600)
}

func ResolveOptions(opts Options) (Config, error) {
	cfg, err := LoadConfig(opts.ConfigPath)
	if err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(opts.Repo) != "" {
		cfg.Repo = opts.Repo
	}
	if strings.TrimSpace(opts.Remote) != "" {
		cfg.Remote = opts.Remote
	}
	if strings.TrimSpace(opts.Identity) != "" {
		cfg.Identity = opts.Identity
	}
	if len(opts.Recipients) > 0 {
		cfg.Recipients = opts.Recipients
	}
	cfg.Repo = expandHome(cfg.Repo)
	cfg.Identity = expandHome(cfg.Identity)
	return cfg, nil
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if after, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, after)
		}
	}
	return path
}
