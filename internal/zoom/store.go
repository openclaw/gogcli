//nolint:wsl_v5
package zoom

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
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

func NormalizeAlias(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return defaultAlias
	}
	return alias
}

func EnvClientSecretSet() bool {
	_, ok := os.LookupEnv(envClientSecret)
	return ok
}

func LoadCredentials(alias string) (Credentials, error) {
	if creds, ok := credentialsFromEnv(); ok {
		return creds, nil
	}
	alias = NormalizeAlias(alias)
	meta, err := LoadMetadata(alias)
	if err != nil {
		return Credentials{}, ErrCredentialsNotFound
	}
	secret, err := secrets.GetSecret(clientSecretKey(alias))
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

func credentialsFromEnv() (Credentials, bool) {
	creds := Credentials{
		AccountID:    strings.TrimSpace(os.Getenv(envAccountID)),
		ClientID:     strings.TrimSpace(os.Getenv(envClientID)),
		ClientSecret: strings.TrimSpace(os.Getenv(envClientSecret)),
	}
	if creds.AccountID == "" && creds.ClientID == "" && creds.ClientSecret == "" {
		return Credentials{}, false
	}
	if creds.AccountID == "" || creds.ClientID == "" || creds.ClientSecret == "" {
		return Credentials{}, false
	}
	return creds, true
}

func StoreCredentials(alias string, metadata Metadata, clientSecret string) error {
	alias = NormalizeAlias(alias)
	metadata.Alias = alias
	if strings.TrimSpace(metadata.AccountID) == "" || strings.TrimSpace(metadata.ClientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return ErrCredentialsNotFound
	}
	if err := WriteMetadata(alias, metadata); err != nil {
		return err
	}
	if err := secrets.SetSecret(clientSecretKey(alias), []byte(clientSecret)); err != nil {
		return fmt.Errorf("store zoom client secret: %w", err)
	}
	return nil
}

func LoadMetadata(alias string) (Metadata, error) {
	path, err := metadataPath(alias)
	if err != nil {
		return Metadata{}, err
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is inside gogcli config dir
	if err != nil {
		return Metadata{}, fmt.Errorf("read zoom metadata: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(b, &meta); err != nil {
		return Metadata{}, fmt.Errorf("decode zoom metadata: %w", err)
	}
	return meta, nil
}

func WriteMetadata(alias string, metadata Metadata) error {
	dir, err := metadataDir()
	if err != nil {
		return err
	}
	if mkdirErr := os.MkdirAll(dir, metadataDirMode); mkdirErr != nil {
		return fmt.Errorf("ensure zoom config dir: %w", mkdirErr)
	}
	b, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode zoom metadata: %w", err)
	}
	path, err := metadataPath(alias)
	if err != nil {
		return err
	}
	if err := config.WriteFileAtomic(path, append(b, '\n'), metadataFileMode); err != nil {
		return fmt.Errorf("write zoom metadata: %w", err)
	}
	return nil
}

func LoadCachedToken(alias string) (CachedToken, error) {
	b, err := secrets.GetSecret(accessTokenKey(NormalizeAlias(alias)))
	if err != nil {
		return CachedToken{}, fmt.Errorf("read zoom cached token: %w", err)
	}
	var tok CachedToken
	if err := json.Unmarshal(b, &tok); err != nil {
		return CachedToken{}, fmt.Errorf("decode zoom cached token: %w", err)
	}
	return tok, nil
}

func StoreCachedToken(alias string, tok CachedToken) error {
	b, err := json.Marshal(tok) //nolint:gosec // Token cache is intentionally stored in the keyring.
	if err != nil {
		return fmt.Errorf("encode zoom cached token: %w", err)
	}
	if err := secrets.SetSecret(accessTokenKey(NormalizeAlias(alias)), b); err != nil {
		return fmt.Errorf("store zoom cached token: %w", err)
	}
	return nil
}

func CachedTokenExpiry(alias string) (time.Time, bool) {
	tok, err := LoadCachedToken(alias)
	if err != nil {
		return time.Time{}, false
	}
	return tok.ExpiresAt, !tok.ExpiresAt.IsZero()
}

func metadataDir() (string, error) {
	dir, err := config.EnsureDir()
	if err != nil {
		return "", fmt.Errorf("ensure config dir: %w", err)
	}
	return filepath.Join(dir, metadataDirComponent), nil
}

func metadataPath(alias string) (string, error) {
	dir, err := metadataDir()
	if err != nil {
		return "", err
	}
	alias = strings.ReplaceAll(NormalizeAlias(alias), string(filepath.Separator), "_")
	return filepath.Join(dir, alias+".json"), nil
}

func clientSecretKey(alias string) string {
	return fmt.Sprintf(clientSecretKeyFmt, NormalizeAlias(alias))
}

func accessTokenKey(alias string) string {
	return fmt.Sprintf(accessTokenKeyFmt, NormalizeAlias(alias))
}
