package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
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

func TestMCPPolicyDefaultsToReadOnly(t *testing.T) {
	policy, err := selectMCPPolicy(config.MCPConfig{}, "")
	if err != nil {
		t.Fatalf("selectMCPPolicy: %v", err)
	}
	tools, err := mcpEnabledToolsWithPolicy(McpCmd{}, &RootFlags{}, policy)
	if err != nil {
		t.Fatalf("mcpEnabledToolsWithPolicy: %v", err)
	}
	if !hasMCPTool(tools, "gmail_search") || hasMCPTool(tools, "docs_write") {
		t.Fatalf("unexpected default policy tools: %#v", toolNames(tools))
	}
}

func TestMCPPolicyAccountReplacesGlobalAndEnablesNarrowWrites(t *testing.T) {
	cfg := config.MCPConfig{
		MCPPolicy: config.MCPPolicy{AllowTools: []string{"read"}},
		Accounts: map[string]config.MCPPolicy{
			" Personal@Example.com ": {AllowTools: []string{"docs.*"}, AllowWrite: true},
		},
	}
	policy, err := selectMCPPolicy(cfg, "personal@example.com")
	if err != nil {
		t.Fatalf("selectMCPPolicy: %v", err)
	}
	tools, err := mcpEnabledToolsWithPolicy(McpCmd{}, &RootFlags{}, policy)
	if err != nil {
		t.Fatalf("mcpEnabledToolsWithPolicy: %v", err)
	}
	if !hasMCPTool(tools, "docs_get") || !hasMCPTool(tools, "docs_write") {
		t.Fatalf("expected configured Docs tools: %#v", toolNames(tools))
	}
	if hasMCPTool(tools, "gmail_search") {
		t.Fatalf("global policy leaked into account replacement: %#v", toolNames(tools))
	}
}

func TestMCPPolicyRuntimeCanOnlyNarrow(t *testing.T) {
	policy, err := normalizeMCPPolicy(config.MCPPolicy{AllowTools: []string{"docs.*"}, AllowWrite: true})
	if err != nil {
		t.Fatalf("normalizeMCPPolicy: %v", err)
	}
	tools, err := mcpEnabledToolsWithPolicy(McpCmd{AllowTool: []string{"docs_get"}}, &RootFlags{}, policy)
	if err != nil {
		t.Fatalf("mcpEnabledToolsWithPolicy: %v", err)
	}
	if got := toolNames(tools); len(got) != 1 || got[0] != "docs_get" {
		t.Fatalf("runtime narrowed tools = %#v", got)
	}

	_, err = mcpEnabledToolsWithPolicy(McpCmd{AllowWrite: true}, &RootFlags{}, config.MCPPolicy{AllowTools: []string{"read"}})
	if err == nil || !strings.Contains(err.Error(), "cannot widen") {
		t.Fatalf("allow-write widening error = %v", err)
	}
}

func TestMCPPolicyReadOnlyRootHidesConfiguredWrites(t *testing.T) {
	policy, err := normalizeMCPPolicy(config.MCPPolicy{AllowTools: []string{"docs.*"}, AllowWrite: true})
	if err != nil {
		t.Fatalf("normalizeMCPPolicy: %v", err)
	}
	tools, err := mcpEnabledToolsWithPolicy(McpCmd{}, &RootFlags{ReadOnly: true}, policy)
	if err != nil {
		t.Fatalf("mcpEnabledToolsWithPolicy: %v", err)
	}
	if !hasMCPTool(tools, "docs_get") || hasMCPTool(tools, "docs_write") {
		t.Fatalf("readonly tools = %#v", toolNames(tools))
	}
}

func TestMCPPolicyRejectsUnsafeOrUnknownConfig(t *testing.T) {
	for _, policy := range []config.MCPPolicy{
		{AllowWrite: true},
		{AllowTools: []string{}},
		{AllowTools: []string{"not_a_tool"}},
	} {
		if _, err := normalizeMCPPolicy(policy); err == nil {
			t.Fatalf("expected policy error for %#v", policy)
		}
	}

	duplicateAccount := " user@example.com "
	_, err := selectMCPPolicy(config.MCPConfig{Accounts: map[string]config.MCPPolicy{
		"User@example.com": {},
		duplicateAccount:   {},
	}}, "user@example.com")
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate account error = %v", err)
	}

	_, err = selectMCPPolicy(config.MCPConfig{
		Accounts: map[string]config.MCPPolicy{
			"selected@example.com": {AllowTools: []string{"read"}},
			"other@example.com":    {AllowTools: []string{"not_a_tool"}},
		},
	}, "selected@example.com")
	if err == nil || !strings.Contains(err.Error(), "other@example.com") || !strings.Contains(err.Error(), "matches no tool") {
		t.Fatalf("unselected account validation error = %v", err)
	}
}

