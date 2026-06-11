package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestCapabilitiesDefaultIsRedactedAndKeyringFree(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	origOpen := openSecretsStore
	origAccountOpen := openSecretsStoreForAccount
	t.Cleanup(func() {
		openSecretsStore = origOpen
		openSecretsStoreForAccount = origAccountOpen
	})
	openSecretsStore = func() (secrets.Store, error) {
		return nil, errors.New("keyring must not be opened")
	}
	openSecretsStoreForAccount = func() (secrets.Store, error) {
		return nil, errors.New("account keyring must not be opened")
	}

	snapshot, err := buildCapabilities(&RootFlags{}, capabilitiesOptions{})
	if err != nil {
		t.Fatalf("buildCapabilities: %v", err)
	}
	if snapshot.Disclosure.AuthInspected || snapshot.Disclosure.AccountIncluded {
		t.Fatalf("unexpected disclosure: %#v", snapshot.Disclosure)
	}
	if snapshot.Auth.AccountSelected || snapshot.Auth.Account != "" || snapshot.Auth.Method != "" {
		t.Fatalf("unexpected auth details: %#v", snapshot.Auth)
	}
	if snapshot.Discovery.SchemaCommand != "gog schema --json" {
		t.Fatalf("schema command = %q", snapshot.Discovery.SchemaCommand)
	}
	if snapshot.Discovery.ExitCodesCommand != "gog exit-codes --json" {
		t.Fatalf("exit codes command = %q", snapshot.Discovery.ExitCodesCommand)
	}
	if snapshot.MCP != nil {
		t.Fatalf("CLI snapshot should not include MCP tools: %#v", snapshot.MCP)
	}
}

func TestCapabilitiesReportsRuntimeSafety(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	flags := &RootFlags{
		DryRun:              true,
		NoInput:             true,
		WrapUntrusted:       true,
		GmailNoSend:         true,
		EnableCommands:      "docs.cat,gmail.search,docs.cat",
		EnableCommandsExact: "drive.ls",
		DisableCommands:     "gmail.send",
	}
	snapshot, err := buildCapabilities(flags, capabilitiesOptions{})
	if err != nil {
		t.Fatalf("buildCapabilities: %v", err)
	}
	if !snapshot.Safety.DryRun || !snapshot.Safety.NoInput || !snapshot.Safety.WrapUntrusted || !snapshot.Safety.GmailNoSend {
		t.Fatalf("runtime safety missing: %#v", snapshot.Safety)
	}
	wantPrefixes := []string{"docs.cat", "gmail.search"}
	if !equalStrings(snapshot.Safety.CommandRules.EnabledPrefixes, wantPrefixes) {
		t.Fatalf("enabled prefixes = %#v, want %#v", snapshot.Safety.CommandRules.EnabledPrefixes, wantPrefixes)
	}
}

func TestCapabilitiesExplicitOAuthDisclosure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOG_HOME", filepath.Join(home, "gog"))

	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	expires := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	if err := store.SetToken(config.DefaultClientName, "user@example.com", secrets.Token{
		Services:             []string{"drive", "gmail", "drive"},
		Scopes:               []string{"scope-b", "scope-a"},
		RefreshToken:         "redacted",
		AccessTokenExpiresAt: expires,
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	snapshot, err := buildCapabilities(&RootFlags{
		Account: "user@example.com",
		Client:  config.DefaultClientName,
	}, capabilitiesOptions{
		IncludeAuth:    true,
		IncludeAccount: true,
	})
	if err != nil {
		t.Fatalf("buildCapabilities: %v", err)
	}
	if !snapshot.Disclosure.AuthInspected || !snapshot.Disclosure.AccountIncluded {
		t.Fatalf("disclosure = %#v", snapshot.Disclosure)
	}
	if snapshot.Auth.Method != authTypeOAuth || snapshot.Auth.Account != "user@example.com" {
		t.Fatalf("auth = %#v", snapshot.Auth)
	}
	if !equalStrings(snapshot.Auth.Services, []string{"drive", "gmail"}) {
		t.Fatalf("services = %#v", snapshot.Auth.Services)
	}
	if !equalStrings(snapshot.Auth.Scopes, []string{"scope-a", "scope-b"}) {
		t.Fatalf("scopes = %#v", snapshot.Auth.Scopes)
	}
	if snapshot.Auth.AccessTokenExpiresAt != expires.Format(time.RFC3339) {
		t.Fatalf("expiry = %q", snapshot.Auth.AccessTokenExpiresAt)
	}
}

func TestCapabilitiesMCPIsRedacted(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })
	openSecretsStore = func() (secrets.Store, error) {
		return nil, errors.New("keyring must not be opened")
	}

	snapshot, err := buildCapabilities(&RootFlags{
		Account: "private@example.com",
	}, capabilitiesOptions{
		MCPTools: []mcpToolSpec{
			{Name: "gog_capabilities", Service: "gog", Risk: mcpRiskRead},
			{Name: "docs_write", Service: "docs", Risk: mcpRiskWrite},
		},
	})
	if err != nil {
		t.Fatalf("buildCapabilities: %v", err)
	}
	if !snapshot.Auth.AccountSelected {
		t.Fatal("expected selected account state")
	}
	if snapshot.Auth.Account != "" || snapshot.Auth.Method != "" || len(snapshot.Auth.Scopes) != 0 {
		t.Fatalf("MCP snapshot disclosed auth: %#v", snapshot.Auth)
	}
	if snapshot.MCP == nil || len(snapshot.MCP.Tools) != 2 || !snapshot.MCP.WriteToolsExposed {
		t.Fatalf("MCP snapshot = %#v", snapshot.MCP)
	}
}

func TestCapabilitiesJSONOutput(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	snapshot, err := buildCapabilities(&RootFlags{}, capabilitiesOptions{})
	if err != nil {
		t.Fatalf("buildCapabilities: %v", err)
	}
	out := captureStdout(t, func() {
		if err := writeCapabilities(ctx, snapshot); err != nil {
			t.Fatalf("writeCapabilities: %v", err)
		}
	})

	var doc capabilitiesSnapshot
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, out)
	}
	if doc.SchemaVersion != capabilitiesSchemaVersion {
		t.Fatalf("schema version = %d", doc.SchemaVersion)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
