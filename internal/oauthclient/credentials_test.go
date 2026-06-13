//nolint:wsl_v5
package oauthclient

import (
	"errors"
	"sync"
	"testing"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

var (
	errTestDeleteSecret = errors.New("delete failed")
	errTestSetSecret    = errors.New("set failed")
)

type memorySecretStore struct {
	mu        sync.Mutex
	values    map[string][]byte
	setErr    error
	deleteErr error
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{values: map[string][]byte{}}
}

func (s *memorySecretStore) SetSecret(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.setErr != nil {
		return s.setErr
	}

	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *memorySecretStore) GetSecret(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	value, ok := s.values[key]
	if !ok {
		return nil, keyring.ErrKeyNotFound
	}

	return append([]byte(nil), value...), nil
}

func (s *memorySecretStore) DeleteSecret(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.deleteErr != nil {
		return s.deleteErr
	}
	if _, ok := s.values[key]; !ok {
		return keyring.ErrKeyNotFound
	}

	delete(s.values, key)
	return nil
}

func newCredentialsStoreForTest(t *testing.T) (*CredentialsStore, *config.ClientCredentialsStore, *memorySecretStore) {
	t.Helper()

	root := t.TempDir()
	files := config.NewClientCredentialsStore(config.Layout{
		ConfigDir:      root,
		DataDir:        root,
		ExplicitConfig: true,
		ExplicitData:   true,
	})
	secretStore := newMemorySecretStore()
	store, err := NewCredentialsStore(files, secretStore)
	if err != nil {
		t.Fatalf("NewCredentialsStore: %v", err)
	}

	return store, files, secretStore
}

func TestNewCredentialsStoreRequiresDependencies(t *testing.T) {
	secretStore := newMemorySecretStore()
	if _, err := NewCredentialsStore(nil, secretStore); !errors.Is(err, errNilCredentialFiles) {
		t.Fatalf("nil files error = %v", err)
	}

	files := config.NewClientCredentialsStore(config.Layout{})
	if _, err := NewCredentialsStore(files, nil); !errors.Is(err, errNilSecretStore) {
		t.Fatalf("nil secret store error = %v", err)
	}
}

func TestCredentialsStoreWriteReadKeyringSecret(t *testing.T) {
	store, files, secretStore := newCredentialsStoreForTest(t)

	if err := store.Write("work", config.ClientCredentials{ClientID: "id", ClientSecret: "sec"}, false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	key, err := ClientSecretKey("work")
	if err != nil {
		t.Fatalf("ClientSecretKey: %v", err)
	}
	if string(secretStore.values[key]) != "sec" {
		t.Fatalf("secret not stored: %#v", secretStore.values)
	}

	metadata, err := files.ReadMetadata("work")
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if metadata.ClientSecret != "" {
		t.Fatalf("metadata leaked client secret: %#v", metadata)
	}

	creds, err := store.Read("work")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if creds.ClientID != "id" || creds.ClientSecret != "sec" {
		t.Fatalf("unexpected credentials: %#v", creds)
	}
}

func TestCredentialsStoreReadLegacyPlaintext(t *testing.T) {
	store, files, _ := newCredentialsStoreForTest(t)

	if err := files.Write("work", config.ClientCredentials{ClientID: "id", ClientSecret: "legacy-sec"}); err != nil {
		t.Fatalf("Write legacy: %v", err)
	}

	creds, err := store.Read("work")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if creds.ClientSecret != "legacy-sec" {
		t.Fatalf("unexpected legacy secret: %#v", creds)
	}
}

func TestCredentialsStoreKeyringFailurePreservesPlaintext(t *testing.T) {
	store, files, secretStore := newCredentialsStoreForTest(t)

	if err := files.Write("work", config.ClientCredentials{ClientID: "old-id", ClientSecret: "old-sec"}); err != nil {
		t.Fatalf("Write legacy: %v", err)
	}
	secretStore.setErr = errTestSetSecret
	if err := store.Write("work", config.ClientCredentials{ClientID: "new-id", ClientSecret: "new-sec"}, false); err == nil {
		t.Fatalf("expected set secret error")
	}

	creds, err := files.ReadMetadata("work")
	if err != nil {
		t.Fatalf("ReadMetadata legacy: %v", err)
	}
	if creds.ClientID != "old-id" || creds.ClientSecret != "old-sec" {
		t.Fatalf("expected existing plaintext credentials preserved, got %#v", creds)
	}
}

func TestCredentialsStoreDeleteMetadataAndSecret(t *testing.T) {
	store, files, secretStore := newCredentialsStoreForTest(t)

	if err := store.Write("work", config.ClientCredentials{ClientID: "id", ClientSecret: "sec"}, false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := store.Delete("work"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(secretStore.values) != 0 {
		t.Fatalf("expected secret deleted: %#v", secretStore.values)
	}
	if _, err := files.ReadMetadata("work"); err == nil {
		t.Fatalf("expected metadata missing")
	}
}

func TestCredentialsStoreDeletePropagatesSecretError(t *testing.T) {
	store, files, secretStore := newCredentialsStoreForTest(t)

	if err := files.WriteMetadata("work", config.ClientCredentials{ClientID: "id"}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	secretStore.deleteErr = errTestDeleteSecret
	if err := store.Delete("work"); err == nil {
		t.Fatalf("expected delete error")
	}
	if _, err := files.ReadMetadata("work"); err != nil {
		t.Fatalf("expected metadata preserved after secret delete failure: %v", err)
	}
}

func TestCredentialsStoreDeletePlaintextDoesNotRequireKeyring(t *testing.T) {
	store, files, secretStore := newCredentialsStoreForTest(t)

	if err := files.Write("work", config.ClientCredentials{ClientID: "id", ClientSecret: "legacy-sec"}); err != nil {
		t.Fatalf("Write legacy: %v", err)
	}
	secretStore.deleteErr = errTestDeleteSecret
	if err := store.Delete("work"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := files.ReadMetadata("work"); err == nil {
		t.Fatalf("expected credentials file deleted")
	}
}
