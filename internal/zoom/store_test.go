package zoom

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

type testSecretStore struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newTestSecretStore() *testSecretStore {
	return &testSecretStore{values: make(map[string][]byte)}
}

func (s *testSecretStore) SetSecret(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.values[key] = append([]byte(nil), value...)

	return nil
}

func (s *testSecretStore) GetSecret(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	value, ok := s.values[key]
	if !ok {
		return nil, keyring.ErrKeyNotFound
	}

	return append([]byte(nil), value...), nil
}

func (s *testSecretStore) DeleteSecret(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.values[key]; !ok {
		return keyring.ErrKeyNotFound
	}

	delete(s.values, key)

	return nil
}

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()

	configDir := t.TempDir()
	secretStore := newTestSecretStore()

	store, err := NewStore(config.Layout{
		ConfigDir:      configDir,
		ExplicitConfig: true,
	}, func() (secrets.SecretStore, error) {
		return secretStore, nil
	}, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	return store, configDir
}

func TestStoreCredentialsRoundTrip(t *testing.T) {
	store, configDir := newTestStore(t)

	err := store.StoreCredentials("work", Metadata{
		AccountID: "acct",
		ClientID:  "client",
		Scopes:    []string{"meeting:write"},
	}, "secret")
	if err != nil {
		t.Fatalf("StoreCredentials: %v", err)
	}

	creds, err := store.LoadCredentials("work")
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}

	if creds.AccountID != "acct" || creds.ClientID != "client" || creds.ClientSecret != "secret" {
		t.Fatalf("credentials = %#v", creds)
	}

	info, err := os.Stat(filepath.Join(configDir, "zoom", "work.json"))
	if err != nil {
		t.Fatalf("stat metadata: %v", err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm() != metadataFileMode {
		t.Fatalf("metadata mode = %v", info.Mode().Perm())
	}
}

func TestStoreEnvironmentCredentialsTakePrecedence(t *testing.T) {
	env := map[string]string{
		envAccountID:    "env-account",
		envClientID:     "env-client",
		envClientSecret: "env-secret",
	}
	var opens atomic.Int32

	store, err := NewStore(config.Layout{
		ConfigDir:      t.TempDir(),
		ExplicitConfig: true,
	}, func() (secrets.SecretStore, error) {
		opens.Add(1)
		return newTestSecretStore(), nil
	}, func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	creds, err := store.LoadCredentials("missing")
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}

	if creds.AccountID != "env-account" || creds.ClientID != "env-client" || creds.ClientSecret != "env-secret" {
		t.Fatalf("credentials = %#v", creds)
	}

	if !store.EnvClientSecretSet() {
		t.Fatal("expected env client secret")
	}

	if opens.Load() != 0 {
		t.Fatalf("secret store opened %d times", opens.Load())
	}
}

func TestStoreMetadataReadDoesNotCreateDirectory(t *testing.T) {
	store, configDir := newTestStore(t)
	if _, err := store.LoadMetadata("missing"); err == nil {
		t.Fatal("expected missing metadata")
	}

	if _, err := os.Stat(filepath.Join(configDir, "zoom")); !os.IsNotExist(err) {
		t.Fatalf("metadata read created directory: %v", err)
	}
}

func TestStoreSanitizesAliasPathSeparators(t *testing.T) {
	store, configDir := newTestStore(t)
	if err := store.StoreCredentials(`team/work\primary`, Metadata{AccountID: "acct", ClientID: "client"}, "secret"); err != nil {
		t.Fatalf("StoreCredentials: %v", err)
	}

	if _, err := os.Stat(filepath.Join(configDir, "zoom", "team_work_primary.json")); err != nil {
		t.Fatalf("sanitized metadata missing: %v", err)
	}

	creds, err := store.LoadCredentials(`team\work/primary`)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}

	if creds.ClientSecret != "secret" {
		t.Fatalf("client secret = %q", creds.ClientSecret)
	}
}

func TestStoreCachedTokenRoundTrip(t *testing.T) {
	store, _ := newTestStore(t)

	want := CachedToken{AccessToken: "token", ExpiresAt: time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)}
	if err := store.StoreCachedToken("work", want); err != nil {
		t.Fatalf("StoreCachedToken: %v", err)
	}

	got, err := store.LoadCachedToken("work")
	if err != nil {
		t.Fatalf("LoadCachedToken: %v", err)
	}

	if got != want {
		t.Fatalf("token = %#v, want %#v", got, want)
	}
}
