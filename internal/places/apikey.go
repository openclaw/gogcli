package places

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

const (
	EnvAPIKey   = "GOOGLE_PLACES_API_KEY" //nolint:gosec // env var name, not a credential
	secretKeyID = "places/api_key"
)

type APIKeySource string

const (
	APIKeySourceNone     APIKeySource = "none"
	APIKeySourceEnv      APIKeySource = "env"
	APIKeySourceKeychain APIKeySource = "keychain"
	APIKeySourceConfig   APIKeySource = "config"
)

type APIKeyState struct {
	Key    string
	Source APIKeySource
}

func LoadAPIKey() (APIKeyState, error) {
	if key := strings.TrimSpace(os.Getenv(EnvAPIKey)); key != "" {
		return APIKeyState{Key: key, Source: APIKeySourceEnv}, nil
	}

	val, err := secrets.GetSecret(secretKeyID)
	if err == nil {
		return APIKeyState{Key: string(val), Source: APIKeySourceKeychain}, nil
	}
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		return APIKeyState{}, fmt.Errorf("read places api key: %w", err)
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		return APIKeyState{}, fmt.Errorf("read places api key config: %w", err)
	}
	if key := strings.TrimSpace(cfg.PlacesAPIKey); key != "" {
		return APIKeyState{Key: key, Source: APIKeySourceConfig}, nil
	}

	return APIKeyState{Source: APIKeySourceNone}, nil
}

func SaveAPIKeyKeychain(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("empty places api key")
	}
	if err := secrets.SetSecret(secretKeyID, []byte(key)); err != nil {
		return fmt.Errorf("store places api key: %w", err)
	}
	return nil
}

func SaveAPIKeyConfig(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("empty places api key")
	}
	cfg, err := config.ReadConfig()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	cfg.PlacesAPIKey = key
	if err := config.WriteConfig(cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func ClearAPIKeyKeychain() error {
	if err := secrets.DeleteSecret(secretKeyID); err != nil {
		return fmt.Errorf("delete places api key: %w", err)
	}
	return nil
}

func ClearAPIKeyConfig() error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if cfg.PlacesAPIKey == "" {
		return nil
	}
	cfg.PlacesAPIKey = ""
	if err := config.WriteConfig(cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func MaskAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}
