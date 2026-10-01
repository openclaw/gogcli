package cmd

import (
	"context"
	"os"
	"sort"
	"testing"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/config"
)

// This is a saved policy in the pre-capability-extension format: the new
// notification and settings/Calendar deletion keys are deliberately absent.
func TestMCPSavedPolicyUpgrade(t *testing.T) {
	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	saved := `{"mcp":{"allow_tools":["*"],"allow_write":true,"accounts":{
 "broad@example.invalid":{"allow_tools":["*"],"allow_write":true},
 "narrow@example.invalid":{"allow_tools":["gmail_list_labels","gmail_list_drafts"],"allow_write":true},
 "readonly@example.invalid":{"allow_tools":["read"]}
 }}}`
	if err := os.WriteFile(store.Path(), []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCP == nil {
		t.Fatal("missing saved policy")
	}
	for _, row := range []struct {
		name, account string
		flags         RootFlags
		ordinary      bool
		narrow        bool
	}{
		{name: "global broad", ordinary: true},
		{name: "account broad", account: "broad@example.invalid", ordinary: true},
		{name: "complete narrow replacement", account: "narrow@example.invalid", narrow: true},
		{name: "complete readonly replacement", account: "readonly@example.invalid"},
		{name: "root readonly ceiling", flags: RootFlags{ReadOnly: true}},
	} {
		t.Run(row.name, func(t *testing.T) {
			policy, err := selectMCPPolicy(*cfg.MCP, row.account)
			if err != nil {
				t.Fatal(err)
			}
			tools, err := mcpEnabledToolsWithPolicy(McpCmd{}, &row.flags, policy)
			if err != nil {
				t.Fatal(err)
			}
			names := toolNames(tools)
			sort.Strings(names)
			t.Logf("saved policy catalog: %v", names)
			if row.narrow && (len(names) != 2 || !hasMCPTool(tools, "gmail_list_labels") || !hasMCPTool(tools, "gmail_list_drafts")) {
				t.Fatal("account policy inherited global permissions", names)
			}
			denied := []string{"calendar_respond", "calendar_cancel_event", "gmail_delete_label", "gmail_delete_filter", "gmail_send_message", "gmail_send_draft", "gmail_delete_messages", "gmail_delete_draft"}
			for _, name := range []string{"calendar_create_event", "calendar_update_event", "calendar_move_event", "gmail_create_filter", "gmail_rename_label"} {
				if hasMCPTool(tools, name) != row.ordinary {
					t.Errorf("ordinary tool %s present=%t, want %t", name, hasMCPTool(tools, name), row.ordinary)
				}
				if !row.ordinary {
					denied = append(denied, name)
				}
			}
			s := newMCPServer()
			providerCalls := 0
			for _, tool := range tools {
				s.AddTool(newMCPTool(tool), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					providerCalls++
					return mcp.NewToolResultText("unexpected provider execution"), nil
				})
			}
			client, err := mcpclient.NewInProcessClient(s)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := client.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			init := mcp.InitializeRequest{}
			init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
			init.Params.ClientInfo = mcp.Implementation{Name: "policy-upgrade-fixture", Version: "1"}
			if _, err := client.Initialize(t.Context(), init); err != nil {
				t.Fatal(err)
			}
			for _, name := range denied {
				if hasMCPTool(tools, name) {
					t.Fatalf("saved policy gained guarded tool %s", name)
				}
				req := mcp.CallToolRequest{}
				req.Params.Name = name
				req.Params.Arguments = map[string]any{}
				result, err := client.CallTool(t.Context(), req)
				if err == nil && (result == nil || !result.IsError) {
					t.Fatalf("hidden RPC %s accepted: %#v", name, result)
				}
			}
			if providerCalls != 0 {
				t.Fatalf("denied calls reached provider %d times", providerCalls)
			}
			t.Logf("denied RPCs=%d, provider calls=%d", len(denied), providerCalls)
		})
	}
}
