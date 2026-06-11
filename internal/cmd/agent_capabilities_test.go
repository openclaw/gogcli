package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestAgentCapabilitiesDefaultDoesNotOpenSecretsStore(t *testing.T) {
	withCapabilityTestHome(t)
	originalOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = originalOpen })
	openSecretsStore = func() (secrets.Store, error) {
		t.Fatal("default capability discovery must not open the secrets store")
		return nil, errors.New("unreachable")
	}

	snapshot, err := buildAgentCapabilities(&RootFlags{
		Account:             "private@example.com",
		EnableCommandsExact: "docs.cat,mcp",
		DisableCommands:     "gmail.send",
		GmailNoSend:         true,
	}, agentCapabilitiesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Disclosure.AuthInspected {
		t.Fatal("auth should not be inspected by default")
	}
	if snapshot.Auth.Account != "" {
		t.Fatalf("account leaked by default: %q", snapshot.Auth.Account)
	}
	if !snapshot.Safety.GmailNoSend {
		t.Fatal("gmail no-send state missing")
	}
	if got := snapshot.Safety.CommandRules.EnabledExact; len(got) != 2 || got[0] != "docs.cat" || got[1] != "mcp" {
		t.Fatalf("enabled exact = %#v", got)
	}
	if got := snapshot.Safety.CommandRules.Disabled; len(got) != 1 || got[0] != "gmail.send" {
		t.Fatalf("disabled = %#v", got)
	}
}

func TestAgentCapabilitiesIncludeOAuthMetadata(t *testing.T) {
	withCapabilityTestHome(t)
	store := newMemSecretsStore()
	expiry := time.Date(2026, 6, 11, 12, 0, 0, 0, time.FixedZone("offset", 2*60*60))
	if err := store.SetToken(config.DefaultClientName, "user@example.com", secrets.Token{
		Email:                "user@example.com",
		RefreshToken:         "secret-refresh-token",
		AccessToken:          "secret-access-token",
		AccessTokenExpiresAt: expiry,
		Services:             []string{"drive", "gmail", "drive"},
		Scopes:               []string{"scope:z", "scope:a", "scope:z"},
	}); err != nil {
		t.Fatal(err)
	}
	originalOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = originalOpen })
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	snapshot, err := buildAgentCapabilities(&RootFlags{Account: "user@example.com"}, agentCapabilitiesOptions{
		IncludeAuth:    true,
		IncludeAccount: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Auth.Method != authTypeOAuth {
		t.Fatalf("method = %q", snapshot.Auth.Method)
	}
	if snapshot.Auth.Account != "user@example.com" {
		t.Fatalf("account = %q", snapshot.Auth.Account)
	}
	if got := snapshot.Auth.Services; len(got) != 2 || got[0] != "drive" || got[1] != "gmail" {
		t.Fatalf("services = %#v", got)
	}
	if got := snapshot.Auth.Scopes; len(got) != 2 || got[0] != "scope:a" || got[1] != "scope:z" {
		t.Fatalf("scopes = %#v", got)
	}
	if snapshot.Auth.AccessTokenExpiresAt != "2026-06-11T10:00:00Z" {
		t.Fatalf("expiry = %q", snapshot.Auth.AccessTokenExpiresAt)
	}
}

func TestAgentCapabilitiesJSONIsNotWrappedAsUntrusted(t *testing.T) {
	withCapabilityTestHome(t)
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	ctx = outfmt.WithUntrustedWrapper(ctx, outfmt.UntrustedWrapOptions{Enabled: true})

	out := captureStdout(t, func() {
		if err := (&AgentCapabilitiesCmd{}).Run(ctx, &RootFlags{}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, `"external_untrusted_content"`) {
		t.Fatalf("local capability output was wrapped as untrusted: %s", out)
	}
	if !strings.Contains(out, `"schema_version": 1`) {
		t.Fatalf("missing capability schema: %s", out)
	}
}

func TestMCPCapabilitiesReportsFilteredToolSurface(t *testing.T) {
	withCapabilityTestHome(t)
	tools := mcpEnabledTools(McpCmd{AllowTool: []string{"agent"}})
	if len(tools) != 1 || tools[0].Name != "gog_capabilities" {
		t.Fatalf("tools = %#v", toolNames(tools))
	}

	result := mcpRunCapabilitiesTool(context.Background(), mcp.CallToolRequest{}, &RootFlags{}, tools)
	if result.IsError {
		t.Fatal("capabilities tool returned an error")
	}
	snapshot, ok := result.StructuredContent.(agentCapabilitiesSnapshot)
	if !ok {
		t.Fatalf("structured content type = %T", result.StructuredContent)
	}
	if snapshot.MCP == nil || len(snapshot.MCP.Tools) != 1 || snapshot.MCP.Tools[0].Name != "gog_capabilities" {
		t.Fatalf("mcp snapshot = %#v", snapshot.MCP)
	}
	if snapshot.MCP.WriteToolsExposed {
		t.Fatal("write tools should not be exposed")
	}
}

func withCapabilityTestHome(t *testing.T) {
	t.Helper()
	restore, err := config.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	t.Setenv("GOG_ACCOUNT", "")
	t.Setenv("GOG_AUTH_MODE", "")
}
