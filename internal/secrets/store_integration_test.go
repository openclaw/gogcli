package secrets

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/openclaw/gogcli/internal/config"
)

func setupKeyringEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")
}

func TestSetAndGetSecret_FileBackend(t *testing.T) {
	setupKeyringEnv(t)
	store := openSystemTestStore(t)

	if err := store.SetSecret("test/key", []byte("value")); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	if val, err := store.GetSecret("test/key"); err != nil {
		t.Fatalf("GetSecret: %v", err)
	} else if string(val) != "value" {
		t.Fatalf("unexpected value: %q", val)
	}
}

func TestKeyringStore_TokenRoundTrip(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := config.DefaultClientName

	tok := Token{RefreshToken: "rt", Services: []string{"gmail"}, Scopes: []string{"s"}, CreatedAt: time.Now()}
	if err := store.SetToken(client, "a@b.com", tok); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	if got, err := store.GetToken(client, "a@b.com"); err != nil {
		t.Fatalf("GetToken: %v", err)
	} else if got.RefreshToken != "rt" {
		t.Fatalf("unexpected token: %#v", got)
	}

	keys, err := store.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}

	if len(keys) == 0 {
		t.Fatalf("expected keys")
	}
}

func TestEnsureKeyringDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	_, err := testSystemLayout(t, config.PathKindConfig, config.PathKindData).EnsureKeyringDir()
	if err != nil {
		t.Fatalf("EnsureKeyringDir: %v", err)
	}
}
