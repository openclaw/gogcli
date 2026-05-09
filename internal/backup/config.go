//nolint:wrapcheck,wsl_v5 // Backup config errors are surfaced directly to the CLI.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "backup.json"
	}
	return filepath.Join(home, ".gog", "backup.json")
}

func LoadConfig(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultConfigPath()
	}
	cfg := DefaultConfig()
	data, err := os.ReadFile(expandHome(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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
		path = DefaultConfigPath()
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
	return os.WriteFile(path, data, 0o600)
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
