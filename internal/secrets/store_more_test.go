package secrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

var (
	errTestDuplicateKeychain = errors.New("failed to update item in keychain: the specified item already exists in the keychain. (-25299)")
	errTestKeychain          = errors.New("test -25308 error")
	errTestLegacyRead        = errors.New("legacy keychain denied access")
	errTestReadBack          = errors.New("test read-back failure")
)

func TestKeyringStore_ListDeleteDefault(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := config.DefaultClientName

	tok1 := Token{Email: "a@b.com", RefreshToken: "rt1", CreatedAt: time.Now()}
	if err := store.SetToken(client, tok1.Email, tok1); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	tok2 := Token{Email: "c@d.com", RefreshToken: "rt2", CreatedAt: time.Now()}
	if err := store.SetToken(client, tok2.Email, tok2); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	tokens, err := store.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}

	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}

	err = store.DeleteToken(client, tok1.Email)
	if err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	if _, getErr := store.GetToken(client, tok1.Email); getErr == nil {
		t.Fatalf("expected error for deleted token")
	}

	err = store.SetDefaultAccount(client, "a@b.com")
	if err != nil {
		t.Fatalf("SetDefaultAccount: %v", err)
	}

	if def, err := store.GetDefaultAccount(client); err != nil {
		t.Fatalf("GetDefaultAccount: %v", err)
	} else if def != "a@b.com" {
		t.Fatalf("unexpected default account: %q", def)
	}

	emptyStore := &KeyringStore{ring: keyring.NewArrayKeyring(nil)}
	if def, err := emptyStore.GetDefaultAccount(client); err != nil || def != "" {
		t.Fatalf("expected empty default account, got %q err=%v", def, err)
	}
}

func TestKeyringStore_CustomClientDefaultDoesNotUseLegacyKey(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}

	if err := ring.Set(keyringItem(defaultAccountKey, []byte("default@example.com"))); err != nil {
		t.Fatalf("seed legacy default: %v", err)
	}

	if got, err := store.GetDefaultAccount("work"); err != nil {
		t.Fatalf("GetDefaultAccount(work): %v", err)
	} else if got != "" {
		t.Fatalf("custom client should not read legacy default, got %q", got)
	}

	if err := store.SetDefaultAccount("work", "work@example.com"); err != nil {
		t.Fatalf("SetDefaultAccount(work): %v", err)
	}

	if got, err := store.GetDefaultAccount("work"); err != nil {
		t.Fatalf("GetDefaultAccount(work): %v", err)
	} else if got != "work@example.com" {
		t.Fatalf("custom default = %q", got)
	}

	if got, err := store.GetDefaultAccount(config.DefaultClientName); err != nil {
		t.Fatalf("GetDefaultAccount(default): %v", err)
	} else if got != "default@example.com" {
		t.Fatalf("legacy default was overwritten: %q", got)
	}

	if err := store.DeleteDefaultAccount("work"); err != nil {
		t.Fatalf("DeleteDefaultAccount(work): %v", err)
	}

	if got, err := store.GetDefaultAccount(config.DefaultClientName); err != nil {
		t.Fatalf("GetDefaultAccount(default): %v", err)
	} else if got != "default@example.com" {
		t.Fatalf("legacy default was deleted: %q", got)
	}
}

func TestKeyringStore_DefaultClientPrefersScopedLegacyKey(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}

	if err := ring.Set(keyringItem(defaultAccountKeyForClient(config.DefaultClientName), []byte("personal@example.com"))); err != nil {
		t.Fatalf("seed scoped default: %v", err)
	}

	if err := ring.Set(keyringItem(defaultAccountKey, []byte("work@example.com"))); err != nil {
		t.Fatalf("seed legacy default: %v", err)
	}

	got, err := store.GetDefaultAccount(config.DefaultClientName)
	if err != nil {
		t.Fatalf("GetDefaultAccount(default): %v", err)
	}

	if got != "personal@example.com" {
		t.Fatalf("default client should prefer scoped legacy key, got %q", got)
	}
}

