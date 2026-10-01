package cmd

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/config"
)

func TestMCPNewDestructiveCapabilityMatrix(t *testing.T) {
	for _, write := range []bool{false, true} {
		for _, grant := range []bool{false, true} {
			cmd := McpCmd{AllowTool: []string{"*"}, AllowWrite: write, AllowCalendarDelete: grant, AllowGmailSettingsDelete: grant}
			tools := mcpEnabledTools(cmd, nil)
			for _, name := range []string{"calendar_cancel_event", "gmail_delete_label", "gmail_delete_filter"} {
				if hasMCPTool(tools, name) != (write && grant) {
					t.Fatal(write, grant, name)
				}
			}
			if hasMCPTool(mcpEnabledTools(cmd, &RootFlags{ReadOnly: true}), "calendar_cancel_event") {
				t.Fatal("readonly exposed deletion")
			}
		}
	}
	policy := config.MCPPolicy{AllowTools: []string{"*"}, AllowWrite: true}
	for _, name := range []string{"calendar_cancel_event", "gmail_delete_label", "gmail_delete_filter"} {
		if hasMCPTool(mcpFilterTools(policy, nil, nil), name) {
			t.Fatal("legacy write glob gained deletion", name)
		}
	}
}

func TestMCPDeleteScopesAndProviderFailures(t *testing.T) {
	rt := &mcpToolRuntime{budget: 4096, calls: make(chan struct{}, 8), policy: config.MCPPolicy{AllowWrite: true, AllowCalendarDelete: true}}
	calls := 0
	rt.runChild = func(context.Context, mcpToolSpec, []string) (mcpCommandResult, error) {
		calls++
		return mcpCommandResult{}, nil
	}
	tool := findMCPTool(t, "calendar_cancel_event")
	req := mcp.CallToolRequest{}
	req.Params.Name = tool.Name
	req.Params.Arguments = map[string]any{"event_id": "e1"}
	if result := rt.runSpecial(context.Background(), tool, req); !result.IsError || calls != 0 {
		t.Fatal("no-input deletion reached provider without startup force")
	}
	rt.flags.Force = true
	req.Params.Arguments = map[string]any{"event_id": "e1", "send_updates": "all"}
	if result := rt.runSpecial(context.Background(), tool, req); !result.IsError || calls != 0 {
		t.Fatal("delete grant gained notifications")
	}
	req.Params.Arguments = map[string]any{"event_id": "e1", "scope": "single"}
	if _, err := tool.BuildArgs(req); err == nil {
		t.Fatal("missing original_start accepted")
	}
	req.Params.Arguments = map[string]any{"event_id": "e1", "force": true}
	if result := rt.runSpecial(context.Background(), tool, req); !result.IsError || calls != 0 {
		t.Fatal("model force accepted")
	}
}
