package googleapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/config"
)

type keepTestSecrets map[string][]byte

func (m keepTestSecrets) SetSecret(key string, value []byte) error {
	m[key] = append([]byte(nil), value...)
	return nil
}

func (m keepTestSecrets) GetSecret(key string) ([]byte, error) {
	value, ok := m[key]
	if !ok {
		return nil, keyring.ErrKeyNotFound
	}

	return append([]byte(nil), value...), nil
}

func (m keepTestSecrets) DeleteSecret(key string) error {
	delete(m, key)
	return nil
}

func TestNewKeepWithServiceAccountResolvesKeyringReference(t *testing.T) {
	root := t.TempDir()
	layout := config.Layout{ConfigDir: filepath.Join(root, "config"), DataDir: filepath.Join(root, "data")}
	secrets := keepTestSecrets{}
	store := config.NewServiceAccountStore(layout).WithSecrets(func() (config.ServiceAccountSecrets, error) { return secrets, nil })
	key := []byte(`{"type":"service_account","client_email":"svc@example.com","private_key":"FAKE-PRIVATE-KEY"}`)

	path, err := store.WriteKeyring("admin@example.com", key)
	if err != nil {
		t.Fatalf("WriteKeyring: %v", err)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(onDisk) == string(key) {
		t.Fatal("expected the file to hold only a reference")
	}

	ctx := WithAuthDependencies(context.Background(), AuthDependencies{
		ServiceAccounts: func() (*config.ServiceAccountStore, error) { return store, nil },
	})

	var gotKey []byte
	tokenSourceFactory := func(_ context.Context, keyJSON []byte, _ string, _ []string) (oauth2.TokenSource, error) {
		gotKey = keyJSON
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"}), nil
	}

	if _, err := newKeepWithServiceAccount(ctx, path, "admin@example.com", tokenSourceFactory); err != nil {
		t.Fatalf("newKeepWithServiceAccount: %v", err)
	}

	if string(gotKey) != string(key) {
		t.Fatal("expected Keep to get the key from the keyring, not the reference")
	}
}
