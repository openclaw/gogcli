package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/99designs/keyring"
)

var errInvalidTestFilename = errors.New("invalid filename")

func TestFileSafeKeyRoundTrip(t *testing.T) {
	keys := []string{
		"token:default:user@example.com",
		`token:work:user@example.com`,
		`tracking/user@example.com/tracking_key`,
		`<>:"/\|?*%`,
		"",
	}

	for _, key := range keys {
		encoded := fileSafeKey(key)
		if encoded == key {
			t.Fatalf("expected encoded key for %q", key)
		}

		if strings.ContainsAny(encoded, `<>:"/\|?*`) {
			t.Fatalf("encoded key %q still contains a Windows filename separator/reserved char", encoded)
		}

		if got := decodeFileSafeKey(encoded); got != key {
			t.Fatalf("decodeFileSafeKey(%q)=%q, want %q", encoded, got, key)
		}
	}

	rawPrefixKey := fileKeyPrefix + "dGVzdA"
	if got := decodeFileSafeKey(rawPrefixKey); got != "test" {
		t.Fatalf("expected canonical encoded key to decode, got %q", got)
	}

	if got := decodeFileSafeKey(fileKeyPrefix + "not valid"); got != fileKeyPrefix+"not valid" {
		t.Fatalf("expected invalid encoded key to remain raw, got %q", got)
	}
}

func TestFileSafeKeyringRoundTripWithFileBackend(t *testing.T) {
	dir := t.TempDir()

	inner, err := keyring.Open(keyring.Config{
		ServiceName:      keyringServiceName(),
		AllowedBackends:  []keyring.BackendType{keyring.FileBackend},
		FileDir:          dir,
		FilePasswordFunc: keyring.FixedStringPrompt("test-pass"),
	})
	if err != nil {
		t.Fatalf("open file keyring: %v", err)
	}

	ring := newFileSafeKeyring(inner)
	key := "token:default:user@example.com"

	if setErr := ring.Set(keyring.Item{Key: key, Data: []byte("secret")}); setErr != nil {
		t.Fatalf("Set: %v", setErr)
	}

	item, err := ring.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if item.Key != key || string(item.Data) != "secret" {
		t.Fatalf("unexpected item: key=%q data=%q", item.Key, item.Data)
	}

	keys, err := ring.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}

	if !slices.Contains(keys, key) {
		t.Fatalf("expected decoded key %q in %v", key, keys)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected one keyring file, got %d", len(entries))
	}

	if name := entries[0].Name(); strings.ContainsAny(name, `<>:"/\|?*`) {
		t.Fatalf("keyring filename %q contains a Windows filename separator/reserved char", name)
	}

	if err := ring.Remove(key); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := ring.Get(key); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected not found after remove, got %v", err)
	}
}

func TestFileSafeKeyringReadsAndRemovesLegacyRawKeys(t *testing.T) {
	inner := keyring.NewArrayKeyring(nil)
	ring := newFileSafeKeyring(inner)
	key := "token:default:user@example.com"

	if err := inner.Set(keyring.Item{Key: key, Data: []byte("legacy")}); err != nil {
		t.Fatalf("set legacy key: %v", err)
	}

	item, err := ring.Get(key)
	if err != nil {
		t.Fatalf("Get legacy key: %v", err)
	}

	if item.Key != key || string(item.Data) != "legacy" {
		t.Fatalf("unexpected legacy item: key=%q data=%q", item.Key, item.Data)
	}

	if setErr := ring.Set(keyring.Item{Key: key, Data: []byte("new")}); setErr != nil {
		t.Fatalf("Set encoded key: %v", setErr)
	}

	keys, err := ring.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}

	got := 0

	for _, listedKey := range keys {
		if listedKey == key {
			got++
		}
	}

	if got != 1 {
		t.Fatalf("expected one decoded key in %v, got count %d", keys, got)
	}

	if err := ring.Remove(key); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := inner.Get(key); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected legacy key removed, got %v", err)
	}

	if _, err := inner.Get(fileSafeKey(key)); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected encoded key removed, got %v", err)
	}
}

func TestFileSafeKeyringTreatsInvalidLegacyFilenameAsNotFound(t *testing.T) {
	orig := isInvalidFileKeyError
	isInvalidFileKeyError = func(err error) bool { return errors.Is(err, errInvalidTestFilename) }

	t.Cleanup(func() { isInvalidFileKeyError = orig })

	ring := newFileSafeKeyring(&invalidFilenameKeyring{})
	if _, err := ring.Get("token:default:user@example.com"); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected not found for invalid legacy filename, got %v", err)
	}
}

func TestOpenKeyringWrapsExplicitFileBackend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "test-pass")

	origOpen := keyringOpenFunc
	keyringOpenFunc = func(_ keyring.Config) (keyring.Keyring, error) {
		return keyring.NewArrayKeyring(nil), nil
	}

	t.Cleanup(func() { keyringOpenFunc = origOpen })

	store, err := OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}

	keyringStore, ok := store.(*KeyringStore)
	if !ok {
		t.Fatalf("expected *KeyringStore, got %T", store)
	}

	if _, ok := keyringStore.ring.(*fileSafeKeyring); !ok {
		t.Fatalf("expected file-safe keyring, got %T", keyringStore.ring)
	}
}

type invalidFilenameKeyring struct{}

func (k *invalidFilenameKeyring) Get(string) (keyring.Item, error) {
	return keyring.Item{}, errInvalidTestFilename
}

func (k *invalidFilenameKeyring) GetMetadata(string) (keyring.Metadata, error) {
	return keyring.Metadata{}, errInvalidTestFilename
}

func (k *invalidFilenameKeyring) Set(keyring.Item) error {
	return nil
}

func (k *invalidFilenameKeyring) Remove(string) error {
	return errInvalidTestFilename
}

func (k *invalidFilenameKeyring) Keys() ([]string, error) {
	return nil, nil
}
