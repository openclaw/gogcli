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
	YoutubeAPIKey   string            `json:"youtube_api_key,omitempty"`
	PlacesAPIKey    string            `json:"places_api_key,omitempty"`
	AccountAliases  map[string]string `json:"account_aliases,omitempty"`
	AccountClients  map[string]string `json:"account_clients,omitempty"`
	ClientDomains   map[string]string `json:"client_domains,omitempty"`
	CalendarAliases map[string]string `json:"calendar_aliases,omitempty"`
	GmailNoSend     bool              `json:"gmail_no_send,omitempty"`
	NoSendAccounts  map[string]bool   `json:"no_send_accounts,omitempty"`
}

var errConfigLockTimeout = errors.New("acquire config lock timeout")

type ConfigStore struct {
	layout Layout
}

func NewConfigStore(layout Layout) *ConfigStore {
	return &ConfigStore{layout: layout}
}

func (s *ConfigStore) Layout() Layout {
	if s == nil {
		return Layout{}
	}

	return s.layout
}

func ConfigPath() (string, error) {
	store, err := defaultConfigStore()
	if err != nil {
		return "", err
	}

	return store.Path(), nil
}

func (s *ConfigStore) Path() string {
	return s.layout.ConfigPath()
}

func (s *ConfigStore) ensureDir() (string, error) {
	dir := s.layout.ConfigDir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure config dir: %w", err)
	}

	return dir, nil
}

func (s *ConfigStore) lockPath() (string, error) {
	dir, err := s.ensureDir()
	if err != nil {
		return "", fmt.Errorf("ensure config dir: %w", err)
	}

	return filepath.Join(dir, "config.lock"), nil
}

func (s *ConfigStore) acquireLock() (func(), error) {
	path, err := s.lockPath()
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
	store, err := defaultConfigStore()
	if err != nil {
		return err
	}

	return store.Write(cfg)
}

func (s *ConfigStore) Write(cfg File) error {
	unlock, err := s.acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	return s.write(cfg)
}

func (s *ConfigStore) write(cfg File) error {
	_, err := s.ensureDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path := s.Path()

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config json: %w", err)
	}

	b = append(b, '\n')

	if err := WriteFileAtomic(path, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func (s *ConfigStore) Update(update func(*File) error) error {
	unlock, err := s.acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	cfg, err := s.Read()
	if err != nil {
		return err
	}

	if err := update(&cfg); err != nil {
		return err
	}

	return s.write(cfg)
}

// WriteFileAtomic writes data to path via a same-directory temp file and rename.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false

	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	committed = true

	return nil
}

func (s *ConfigStore) Exists() (bool, error) {
	path := s.Path()

	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}

		return false, fmt.Errorf("stat config: %w", statErr)
	}

	return true, nil
}

func ReadConfig() (File, error) {
	store, err := defaultConfigStore()
	if err != nil {
		return File{}, err
	}

	return store.Read()
}

func (s *ConfigStore) Read() (File, error) {
	path := s.Path()

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

func defaultConfigStore() (*ConfigStore, error) {
	layout, err := currentLayoutFor(PathKindConfig)
	if err != nil {
		return nil, err
	}

	return NewConfigStore(layout), nil
}

func DefaultConfigStore() (*ConfigStore, error) {
	return defaultConfigStore()
}
