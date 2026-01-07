package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

type memSecretsStore struct {
	tokens map[string]secrets.Token
}

func newMemSecretsStore() *memSecretsStore {
	return &memSecretsStore{tokens: make(map[string]secrets.Token)}
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (s *memSecretsStore) Keys() ([]string, error) {
	keys := make([]string, 0, len(s.tokens))
	for email := range s.tokens {
		keys = append(keys, "token:"+email)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *memSecretsStore) SetToken(email string, tok secrets.Token) error {
	email = normalizeEmail(email)
	if email == "" {
		return errors.New("missing email")
	}
	if strings.TrimSpace(tok.RefreshToken) == "" {
		return errors.New("missing refresh token")
	}
	tok.Email = email
	s.tokens[email] = tok
	return nil
}

func (s *memSecretsStore) GetToken(email string) (secrets.Token, error) {
	email = normalizeEmail(email)
	if email == "" {
		return secrets.Token{}, errors.New("missing email")
	}
	if tok, ok := s.tokens[email]; ok {
		return tok, nil
	}
	return secrets.Token{}, keyring.ErrKeyNotFound
}

func (s *memSecretsStore) DeleteToken(email string) error {
	email = normalizeEmail(email)
	if email == "" {
		return errors.New("missing email")
	}
	if _, ok := s.tokens[email]; !ok {
		return keyring.ErrKeyNotFound
	}
	delete(s.tokens, email)
	return nil
}

func (s *memSecretsStore) ListTokens() ([]secrets.Token, error) {
	out := make([]secrets.Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		out = append(out, t)
	}
	return out, nil
}

func (s *memSecretsStore) GetDefaultAccount() (string, error) {
	return "", nil
}

func (s *memSecretsStore) SetDefaultAccount(email string) error {
	return nil
}

func TestAuthTokens_ExportImportRoundtrip_JSON(t *testing.T) {
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	t.Cleanup(func() {
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
	})

	ensureKeychainAccess = func() error { return nil }
	store := newMemSecretsStore()
	createdAt := time.Date(2025, 12, 12, 0, 0, 0, 0, time.UTC)
	if err := store.SetToken("A@B.COM", secrets.Token{
		Services:     []string{"gmail"},
		Scopes:       []string{"s1"},
		CreatedAt:    createdAt,
		RefreshToken: "rt",
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	outPath := filepath.Join(t.TempDir(), "token.json")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "tokens", "export", "a@b.com", "--out", outPath}); err != nil {
				t.Fatalf("Execute export: %v", err)
			}
		})
	})

	var exportResp struct {
		Exported bool   `json:"exported"`
		Email    string `json:"email"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &exportResp); err != nil {
		t.Fatalf("export json: %v\nout=%q", err, stdout)
	}
	if !exportResp.Exported || exportResp.Email != "a@b.com" || exportResp.Path != outPath {
		t.Fatalf("unexpected export resp: %#v", exportResp)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read outPath: %v", err)
	}
	if !strings.Contains(string(b), "\"refresh_token\"") {
		t.Fatalf("expected refresh_token in file: %q", string(b))
	}

	// Clear token, then import it back.
	if err := store.DeleteToken("a@b.com"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	importOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "tokens", "import", outPath}); err != nil {
				t.Fatalf("Execute import: %v", err)
			}
		})
	})
	var importResp struct {
		Imported bool   `json:"imported"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal([]byte(importOut), &importResp); err != nil {
		t.Fatalf("import json: %v\nout=%q", err, importOut)
	}
	if !importResp.Imported || importResp.Email != "a@b.com" {
		t.Fatalf("unexpected import resp: %#v", importResp)
	}
	if tok, err := store.GetToken("a@b.com"); err != nil || tok.RefreshToken != "rt" {
		t.Fatalf("expected token restored, got tok=%#v err=%v", tok, err)
	}
}

func TestAuthStatus_JSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_KEYRING_BACKEND", "file")

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var payload struct {
		Config struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
		} `json:"config"`
		Keyring struct {
			Backend string `json:"backend"`
			Source  string `json:"source"`
		} `json:"keyring"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Keyring.Backend != "file" {
		t.Fatalf("unexpected backend: %q", payload.Keyring.Backend)
	}
	if payload.Keyring.Source != "env" {
		t.Fatalf("unexpected backend source: %q", payload.Keyring.Source)
	}
	if payload.Config.Path == "" {
		t.Fatalf("expected config path")
	}
}

func TestAuthStatus_Text_ConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	cfgPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{ keyring_backend: "file" }`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "config_exists\ttrue") {
		t.Fatalf("expected config_exists true, got: %q", out)
	}
	if !strings.Contains(out, "keyring_backend\tfile") {
		t.Fatalf("expected keyring_backend file, got: %q", out)
	}
	if !strings.Contains(out, "keyring_backend_source\tconfig") {
		t.Fatalf("expected keyring_backend_source config, got: %q", out)
	}
}

