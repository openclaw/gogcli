package secrets

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/99designs/keyring"
)

func TestEncodeFilenameKey_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"primary token key", "token:default:user@gmail.com"},
		{"legacy single-colon", "token:user@gmail.com"},
		{"default account scoped", "default_account:default"},
		{"all illegal chars", `evil:<>"|?*key`},
		{"already contains percent", "weird%key:value"},
		{"plain ascii no encoding", "default_account"},
		{"empty string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := encodeFilenameKey(tc.raw)
			decoded := decodeFilenameKey(encoded)

			if decoded != tc.raw {
				t.Fatalf("round-trip mismatch: raw=%q encoded=%q decoded=%q", tc.raw, encoded, decoded)
			}
		})
	}
}

func TestEncodeFilenameKey_IsNoopForSafeKeys(t *testing.T) {
	for _, key := range []string{"default_account", "token", "abcdef", ""} {
		if got := encodeFilenameKey(key); got != key {
			t.Fatalf("safe key %q was rewritten to %q; expected unchanged", key, got)
		}
	}
}

func TestEncodeFilenameKey_StripsAllIllegalChars(t *testing.T) {
	raw := `token:default:foo|bar?baz*qux<x>y"z`
	encoded := encodeFilenameKey(raw)

	for _, ch := range []string{":", "<", ">", `"`, "|", "?", "*"} {
		if strings.Contains(encoded, ch) {
			t.Fatalf("encoded form %q still contains illegal char %q", encoded, ch)
		}
	}
}

func TestDecodeFilenameKey_PassesThroughLegacyNames(t *testing.T) {
	// Older gogcli versions wrote raw keys directly to disk on Linux/macOS. The
	// decoder must return them unchanged so existing keyring directories keep
	// working without manual migration.
	for _, name := range []string{"token:default:user@example.com", "default_account:default"} {
		if got := decodeFilenameKey(name); got != name {
			t.Fatalf("decode rewrote legacy raw filename %q to %q", name, got)
		}
	}
}

