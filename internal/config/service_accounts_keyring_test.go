package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/99designs/keyring"
)

type memoryServiceAccountSecrets map[string][]byte

func (m memoryServiceAccountSecrets) SetSecret(key string, value []byte) error {
	m[key] = append([]byte(nil), value...)
	return nil
}

func (m memoryServiceAccountSecrets) GetSecret(key string) ([]byte, error) {
	value, ok := m[key]
	if !ok {
		return nil, keyring.ErrKeyNotFound
	}

	return append([]byte(nil), value...), nil
}

func (m memoryServiceAccountSecrets) DeleteSecret(key string) error {
	if _, ok := m[key]; !ok {
		return keyring.ErrKeyNotFound
	}

	delete(m, key)

	return nil
}

func TestServiceAccountStoreWriteKeyringKeepsTheKeyOutOfTheFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := Layout{ConfigDir: filepath.Join(root, "config"), DataDir: filepath.Join(root, "data")}
	secrets := memoryServiceAccountSecrets{}
	store := NewServiceAccountStore(layout).WithSecrets(func() (ServiceAccountSecrets, error) { return secrets, nil })
	key := []byte(`{"type":"service_account","client_email":"svc@example.com","private_key":"FAKE-PRIVATE-KEY"}`)

	// A plain copy left at the legacy path is removed.
	if err := os.MkdirAll(layout.ConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(layout.ServiceAccountLegacyPath("user@example.com"), key, 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	path, err := store.WriteKeyring("User@Example.com", key)
	if err != nil {
		t.Fatalf("WriteKeyring: %v", err)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if bytes.Contains(onDisk, []byte("FAKE-PRIVATE-KEY")) {
		t.Fatalf("service account file holds the key: %s", onDisk)
	}

	if _, statErr := os.Stat(layout.ServiceAccountLegacyPath("user@example.com")); !os.IsNotExist(statErr) {
		t.Fatalf("legacy plain copy still present: %v", statErr)
	}

	file, exists, err := store.Read("user@example.com", false)
	if err != nil || !exists || !bytes.Equal(file.Data, key) || file.Path != path {
		t.Fatalf("Read = %#v exists=%t err=%v", file, exists, err)
	}

	emails, err := store.ListEmails()
	if err != nil || len(emails) != 1 || emails[0] != "user@example.com" {
		t.Fatalf("ListEmails = %#v, %v", emails, err)
	}

	removed, err := store.Remove("user@example.com")
	if err != nil || !removed {
		t.Fatalf("Remove = %t, %v", removed, err)
	}

	if len(secrets) != 0 {
		t.Fatalf("keyring still holds %d key(s) after Remove", len(secrets))
	}
}

func TestServiceAccountStoreKeyringReferenceNeedsTheKeyring(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := Layout{DataDir: filepath.Join(root, "data"), ExplicitData: true}
	secrets := memoryServiceAccountSecrets{}

	if _, err := NewServiceAccountStore(layout).WriteKeyring("user@example.com", []byte(`{}`)); !errors.Is(err, errServiceAccountSecretsMissing) {
		t.Fatalf("WriteKeyring without a keyring: %v", err)
	}

	writer := NewServiceAccountStore(layout).WithSecrets(func() (ServiceAccountSecrets, error) { return secrets, nil })
	if _, err := writer.WriteKeyring("user@example.com", []byte(`{"type":"service_account"}`)); err != nil {
		t.Fatalf("WriteKeyring: %v", err)
	}

	if _, _, err := NewServiceAccountStore(layout).Read("user@example.com", false); !errors.Is(err, errServiceAccountSecretsMissing) {
		t.Fatalf("Read of a keyring reference without a keyring: %v", err)
	}
}

func TestServiceAccountStoreWriteRemovesKeyringCopy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := Layout{ConfigDir: filepath.Join(root, "config"), DataDir: filepath.Join(root, "data")}
	secrets := memoryServiceAccountSecrets{}
	store := NewServiceAccountStore(layout).WithSecrets(func() (ServiceAccountSecrets, error) { return secrets, nil })
	key := []byte(`{"type":"service_account","client_email":"svc@example.com","private_key":"FAKE-PRIVATE-KEY"}`)

	if _, err := store.WriteKeyring("user@example.com", key); err != nil {
		t.Fatalf("WriteKeyring: %v", err)
	}

	// Back to file storage: the keyring copy goes.
	if _, err := store.Write("user@example.com", key); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if len(secrets) != 0 {
		t.Fatalf("expected no keyring entry after returning to file storage, got %d", len(secrets))
	}

	if _, err := store.Remove("user@example.com"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if len(secrets) != 0 {
		t.Fatalf("expected no keyring entry after unset, got %d", len(secrets))
	}
}

func TestServiceAccountKeyringSeparatesLayouts(t *testing.T) {
	secrets := memoryServiceAccountSecrets{}

	stores := []*ServiceAccountStore{
		NewServiceAccountStore(Layout{DataDir: t.TempDir(), ExplicitData: true}).WithSecrets(func() (ServiceAccountSecrets, error) { return secrets, nil }),
		NewServiceAccountStore(Layout{DataDir: t.TempDir(), ExplicitData: true}).WithSecrets(func() (ServiceAccountSecrets, error) { return secrets, nil }),
	}
	for i, store := range stores {
		if _, err := store.WriteKeyring("user@example.com", []byte(fmt.Sprintf("key-%d", i))); err != nil {
			t.Fatal(err)
		}
	}

	for i, store := range stores {
		file, _, err := store.Read("user@example.com", false)
		if err != nil || string(file.Data) != fmt.Sprintf("key-%d", i) {
			t.Fatalf("layout %d key changed: %q %v", i, file.Data, err)
		}
	}

	if _, err := stores[0].Remove("user@example.com"); err != nil {
		t.Fatal(err)
	}

	file, _, err := stores[1].Read("user@example.com", false)
	if err != nil || string(file.Data) != "key-1" {
		t.Fatalf("removing one layout affected another: %q %v", file.Data, err)
	}
}

func TestServiceAccountKeyringFailedReferenceWriteKeepsOldKey(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions as non-root")
	}
	dir := t.TempDir()
	secrets := memoryServiceAccountSecrets{}
	store := NewServiceAccountStore(Layout{DataDir: dir, ExplicitData: true}).WithSecrets(func() (ServiceAccountSecrets, error) { return secrets, nil })

	path, err := store.WriteKeyring("user@example.com", []byte("old-key"))
	if err != nil {
		t.Fatal(err)
	}

	oldRef, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if chmodErr := os.Chmod(dir, 0o500); chmodErr != nil {
		t.Fatal(chmodErr)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, writeErr := store.WriteKeyring("user@example.com", []byte("new-key")); writeErr == nil {
		t.Fatal("expected failed reference write")
	}

	ref, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(ref, oldRef) {
		t.Fatalf("old reference changed: %v", err)
	}

	file, _, err := store.Read("user@example.com", false)
	if err != nil || string(file.Data) != "old-key" {
		t.Fatalf("failed write replaced active key: %q %v", file.Data, err)
	}

	if len(secrets) != 1 {
		t.Fatalf("failed write leaked %d keys", len(secrets))
	}
}
