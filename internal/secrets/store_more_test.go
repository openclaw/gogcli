package secrets

import (
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

var (
	errTestKeychain = errors.New("test -25308 error")
	errTestReadBack = errors.New("test read-back failure")
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
	if err := SetSecret(" ", []byte("data")); !errors.Is(err, errMissingSecretKey) {
		t.Fatalf("expected missing key, got %v", err)
	}
}

func TestOpenDefaultError(t *testing.T) {
	origOpen := openKeyringFunc

	t.Cleanup(func() { openKeyringFunc = origOpen })

	openKeyringFunc = func() (keyring.Keyring, error) {
		return nil, errTestKeychain
	}

	if _, err := OpenDefault(); err == nil {
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

	for _, k := range []string{
		defaultAccountKeyForClient(client),
		defaultAccountKey,
	} {
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

func TestSetSecretSetsLabel(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	origOpen := openKeyringFunc

	t.Cleanup(func() { openKeyringFunc = origOpen })

	openKeyringFunc = func() (keyring.Keyring, error) { return ring, nil }

	key := "test/secret"
	if err := SetSecret(key, []byte("value")); err != nil {
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
