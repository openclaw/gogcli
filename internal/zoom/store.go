//nolint:wsl_v5
package zoom

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

const (
	defaultAlias         = "default"
	envAccountID         = "GOG_ZOOM_ACCOUNT_ID"
	envClientID          = "GOG_ZOOM_CLIENT_ID"
	envClientSecret      = "GOG_ZOOM_CLIENT_SECRET"        //nolint:gosec // env var name, not a credential value
	clientSecretKeyFmt   = "zoom-account/%s/client-secret" //nolint:gosec // keyring item name, not a secret value.
	accessTokenKeyFmt    = "zoom-account/%s/access-token"  //nolint:gosec // keyring item name, not a secret value.
	metadataFileMode     = 0o600
	metadataDirMode      = 0o700
	metadataDirComponent = "zoom"
)

var (
	errZoomConfigDirRequired = errors.New("zoom config directory is required")
	errZoomSecretStoreNil    = errors.New("zoom secret store is nil")
	errZoomEnvLookupNil      = errors.New("zoom environment lookup is nil")
)

type Metadata struct {
	AccountID string   `json:"account_id"`
	ClientID  string   `json:"client_id"`
	Alias     string   `json:"alias,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
}

type CachedToken struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type (
	EnvLookup          func(string) (string, bool)
	SecretStoreFactory func() (secrets.SecretStore, error)
)

type TokenStore interface {
	LoadCachedToken(alias string) (CachedToken, error)
	StoreCachedToken(alias string, tok CachedToken) error
}

type Store struct {
	metadataDir string
	lookupEnv   EnvLookup
	openSecrets SecretStoreFactory
	secretOnce  sync.Once
	secrets     secrets.SecretStore
	secretErr   error
}

func NewStore(layout config.Layout, openSecrets SecretStoreFactory, lookupEnv EnvLookup) (*Store, error) {
	configDir := strings.TrimSpace(layout.ConfigDir)
	if configDir == "" || !filepath.IsAbs(configDir) {
		return nil, fmt.Errorf("%w: %s", errZoomConfigDirRequired, configDir)
	}
	if openSecrets == nil {
		return nil, errZoomSecretStoreNil
	}
	if lookupEnv == nil {
		return nil, errZoomEnvLookupNil
	}

	return &Store{
		metadataDir: filepath.Join(configDir, metadataDirComponent),
		lookupEnv:   lookupEnv,
		openSecrets: openSecrets,
	}, nil
}

func NormalizeAlias(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return defaultAlias
	}
	return alias
}

func (s *Store) EnvClientSecretSet() bool {
	if s == nil || s.lookupEnv == nil {
		return false
	}
	_, ok := s.lookupEnv(envClientSecret)
	return ok
}

func (s *Store) LoadCredentials(alias string) (Credentials, error) {
	if s == nil {
		return Credentials{}, ErrCredentialsNotFound
	}
	if creds, ok := s.credentialsFromEnv(); ok {
		return creds, nil
	}

	alias = storageAlias(alias)
	meta, err := s.LoadMetadata(alias)
	if err != nil {
		return Credentials{}, ErrCredentialsNotFound
	}
	secretStore, err := s.secretStore()
	if err != nil {
		return Credentials{}, ErrCredentialsNotFound
	}
	secret, err := secretStore.GetSecret(clientSecretKey(alias))
	if err != nil {
		return Credentials{}, ErrCredentialsNotFound
	}
	creds := Credentials{
		AccountID:    strings.TrimSpace(meta.AccountID),
		ClientID:     strings.TrimSpace(meta.ClientID),
		ClientSecret: strings.TrimSpace(string(secret)),
	}
	if creds.AccountID == "" || creds.ClientID == "" || creds.ClientSecret == "" {
		return Credentials{}, ErrCredentialsNotFound
	}
	return creds, nil
}

func (s *Store) credentialsFromEnv() (Credentials, bool) {
	creds := Credentials{
		AccountID:    strings.TrimSpace(s.env(envAccountID)),
		ClientID:     strings.TrimSpace(s.env(envClientID)),
		ClientSecret: strings.TrimSpace(s.env(envClientSecret)),
	}
	if creds.AccountID == "" && creds.ClientID == "" && creds.ClientSecret == "" {
		return Credentials{}, false
	}
	if creds.AccountID == "" || creds.ClientID == "" || creds.ClientSecret == "" {
		return Credentials{}, false
	}
	return creds, true
}

func (s *Store) StoreCredentials(alias string, metadata Metadata, clientSecret string) error {
	if s == nil {
		return ErrCredentialsNotFound
	}
	alias = storageAlias(alias)
	metadata.Alias = alias
	if strings.TrimSpace(metadata.AccountID) == "" || strings.TrimSpace(metadata.ClientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return ErrCredentialsNotFound
	}
	if err := s.WriteMetadata(alias, metadata); err != nil {
		return err
	}
	secretStore, err := s.secretStore()
	if err != nil {
		return err
	}
	if err := secretStore.SetSecret(clientSecretKey(alias), []byte(clientSecret)); err != nil {
		return fmt.Errorf("store zoom client secret: %w", err)
	}
	return nil
}

func (s *Store) LoadMetadata(alias string) (Metadata, error) {
	path, err := s.metadataPath(alias)
	if err != nil {
		return Metadata{}, err
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is inside the injected gogcli config dir.
	if err != nil {
		return Metadata{}, fmt.Errorf("read zoom metadata: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(b, &meta); err != nil {
		return Metadata{}, fmt.Errorf("decode zoom metadata: %w", err)
	}
	return meta, nil
}

func (s *Store) WriteMetadata(alias string, metadata Metadata) error {
	path, err := s.metadataPath(alias)
	if err != nil {
		return err
	}
	if mkdirErr := os.MkdirAll(s.metadataDir, metadataDirMode); mkdirErr != nil {
		return fmt.Errorf("ensure zoom config dir: %w", mkdirErr)
	}
	b, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode zoom metadata: %w", err)
	}
	if err := config.WriteFileAtomic(path, append(b, '\n'), metadataFileMode); err != nil {
		return fmt.Errorf("write zoom metadata: %w", err)
	}
	return nil
}

func (s *Store) LoadCachedToken(alias string) (CachedToken, error) {
	if s == nil {
		return CachedToken{}, errZoomSecretStoreNil
	}
	secretStore, err := s.secretStore()
	if err != nil {
		return CachedToken{}, err
	}
	b, err := secretStore.GetSecret(accessTokenKey(storageAlias(alias)))
	if err != nil {
		return CachedToken{}, fmt.Errorf("read zoom cached token: %w", err)
	}
	var tok CachedToken
	if err := json.Unmarshal(b, &tok); err != nil {
		return CachedToken{}, fmt.Errorf("decode zoom cached token: %w", err)
	}
	return tok, nil
}

func (s *Store) StoreCachedToken(alias string, tok CachedToken) error {
	if s == nil {
		return errZoomSecretStoreNil
	}
	b, err := json.Marshal(tok) //nolint:gosec // Token cache is intentionally stored in the keyring.
	if err != nil {
		return fmt.Errorf("encode zoom cached token: %w", err)
	}
	secretStore, err := s.secretStore()
	if err != nil {
		return err
	}
	if err := secretStore.SetSecret(accessTokenKey(storageAlias(alias)), b); err != nil {
		return fmt.Errorf("store zoom cached token: %w", err)
	}
	return nil
}

func (s *Store) CachedTokenExpiry(alias string) (time.Time, bool) {
	tok, err := s.LoadCachedToken(alias)
	if err != nil {
		return time.Time{}, false
	}
	return tok.ExpiresAt, !tok.ExpiresAt.IsZero()
}

func (s *Store) metadataPath(alias string) (string, error) {
	if s == nil || strings.TrimSpace(s.metadataDir) == "" {
		return "", errZoomConfigDirRequired
	}
	alias = storageAlias(alias)
	return filepath.Join(s.metadataDir, alias+".json"), nil
}

func (s *Store) env(key string) string {
	if s == nil || s.lookupEnv == nil {
		return ""
	}
	value, _ := s.lookupEnv(key)
	return value
}

func (s *Store) secretStore() (secrets.SecretStore, error) {
	if s == nil || s.openSecrets == nil {
		return nil, errZoomSecretStoreNil
	}
	s.secretOnce.Do(func() {
		s.secrets, s.secretErr = s.openSecrets()
		if s.secretErr == nil && s.secrets == nil {
			s.secretErr = errZoomSecretStoreNil
		}
	})
	return s.secrets, s.secretErr
}

func clientSecretKey(alias string) string {
	return fmt.Sprintf(clientSecretKeyFmt, storageAlias(alias))
}

func accessTokenKey(alias string) string {
	return fmt.Sprintf(accessTokenKeyFmt, storageAlias(alias))
}

func storageAlias(alias string) string {
	return strings.NewReplacer("/", "_", "\\", "_").Replace(NormalizeAlias(alias))
}
