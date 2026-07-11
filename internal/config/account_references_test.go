package config

import (
	"errors"
	"sync"
	"testing"
)

func TestConfigStoreMigrateAccountEmailReferences(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})
	if err := store.Write(File{
		AccountAliases: map[string]string{
			"work": "Old@Example.com",
			"keep": "other@example.com",
		},
		AccountClients: map[string]string{
			"old@example.com":   "work-client",
			"other@example.com": "default",
		},
		MCP: &MCPConfig{Accounts: map[string]MCPPolicy{
			" Old@Example.com ": {AllowTools: []string{"docs.*"}, AllowWrite: true},
		}},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := store.MigrateAccountEmailReferences(" OLD@example.com ", " New@Example.com "); err != nil {
		t.Fatalf("MigrateAccountEmailReferences: %v", err)
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if cfg.AccountAliases["work"] != "new@example.com" || cfg.AccountAliases["keep"] != "other@example.com" {
		t.Fatalf("account aliases = %#v", cfg.AccountAliases)
	}

	if cfg.AccountClients["new@example.com"] != "work-client" {
		t.Fatalf("account clients = %#v", cfg.AccountClients)
	}

	if _, ok := cfg.AccountClients["old@example.com"]; ok {
		t.Fatalf("old account client retained: %#v", cfg.AccountClients)
	}

	if policy, ok := cfg.MCP.Accounts["new@example.com"]; !ok || !policy.AllowWrite {
		t.Fatalf("MCP account policies = %#v", cfg.MCP.Accounts)
	}

	if _, ok := cfg.MCP.Accounts[" Old@Example.com "]; ok {
		t.Fatalf("old MCP account policy retained: %#v", cfg.MCP.Accounts)
	}
}

func TestConfigStoreMigrateAccountEmailReferencesRejectsMCPPolicyCollision(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	original := File{MCP: &MCPConfig{Accounts: map[string]MCPPolicy{
		"old@example.com": {AllowTools: []string{"docs.*"}, AllowWrite: true},
		"new@example.com": {AllowTools: []string{"read"}},
	}}}
	if err := store.Write(original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	err := store.MigrateAccountEmailReferences("old@example.com", "new@example.com")
	if !errors.Is(err, errMCPPolicyDestinationExists) {
		t.Fatalf("migration collision error = %v", err)
	}

	cfg, readErr := store.Read()
	if readErr != nil {
		t.Fatalf("Read: %v", readErr)
	}

	if len(cfg.MCP.Accounts) != 2 || !cfg.MCP.Accounts["old@example.com"].AllowWrite {
		t.Fatalf("collision changed MCP policies: %#v", cfg.MCP.Accounts)
	}
}

func TestConfigStoreMigrateAccountEmailReferencesSerializesConcurrentUpdate(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})
	if err := store.Write(File{
		AccountAliases: map[string]string{"work": "old@example.com"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		<-start

		errs <- store.MigrateAccountEmailReferences("old@example.com", "new@example.com")
	}()
	go func() {
		defer wg.Done()

		<-start

		errs <- store.Update(func(cfg *File) error {
			cfg.DefaultTimezone = "UTC"
			return nil
		})
	}()

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent update: %v", err)
		}
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if cfg.AccountAliases["work"] != "new@example.com" || cfg.DefaultTimezone != "UTC" {
		t.Fatalf("config = %#v", cfg)
	}
}
