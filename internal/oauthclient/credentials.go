//nolint:wsl_v5
package oauthclient

import (
	"errors"
	"fmt"
	"strings"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

const clientSecretKeyFmt = "client/%s/client-secret"

var (
	errEmptyClientSecret  = errors.New("OAuth client secret in keyring is empty")
	errNilCredentialFiles = errors.New("credential file store is nil")
	errNilSecretStore     = errors.New("secret store is nil")
)

type CredentialsStore struct {
	files   *config.ClientCredentialsStore
	secrets secrets.SecretStore
}

func NewCredentialsStore(files *config.ClientCredentialsStore, secretStore secrets.SecretStore) (*CredentialsStore, error) {
	if files == nil {
		return nil, errNilCredentialFiles
	}
	if secretStore == nil {
		return nil, errNilSecretStore
	}

	return &CredentialsStore{files: files, secrets: secretStore}, nil
}

func ClientSecretKey(client string) (string, error) {
	normalized, err := config.NormalizeClientNameOrDefault(client)
	if err != nil {
		return "", fmt.Errorf("normalize client: %w", err)
	}
	return fmt.Sprintf(clientSecretKeyFmt, normalized), nil
}

func (s *CredentialsStore) PathFor(client string) (string, error) {
	path, err := s.files.PathFor(client)
	if err != nil {
		return "", fmt.Errorf("resolve credentials path: %w", err)
	}
	return path, nil
}

func (s *CredentialsStore) Write(client string, creds config.ClientCredentials, insecure bool) error {
	normalized, err := config.NormalizeClientNameOrDefault(client)
	if err != nil {
		return fmt.Errorf("normalize client: %w", err)
	}
	if insecure {
		if writeErr := s.files.Write(normalized, creds); writeErr != nil {
			return fmt.Errorf("write legacy credentials: %w", writeErr)
		}
		key, keyErr := ClientSecretKey(normalized)
		if keyErr != nil {
			return keyErr
		}
		if deleteErr := s.secrets.DeleteSecret(key); deleteErr != nil && !errors.Is(deleteErr, keyring.ErrKeyNotFound) {
			return fmt.Errorf("delete OAuth client secret from keyring: %w", deleteErr)
		}
		return nil
	}
	key, err := ClientSecretKey(normalized)
	if err != nil {
		return err
	}
	if err := s.secrets.SetSecret(key, []byte(strings.TrimSpace(creds.ClientSecret))); err != nil {
		return fmt.Errorf("store OAuth client secret: %w", err)
	}
	if writeErr := s.files.WriteMetadata(normalized, creds); writeErr != nil {
		return fmt.Errorf("write credentials metadata: %w", writeErr)
	}
	return nil
}

func (s *CredentialsStore) Read(client string) (config.ClientCredentials, error) {
	normalized, err := config.NormalizeClientNameOrDefault(client)
	if err != nil {
		return config.ClientCredentials{}, fmt.Errorf("normalize client: %w", err)
	}
	creds, err := s.files.ReadMetadata(normalized)
	if err != nil {
		return config.ClientCredentials{}, fmt.Errorf("read credentials metadata: %w", err)
	}
	if strings.TrimSpace(creds.ClientSecret) != "" {
		return creds, nil
	}
	key, err := ClientSecretKey(normalized)
	if err != nil {
		return config.ClientCredentials{}, err
	}
	secret, err := s.secrets.GetSecret(key)
	if err != nil {
		return config.ClientCredentials{}, fmt.Errorf("read OAuth client secret from keyring: %w", err)
	}
	creds.ClientSecret = strings.TrimSpace(string(secret))
	if creds.ClientSecret == "" {
		return config.ClientCredentials{}, errEmptyClientSecret
	}
	return creds, nil
}

func (s *CredentialsStore) Delete(client string) error {
	normalized, err := config.NormalizeClientNameOrDefault(client)
	if err != nil {
		return fmt.Errorf("normalize client: %w", err)
	}
	key, keyErr := ClientSecretKey(normalized)
	if keyErr != nil {
		return keyErr
	}

	creds, readErr := s.files.ReadMetadata(normalized)
	hasPlaintextSecret := readErr == nil && strings.TrimSpace(creds.ClientSecret) != ""
	secretErr := s.secrets.DeleteSecret(key)
	if secretErr != nil {
		if !errors.Is(secretErr, keyring.ErrKeyNotFound) && !hasPlaintextSecret {
			return fmt.Errorf("delete OAuth client secret: %w", secretErr)
		}
	}

	fileErr := s.files.Delete(normalized)
	if fileErr != nil {
		return fmt.Errorf("delete credentials metadata: %w", fileErr)
	}
	return nil
}

func (s *CredentialsStore) ClientSecretInKeyring(client string) bool {
	key, err := ClientSecretKey(client)
	if err != nil {
		return false
	}
	secret, err := s.secrets.GetSecret(key)
	return err == nil && strings.TrimSpace(string(secret)) != ""
}
