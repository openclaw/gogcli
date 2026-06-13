//nolint:wsl_v5
package oauthclient

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

var (
	errTestDeleteSecret = errors.New("delete failed")
	errTestSetSecret    = errors.New("set failed")
)

func withSecretStore(t *testing.T) map[string][]byte {
	t.Helper()
	origSet := setSecret
	origGet := getSecret
	origDelete := deleteSecret
	secrets := make(map[string][]byte)
	t.Cleanup(func() {
		setSecret = origSet
		getSecret = origGet
		deleteSecret = origDelete
	})
	setSecret = func(key string, value []byte) error {
		secrets[key] = append([]byte(nil), value...)
		return nil
	}
	getSecret = func(key string) ([]byte, error) {
		value, ok := secrets[key]
		if !ok {
			return nil, keyring.ErrKeyNotFound
		}
		return append([]byte(nil), value...), nil
	}
	deleteSecret = func(key string) error {
		if _, ok := secrets[key]; !ok {
			return keyring.ErrKeyNotFound
		}
		delete(secrets, key)
		return nil
	}
	return secrets
}

func defaultCredentialsStoreForTest(t *testing.T) *CredentialsStore {
	t.Helper()
	store, err := defaultCredentialsStore()
	if err != nil {
		t.Fatalf("defaultCredentialsStore: %v", err)
	}
	return store
}

func TestWriteReadClientCredentials_KeyringSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	secrets := withSecretStore(t)

	if err := defaultCredentialsStoreForTest(t).Write("work", config.ClientCredentials{ClientID: "id", ClientSecret: "sec"}, false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	key, err := ClientSecretKey("work")
	if err != nil {
		t.Fatalf("ClientSecretKey: %v", err)
	}
	if string(secrets[key]) != "sec" {
		t.Fatalf("secret not stored in keyring map: %#v", secrets)
	}

	metadata, err := config.ReadClientCredentialsMetadataFor("work")
	if err != nil {
		t.Fatalf("ReadClientCredentialsMetadataFor: %v", err)
	}
	if metadata.ClientSecret != "" {
		t.Fatalf("metadata leaked client secret: %#v", metadata)
	}

	creds, err := ReadClientCredentialsFor("work")
	if err != nil {
		t.Fatalf("ReadClientCredentialsFor: %v", err)
	}
	if creds.ClientID != "id" || creds.ClientSecret != "sec" {
		t.Fatalf("unexpected credentials: %#v", creds)
	}
}

func TestReadClientCredentials_LegacyPlaintext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	withSecretStore(t)

	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "id", ClientSecret: "legacy-sec"}); err != nil {
		t.Fatalf("WriteClientCredentialsFor legacy: %v", err)
	}

	creds, err := ReadClientCredentialsFor("work")
	if err != nil {
		t.Fatalf("ReadClientCredentialsFor: %v", err)
	}
	if creds.ClientSecret != "legacy-sec" {
		t.Fatalf("unexpected legacy secret: %#v", creds)
	}
}

func TestWriteClientCredentials_KeyringFailurePreservesPlaintext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	withSecretStore(t)
	setSecret = func(string, []byte) error { return errTestSetSecret }

	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "old-id", ClientSecret: "old-sec"}); err != nil {
		t.Fatalf("WriteClientCredentialsFor legacy: %v", err)
	}
	if err := defaultCredentialsStoreForTest(t).Write("work", config.ClientCredentials{ClientID: "new-id", ClientSecret: "new-sec"}, false); err == nil {
		t.Fatalf("expected set secret error")
	}

	creds, err := config.ReadClientCredentialsFor("work")
	if err != nil {
		t.Fatalf("ReadClientCredentialsFor legacy: %v", err)
	}
	if creds.ClientID != "old-id" || creds.ClientSecret != "old-sec" {
		t.Fatalf("expected existing plaintext credentials preserved, got %#v", creds)
	}
}

func TestDeleteClientCredentials_DeletesMetadataAndSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	secrets := withSecretStore(t)

	store := defaultCredentialsStoreForTest(t)
	if err := store.Write("work", config.ClientCredentials{ClientID: "id", ClientSecret: "sec"}, false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := store.Delete("work"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected secret deleted: %#v", secrets)
	}
	if _, err := config.ReadClientCredentialsMetadataFor("work"); err == nil {
		t.Fatalf("expected metadata missing")
	}
}

func TestDeleteClientCredentials_PropagatesSecretDeleteError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	withSecretStore(t)
	deleteSecret = func(string) error { return errTestDeleteSecret }

	if err := config.WriteClientCredentialsMetadataFor("work", config.ClientCredentials{ClientID: "id"}); err != nil {
		t.Fatalf("WriteClientCredentialsMetadataFor: %v", err)
	}
	if err := defaultCredentialsStoreForTest(t).Delete("work"); err == nil {
		t.Fatalf("expected delete error")
	}
	if _, err := config.ReadClientCredentialsMetadataFor("work"); err != nil {
		t.Fatalf("expected metadata preserved after secret delete failure: %v", err)
	}
}

func TestDeleteClientCredentials_PlaintextDoesNotRequireKeyring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	withSecretStore(t)
	deleteSecret = func(string) error { return errTestDeleteSecret }

	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "id", ClientSecret: "legacy-sec"}); err != nil {
		t.Fatalf("WriteClientCredentialsFor legacy: %v", err)
	}
	if err := defaultCredentialsStoreForTest(t).Delete("work"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := config.ReadClientCredentialsMetadataFor("work"); err == nil {
		t.Fatalf("expected credentials file deleted")
	}
}
