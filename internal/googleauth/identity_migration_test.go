package googleauth

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

type migrationStore struct {
	tokens       map[string]secrets.Token
	defaultEmail string
}

func newMigrationStore() *migrationStore {
	return &migrationStore{tokens: make(map[string]secrets.Token)}
}

func (s *migrationStore) Keys() ([]string, error) { return nil, nil }

func (s *migrationStore) SetToken(client string, email string, tok secrets.Token) error {
	if client == "" {
		client = config.DefaultClientName
	}

	tok.Client = client
	tok.Email = email
	s.tokens[client+"\n"+email] = tok

	return nil
}

func (s *migrationStore) GetToken(client string, email string) (secrets.Token, error) {
	if client == "" {
		client = config.DefaultClientName
	}

	tok, ok := s.tokens[client+"\n"+email]
	if !ok {
		return secrets.Token{}, keyring.ErrKeyNotFound
	}

	return tok, nil
}

func (s *migrationStore) DeleteToken(client string, email string) error {
	if client == "" {
		client = config.DefaultClientName
	}

	delete(s.tokens, client+"\n"+email)

	return nil
}

func (s *migrationStore) ListTokens() ([]secrets.Token, error) {
	out := make([]secrets.Token, 0, len(s.tokens))
	for _, tok := range s.tokens {
		out = append(out, tok)
	}

	return out, nil
}

func (s *migrationStore) GetDefaultAccount(string) (string, error) {
	return s.defaultEmail, nil
}

func (s *migrationStore) SetDefaultAccount(_ string, email string) error {
	s.defaultEmail = email
	return nil
}

func TestMigrateStoredSubjectIdentityUpdatesEmailState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	cfg := config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
		AccountClients: map[string]string{"old@example.com": "work-client"},
	}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	store := newMigrationStore()
	if err := store.SetToken(config.DefaultClientName, "old@example.com", secrets.Token{
		Subject:      "sub-123",
		Email:        "old@example.com",
		RefreshToken: "rt",
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	store.defaultEmail = "old@example.com"

	migrated, err := MigrateStoredSubjectIdentity(store, config.DefaultClientName, Identity{
		Subject: "sub-123",
		Email:   "new@example.com",
	})
	if err != nil {
		t.Fatalf("MigrateStoredSubjectIdentity: %v", err)
	}

	if migrated != "old@example.com" {
		t.Fatalf("expected migrated old email, got %q", migrated)
	}

	if _, getErr := store.GetToken(config.DefaultClientName, "old@example.com"); !errors.Is(getErr, keyring.ErrKeyNotFound) {
		t.Fatalf("expected old token deleted, got %v", getErr)
	}

	if store.defaultEmail != "new@example.com" {
		t.Fatalf("expected default migrated, got %q", store.defaultEmail)
	}

	updated, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if updated.AccountAliases["work"] != "new@example.com" {
		t.Fatalf("expected alias migrated, got %#v", updated.AccountAliases)
	}

	if updated.AccountClients["new@example.com"] != "work-client" {
		t.Fatalf("expected account client migrated, got %#v", updated.AccountClients)
	}

	if _, ok := updated.AccountClients["old@example.com"]; ok {
		t.Fatalf("expected old account client removed, got %#v", updated.AccountClients)
	}
}
