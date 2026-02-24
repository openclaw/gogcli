package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

func TestAuthTokensExportImport_JSON(t *testing.T) {
	origOpen := openSecretsStore
	origEnsure := ensureKeychainAccess
	t.Cleanup(func() {
		openSecretsStore = origOpen
		ensureKeychainAccess = origEnsure
	})

	store := newMemStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }
	ensureKeychainAccess = func() error { return nil }

	tok := secrets.Token{
		Email:        "a@b.com",
		RefreshToken: "rt",
		Services:     []string{"gmail"},
		Scopes:       []string{"s1"},
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.SetToken(config.DefaultClientName, tok.Email, tok); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "token.json")
	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	var err error

	exportCmd := AuthTokensExportCmd{
		Email:     tok.Email,
		Output:    OutputPathRequiredFlag{Path: outPath},
		Overwrite: true,
	}
	err = exportCmd.Run(ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	var payload map[string]any
	err = json.Unmarshal(data, &payload)
	if err != nil {
		t.Fatalf("parse export: %v", err)
	}
	if payload["refresh_token"] != "rt" {
		t.Fatalf("unexpected export payload: %#v", payload)
	}

	// Import back into a fresh store.
	newStore := newMemStore()
	openSecretsStore = func() (secrets.Store, error) { return newStore, nil }

	importCmd := AuthTokensImportCmd{InPath: outPath}
	err = importCmd.Run(ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	imported, err := newStore.GetToken(config.DefaultClientName, tok.Email)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if imported.RefreshToken != "rt" {
		t.Fatalf("unexpected imported token: %#v", imported)
	}
}

func TestAuthList_CheckJSON(t *testing.T) {
	origOpen := openSecretsStore
	origCheck := checkRefreshToken
	t.Cleanup(func() {
		openSecretsStore = origOpen
		checkRefreshToken = origCheck
	})

	store := newMemStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }
	checkRefreshToken = func(context.Context, string, string, []string, time.Duration) error {
		return nil
	}

	if err := store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{Email: "a@b.com", RefreshToken: "rt"}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	var err error

	listCmd := AuthListCmd{Check: true}
	out := captureStdout(t, func() {
		runErr := listCmd.Run(ctx, &RootFlags{})
		if runErr != nil {
			t.Fatalf("list: %v", runErr)
		}
	})
	var payload struct {
		Accounts []struct {
			Email string `json:"email"`
			Valid *bool  `json:"valid"`
		} `json:"accounts"`
	}
	err = json.Unmarshal([]byte(out), &payload)
	if err != nil {
		t.Fatalf("decode list output: %v", err)
	}
	if len(payload.Accounts) != 1 || payload.Accounts[0].Email != "a@b.com" || payload.Accounts[0].Valid == nil || !*payload.Accounts[0].Valid {
		t.Fatalf("unexpected list payload: %#v", payload.Accounts)
	}
}

type memStore struct {
	tokens       map[string]secrets.Token
	defaultEmail string
}

func newMemStore() *memStore {
	return &memStore{tokens: make(map[string]secrets.Token)}
}

func (m *memStore) Keys() ([]string, error) {
	keys := make([]string, 0, len(m.tokens))
	for k := range m.tokens {
		parts := strings.SplitN(k, ":", 2)
		if len(parts) != 2 {
			continue
		}
		keys = append(keys, secrets.TokenKey(parts[0], parts[1]))
	}
	return keys, nil
}

func (m *memStore) SetToken(client string, email string, tok secrets.Token) error {
	if strings.TrimSpace(email) == "" {
		return errors.New("missing email")
	}
	if strings.TrimSpace(tok.RefreshToken) == "" {
		return errors.New("missing refresh token")
	}
	if client == "" {
		client = config.DefaultClientName
	}
	tok.Client = client
	tok.Email = email
	m.tokens[client+":"+email] = tok
	return nil
}

func (m *memStore) GetToken(client string, email string) (secrets.Token, error) {
	if client == "" {
		client = config.DefaultClientName
	}
	tok, ok := m.tokens[client+":"+email]
	if !ok {
		return secrets.Token{}, errors.New("not found")
	}
	return tok, nil
}

func (m *memStore) DeleteToken(client string, email string) error {
	if client == "" {
		client = config.DefaultClientName
	}
	delete(m.tokens, client+":"+email)
	return nil
}

func (m *memStore) ListTokens() ([]secrets.Token, error) {
	out := make([]secrets.Token, 0, len(m.tokens))
	for _, tok := range m.tokens {
		out = append(out, tok)
	}
	return out, nil
}

func (m *memStore) GetDefaultAccount(client string) (string, error) {
	return m.defaultEmail, nil
}

func (m *memStore) SetDefaultAccount(client string, email string) error {
	m.defaultEmail = email
	return nil
}
