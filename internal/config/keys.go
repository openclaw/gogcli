package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Key string

const (
	KeyTimezone       Key = "timezone"
	KeyKeyringBackend Key = "keyring_backend"
)

type KeySpec struct {
	Key       Key
	Get       func(File) string
	Set       func(*File, string) error
	Unset     func(*File)
	EmptyHint func() string
}

var keyOrder = []Key{
	KeyTimezone,
	KeyKeyringBackend,
}

var keySpecs = map[Key]KeySpec{
	KeyTimezone: {
		Key: KeyTimezone,
		Get: func(cfg File) string {
			return cfg.DefaultTimezone
		},
		Set: func(cfg *File, value string) error {
			if _, err := time.LoadLocation(value); err != nil {
				return fmt.Errorf("invalid timezone %q: %w (use IANA timezone names like America/New_York, UTC, Europe/London)", value, err)
			}
			cfg.DefaultTimezone = value
			return nil
		},
		Unset: func(cfg *File) {
			cfg.DefaultTimezone = ""
		},
		EmptyHint: func() string {
			return "(not set, using local: " + time.Local.String() + ")"
		},
	},
	KeyKeyringBackend: {
		Key: KeyKeyringBackend,
		Get: func(cfg File) string {
			return cfg.KeyringBackend
		},
		Set: func(cfg *File, value string) error {
			cfg.KeyringBackend = value
			return nil
		},
		Unset: func(cfg *File) {
			cfg.KeyringBackend = ""
		},
		EmptyHint: func() string {
			return "(not set, using auto)"
		},
	},
}

var (
	errUnknownConfigKey     = errors.New("unknown config key")
	errConfigKeyCannotSet   = errors.New("config key cannot be set")
	errConfigKeyCannotUnset = errors.New("config key cannot be unset")
)

func (k Key) String() string {
	return string(k)
}

func (k Key) Validate() error {
	if _, ok := keySpecs[k]; ok {
		return nil
	}

	return fmt.Errorf("%w: %s (valid keys: %s)", errUnknownConfigKey, k, strings.Join(KeyNames(), ", "))
}

func ParseKey(raw string) (Key, error) {
	key := Key(raw)
	if err := key.Validate(); err != nil {
		return "", err
	}

	return key, nil
}

func KeySpecFor(key Key) (KeySpec, error) {
	if err := key.Validate(); err != nil {
		return KeySpec{}, err
	}

	return keySpecs[key], nil
}

func KeyList() []Key {
	keys := make([]Key, len(keyOrder))
	copy(keys, keyOrder)

	return keys
}

func KeyNames() []string {
	names := make([]string, 0, len(keyOrder))
	for _, key := range keyOrder {
		names = append(names, key.String())
	}

	return names
}

func GetValue(cfg File, key Key) string {
	spec, ok := keySpecs[key]
	if !ok || spec.Get == nil {
		return ""
	}

	return spec.Get(cfg)
}

func SetValue(cfg *File, key Key, value string) error {
	if err := key.Validate(); err != nil {
		return err
	}

	if spec := keySpecs[key]; spec.Set != nil {
		return spec.Set(cfg, value)
	}

	return fmt.Errorf("%w: %s", errConfigKeyCannotSet, key)
}

func UnsetValue(cfg *File, key Key) error {
	if err := key.Validate(); err != nil {
		return err
	}

	if spec := keySpecs[key]; spec.Unset != nil {
		spec.Unset(cfg)
		return nil
	}

	return fmt.Errorf("%w: %s", errConfigKeyCannotUnset, key)
}
