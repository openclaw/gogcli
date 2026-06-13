package googleauth

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

var errIdentityReferenceUpdate = errors.New("identity reference update failed")

type migrationStore struct {
	tokens       map[string]secrets.Token
	defaultEmail string
	listNewOnly  bool
}

func newMigrationStore() *migrationStore {
	return &migrationStore{tokens: make(map[string]secrets.Token)}
}

func (s *migrationStore) Keys() ([]string, error) {
	keys := make([]string, 0, len(s.tokens))
	for key := range s.tokens {
		client, email, ok := strings.Cut(key, "\n")
		if !ok {
			continue
		}

		keys = append(keys, secrets.TokenKey(client, email))
	}

	sort.Strings(keys)

	return keys, nil
}

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
		if s.listNewOnly && tok.Email != "new@example.com" {
			continue
		}

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

func TestMigrateStoredSubjectIdentityUpdatesEmailReferences(t *testing.T) {
	cfg := config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
		AccountClients: map[string]string{"old@example.com": "work-client"},
	}

	configStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := configStore.Write(cfg); err != nil {
		t.Fatalf("write config: %v", err)
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

	migrated, err := MigrateStoredSubjectIdentity(store, configStore.MigrateAccountEmailReferences, config.DefaultClientName, Identity{
		Subject: "sub-123",
		Email:   "new@example.com",
	})
	if err != nil {
		t.Fatalf("MigrateStoredSubjectIdentity: %v", err)
	}

	if migrated != "old@example.com" {
		t.Fatalf("expected migrated old email, got %q", migrated)
	}

	if _, getErr := store.GetToken(config.DefaultClientName, "old@example.com"); getErr != nil {
		t.Fatalf("expected old token preserved until replacement is stored, got %v", getErr)
	}

	if store.defaultEmail != "new@example.com" {
		t.Fatalf("expected default migrated, got %q", store.defaultEmail)
	}

	updated, err := configStore.Read()
	if err != nil {
		t.Fatalf("read config: %v", err)
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

func TestMigrateStoredSubjectIdentityPreservesOldTokenWhenNewAlreadyStored(t *testing.T) {
	store := newMigrationStore()

	store.listNewOnly = true
	if err := store.SetToken(config.DefaultClientName, "new@example.com", secrets.Token{
		Subject:      "sub-123",
		RefreshToken: "rt-new",
	}); err != nil {
		t.Fatalf("SetToken new: %v", err)
	}

	if err := store.SetToken(config.DefaultClientName, "old@example.com", secrets.Token{
		Subject:      "sub-123",
		RefreshToken: "rt-old",
	}); err != nil {
		t.Fatalf("SetToken old: %v", err)
	}

	migrated, err := MigrateStoredSubjectIdentity(store, func(string, string) error { return nil }, config.DefaultClientName, Identity{
		Subject: "sub-123",
		Email:   "new@example.com",
	})
	if err != nil {
		t.Fatalf("MigrateStoredSubjectIdentity: %v", err)
	}

	if migrated != "old@example.com" {
		t.Fatalf("expected migrated old email, got %q", migrated)
	}

	if _, getErr := store.GetToken(config.DefaultClientName, "old@example.com"); getErr != nil {
		t.Fatalf("expected old token preserved until alias cleanup, got %v", getErr)
	}

	if _, getErr := store.GetToken(config.DefaultClientName, "new@example.com"); getErr != nil {
		t.Fatalf("expected new token kept, got %v", getErr)
	}
}

func TestMigrateStoredEmailReferencesRequiresUpdaterBeforeDefaultChange(t *testing.T) {
	store := newMigrationStore()
	store.defaultEmail = "old@example.com"

	err := MigrateStoredEmailReferences(store, nil, config.DefaultClientName, "old@example.com", "new@example.com")
	if !errors.Is(err, errEmailReferenceUpdaterRequired) {
		t.Fatalf("error = %v, want updater-required", err)
	}

	if store.defaultEmail != "old@example.com" {
		t.Fatalf("default email changed to %q", store.defaultEmail)
	}
}

func TestMigrateStoredEmailReferencesPreservesConfigFailureContract(t *testing.T) {
	store := newMigrationStore()
	store.defaultEmail = "old@example.com"

	err := MigrateStoredEmailReferences(store, func(string, string) error {
		return errIdentityReferenceUpdate
	}, config.DefaultClientName, "old@example.com", "new@example.com")
	if !errors.Is(err, errIdentityReferenceUpdate) {
		t.Fatalf("error = %v, want update failure", err)
	}

	if store.defaultEmail != "new@example.com" {
		t.Fatalf("default email = %q, want migrated before config failure", store.defaultEmail)
	}
}
