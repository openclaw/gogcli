package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Key string

const (
	KeyTimezone             Key = "timezone"
	KeyKeyringBackend       Key = "keyring_backend"
	KeyOnePasswordAuth      Key = "onepassword_auth"
	KeyOnePasswordAccount   Key = "onepassword_account"
	KeyOnePasswordVault     Key = "onepassword_vault"
	KeyOnePasswordItemTitle Key = "onepassword_item_title"
	KeyOnePasswordTimeout   Key = "onepassword_timeout"
	KeyGmailNoSend          Key = "gmail_no_send"
	KeyYoutubeAPIKey        Key = "youtube_api_key"
	KeyPlacesAPIKey         Key = "places_api_key"
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
	KeyOnePasswordAuth,
	KeyOnePasswordAccount,
	KeyOnePasswordVault,
	KeyOnePasswordItemTitle,
	KeyOnePasswordTimeout,
	KeyGmailNoSend,
	KeyYoutubeAPIKey,
	KeyPlacesAPIKey,
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
	KeyOnePasswordAuth: {
		Key: KeyOnePasswordAuth,
		Get: func(cfg File) string {
			return cfg.OnePasswordAuth
		},
		Set: func(cfg *File, value string) error {
			cfg.OnePasswordAuth = value
			return nil
		},
		Unset: func(cfg *File) {
			cfg.OnePasswordAuth = ""
		},
		EmptyHint: func() string {
			return "(not set, using auto)"
		},
	},
	KeyOnePasswordAccount: {
		Key: KeyOnePasswordAccount,
		Get: func(cfg File) string {
			return cfg.OnePasswordAccount
		},
		Set: func(cfg *File, value string) error {
			cfg.OnePasswordAccount = value
			return nil
		},
		Unset: func(cfg *File) {
			cfg.OnePasswordAccount = ""
		},
		EmptyHint: func() string {
			return "(not set)"
		},
	},
	KeyOnePasswordVault: {
		Key: KeyOnePasswordVault,
		Get: func(cfg File) string {
			return cfg.OnePasswordVault
		},
		Set: func(cfg *File, value string) error {
			cfg.OnePasswordVault = value
			return nil
		},
		Unset: func(cfg *File) {
			cfg.OnePasswordVault = ""
		},
		EmptyHint: func() string {
			return "(not set)"
		},
	},
	KeyOnePasswordItemTitle: {
		Key: KeyOnePasswordItemTitle,
		Get: func(cfg File) string {
			return cfg.OnePasswordItemTitle
		},
		Set: func(cfg *File, value string) error {
			cfg.OnePasswordItemTitle = value
			return nil
		},
		Unset: func(cfg *File) {
			cfg.OnePasswordItemTitle = ""
		},
		EmptyHint: func() string {
			return "(not set, using gogcli-keyring)"
		},
	},
	KeyOnePasswordTimeout: {
		Key: KeyOnePasswordTimeout,
		Get: func(cfg File) string {
			return cfg.OnePasswordTimeout
		},
		Set: func(cfg *File, value string) error {
			timeout, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("%w: %q: %w (use a duration like 10s)", errInvalidOnePasswordTimeout, value, err)
			}

			if timeout <= 0 {
				return fmt.Errorf("%w: %q must be positive (use a duration like 10s)", errInvalidOnePasswordTimeout, value)
			}

			cfg.OnePasswordTimeout = value

			return nil
		},
		Unset: func(cfg *File) {
			cfg.OnePasswordTimeout = ""
		},
		EmptyHint: func() string {
			return "(not set, using 10s)"
		},
	},
	KeyGmailNoSend: {
		Key: KeyGmailNoSend,
		Get: func(cfg File) string {
			return boolConfigString(cfg.GmailNoSend)
		},
		Set: func(cfg *File, value string) error {
			parsed, err := parseConfigBool(value)
			if err != nil {
				return err
			}
			cfg.GmailNoSend = parsed

			return nil
		},
		Unset: func(cfg *File) {
			cfg.GmailNoSend = false
		},
		EmptyHint: func() string {
			return "false"
		},
	},
	KeyYoutubeAPIKey: {
		Key: KeyYoutubeAPIKey,
		Get: func(cfg File) string {
			if v := os.Getenv("GOG_YOUTUBE_API_KEY"); v != "" {
				return v
			}

			return cfg.YoutubeAPIKey
		},
		Set: func(cfg *File, value string) error {
			cfg.YoutubeAPIKey = value
			return nil
		},
		Unset: func(cfg *File) {
			cfg.YoutubeAPIKey = ""
		},
		EmptyHint: func() string {
			return "(not set; set for YouTube Data API: config set youtube_api_key KEY or GOG_YOUTUBE_API_KEY)"
		},
	},
	KeyPlacesAPIKey: {
		Key: KeyPlacesAPIKey,
		Get: func(cfg File) string {
			if v := os.Getenv("GOG_PLACES_API_KEY"); v != "" {
				return v
			}

			if v := os.Getenv("GOOGLE_PLACES_API_KEY"); v != "" {
				return v
			}

			return cfg.PlacesAPIKey
		},
		Set: func(cfg *File, value string) error {
			cfg.PlacesAPIKey = value
			return nil
		},
		Unset: func(cfg *File) {
			cfg.PlacesAPIKey = ""
		},
		EmptyHint: func() string {
			return "(not set; set for Places API: config set places_api_key KEY or GOG_PLACES_API_KEY)"
		},
	},
}

var (
	errUnknownConfigKey          = errors.New("unknown config key")
	errConfigKeyCannotSet        = errors.New("config key cannot be set")
	errConfigKeyCannotUnset      = errors.New("config key cannot be unset")
	errInvalidConfigBool         = errors.New("invalid boolean")
	errInvalidOnePasswordTimeout = errors.New("invalid onepassword_timeout")
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

func parseConfigBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %q (use true or false)", errInvalidConfigBool, value)
	}
}

func boolConfigString(value bool) string {
	if value {
		return "true"
	}

	return "false"
}