func TestParseTokenKey(t *testing.T) {
	if client, email, ok := ParseTokenKey("token:a@b.com"); !ok || email != "a@b.com" || client != config.DefaultClientName {
		t.Fatalf("unexpected parse: client=%q email=%q ok=%v", client, email, ok)
	}

	if client, email, ok := ParseTokenKey("token:org:a@b.com"); !ok || email != "a@b.com" || client != "org" {
		t.Fatalf("unexpected parse: client=%q email=%q ok=%v", client, email, ok)
	}

	if _, _, ok := ParseTokenKey("nope"); ok {
		t.Fatalf("expected invalid token key")
	}
}

func TestAllowedBackends(t *testing.T) {
	if _, err := allowedBackends(KeyringBackendInfo{Value: "keychain"}); err != nil {
		t.Fatalf("keychain allowed: %v", err)
	}

	if _, err := allowedBackends(KeyringBackendInfo{Value: "file"}); err != nil {
		t.Fatalf("file allowed: %v", err)
	}
}

func TestWrapKeychainError(t *testing.T) {
	wrapped := wrapKeychainError(errTestKeychain)
	if runtime.GOOS == "darwin" {
		if !errors.Is(wrapped, errTestKeychain) || !strings.Contains(wrapped.Error(), "keychain is locked") {
			t.Fatalf("expected wrapped keychain error, got: %v", wrapped)
		}

		return
	}

	if !errors.Is(wrapped, errTestKeychain) || wrapped.Error() != errTestKeychain.Error() {
		t.Fatalf("expected passthrough error, got: %v", wrapped)
	}
}

func TestFileKeyringPasswordFuncFrom(t *testing.T) {
	// Non-empty password with passwordSet=true returns that password.
	fn := fileKeyringPasswordFuncFrom("pw", true, false)
	if got, err := fn("prompt"); err != nil {
		t.Fatalf("expected password, got err: %v", err)
	} else if got != "pw" {
		t.Fatalf("unexpected password: %q", got)
	}

	// Empty password with passwordSet=true returns empty string (not an error).
	fn = fileKeyringPasswordFuncFrom("", true, false)
	if got, err := fn("prompt"); err != nil {
		t.Fatalf("expected empty password, got err: %v", err)
	} else if got != "" {
		t.Fatalf("expected empty password, got: %q", got)
	}

	// Env var not set and no TTY returns errNoTTY.
	fn = fileKeyringPasswordFuncFrom("", false, false)
	if _, err := fn("prompt"); err == nil || !errors.Is(err, errNoTTY) {
		t.Fatalf("expected no TTY error, got: %v", err)
	}
}

func TestKeyringStoreSetTokenErrors(t *testing.T) {
	store := &KeyringStore{ring: keyring.NewArrayKeyring(nil)}
	client := config.DefaultClientName

	if err := store.SetToken(client, " ", Token{RefreshToken: "rt"}); !errors.Is(err, errMissingEmail) {
		t.Fatalf("expected missing email, got %v", err)
	}

	if err := store.SetToken(client, "a@b.com", Token{}); !errors.Is(err, errMissingRefreshToken) {
		t.Fatalf("expected missing refresh token, got %v", err)
	}
}

func TestSetSecretMissingKey(t *testing.T) {
	store := &KeyringStore{ring: keyring.NewArrayKeyring(nil)}
	if err := store.SetSecret(" ", []byte("data")); !errors.Is(err, errMissingSecretKey) {
		t.Fatalf("expected missing key, got %v", err)
	}
}

func TestDeleteSecretMissingKey(t *testing.T) {
	store := &KeyringStore{ring: keyring.NewArrayKeyring(nil)}
	if err := store.DeleteSecret(" "); !errors.Is(err, errMissingSecretKey) {
		t.Fatalf("expected missing key, got %v", err)
	}
}