func TestNeedsFilenameEncoding(t *testing.T) {
	cases := map[string]bool{
		"":                               false,
		"default_account":                false,
		"token:user@example.com":         true,
		"token:default:user@example.com": true,
		`weird"key`:                      true,
		"contains%already":               true,
		`with\backslash`:                 true,
	}

	for input, want := range cases {
		if got := needsFilenameEncoding(input); got != want {
			t.Errorf("needsFilenameEncoding(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestEncodeFilenameKey_PrefixesEncodedKeysOnly(t *testing.T) {
	encoded := encodeFilenameKey("token:default:user@example.com")
	if !strings.HasPrefix(encoded, encodedKeyPrefix) {
		t.Errorf("expected encoded form to start with %q sentinel, got %q", encodedKeyPrefix, encoded)
	}

	plain := encodeFilenameKey("default_account")
	if strings.HasPrefix(plain, encodedKeyPrefix) {
		t.Errorf("plain key was prefixed with sentinel: %q", plain)
	}
}

func TestDecodeFilenameKey_PreservesLegacyPercentTriplets(t *testing.T) {
	// A pre-existing keyring item whose raw key happened to contain "%3A"
	// must round-trip unchanged, since older gogcli wrote no prefix marker.
	// Decoding the percent triplet would corrupt the key and stop the
	// caller from finding the item.
	legacy := "token:default:user%3Afoo@example.com"
	if got := decodeFilenameKey(legacy); got != legacy {
		t.Fatalf("decode rewrote legacy %%XX-bearing key %q to %q", legacy, got)
	}
}

func TestEncodeFilenameKey_ReservesSentinelPrefix(t *testing.T) {
	// A legitimate caller-supplied key that happens to start with
	// encodedKeyPrefix must round-trip cleanly. If it passed through
	// unchanged on Set, Keys() would mistakenly strip the prefix and corrupt
	// the key on read.
	raw := encodedKeyPrefix + "foo"

	encoded := encodeFilenameKey(raw)
	if encoded == raw {
		t.Fatalf("key starting with sentinel %q was not encoded", raw)
	}

	if !strings.HasPrefix(encoded, encodedKeyPrefix) {
		t.Fatalf("encoded form lost sentinel prefix: %q", encoded)
	}

	if got := decodeFilenameKey(encoded); got != raw {
		t.Fatalf("sentinel-collision round-trip failed: %q -> %q -> %q", raw, encoded, got)
	}
}

func TestEncodeFilenameKey_HandlesBackslash(t *testing.T) {
	raw := `weird\path:thing`

	encoded := encodeFilenameKey(raw)
	if strings.ContainsAny(encoded, `\:`) {
		t.Fatalf("encoded form %q still contains Windows-illegal chars", encoded)
	}

	decoded := decodeFilenameKey(encoded)
	if decoded != raw {
		t.Fatalf("backslash round-trip failed: %q -> %q -> %q", raw, encoded, decoded)
	}
}

// fileSafeKeyringFixture spins up a real 99designs/keyring file backend backed
// by a temp directory plus our wrapper. End-to-end tests use this fixture to
// catch interactions between encoding, the underlying percent-escaping in the
// upstream library, and on-disk filenames.
func fileSafeKeyringFixture(t *testing.T) (*fileSafeKeyring, string) {
	t.Helper()

	dir := t.TempDir()

	inner, err := keyring.Open(keyring.Config{
		ServiceName:      "gogcli-filename-safe-test",
		AllowedBackends:  []keyring.BackendType{keyring.FileBackend},
		FileDir:          dir,
		FilePasswordFunc: keyring.FixedStringPrompt("test-password"),
	})
	if err != nil {
		t.Fatalf("open file keyring: %v", err)
	}

	return newFileSafeKeyring(inner), dir
}

func TestFileSafeKeyring_SetGetRoundTrip(t *testing.T) {
	ring, dir := fileSafeKeyringFixture(t)

	key := "token:default:user@example.com"
	payload := []byte(`{"refresh_token":"abc"}`)

	if err := ring.Set(keyring.Item{Key: key, Data: payload, Label: "gogcli"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := ring.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Key != key {
		t.Errorf("returned key %q, want %q (wrapper must hide encoded form from callers)", got.Key, key)
	}

	if string(got.Data) != string(payload) {
		t.Errorf("payload mismatch: got %q want %q", got.Data, payload)
	}

	// On-disk filename must contain no NTFS-illegal chars (the whole point).
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	for _, e := range entries {
		for _, ch := range []string{":", "<", ">", `"`, "|", "?", "*"} {
			if strings.Contains(e.Name(), ch) {
				t.Errorf("on-disk filename %q contains NTFS-illegal char %q", e.Name(), ch)
			}
		}
	}
}

func TestFileSafeKeyring_KeysRoundTripsEncodedNames(t *testing.T) {
	ring, _ := fileSafeKeyringFixture(t)

	keys := []string{
		"token:default:user@example.com",
		"token:default:other@example.com",
		"default_account",
	}

	for _, k := range keys {
		if err := ring.Set(keyring.Item{Key: k, Data: []byte("x")}); err != nil {
			t.Fatalf("Set %q: %v", k, err)
		}
	}

	got, err := ring.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}

	sort.Strings(got)

	wanted := append([]string(nil), keys...)
	sort.Strings(wanted)

	if strings.Join(got, "|") != strings.Join(wanted, "|") {
		t.Fatalf("Keys() = %v, want %v", got, wanted)
	}
}

func TestFileSafeKeyring_GetFallsBackToLegacyRawFile(t *testing.T) {
	// Simulate a directory written by an older gogcli version that stored
	// keys with raw colons. The wrapper must still find them via the
	// fallback path so users do not have to re-authenticate.
	ring, dir := fileSafeKeyringFixture(t)

	legacyKey := "token:default:legacy@example.com"

	// Drive the underlying keyring directly with the raw key so the on-disk
	// filename matches the legacy format. (On NTFS this would already have
	// failed; this test runs on the host's filesystem which accepts colons.)
	if err := ring.inner.Set(keyring.Item{Key: legacyKey, Data: []byte("legacy-data")}); err != nil {
		t.Fatalf("seed legacy item: %v", err)
	}

	// Verify the fixture really wrote a raw-form file (otherwise the test
	// is vacuous).
	if _, err := os.Stat(filepath.Join(dir, legacyKey)); err != nil {
		t.Skipf("filesystem rejected legacy filename (likely Windows); fallback path is irrelevant here: %v", err)
	}

	got, err := ring.Get(legacyKey)
	if err != nil {
		t.Fatalf("Get legacy key: %v", err)
	}

	if string(got.Data) != "legacy-data" {
		t.Errorf("legacy fallback returned %q, want %q", got.Data, "legacy-data")
	}

	if got.Key != legacyKey {
		t.Errorf("returned key %q, want %q", got.Key, legacyKey)
	}
}

func TestFileSafeKeyring_SetMigratesLegacyFile(t *testing.T) {
	ring, dir := fileSafeKeyringFixture(t)

	key := "token:default:migrate@example.com"

	if err := ring.inner.Set(keyring.Item{Key: key, Data: []byte("old")}); err != nil {
		t.Fatalf("seed legacy item: %v", err)
	}

	legacyPath := filepath.Join(dir, key)
	if _, err := os.Stat(legacyPath); err != nil {
		t.Skipf("filesystem rejected legacy filename; migration test is irrelevant: %v", err)
	}

	if err := ring.Set(keyring.Item{Key: key, Data: []byte("new")}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("legacy file %s still present after Set; expected best-effort cleanup. err=%v", legacyPath, err)
	}

	got, err := ring.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if string(got.Data) != "new" {
		t.Errorf("post-migration payload = %q, want %q", got.Data, "new")
	}
}

func TestFileSafeKeyring_RemoveEncodedAndLegacy(t *testing.T) {
	ring, dir := fileSafeKeyringFixture(t)

	key := "token:default:rm@example.com"

	// Seed both a legacy raw file and a new encoded file for the same key,
	// to exercise dual cleanup.
	if err := ring.inner.Set(keyring.Item{Key: key, Data: []byte("legacy")}); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	if err := ring.inner.Set(keyring.Item{Key: encodeFilenameKey(key), Data: []byte("new")}); err != nil {
		t.Fatalf("seed encoded: %v", err)
	}

	if err := ring.Remove(key); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	for _, e := range entries {
		decoded := decodeFilenameKey(e.Name())
		if decoded == key {
			t.Errorf("file %q (decoded %q) still present after Remove", e.Name(), decoded)
		}
	}
}

func TestFileSafeKeyring_GetMetadataPrefersEncodedThenLegacy(t *testing.T) {
	ring, dir := fileSafeKeyringFixture(t)

	encodedKey := "token:default:meta@example.com"

	if err := ring.Set(keyring.Item{Key: encodedKey, Data: []byte("x")}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	md, err := ring.GetMetadata(encodedKey)
	if err != nil {
		t.Fatalf("GetMetadata encoded: %v", err)
	}

	if md.ModificationTime.IsZero() {
		t.Errorf("GetMetadata returned zero ModificationTime")
	}

	// Now seed a legacy-only key (raw filename, no wrapper Set) and confirm
	// GetMetadata reaches it via the fallback branch.
	legacyKey := "token:default:legacy-meta@example.com"
	if err := ring.inner.Set(keyring.Item{Key: legacyKey, Data: []byte("y")}); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, legacyKey)); err != nil {
		t.Skipf("filesystem rejected legacy filename; fallback path is irrelevant: %v", err)
	}

	if _, err := ring.GetMetadata(legacyKey); err != nil {
		t.Fatalf("GetMetadata legacy fallback: %v", err)
	}
}

func TestFileSafeKeyring_GetMetadataMissing(t *testing.T) {
	ring, _ := fileSafeKeyringFixture(t)

	_, err := ring.GetMetadata("token:default:does-not-exist@example.com")
	if !errors.Is(err, keyring.ErrKeyNotFound) && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("GetMetadata of missing key: got %v, want ErrKeyNotFound or fs.ErrNotExist", err)
	}
}

func TestFileSafeKeyring_RemoveMissingReturnsErrKeyNotFound(t *testing.T) {
	ring, _ := fileSafeKeyringFixture(t)

	err := ring.Remove("token:default:does-not-exist@example.com")
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("Remove of missing key: got %v, want ErrKeyNotFound", err)
	}
}

func TestFileSafeKeyring_GetMissingReturnsErrKeyNotFound(t *testing.T) {
	ring, _ := fileSafeKeyringFixture(t)

	_, err := ring.Get("token:default:does-not-exist@example.com")
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("Get of missing key: got %v, want ErrKeyNotFound", err)
	}
}

