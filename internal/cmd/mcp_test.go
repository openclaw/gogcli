package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPEnabledToolsDefaultReadOnly(t *testing.T) {
	tools := mcpEnabledTools(McpCmd{})
	if len(tools) == 0 {
		t.Fatal("expected default tools")
	}
	for _, tool := range tools {
		if tool.Risk != mcpRiskRead {
			t.Fatalf("default enabled write tool %s", tool.Name)
		}
	}
	if hasMCPTool(tools, "docs_write") {
		t.Fatal("docs_write should require --allow-write")
	}
	if !hasMCPTool(tools, "gmail_search") {
		t.Fatal("gmail_search should be enabled by default")
	}
}

func TestMCPEnabledToolsAllowWriteAndFilter(t *testing.T) {
	tools := mcpEnabledTools(McpCmd{AllowWrite: true, AllowTool: []string{"docs.*"}})
	if !hasMCPTool(tools, "docs_get") || !hasMCPTool(tools, "docs_write") {
		t.Fatalf("expected docs read and write tools, got %#v", toolNames(tools))
	}
	if hasMCPTool(tools, "gmail_search") {
		t.Fatalf("gmail tool leaked through docs filter: %#v", toolNames(tools))
	}
}

func TestMCPListToolsUsesRuntimeStdout(t *testing.T) {
	var output bytes.Buffer
	err := (&McpCmd{
		ListTools:      true,
		TimeoutSeconds: 60,
		MaxOutputBytes: 1024,
	}).Run(newCmdRuntimeOutputContext(t, &output, io.Discard), &RootFlags{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); !strings.Contains(got, `"tools"`) || !strings.Contains(got, `"gmail_search"`) {
		t.Fatalf("unexpected tool list: %s", got)
	}
}

func TestMCPParentArgsPreserveContextAndSafety(t *testing.T) {
	flags := &RootFlags{
		Home:                "/tmp/gog-home",
		Account:             "bot@example.com",
		Client:              "test-client",
		ResultsOnly:         true,
		Select:              "messages",
		DryRun:              true,
		GmailNoSend:         true,
		EnableCommands:      "gmail.search,docs.cat",
		EnableCommandsExact: "mcp,gmail.messages.search",
		DisableCommands:     "drive.delete",
	}
	base := strings.Join(mcpParentRootArgs(flags), "\x00")
	for _, want := range []string{"--json", "--wrap-untrusted", "--no-input", "--color=never", "--home\x00/tmp/gog-home", "--account\x00bot@example.com", "--client\x00test-client", "--results-only", "--select\x00messages", "--dry-run"} {
		if !strings.Contains(base, want) {
			t.Fatalf("base args missing %q in %#v", want, mcpParentRootArgs(flags))
		}
	}
	safety := strings.Join(mcpParentSafetyArgs(flags), "\x00")
	for _, want := range []string{"--gmail-no-send", "--enable-commands=gmail.search,docs.cat", "--enable-commands-exact=mcp,gmail.messages.search", "--disable-commands=drive.delete"} {
		if !strings.Contains(safety, want) {
			t.Fatalf("safety args missing %q in %#v", want, mcpParentSafetyArgs(flags))
		}
	}
}

func TestMCPToolBuildArgsTypedOnly(t *testing.T) {
	tool := findMCPTool(t, "sheets_update_range")
	args, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"spreadsheet_id": "sheet1",
			"range":          "Sheet1!A1:B1",
			"values_json":    `[[1,2]]`,
			"input":          "RAW",
			"args":           []any{"drive", "delete", "file"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, " ")
	if strings.Contains(got, "drive delete") {
		t.Fatalf("generic args leaked into typed tool argv: %#v", args)
	}
	want := []string{"sheets", "update", "--values-json", "[[1,2]]", "--input", "RAW", "--", "sheet1", "Sheet1!A1:B1"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestMCPServerValidatesToolInputSchema(t *testing.T) {
	s := newMCPServer()
	handlerCalls := 0
	s.AddTool(newMCPTool(findMCPTool(t, "docs_write")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls++
		return mcp.NewToolResultText("ok"), nil
	})

	client, err := mcpclient.NewInProcessClient(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})
	if err := client.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "gog-test", Version: "1"}
	if _, err := client.Initialize(t.Context(), initRequest); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		arguments map[string]any
		wantError bool
		wantText  string
	}{
		{
			name: "unknown field",
			arguments: map[string]any{
				"document_id": "doc1",
				"text":        "hello",
				"argv":        []any{"drive", "delete", "file"},
			},
			wantError: true,
			wantText:  "argv",
		},
		{
			name: "wrong type",
			arguments: map[string]any{
				"document_id": "doc1",
				"text":        "hello",
				"append":      "yes",
			},
			wantError: true,
			wantText:  "append",
		},
		{
			name: "missing required field",
			arguments: map[string]any{
				"text": "hello",
			},
			wantError: true,
			wantText:  "document_id",
		},
		{
			name: "valid",
			arguments: map[string]any{
				"document_id": "doc1",
				"text":        "hello",
				"append":      true,
			},
			wantText: "ok",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := handlerCalls
			result, err := client.CallTool(t.Context(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "docs_write",
					Arguments: tt.arguments,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError != tt.wantError {
				t.Fatalf("IsError = %v, want %v: %#v", result.IsError, tt.wantError, result.Content)
			}
			if tt.wantError && handlerCalls != before {
				t.Fatal("invalid arguments reached the tool handler")
			}
			if !strings.Contains(mcpResultText(result), tt.wantText) {
				t.Fatalf("result = %#v, want text containing %q", result.Content, tt.wantText)
			}
		})
	}
	if handlerCalls != 1 {
		t.Fatalf("handler calls = %d, want 1", handlerCalls)
	}
}

func TestMCPDocsWritePreservesTextWhitespace(t *testing.T) {
	tool := findMCPTool(t, "docs_write")
	args, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"document_id": "doc1",
			"text":        "  indented\n",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range args {
		if arg == "--text" && i+1 < len(args) {
			if args[i+1] != "  indented\n" {
				t.Fatalf("text = %q", args[i+1])
			}
			return
		}
	}
	t.Fatalf("missing --text in %#v", args)
}

func TestMCPDocsWriteRejectsNeitherAppendNorReplace(t *testing.T) {
	tool := findMCPTool(t, "docs_write")
	_, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"document_id": "doc1",
			"text":        "hello",
			"append":      false,
			"replace":     false,
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "append=false") {
		t.Fatalf("expected append=false error, got %v", err)
	}
}