func TestOpenError(t *testing.T) {
	layout := config.Layout{ConfigDir: t.TempDir(), DataDir: t.TempDir()}
	options := OpenOptions{
		Layout:  layout,
		Config:  config.NewConfigStore(layout),
		Backend: "file",
		GOOS:    runtime.GOOS,
		openKeyringFn: func(keyring.Config) (keyring.Keyring, error) {
			return nil, errTestKeychain
		},
	}

	if _, err := Open(options); err == nil {
		t.Fatalf("expected error")
	}
}

func TestKeyringStoreDeleteAndDefaultErrors(t *testing.T) {
	store := &KeyringStore{ring: keyring.NewArrayKeyring(nil)}
	client := config.DefaultClientName

	if err := store.DeleteToken(client, " "); !errors.Is(err, errMissingEmail) {
		t.Fatalf("expected missing email, got %v", err)
	}

	if err := store.SetDefaultAccount(client, " "); !errors.Is(err, errMissingEmail) {
		t.Fatalf("expected missing email, got %v", err)
	}
}

func TestKeyringStoreSubjectKeyRoundTripAndDelete(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := config.DefaultClientName

	tok := Token{
		Subject:      "google-sub-123",
		Email:        "User@Example.com",
		RefreshToken: "rt",
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.SetToken(client, tok.Email, tok); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	if _, err := ring.Get(subjectTokenKey(client, tok.Subject)); err != nil {
		t.Fatalf("expected subject key: %v", err)
	}

	got, err := store.GetToken(client, "user@example.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	if got.Subject != tok.Subject || got.Email != "user@example.com" {
		t.Fatalf("unexpected token identity: %#v", got)
	}

	listed, err := store.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}

	if len(listed) != 1 || listed[0].Subject != tok.Subject || listed[0].Email != "user@example.com" {
		t.Fatalf("expected one subject-deduped token, got %#v", listed)
	}

	if err := store.DeleteToken(client, "user@example.com"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	if _, err := ring.Get(subjectTokenKey(client, tok.Subject)); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected subject key deleted, got %v", err)
	}
}

