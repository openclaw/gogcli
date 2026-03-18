package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yosuke-furukawa/json5/encoding/json5"
)

type File struct {
	KeyringBackend  string            `json:"keyring_backend,omitempty"`
	DefaultTimezone string            `json:"default_timezone,omitempty"`
	AccountAliases  map[string]string `json:"account_aliases,omitempty"`
	AccountClients  map[string]string `json:"account_clients,omitempty"`
	ClientDomains   map[string]string `json:"client_domains,omitempty"`
	CalendarAliases map[string]string `json:"calendar_aliases,omitempty"`
}

var errConfigLockTimeout = errors.New("acquire config lock timeout")

func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.json"), nil
}

func configLockPath() (string, error) {
	dir, err := EnsureDir()
	if err != nil {
		return "", fmt.Errorf("ensure config dir: %w", err)
	}

	return filepath.Join(dir, "config.lock"), nil
}

func acquireConfigLock() (func(), error) {
	path, err := configLockPath()
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(2 * time.Second)

	for {
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // lock path is computed inside the config dir
		if openErr == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()

			return func() { _ = os.Remove(path) }, nil
		}

		if !os.IsExist(openErr) {
			return nil, fmt.Errorf("acquire config lock: %w", openErr)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s", errConfigLockTimeout, path)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func WriteConfig(cfg File) error {
	_, err := EnsureDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config json: %w", err)
	}

	b = append(b, '\n')

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}

	return nil
}

func UpdateConfig(update func(*File) error) error {
	unlock, err := acquireConfigLock()
	if err != nil {
		return err
	}
	defer unlock()

	cfg, err := ReadConfig()
	if err != nil {
		return err
	}

	if err := update(&cfg); err != nil {
		return err
	}

	return WriteConfig(cfg)
}

func ConfigExists() (bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return false, err
	}

	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}

		return false, fmt.Errorf("stat config: %w", statErr)
	}

	return true, nil
}

func ReadConfig() (File, error) {
	path, err := ConfigPath()
	if err != nil {
		return File{}, err
	}

	b, err := os.ReadFile(path) //nolint:gosec // config file path
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}

		return File{}, fmt.Errorf("read config: %w", err)
	}

	var cfg File
	if err := json5.Unmarshal(b, &cfg); err != nil {
		return File{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}