// errFakeInvalidName mimics Windows ERROR_INVALID_NAME so platform-behaviour
// tests can drive fileSafeKeyring's "raw fallback would fail" code paths
// without needing an actual NTFS volume.
var errFakeInvalidName = errors.New("fake: filename invalid for backing filesystem")

// windowsLikeIllegalRunes is the set of characters fakeInnerKeyring rejects,
// matching the NTFS-illegal set fileSafeKeyring percent-encodes.
const windowsLikeIllegalRunes = `:<>"|?*\`

// fakeInnerKeyring stands in for the 99designs/keyring file backend during
// platform-behaviour tests. Any key containing a rune in
// windowsLikeIllegalRunes returns errFakeInvalidName from Get / GetMetadata /
// Set / Remove, mimicking the way Windows surfaces ERROR_INVALID_NAME for
// NTFS-illegal filenames.
type fakeInnerKeyring struct {
	store             map[string][]byte
	removeAttemptKeys []string
}

func newFakeInnerKeyring() *fakeInnerKeyring {
	return &fakeInnerKeyring{store: map[string][]byte{}}
}

func (f *fakeInnerKeyring) wouldReject(key string) bool {
	return strings.ContainsAny(key, windowsLikeIllegalRunes)
}

func (f *fakeInnerKeyring) Get(key string) (keyring.Item, error) {
	if f.wouldReject(key) {
		return keyring.Item{}, errFakeInvalidName
	}

	data, ok := f.store[key]
	if !ok {
		return keyring.Item{}, keyring.ErrKeyNotFound
	}

	return keyring.Item{Key: key, Data: data}, nil
}

func (f *fakeInnerKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	if f.wouldReject(key) {
		return keyring.Metadata{}, errFakeInvalidName
	}

	if _, ok := f.store[key]; !ok {
		return keyring.Metadata{}, keyring.ErrKeyNotFound
	}

	return keyring.Metadata{}, nil
}

func (f *fakeInnerKeyring) Set(item keyring.Item) error {
	if f.wouldReject(item.Key) {
		return errFakeInvalidName
	}

	f.store[item.Key] = item.Data

	return nil
}

func (f *fakeInnerKeyring) Remove(key string) error {
	f.removeAttemptKeys = append(f.removeAttemptKeys, key)

	if f.wouldReject(key) {
		return errFakeInvalidName
	}

	if _, ok := f.store[key]; !ok {
		return keyring.ErrKeyNotFound
	}

	delete(f.store, key)

	return nil
}

func (f *fakeInnerKeyring) Keys() ([]string, error) {
	out := make([]string, 0, len(f.store))
	for k := range f.store {
		out = append(out, k)
	}

	return out, nil
}

// withFakeInvalidFilenameClassifier swaps isInvalidFilenameError for one
// that recognises errFakeInvalidName for the duration of the test. This
// lets cross-platform CI exercise the Windows ERROR_INVALID_NAME branches
// without needing an NTFS volume.
func withFakeInvalidFilenameClassifier(t *testing.T) {
	t.Helper()

	previous := isInvalidFilenameError
	isInvalidFilenameError = func(err error) bool {
		return errors.Is(err, errFakeInvalidName)
	}

	t.Cleanup(func() { isInvalidFilenameError = previous })
}

func TestFileSafeKeyring_GetTranslatesInvalidFilenameToErrKeyNotFound(t *testing.T) {
	withFakeInvalidFilenameClassifier(t)

	inner := newFakeInnerKeyring()
	ring := newFileSafeKeyring(inner)

	_, err := ring.Get("token:default:does-not-exist@example.com")
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("Get with NTFS-illegal raw key: got %v, want ErrKeyNotFound", err)
	}
}

func TestFileSafeKeyring_GetMetadataTranslatesInvalidFilenameToErrKeyNotFound(t *testing.T) {
	withFakeInvalidFilenameClassifier(t)

	inner := newFakeInnerKeyring()
	ring := newFileSafeKeyring(inner)

	_, err := ring.GetMetadata("token:default:does-not-exist@example.com")
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("GetMetadata with NTFS-illegal raw key: got %v, want ErrKeyNotFound", err)
	}
}

func TestFileSafeKeyring_RemoveSurvivesInvalidFilenameLegacyCleanup(t *testing.T) {
	withFakeInvalidFilenameClassifier(t)

	inner := newFakeInnerKeyring()
	ring := newFileSafeKeyring(inner)

	key := "token:default:rm@example.com"
	if err := ring.Set(keyring.Item{Key: key, Data: []byte("x")}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Remove must not surface the inner's invalid-filename error from the
	// raw-form cleanup attempt as a failure: the encoded form was deleted
	// successfully and the legacy form provably can't exist.
	if err := ring.Remove(key); err != nil {
		t.Fatalf("Remove with NTFS-illegal raw key: got %v, want nil", err)
	}
}

func TestFileSafeKeyring_RemoveMissingTranslatesInvalidFilename(t *testing.T) {
	withFakeInvalidFilenameClassifier(t)

	inner := newFakeInnerKeyring()
	ring := newFileSafeKeyring(inner)

	err := ring.Remove("token:default:missing@example.com")
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("Remove of missing key with NTFS-illegal raw key: got %v, want ErrKeyNotFound", err)
	}
}

func TestFileSafeKeyring_SetSurvivesInvalidFilenameLegacyCleanup(t *testing.T) {
	withFakeInvalidFilenameClassifier(t)

	inner := newFakeInnerKeyring()
	ring := newFileSafeKeyring(inner)

	key := "token:default:setwin@example.com"
	// Set must succeed even though the best-effort legacy cleanup of the
	// raw-form key would surface invalid-filename from the inner backend.
	if err := ring.Set(keyring.Item{Key: key, Data: []byte("x")}); err != nil {
		t.Fatalf("Set with NTFS-illegal raw key: got %v, want nil", err)
	}
}

// acceptingFakeKeyring stands in for backends like Windows WinCred that
// store opaque strings and accept colon-bearing keys verbatim. It exists
// to verify that fileSafeKeyring's raw-key fallback still finds legacy
// items after the wrapper is layered on top of a non-file backend.
type acceptingFakeKeyring struct {
	store map[string][]byte
}

func newAcceptingFakeKeyring() *acceptingFakeKeyring {
	return &acceptingFakeKeyring{store: map[string][]byte{}}
}

func (f *acceptingFakeKeyring) Get(key string) (keyring.Item, error) {
	data, ok := f.store[key]
	if !ok {
		return keyring.Item{}, keyring.ErrKeyNotFound
	}

	return keyring.Item{Key: key, Data: data}, nil
}

func (f *acceptingFakeKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	if _, ok := f.store[key]; !ok {
		return keyring.Metadata{}, keyring.ErrKeyNotFound
	}

	return keyring.Metadata{}, nil
}

func (f *acceptingFakeKeyring) Set(item keyring.Item) error {
	f.store[item.Key] = item.Data

	return nil
}

func (f *acceptingFakeKeyring) Remove(key string) error {
	if _, ok := f.store[key]; !ok {
		return keyring.ErrKeyNotFound
	}

	delete(f.store, key)

	return nil
}

func (f *acceptingFakeKeyring) Keys() ([]string, error) {
	out := make([]string, 0, len(f.store))
	for k := range f.store {
		out = append(out, k)
	}

	return out, nil
}

func TestFileSafeKeyring_LegacyKeysSurviveOnAcceptingBackend(t *testing.T) {
	// Models the Windows WinCred upgrade scenario: existing tokens were
	// written under raw colon-bearing keys before the shim landed. The
	// wrapper must still find them via the raw-key fallback, otherwise
	// upgrading users would silently lose every credential.
	inner := newAcceptingFakeKeyring()

	legacyKey := "token:default:legacy@example.com"
	if err := inner.Set(keyring.Item{Key: legacyKey, Data: []byte("legacy")}); err != nil {
		t.Fatalf("seed legacy item: %v", err)
	}

	ring := newFileSafeKeyring(inner)

	got, err := ring.Get(legacyKey)
	if err != nil {
		t.Fatalf("Get legacy on accepting backend: %v", err)
	}

	if string(got.Data) != "legacy" {
		t.Errorf("legacy fallback returned %q, want %q", got.Data, "legacy")
	}

	if got.Key != legacyKey {
		t.Errorf("returned key %q, want %q", got.Key, legacyKey)
	}
}