func TestMCPPolicyAccountResolutionPinsAliasAndRejectsUnverifiableIdentity(t *testing.T) {
	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := store.Write(config.File{AccountAliases: map[string]string{"personal": "Personal@Example.com"}}); err != nil {
		t.Fatalf("write config: %v", err)
	}
	flags := &RootFlags{
		Account: "personal",
		configStoreResolver: func() (*config.ConfigStore, error) {
			return store, nil
		},
	}
	account, err := resolveMCPPolicyAccount(flags)
	if err != nil {
		t.Fatalf("resolveMCPPolicyAccount: %v", err)
	}
	if account != "Personal@Example.com" {
		t.Fatalf("resolved account = %q", account)
	}

	for _, unverifiable := range []*RootFlags{
		{AccessToken: "token", Account: "label@example.com"},
		{authMode: googleapi.AuthModeADC, Account: "label@example.com"},
	} {
		account, err := resolveMCPPolicyAccount(unverifiable)
		if err != nil || account != "" {
			t.Fatalf("unverifiable identity resolution = %q, %v", account, err)
		}
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
		ReadOnly:            true,
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
	for _, want := range []string{"--gmail-no-send", "--readonly", "--enable-commands=gmail.search,docs.cat", "--enable-commands-exact=mcp,gmail.messages.search", "--disable-commands=drive.delete"} {
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

func TestMCPGmailDraftToolsAreExactOptIn(t *testing.T) {
	write := findMCPTool(t, "gmail_create_reply_draft")
	if write.Risk != mcpRiskWrite || !write.ExactOptIn {
		t.Fatalf("gmail_create_reply_draft = risk %q optIn %v, want write/true", write.Risk, write.ExactOptIn)
	}
	read := findMCPTool(t, "gmail_get_draft")
	if read.Risk != mcpRiskRead || !read.ExactOptIn {
		t.Fatalf("gmail_get_draft = risk %q optIn %v, want read/true", read.Risk, read.ExactOptIn)
	}
	// No selector short of the literal tool name may surface either tool —
	// upgrading gog must never widen an existing configuration's surface.
	broad := [][]string{nil, {"gmail"}, {"gmail.*"}, {"write"}, {"read"}, {"all"}, {"*"}}
	for _, allow := range broad {
		tools := mcpEnabledTools(McpCmd{AllowWrite: true, AllowTool: allow})
		for _, name := range []string{"gmail_create_reply_draft", "gmail_get_draft"} {
			if hasMCPTool(tools, name) {
				t.Fatalf("%s leaked through selector %#v", name, allow)
			}
		}
	}
	named := mcpEnabledTools(McpCmd{AllowWrite: true, AllowTool: []string{"gmail_create_reply_draft", "gmail_get_draft"}})
	if !hasMCPTool(named, "gmail_create_reply_draft") || !hasMCPTool(named, "gmail_get_draft") {
		t.Fatalf("exact names did not enable the draft tools: %#v", toolNames(named))
	}
	withoutWrite := mcpEnabledTools(McpCmd{AllowTool: []string{"gmail_create_reply_draft", "gmail_get_draft"}})
	if hasMCPTool(withoutWrite, "gmail_create_reply_draft") {
		t.Fatalf("draft create visible without --allow-write: %#v", toolNames(withoutWrite))
	}
	if !hasMCPTool(withoutWrite, "gmail_get_draft") {
		t.Fatalf("draft get should not need --allow-write: %#v", toolNames(withoutWrite))
	}
	for _, forbidden := range []string{"gmail_send", "gmail_drafts_send", "gmail_delete_draft", "gmail_update_draft", "gmail_reply_all_draft"} {
		if hasMCPTool(mcpEnabledTools(McpCmd{AllowWrite: true, AllowTool: []string{"*"}}), forbidden) {
			t.Fatalf("%s must not exist as an MCP tool", forbidden)
		}
	}
}

func TestMCPGmailReplyDraftUsesDedicatedReplyCommand(t *testing.T) {
	tool := findMCPTool(t, "gmail_create_reply_draft")
	args, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"reply_to_message_id": "msg1",
			"body":                "  ご確認ありがとうございます。\n",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// The dedicated reply command derives the recipients from the original
	// message; a create with only thread headers would save a recipientless
	// draft. --no-quote keeps the body exactly the approved text.
	want := []string{"gmail", "drafts", "reply", "--body", "  ご確認ありがとうございます。\n", "--no-quote", "--", "msg1"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestMCPGmailReplyDraftArgvHoldsOnlySchemaFields(t *testing.T) {
	tool := findMCPTool(t, "gmail_create_reply_draft")
	args, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{
			"reply_to_message_id": "msg1",
			"body":                "hello",
			// None of the following are in the schema. The MCP server rejects
			// them before the handler; even if a caller reached BuildArgs
			// directly, none of them may surface in argv.
			"reply_all": true,
			"to":        "someone@example.com",
			"cc":        "copy@example.com",
			"bcc":       "hidden@example.com",
			"attach":    []any{"/etc/passwd"},
			"from":      "spoofed@example.com",
			"quote":     true,
			"args":      []any{"gmail", "drafts", "send", "draft1"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gmail", "drafts", "reply", "--body", "hello", "--no-quote", "--", "msg1"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for _, forbidden := range []string{"--reply-all", "--to", "--cc", "--bcc", "--attach", "--from", "--quote", "send"} {
		for _, arg := range args {
			if arg == forbidden {
				t.Fatalf("%q leaked into draft argv: %#v", forbidden, args)
			}
		}
	}
}

func TestMCPGmailDraftGetBuildsExactArgs(t *testing.T) {
	if _, err := findMCPTool(t, "gmail_create_reply_draft").BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{"reply_to_message_id": "msg1", "body": "   "},
	}}); err == nil {
		t.Fatal("blank body accepted")
	}
	if _, err := findMCPTool(t, "gmail_get_draft").BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{},
	}}); err == nil {
		t.Fatal("missing draft_id accepted")
	}
	args, err := findMCPTool(t, "gmail_get_draft").BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{"draft_id": "draft1", "download": true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gmail", "drafts", "get", "--", "draft1"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestMCPServerRejectsUnknownReplyDraftFieldsBeforeHandler(t *testing.T) {
	s := newMCPServer()
	handlerCalls := 0
	s.AddTool(newMCPTool(findMCPTool(t, "gmail_create_reply_draft")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	}{
		{name: "recipient override", arguments: map[string]any{"reply_to_message_id": "msg1", "body": "hi", "to": "x@example.com"}, wantError: true},
		{name: "reply_all", arguments: map[string]any{"reply_to_message_id": "msg1", "body": "hi", "reply_all": true}, wantError: true},
		{name: "attachment", arguments: map[string]any{"reply_to_message_id": "msg1", "body": "hi", "attach": []any{"/etc/passwd"}}, wantError: true},
		{name: "argv smuggling", arguments: map[string]any{"reply_to_message_id": "msg1", "body": "hi", "args": []any{"gmail", "drafts", "send"}}, wantError: true},
		{name: "wrong type", arguments: map[string]any{"reply_to_message_id": "msg1", "body": 7}, wantError: true},
		{name: "missing target", arguments: map[string]any{"body": "hi"}, wantError: true},
		{name: "valid", arguments: map[string]any{"reply_to_message_id": "msg1", "body": "hi"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := handlerCalls
			result, err := client.CallTool(t.Context(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: "gmail_create_reply_draft", Arguments: tt.arguments},
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
		})
	}
	if handlerCalls != 1 {
		t.Fatalf("handler calls = %d, want 1", handlerCalls)
	}
}

func TestMCPGmailGetDraftRecoveryModeIsExclusive(t *testing.T) {
	tool := findMCPTool(t, "gmail_get_draft")
	// draft_id -> drafts get
	got, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{"draft_id": "d1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\x00") != strings.Join([]string{"gmail", "drafts", "get", "--", "d1"}, "\x00") {
		t.Fatalf("get args = %#v", got)
	}
	// recover -> bounded drafts find
	rec, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{"recover_reply_to_message_id": "m1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rec, "\x00") != strings.Join([]string{"gmail", "drafts", "find", "--reply-to-message-id", "m1"}, "\x00") {
		t.Fatalf("recover args = %#v", rec)
	}
	// neither -> error
	if _, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{}}}); err == nil {
		t.Fatal("empty args accepted")
	}
	// both -> error (mutually exclusive)
	if _, err := tool.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]any{"draft_id": "d1", "recover_reply_to_message_id": "m1"},
	}}); err == nil {
		t.Fatal("both args accepted")
	}
}
