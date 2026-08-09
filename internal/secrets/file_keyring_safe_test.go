package secrets

import (
	"errors"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/99designs/keyring"

	"github.com/openclaw/gogcli/internal/config"
)

var (
	errInvalidTestFilename = errors.New("invalid filename")
	errLegacyRemoveFailed  = errors.New("legacy remove failed")
)

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
		ServiceName:      config.AppName,
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

func TestFileSafeKeyringRemoveReportsLegacyDeleteError(t *testing.T) {
	key := "token:default:user@example.com"
	ring := newFileSafeKeyring(&legacyRemoveErrorKeyring{
		key: key,
		items: map[string]keyring.Item{
			fileSafeKey(key): {Key: fileSafeKey(key), Data: []byte("encoded")},
			key:              {Key: key, Data: []byte("legacy")},
		},
		err: errLegacyRemoveFailed,
	})

	err := ring.Remove(key)
	if !errors.Is(err, errLegacyRemoveFailed) {
		t.Fatalf("expected legacy remove error, got %v", err)
	}

	item, getErr := ring.Get(key)
	if getErr != nil {
		t.Fatalf("Get: %v", getErr)
	}

	if string(item.Data) != "legacy" {
		t.Fatalf("expected legacy item still readable, got %q", string(item.Data))
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
	layout := config.Layout{
		ConfigDir: t.TempDir(),
		DataDir:   t.TempDir(),
	}
	options := OpenOptions{
		Layout:      layout,
		Config:      config.NewConfigStore(layout),
		Backend:     "file",
		Password:    "test-pass",
		PasswordSet: true,
		GOOS:        runtime.GOOS,
		openKeyringFn: func(_ keyring.Config) (keyring.Keyring, error) {
			return keyring.NewArrayKeyring(nil), nil
		},
	}

	store, err := Open(options)
	if err != nil {
		t.Fatalf("Open: %v", err)
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

type legacyRemoveErrorKeyring struct {
	key   string
	items map[string]keyring.Item
	err   error
}

func (k *legacyRemoveErrorKeyring) Get(key string) (keyring.Item, error) {
	item, ok := k.items[key]
	if !ok {
		return keyring.Item{}, keyring.ErrKeyNotFound
	}

	return item, nil
}

func (k *legacyRemoveErrorKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	if _, ok := k.items[key]; !ok {
		return keyring.Metadata{}, keyring.ErrKeyNotFound
	}

	return keyring.Metadata{}, nil
}

func (k *legacyRemoveErrorKeyring) Set(item keyring.Item) error {
	k.items[item.Key] = item
	return nil
}

func (k *legacyRemoveErrorKeyring) Remove(key string) error {
	if key == k.key {
		return k.err
	}

	if _, ok := k.items[key]; !ok {
		return keyring.ErrKeyNotFound
	}

	delete(k.items, key)

	return nil
}

func (k *legacyRemoveErrorKeyring) Keys() ([]string, error) {
	keys := make([]string, 0, len(k.items))
	for key := range k.items {
		keys = append(keys, key)
	}

	return keys, nil
}