func TestMCPDocsGetRejectsTabWithAllTabs(t *testing.T) {
	tool := findMCPTool(t, "docs_get")
	_, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"document_id": "doc1",
			"tab":         "Overview",
			"all_tabs":    true,
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected tab/all_tabs error, got %v", err)
	}

	_, err = tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"document_id": "doc1",
			"tab":         "",
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "tab cannot be empty") {
		t.Fatalf("expected empty tab error, got %v", err)
	}
}

func TestMCPSheetsUpdateRejectsFileExpansion(t *testing.T) {
	tool := findMCPTool(t, "sheets_update_range")
	_, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"spreadsheet_id": "sheet1",
			"range":          "Sheet1!A1",
			"values_json":    "@/tmp/secret.json",
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "literal JSON") {
		t.Fatalf("expected literal JSON error, got %v", err)
	}
}

func TestMCPSheetsUpdatePreservesLargeJSONNumbers(t *testing.T) {
	tool := findMCPTool(t, "sheets_update_range")
	args, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"spreadsheet_id": "sheet1",
			"range":          "Sheet1!A1",
			"values_json":    `[[1234567890123456789]]`,
			"input":          "RAW",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range args {
		if arg == "--values-json" && i+1 < len(args) {
			if args[i+1] != `[[1234567890123456789]]` {
				t.Fatalf("values_json = %q", args[i+1])
			}
			return
		}
	}
	t.Fatalf("missing --values-json in %#v", args)
}

func TestMCPSheetsUpdateRejectsTrailingJSON(t *testing.T) {
	tool := findMCPTool(t, "sheets_update_range")
	_, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"spreadsheet_id": "sheet1",
			"range":          "Sheet1!A1",
			"values_json":    `[[1]] garbage`,
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "trailing content") {
		t.Fatalf("expected trailing content error, got %v", err)
	}
}

func TestMCPLimitedBufferCapsDuringWrite(t *testing.T) {
	buf := newMCPLimitedBuffer(5)
	n, err := buf.Write([]byte("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	if n != len("hello world") {
		t.Fatalf("Write returned %d", n)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "hello") || !strings.Contains(got, "truncated") {
		t.Fatalf("unexpected buffer: %q", got)
	}
}

func hasMCPTool(tools []mcpToolSpec, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func toolNames(tools []mcpToolSpec) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}

func findMCPTool(t *testing.T, name string) mcpToolSpec {
	t.Helper()
	for _, tool := range mcpAllTools() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return mcpToolSpec{}
}

func mcpResultText(result *mcp.CallToolResult) string {
	var text strings.Builder
	for _, content := range result.Content {
		if item, ok := content.(mcp.TextContent); ok {
			text.WriteString(item.Text)
		}
	}
	return text.String()
}