func TestAuthTokensExport_RequiresOut(t *testing.T) {
	err := Execute([]string{"--json", "auth", "tokens", "export", "a@b.com"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "empty outPath") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthTokensImport_NoInput(t *testing.T) {
	origKeychain := ensureKeychainAccess
	t.Cleanup(func() { ensureKeychainAccess = origKeychain })

	ensureKeychainAccess = func() error {
		return errors.New("keychain locked")
	}

	outPath := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(outPath, []byte(`{"email":"a@b.com","refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	err := Execute([]string{"--json", "--no-input", "auth", "tokens", "import", outPath})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "keychain access") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthListRemoveTokensListDelete_JSON(t *testing.T) {
	origOpen := openSecretsStore
	origCheck := checkRefreshToken
	t.Cleanup(func() {
		openSecretsStore = origOpen
		checkRefreshToken = origCheck
	})

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	checkRefreshToken = func(_ context.Context, refreshToken string, _ []string, _ time.Duration) error {
		if refreshToken == "rt2" {
			return errors.New("invalid_grant")
		}
		return nil
	}

	_ = store.SetToken("b@b.com", secrets.Token{RefreshToken: "rt2"})
	_ = store.SetToken("a@b.com", secrets.Token{RefreshToken: "rt1"})

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "list"}); err != nil {
				t.Fatalf("Execute list: %v", err)
			}
		})
	})
	var listResp struct {
		Accounts []struct {
			Email string `json:"email"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(listOut), &listResp); err != nil {
		t.Fatalf("list json: %v\nout=%q", err, listOut)
	}
	if len(listResp.Accounts) != 2 || listResp.Accounts[0].Email != "a@b.com" || listResp.Accounts[1].Email != "b@b.com" {
		t.Fatalf("unexpected accounts: %#v", listResp.Accounts)
	}

	listOut = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "list", "--check"}); err != nil {
				t.Fatalf("Execute list --check: %v", err)
			}
		})
	})
	var listCheckedResp struct {
		Accounts []struct {
			Email string `json:"email"`
			Valid *bool  `json:"valid,omitempty"`
			Error string `json:"error,omitempty"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(listOut), &listCheckedResp); err != nil {
		t.Fatalf("list --check json: %v\nout=%q", err, listOut)
	}
	if len(listCheckedResp.Accounts) != 2 {
		t.Fatalf("unexpected accounts: %#v", listCheckedResp.Accounts)
	}
	if listCheckedResp.Accounts[0].Email != "a@b.com" || listCheckedResp.Accounts[0].Valid == nil || !*listCheckedResp.Accounts[0].Valid {
		t.Fatalf("expected a@b.com valid, got: %#v", listCheckedResp.Accounts[0])
	}
	if listCheckedResp.Accounts[1].Email != "b@b.com" || listCheckedResp.Accounts[1].Valid == nil || *listCheckedResp.Accounts[1].Valid || !strings.Contains(listCheckedResp.Accounts[1].Error, "invalid_grant") {
		t.Fatalf("expected b@b.com invalid_grant, got: %#v", listCheckedResp.Accounts[1])
	}

	// Tokens list (keys).
	keysOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "tokens", "list"}); err != nil {
				t.Fatalf("Execute tokens list: %v", err)
			}
		})
	})
	var keysResp struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(keysOut), &keysResp); err != nil {
		t.Fatalf("keys json: %v\nout=%q", err, keysOut)
	}
	if len(keysResp.Keys) != 2 {
		t.Fatalf("unexpected keys: %#v", keysResp.Keys)
	}

	// Remove (auth remove)
	rmOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--force", "auth", "remove", "b@b.com"}); err != nil {
				t.Fatalf("Execute remove: %v", err)
			}
		})
	})
	var rmResp struct {
		Deleted bool   `json:"deleted"`
		Email   string `json:"email"`
	}
	if err := json.Unmarshal([]byte(rmOut), &rmResp); err != nil {
		t.Fatalf("remove json: %v\nout=%q", err, rmOut)
	}
	if !rmResp.Deleted || rmResp.Email != "b@b.com" {
		t.Fatalf("unexpected remove resp: %#v", rmResp)
	}

	// Tokens delete (auth tokens delete)
	delOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--force", "auth", "tokens", "delete", "a@b.com"}); err != nil {
				t.Fatalf("Execute tokens delete: %v", err)
			}
		})
	})
	var delResp struct {
		Deleted bool   `json:"deleted"`
		Email   string `json:"email"`
	}
	if err := json.Unmarshal([]byte(delOut), &delResp); err != nil {
		t.Fatalf("delete json: %v\nout=%q", err, delOut)
	}
	if !delResp.Deleted || delResp.Email != "a@b.com" {
		t.Fatalf("unexpected delete resp: %#v", delResp)
	}

	// Now empty.
	emptyKeysOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "tokens", "list"}); err != nil {
				t.Fatalf("Execute tokens list: %v", err)
			}
		})
	})
	var emptyKeysResp struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(emptyKeysOut), &emptyKeysResp); err != nil {
		t.Fatalf("empty keys json: %v\nout=%q", err, emptyKeysOut)
	}
	if len(emptyKeysResp.Keys) != 0 {
		t.Fatalf("expected empty keys, got: %#v", emptyKeysResp.Keys)
	}
}