func TestKeyringStoreTokenAccessTokenRoundTrip(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	expires := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)

	err := store.SetToken(config.DefaultClientName, "a@b.com", Token{
		RefreshToken:         "rt",
		AccessToken:          "at",
		AccessTokenExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	got, err := store.GetToken(config.DefaultClientName, "a@b.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	if got.AccessToken != "at" {
		t.Fatalf("expected access token, got %q", got.AccessToken)
	}

	if !got.AccessTokenExpiresAt.Equal(expires) {
		t.Fatalf("expected access token expiry, got %s", got.AccessTokenExpiresAt)
	}
}

func TestKeyringStoreSetTokenRepairsDuplicateAliasWrites(t *testing.T) {
	email := "a@b.com"
	subject := "google-sub-123"
	expires := time.Date(2026, 6, 9, 16, 0, 42, 0, time.UTC)

	ring := &duplicateOnceKeyring{
		ArrayKeyring: keyring.NewArrayKeyring(nil),
		duplicateKeys: map[string]int{
			legacyTokenKey(email):                              1,
			subjectTokenKey(config.DefaultClientName, subject): 1,
		},
		removedKeys: map[string]int{},
	}

	for _, key := range []string{
		legacyTokenKey(email),
		subjectTokenKey(config.DefaultClientName, subject),
	} {
		if err := ring.ArrayKeyring.Set(keyringItem(key, []byte("stale"))); err != nil {
			t.Fatalf("seed stale alias %q: %v", key, err)
		}
	}

	store := &KeyringStore{ring: ring}

	err := store.SetToken(config.DefaultClientName, email, Token{
		Subject:              subject,
		RefreshToken:         "rt",
		AccessToken:          "at",
		AccessTokenExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	primary, err := ring.Get(tokenKey(config.DefaultClientName, email))
	if err != nil {
		t.Fatalf("read primary token: %v", err)
	}

	for _, key := range []string{
		legacyTokenKey(email),
		subjectTokenKey(config.DefaultClientName, subject),
	} {
		item, getErr := ring.Get(key)
		if getErr != nil {
			t.Fatalf("expected key %q persisted after duplicate repair: %v", key, getErr)
		}

		if !bytes.Equal(item.Data, primary.Data) {
			t.Fatalf("alias %q was not replaced with primary payload", key)
		}

		if ring.removedKeys[key] != 1 {
			t.Fatalf("alias %q remove count = %d, want 1", key, ring.removedKeys[key])
		}
	}

	got, err := store.GetToken(config.DefaultClientName, email)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	if got.AccessToken != "at" || !got.AccessTokenExpiresAt.Equal(expires) {
		t.Fatalf("refreshed access metadata was not preserved: %#v", got)
	}
}

func TestKeyringStoreSetTokenKeepsPrimaryDuplicateStrict(t *testing.T) {
	email := "a@b.com"
	primaryKey := tokenKey(config.DefaultClientName, email)

	ring := &duplicateOnceKeyring{
		ArrayKeyring: keyring.NewArrayKeyring(nil),
		duplicateKeys: map[string]int{
			primaryKey: 1,
		},
		removedKeys: map[string]int{},
	}

	if err := ring.ArrayKeyring.Set(keyringItem(primaryKey, []byte("stale-primary"))); err != nil {
		t.Fatalf("seed stale primary: %v", err)
	}

	store := &KeyringStore{ring: ring}

	err := store.SetToken(config.DefaultClientName, email, Token{RefreshToken: "rt"})
	if err == nil || !isDuplicateKeyringItemError(err) {
		t.Fatalf("expected primary duplicate error, got %v", err)
	}

	if ring.removedKeys[primaryKey] != 0 {
		t.Fatalf("primary token was removed during strict write")
	}

	item, getErr := ring.Get(primaryKey)
	if getErr != nil {
		t.Fatalf("read primary token: %v", getErr)
	}

	if string(item.Data) != "stale-primary" {
		t.Fatalf("primary token changed after failed strict write: %q", item.Data)
	}
}

func TestKeyringStoreDeleteTokenAliasPreservesSubjectKey(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := config.DefaultClientName

	tok := Token{
		Subject:      "google-sub-123",
		Email:        "old@example.com",
		RefreshToken: "rt",
	}
	if err := store.SetToken(client, tok.Email, tok); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	if err := store.DeleteTokenAlias(client, "old@example.com"); err != nil {
		t.Fatalf("DeleteTokenAlias: %v", err)
	}

	if _, err := ring.Get(tokenKey(client, "old@example.com")); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected email token removed, got %v", err)
	}

	if _, err := ring.Get(subjectTokenKey(client, tok.Subject)); err != nil {
		t.Fatalf("expected subject key preserved, got %v", err)
	}
}

func TestKeyringStoreSetTokenRemovesStaleSubjectKey(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := config.DefaultClientName
	email := "user@example.com"

	if err := store.SetToken(client, email, Token{
		Subject:      "old-sub",
		Email:        email,
		RefreshToken: "rt1",
	}); err != nil {
		t.Fatalf("SetToken old: %v", err)
	}

	if err := store.SetToken(client, email, Token{
		Subject:      "new-sub",
		Email:        email,
		RefreshToken: "rt2",
	}); err != nil {
		t.Fatalf("SetToken new: %v", err)
	}

	if _, err := ring.Get(subjectTokenKey(client, "old-sub")); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected old subject key removed, got %v", err)
	}

	if got, err := store.GetToken(client, email); err != nil || got.Subject != "new-sub" || got.RefreshToken != "rt2" {
		t.Fatalf("unexpected token after subject rotation: %#v err=%v", got, err)
	}
}

func TestKeyringStoreWritePathsSetLabel(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	email := "A@B.COM"
	client := config.DefaultClientName
	tok := Token{RefreshToken: "rt", CreatedAt: time.Now().UTC()}

	if err := store.SetToken(client, email, tok); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	for _, k := range []string{
		tokenKey(client, normalize(email)),
		legacyTokenKey(normalize(email)),
	} {
		it, err := ring.Get(k)
		if err != nil {
			t.Fatalf("Get(%q): %v", k, err)
		}

		if it.Label != config.AppName {
			t.Fatalf("expected label %q for key %q, got %q", config.AppName, k, it.Label)
		}
	}

	if err := store.SetDefaultAccount(client, email); err != nil {
		t.Fatalf("SetDefaultAccount: %v", err)
	}

	for _, k := range []string{defaultAccountKeyForClient(client), defaultAccountKey} {
		it, err := ring.Get(k)
		if err != nil {
			t.Fatalf("Get(%q): %v", k, err)
		}

		if it.Label != config.AppName {
			t.Fatalf("expected label %q for key %q, got %q", config.AppName, k, it.Label)
		}
	}
}

func TestGetTokenMigrationSetsLabel(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	email := "a@b.com"
	client := config.DefaultClientName

	payload, err := json.Marshal(storedToken{
		RefreshToken: "rt",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Simulate an old legacy item created before label support.
	if setErr := ring.Set(keyring.Item{Key: legacyTokenKey(email), Data: payload}); setErr != nil {
		t.Fatalf("Set legacy token: %v", setErr)
	}

	if _, getErr := store.GetToken(client, email); getErr != nil {
		t.Fatalf("GetToken: %v", getErr)
	}

	it, err := ring.Get(tokenKey(client, email))
	if err != nil {
		t.Fatalf("Get migrated key: %v", err)
	}

	if it.Label != config.AppName {
		t.Fatalf("expected migrated label %q, got %q", config.AppName, it.Label)
	}
}

func TestGetTokenNoMigrateReadsLegacyWithoutWritingPrimary(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	email := "a@b.com"
	client := config.DefaultClientName

	payload, err := json.Marshal(storedToken{
		Email:        email,
		RefreshToken: "rt",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if setErr := ring.Set(keyring.Item{Key: legacyTokenKey(email), Data: payload}); setErr != nil {
		t.Fatalf("Set legacy token: %v", setErr)
	}

	got, err := store.GetTokenNoMigrate(client, email)
	if err != nil {
		t.Fatalf("GetTokenNoMigrate: %v", err)
	}

	if got.Email != email || got.RefreshToken != "rt" {
		t.Fatalf("unexpected token: %#v", got)
	}

	if _, err := ring.Get(tokenKey(client, email)); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected primary token to remain missing, got %v", err)
	}

	if _, err := ring.Get(legacyTokenKey(email)); err != nil {
		t.Fatalf("expected legacy token to remain readable, got %v", err)
	}
}

func TestGetTokenClassifiesCorruptStoredToken(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := "work"
	email := "a@b.com"

	if err := ring.Set(keyringItem(tokenKey(client, email), []byte("{not-json"))); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	_, err := store.GetToken(client, email)
	if !errors.Is(err, ErrCorruptStoredToken) {
		t.Fatalf("expected corrupt stored token error, got %v", err)
	}

	if errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("corrupt present token should not look missing: %v", err)
	}
}

func TestGetTokenMissingTokenNotCorrupt(t *testing.T) {
	store := &KeyringStore{ring: keyring.NewArrayKeyring(nil)}

	_, err := store.GetToken("work", "a@b.com")
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected missing token error, got %v", err)
	}

	if errors.Is(err, ErrCorruptStoredToken) {
		t.Fatalf("missing token should not look corrupt: %v", err)
	}
}

func TestGetTokenBySubjectClassifiesCorruptStoredToken(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := "work"
	subject := "sub-123"

	if err := ring.Set(keyringItem(subjectTokenKey(client, subject), []byte("{not-json"))); err != nil {
		t.Fatalf("seed subject token: %v", err)
	}

	_, err := store.getTokenBySubjectNoLock(client, subject)
	if !errors.Is(err, ErrCorruptStoredToken) {
		t.Fatalf("expected corrupt stored token error, got %v", err)
	}
}

type legacyTokenReadErrorKeyring struct {
	*keyring.ArrayKeyring
	legacyKey string
	err       error
}

func (l *legacyTokenReadErrorKeyring) Get(key string) (keyring.Item, error) {
	if key == l.legacyKey {
		return keyring.Item{}, l.err
	}

	item, err := l.ArrayKeyring.Get(key)
	if err != nil {
		return keyring.Item{}, fmt.Errorf("array get: %w", err)
	}

	return item, nil
}

func TestGetTokenReportsLegacyReadError(t *testing.T) {
	email := "a@b.com"
	ring := &legacyTokenReadErrorKeyring{
		ArrayKeyring: keyring.NewArrayKeyring(nil),
		legacyKey:    legacyTokenKey(email),
		err:          errTestLegacyRead,
	}
	store := &KeyringStore{ring: ring}

	_, err := store.GetToken(config.DefaultClientName, email)
	if !errors.Is(err, errTestLegacyRead) {
		t.Fatalf("expected legacy read error, got %v", err)
	}

	if !strings.Contains(err.Error(), "read legacy token") {
		t.Fatalf("expected legacy read context, got %v", err)
	}
}

// silentDropKeyring simulates a macOS Keychain that silently writes 0 bytes.
// Set returns nil (success), but Get returns an item with empty Data.
type silentDropKeyring struct {
	keyring.ArrayKeyring
}

func (s *silentDropKeyring) Set(_ keyring.Item) error { return nil }
func (s *silentDropKeyring) Get(_ string) (keyring.Item, error) {
	return keyring.Item{Data: nil}, nil
}
func (s *silentDropKeyring) Keys() ([]string, error) { return nil, nil }

func TestSetTokenVerifyCatchesEmptyWrite(t *testing.T) {
	store := &KeyringStore{ring: &silentDropKeyring{}}
	client := config.DefaultClientName

	err := store.SetToken(client, "a@b.com", Token{RefreshToken: "rt", CreatedAt: time.Now()})
	if err == nil {
		t.Fatal("expected error when keyring silently drops data")
	}

	if !errors.Is(err, errTokenVerifyFailed) {
		t.Fatalf("expected errTokenVerifyFailed, got: %v", err)
	}

	if !strings.Contains(err.Error(), "gog auth keyring file") {
		t.Fatalf("expected workaround suggestion in error, got: %v", err)
	}
}

// readBackErrorKeyring simulates a keyring where Set succeeds but Get fails.
type readBackErrorKeyring struct {
	keyring.ArrayKeyring
}

func (r *readBackErrorKeyring) Set(_ keyring.Item) error { return nil }
func (r *readBackErrorKeyring) Get(_ string) (keyring.Item, error) {
	return keyring.Item{}, errTestReadBack
}
func (r *readBackErrorKeyring) Keys() ([]string, error) { return nil, nil }

type duplicateOnceKeyring struct {
	*keyring.ArrayKeyring
	duplicateKeys map[string]int
	removedKeys   map[string]int
}

func (d *duplicateOnceKeyring) Set(item keyring.Item) error {
	if remaining := d.duplicateKeys[item.Key]; remaining > 0 {
		d.duplicateKeys[item.Key] = remaining - 1
		return errTestDuplicateKeychain
	}

	if err := d.ArrayKeyring.Set(item); err != nil {
		return fmt.Errorf("set array keyring item: %w", err)
	}

	return nil
}

func (d *duplicateOnceKeyring) Remove(key string) error {
	d.removedKeys[key]++

	if err := d.ArrayKeyring.Remove(key); err != nil {
		return fmt.Errorf("remove array keyring item: %w", err)
	}

	return nil
}

func TestSetTokenVerifyCatchesReadBackError(t *testing.T) {
	store := &KeyringStore{ring: &readBackErrorKeyring{}}
	client := config.DefaultClientName

	err := store.SetToken(client, "a@b.com", Token{RefreshToken: "rt", CreatedAt: time.Now()})
	if err == nil {
		t.Fatal("expected error when keyring read-back fails")
	}

	if !errors.Is(err, errTokenVerifyFailed) {
		t.Fatalf("expected errTokenVerifyFailed, got: %v", err)
	}

	if !strings.Contains(err.Error(), "could not read back") {
		t.Fatalf("expected read-back error detail, got: %v", err)
	}
}

type legacyMigrationSilentDropKeyring struct {
	*keyring.ArrayKeyring
	primaryKey string
	setPrimary bool
}

func (l *legacyMigrationSilentDropKeyring) Set(item keyring.Item) error {
	if item.Key == l.primaryKey {
		l.setPrimary = true
		return nil
	}

	if err := l.ArrayKeyring.Set(item); err != nil {
		return fmt.Errorf("array set: %w", err)
	}

	return nil
}

func (l *legacyMigrationSilentDropKeyring) Get(key string) (keyring.Item, error) {
	if key == l.primaryKey {
		if !l.setPrimary {
			return keyring.Item{}, keyring.ErrKeyNotFound
		}

		return keyring.Item{Key: key, Data: nil}, nil
	}

	item, err := l.ArrayKeyring.Get(key)
	if err != nil {
		return keyring.Item{}, fmt.Errorf("array get: %w", err)
	}

	return item, nil
}

func TestGetTokenLegacyMigrationVerifiesWrite(t *testing.T) {
	email := "a@b.com"
	client := config.DefaultClientName
	primaryKey := tokenKey(client, email)
	ring := &legacyMigrationSilentDropKeyring{
		ArrayKeyring: keyring.NewArrayKeyring(nil),
		primaryKey:   primaryKey,
	}
	store := &KeyringStore{ring: ring}

	payload, err := json.Marshal(storedToken{RefreshToken: "rt", Email: email, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("marshal token: %v", err)
	}

	err = ring.ArrayKeyring.Set(keyringItem(legacyTokenKey(email), payload))
	if err != nil {
		t.Fatalf("seed legacy token: %v", err)
	}

	_, err = store.GetToken(client, email)
	if err == nil {
		t.Fatal("expected verified migration write to fail")
	}

	if !errors.Is(err, errTokenVerifyFailed) {
		t.Fatalf("expected errTokenVerifyFailed, got: %v", err)
	}
}

func TestSetSecretSetsLabel(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}

	key := "test/secret"
	if err := store.SetSecret(key, []byte("value")); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	it, err := ring.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if it.Label != config.AppName {
		t.Fatalf("expected label %q, got %q", config.AppName, it.Label)
	}
}

func TestDeleteSecretDeletesKey(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}

	key := "test/secret"
	if err := store.SetSecret(key, []byte("value")); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	if err := store.DeleteSecret(key); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	if _, err := ring.Get(key); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected key deleted, got %v", err)
	}
}

func TestSetSecretVerifyCatchesEmptyWrite(t *testing.T) {
	store := &KeyringStore{ring: &silentDropKeyring{}}

	err := store.SetSecret("test/secret", []byte("value"))
	if err == nil {
		t.Fatal("expected error when keyring silently drops data")
	}

	if !errors.Is(err, errTokenVerifyFailed) {
		t.Fatalf("expected errTokenVerifyFailed, got: %v", err)
	}
}
