package secrets

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

var errTestKeychain = errors.New("test -25308 error")

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
	fn := fileKeyringPasswordFuncFrom("pw", false)
	if got, err := fn("prompt"); err != nil {
		t.Fatalf("expected password, got err: %v", err)
	} else if got != "pw" {
		t.Fatalf("unexpected password: %q", got)
	}

	fn = fileKeyringPasswordFuncFrom("", false)
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

func TestDeleteSecret(t *testing.T) {
	origOpen := openKeyringFunc
	t.Cleanup(func() { openKeyringFunc = origOpen })

	ring := keyring.NewArrayKeyring(nil)
	openKeyringFunc = func() (keyring.Keyring, error) { return ring, nil }

	if err := SetSecret("places/test", []byte("value")); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	if err := DeleteSecret("places/test"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	if _, err := ring.Get("places/test"); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected key to be deleted, got %v", err)
	}

	if err := DeleteSecret("places/test"); err != nil {
		t.Fatalf("DeleteSecret idempotent: %v", err)
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
