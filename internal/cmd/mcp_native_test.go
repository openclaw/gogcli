package cmd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPCatalogUniqueNames(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range mcpAllTools() {
		if seen[tool.Name] {
			t.Fatal("duplicate", tool.Name)
		}
		seen[tool.Name] = true
	}
	if err := validateMCPCatalog(mcpAllTools()); err != nil {
		t.Fatal(err)
	}
	invalid := mcpToolSpec{Name: "invalid"}
	if err := validateMCPCatalog([]mcpToolSpec{invalid}); err == nil {
		t.Fatal("missing handler accepted")
	}
	tool := mcpGmailSearchTool()
	tool.Handle = func(context.Context, mcp.CallToolRequest, *mcpToolRuntime) *mcp.CallToolResult { return nil }
	if err := validateMCPCatalog([]mcpToolSpec{tool}); err == nil {
		t.Fatal("ambiguous dispatch accepted")
	}
	tools := mcpEnabledTools(McpCmd{MaxOutputBytes: 100}, nil)
	if hasMCPTool(tools, "gmail_get_raw") {
		t.Fatal("bounded tool below minimum advertised")
	}
	if !hasMCPTool(mcpEnabledTools(McpCmd{}, nil), "gmail_get_raw") {
		t.Fatal("default budget omits export")
	}
}

func TestMCPExportSafetyOnCacheHit(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 100 << 20, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rt := &mcpToolRuntime{store: store, partition: "p", budget: 4096, flags: RootFlags{}, calls: make(chan struct{}, 8)}
	var fetches int
	rt.fetchExport = func(context.Context, gmailExportKey) ([]byte, gmailExportMetadata, error) {
		fetches++
		return []byte("hello"), gmailExportMetadata{MessageID: "m1"}, nil
	}
	tool := mcpExportTools()[0]
	req := mcp.CallToolRequest{}
	req.Params.Name = tool.Name
	req.Params.Arguments = map[string]any{"message_id": "m1"}
	result := rt.runSpecial(context.Background(), tool, req)
	if result.IsError {
		t.Fatal(result)
	}
	rt.flags.DisableCommands = "gmail.export.raw"
	req.Params.Arguments = map[string]any{"message_id": "m1", "snapshot_id": result.StructuredContent.(mcpCommandResult).Stdout.(mcpExportChunk).SnapshotID, "offset": json.Number("1")}
	if result = rt.runSpecial(context.Background(), tool, req); !result.IsError {
		t.Fatal("cache bypassed deny")
	}
	if fetches != 1 {
		t.Fatal(fetches)
	}
	rt.flags.DisableCommands = ""
	rt.flags.Select = "data_base64"
	if result = rt.runSpecial(context.Background(), tool, req); !result.IsError || fetches != 1 {
		t.Fatal("transform reached provider")
	}
}

func TestCommandPathPolicy(t *testing.T) {
	for _, row := range []struct {
		enabled, exact, disabled string
		ok                       bool
	}{{"gmail", "", "", true}, {"", "gmail.export.raw", "", true}, {"", "gmail.export", "", false}, {"*", "", "gmail.export", false}, {",", "", "", false}} {
		err := enforceCommandPathPolicy([]string{"gmail", "export", "raw"}, row.enabled, row.exact, row.disabled)
		if (err == nil) != row.ok {
			t.Fatal(row, err)
		}
	}
}
