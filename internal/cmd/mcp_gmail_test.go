package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
)

func TestMCPGmailCapabilityCeilings(t *testing.T) {
	for _, selector := range []string{"gmail", "gmail.*", "write", "*", "all", "gmail_send_message", "gmail_send_draft", "gmail_delete_draft", "gmail_delete_messages"} {
		t.Run(selector, func(t *testing.T) {
			policy := config.MCPPolicy{AllowTools: []string{selector}, AllowWrite: true}
			configured, err := mcpEnabledToolsWithPolicy(McpCmd{}, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			for _, tools := range [][]mcpToolSpec{configured, mcpEnabledTools(McpCmd{AllowWrite: true, AllowTool: []string{selector}}, nil)} {
				for _, tool := range tools {
					if tool.Capability != "" {
						t.Fatalf("legacy grant exposed %s", tool.Name)
					}
				}
			}
		})
	}
	for _, tc := range []struct {
		name                   string
		send, delete, readonly bool
	}{
		{name: "send", send: true},
		{name: "delete", delete: true},
		{name: "both", send: true, delete: true},
		{name: "readonly", send: true, delete: true, readonly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := config.MCPPolicy{AllowTools: []string{"gmail"}, AllowWrite: true, AllowGmailSend: tc.send, AllowGmailDelete: tc.delete}
			flags := &RootFlags{ReadOnly: tc.readonly}
			configured, err := mcpEnabledToolsWithPolicy(McpCmd{}, flags, policy)
			if err != nil {
				t.Fatal(err)
			}
			runtime := mcpEnabledTools(McpCmd{AllowTool: policy.AllowTools, AllowWrite: true, AllowGmailSend: tc.send, AllowGmailDelete: tc.delete}, flags)
			for _, tools := range [][]mcpToolSpec{configured, runtime} {
				for _, name := range []string{"gmail_send_message", "gmail_send_draft"} {
					if hasMCPTool(tools, name) != (tc.send && !tc.readonly) {
						t.Fatalf("unexpected %s: %v", name, toolNames(tools))
					}
				}
				for _, name := range []string{"gmail_delete_draft", "gmail_delete_messages"} {
					if hasMCPTool(tools, name) != (tc.delete && !tc.readonly) {
						t.Fatalf("unexpected %s: %v", name, toolNames(tools))
					}
				}
				if hasMCPTool(tools, "gmail_create_draft") == tc.readonly {
					t.Fatalf("ordinary write selection: %v", toolNames(tools))
				}
			}
		})
	}
}

func TestMCPGmailPolicyRejectsWideningAndAccountInheritance(t *testing.T) {
	for _, cmd := range []McpCmd{{AllowGmailSend: true}, {AllowGmailDelete: true}} {
		_, err := mcpEnabledToolsWithPolicy(cmd, nil, config.MCPPolicy{AllowTools: []string{"*"}, AllowWrite: true})
		if err == nil || !strings.Contains(err.Error(), "cannot widen") {
			t.Fatalf("widening error: %v", err)
		}
	}
	for _, policy := range []config.MCPPolicy{
		{AllowTools: []string{"*"}, AllowGmailSend: true},
		{AllowTools: []string{"*"}, AllowGmailDelete: true},
	} {
		if _, err := normalizeMCPPolicy(policy); err == nil {
			t.Fatal("sensitive capability without writes accepted")
		}
	}
	policy, err := selectMCPPolicy(config.MCPConfig{
		MCPPolicy: config.MCPPolicy{AllowTools: []string{"*"}, AllowWrite: true, AllowGmailSend: true, AllowGmailDelete: true},
		Accounts:  map[string]config.MCPPolicy{"restricted@example.com": {AllowTools: []string{"gmail"}, AllowWrite: true}},
	}, "RESTRICTED@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if policy.AllowGmailSend || policy.AllowGmailDelete {
		t.Fatal("account inherited sensitive global capabilities")
	}
	tools, err := mcpEnabledToolsWithPolicy(McpCmd{AllowTool: []string{"gmail_create_draft"}}, nil, config.MCPPolicy{
		AllowTools: []string{"gmail"}, AllowWrite: true, AllowGmailSend: true, AllowGmailDelete: true,
	})
	if err != nil || !reflect.DeepEqual(toolNames(tools), []string{"gmail_create_draft"}) {
		t.Fatalf("runtime narrowing: %v, %v", toolNames(tools), err)
	}
}

func TestMCPGmailStartupAndDiscovery(t *testing.T) {
	for _, flag := range []string{"--allow-gmail-send", "--allow-gmail-delete"} {
		result := executeWithTestRuntime(t, []string{"--home", t.TempDir(), "mcp", flag, "--list-tools"}, nil)
		if ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "require --allow-write") {
			t.Fatalf("startup: %v", result.err)
		}
	}
	var output bytes.Buffer
	if err := mcpPrintTools(&output, mcpEnabledTools(McpCmd{AllowWrite: true, AllowGmailSend: true, AllowTool: []string{"gmail_send_message"}}, nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"capability": "gmail_send"`) {
		t.Fatalf("missing capability: %s", output.String())
	}
}

func TestMCPLegacyPolicyUpgradeThroughCLI(t *testing.T) {
	for _, selector := range []string{"gmail", "gmail.*", "write", "*", "all"} {
		for _, accountPolicy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/account=%v", selector, accountPolicy), func(t *testing.T) {
				home := t.TempDir()
				layout, err := config.NewResolver(config.Env{HomeOverride: home}, config.UserDirs{}).Resolve(config.PathKindConfig)
				if err != nil {
					t.Fatal(err)
				}
				// These are the pre-upgrade serialized policy fields, without new capability grants.
				policyJSON := fmt.Sprintf(`{"allow_tools":[%q],"allow_write":true}`, selector)
				configJSON := `{"mcp":` + policyJSON + `}`
				if accountPolicy {
					configJSON = `{"mcp":{"allow_tools":["read"],"accounts":{"legacy@example.com":` + policyJSON + `}}}`
				}
				if err := os.MkdirAll(layout.ConfigDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(layout.ConfigPath(), []byte(configJSON), 0o600); err != nil {
					t.Fatal(err)
				}
				result := executeWithTestRuntime(t, []string{
					"--home", home, "--account", "legacy@example.com", "mcp", "--list-tools",
				}, nil)
				if result.err != nil {
					t.Fatalf("legacy CLI policy: %v; stderr=%q", result.err, result.stderr)
				}
				var output struct {
					Tools []struct{ Name, Capability string }
				}
				if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
					t.Fatal(err)
				}
				enabled := make(map[string]bool, len(output.Tools))
				for _, tool := range output.Tools {
					enabled[tool.Name] = true
					if tool.Capability != "" {
						t.Fatalf("legacy policy gained sensitive capability %q through %s", tool.Capability, tool.Name)
					}
				}
				for _, name := range []string{
					"gmail_create_draft", "gmail_update_draft", "gmail_create_label",
					"gmail_modify_messages", "gmail_modify_thread", "gmail_mark_read", "gmail_mark_unread",
					"gmail_archive_messages", "gmail_trash_messages", "gmail_restore_messages",
				} {
					if !enabled[name] {
						t.Errorf("legacy broad write selector did not expose ordinary tool %s", name)
					}
				}
				for _, name := range []string{"gmail_send_message", "gmail_send_draft", "gmail_delete_draft", "gmail_delete_messages"} {
					if enabled[name] {
						t.Errorf("legacy broad write selector exposed gated tool %s", name)
					}
				}
			})
		}
	}
}

func TestMCPRuntimeSelectorsRejectEmptyNarrowing(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprintf("configured=%v", configured), func(t *testing.T) {
			home := t.TempDir()
			if configured {
				layout, err := config.NewResolver(config.Env{HomeOverride: home}, config.UserDirs{}).Resolve(config.PathKindConfig)
				if err != nil {
					t.Fatal(err)
				}
				store := config.NewConfigStore(layout)
				if err := store.Write(config.File{MCP: &config.MCPConfig{MCPPolicy: config.MCPPolicy{
					AllowTools: []string{"gmail_create_draft"}, AllowWrite: true,
				}}}); err != nil {
					t.Fatal(err)
				}
			}
			base := []string{"--home", home, "mcp", "--allow-write", "--list-tools"}
			for _, value := range []string{"", " ", ",,,", " , , "} {
				result := executeWithTestRuntime(t, append(append([]string{}, base...), "--allow-tool", value), nil)
				if ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "--allow-tool must contain at least one selector") || result.stdout != "" {
					t.Fatalf("empty narrowing %q: %v; stdout=%q", value, result.err, result.stdout)
				}
			}
			for _, selectors := range [][]string{nil, {"--allow-tool", ",", "--allow-tool", "gmail_create_draft"}} {
				result := executeWithTestRuntime(t, append(append([]string{}, base...), selectors...), nil)
				if result.err != nil || !strings.Contains(result.stdout, `"gmail_create_draft"`) {
					t.Fatalf("valid selector defaults/narrowing: %v; stdout=%q", result.err, result.stdout)
				}
				if (configured || selectors != nil) && strings.Contains(result.stdout, `"docs_write"`) {
					t.Fatalf("tool selection widened: %s", result.stdout)
				}
			}
		})
	}
}

func TestMCPGmailLiteralLabelsReachMessageAndThreadAPIs(t *testing.T) {
	for _, name := range []string{"gmail_modify_messages", "gmail_modify_thread"} {
		t.Run(name, func(t *testing.T) {
			var modified gmail.BatchModifyMessagesRequest
			var modifyPath string
			svc, closeServer := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/labels") {
					_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]string{
						{"id": "Label_Comma", "name": "Status, important"},
						{"id": "Label_Backslash", "name": `Path\Review`},
						{"id": "Label_BackslashComma", "name": `Path\, queued`},
					}})
					return
				}
				modifyPath = r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&modified); err != nil {
					t.Errorf("decode mutation: %v", err)
				}
				fmt.Fprint(w, `{}`)
			})
			defer closeServer()
			request := map[string]any{
				"add_labels":    []any{"Status, important", `Path\Review`},
				"remove_labels": []any{`Path\, queued`, "Label_MiXeD"},
			}
			wantPath := "/messages/batchModify"
			if name == "gmail_modify_thread" {
				request["thread_id"] = "t1"
				wantPath = "/threads/t1/modify"
			} else {
				request["message_ids"] = []any{"m1", "m2"}
			}
			args, err := findMCPTool(t, name).BuildArgs(mcpGmailRequest(name, request))
			if err != nil {
				t.Fatal(err)
			}
			// Legacy CSV and typed literal flags can coexist without changing either contract.
			args = append(append(args[:3:3], "--add=STARRED,UNREAD", "--remove=INBOX,IMPORTANT"), args[3:]...)
			root := mcpParentRootArgs(&RootFlags{Home: t.TempDir(), Account: "me@example.com"})
			result := executeWithGmailTestService(t, append(root, args...), svc)
			if result.err != nil {
				t.Fatalf("modify: %v; stderr=%q", result.err, result.stderr)
			}
			if !strings.HasSuffix(modifyPath, wantPath) || !slices.Equal(modified.AddLabelIds, []string{"STARRED", "UNREAD", "Label_Comma", "Label_Backslash"}) ||
				!slices.Equal(modified.RemoveLabelIds, []string{"INBOX", "IMPORTANT", "Label_BackslashComma", "Label_MiXeD"}) {
				t.Fatalf("mutation %s: %#v", modifyPath, modified)
			}
			if name == "gmail_modify_messages" && !slices.Equal(modified.Ids, []string{"m1", "m2"}) {
				t.Fatalf("message IDs: %q", modified.Ids)
			}
		})
	}
}

func mcpGmailRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}}
}

func TestMCPGmailTypedDryRuns(t *testing.T) {
	for _, tc := range []struct {
		name, op string
		args     map[string]any
	}{
		{"gmail_create_draft", "gmail.drafts.create", map[string]any{"subject": "Synthetic", "body": "  body\n"}},
		{"gmail_update_draft", "gmail.drafts.update", map[string]any{"draft_id": "d1", "subject": "Synthetic", "body_html": "<p>body</p>"}},
		{"gmail_create_label", "gmail.labels.create", map[string]any{"name": "Synthetic"}},
		{"gmail_modify_messages", "gmail.batch.modify", map[string]any{"message_ids": []any{"m1", "m2"}, "add_labels": []any{"Label_ABC"}, "remove_labels": []any{"UNREAD"}}},
		{"gmail_modify_thread", "gmail.thread.modify", map[string]any{"thread_id": "t1", "add_labels": []any{"Label_ABC"}}},
		{"gmail_mark_read", "gmail.read", map[string]any{"message_ids": []any{"m1"}}},
		{"gmail_mark_unread", "gmail.unread", map[string]any{"message_ids": []any{"m1"}}},
		{"gmail_archive_messages", "gmail.archive", map[string]any{"message_ids": []any{"m1"}}},
		{"gmail_trash_messages", "gmail.trash", map[string]any{"message_ids": []any{"m1"}}},
		{"gmail_restore_messages", "gmail.batch.modify", map[string]any{"message_ids": []any{"m1"}}},
		{"gmail_send_message", "gmail.send", map[string]any{"to": "self@example.com", "subject": "Synthetic", "body": "body"}},
		{"gmail_send_draft", "gmail.drafts.send", map[string]any{"draft_id": "d1"}},
		{"gmail_delete_draft", "gmail.drafts.delete", map[string]any{"draft_id": "d1"}},
		{"gmail_delete_messages", "gmail.batch.delete", map[string]any{"message_ids": []any{"m1"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, err := findMCPTool(t, tc.name).BuildArgs(mcpGmailRequest(tc.name, tc.args))
			if err != nil {
				t.Fatal(err)
			}
			root := mcpParentRootArgs(&RootFlags{Home: t.TempDir(), DryRun: true})
			result := executeWithTestRuntime(t, append(root, args...), nil)
			if result.err != nil {
				t.Fatalf("dry run: %v; %s", result.err, result.stderr)
			}
			var output map[string]any
			if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
				t.Fatal(err)
			}
			if output["op"] != tc.op {
				t.Fatalf("operation: %#v", output)
			}
			request := requireRequestMap(t, output)
			if tc.name == "gmail_update_draft" && request["to_keep_existing"] != true {
				t.Fatalf("omitted To lost: %#v", request)
			}
			if tc.name == "gmail_restore_messages" && !reflect.DeepEqual(requestStringSlice(t, request, "remove"), []string{"TRASH"}) {
				t.Fatalf("restore: %#v", request)
			}
		})
	}
}

func TestMCPGmailComposePreservesLiteralInput(t *testing.T) {
	args, err := findMCPTool(t, "gmail_update_draft").BuildArgs(mcpGmailRequest("", map[string]any{
		"draft_id": "--account=other@example.com", "subject": "--force", "body": "  @/tmp/private\n--account=other@example.com\n", "to": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--subject=--force", "--body=  @/tmp/private\n--account=other@example.com\n", "--to="} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing literal %q in %q", want, args)
		}
	}
	if !reflect.DeepEqual(args[len(args)-2:], []string{"--", "--account=other@example.com"}) {
		t.Fatalf("ID escaped positional boundary: %q", args)
	}
	root := mcpParentRootArgs(&RootFlags{Home: t.TempDir(), DryRun: true})
	result := executeWithTestRuntime(t, append(root, args...), nil)
	if result.err != nil || !strings.Contains(result.stdout, `"to_keep_existing": false`) {
		t.Fatalf("explicit empty To: %s, %v", result.stdout, result.err)
	}
}

func TestMCPGmailRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"gmail_create_draft", map[string]any{"subject": "x", "body": " "}},
		{"gmail_modify_messages", map[string]any{"message_ids": []any{"m1"}}},
		{"gmail_modify_thread", map[string]any{"thread_id": "t1", "add_labels": []any{" "}}},
		{"gmail_trash_messages", map[string]any{"message_ids": []any{}}},
		{"gmail_trash_messages", map[string]any{"message_ids": []any{1}}},
		{"gmail_trash_messages", map[string]any{"message_ids": []any{" "}}},
		{"gmail_trash_messages", map[string]any{"message_ids": make([]string, 1001)}},
		{"gmail_delete_draft", map[string]any{"draft_id": " "}},
	} {
		if _, err := findMCPTool(t, tc.name).BuildArgs(mcpGmailRequest(tc.name, tc.args)); err == nil {
			t.Fatalf("accepted %s input %#v", tc.name, tc.args)
		}
	}
}

func newMCPGmailTestClient(t *testing.T, s *server.MCPServer) *mcpclient.Client {
	t.Helper()
	client, err := mcpclient.NewInProcessClient(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	request := mcp.InitializeRequest{}
	request.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	request.Params.ClientInfo = mcp.Implementation{Name: "gmail-mcp-test", Version: "1"}
	if _, err := client.Initialize(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestMCPGmailSchemaRejectsUnauthorizedInput(t *testing.T) {
	s := newMCPServer()
	calls := 0
	for _, name := range []string{"gmail_create_draft", "gmail_delete_messages"} {
		s.AddTool(newMCPTool(findMCPTool(t, name)), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls++
			return mcp.NewToolResultText("unexpected"), nil
		})
	}
	client := newMCPGmailTestClient(t, s)
	for _, key := range []string{"account", "argv", "force", "body_file", "raw_file", "attach", "signature_file", "clear_attachments"} {
		result, err := client.CallTool(t.Context(), mcpGmailRequest("gmail_create_draft", map[string]any{"subject": "x", "body": "x", key: "bad"}))
		if err != nil || !result.IsError {
			t.Fatalf("accepted %s: %v, %v", key, result, err)
		}
	}
	for _, value := range []any{[]any{}, []any{1}, make([]string, 1001), "m1"} {
		result, err := client.CallTool(t.Context(), mcpGmailRequest("gmail_delete_messages", map[string]any{"message_ids": value}))
		if err != nil || !result.IsError {
			t.Fatalf("accepted IDs %T: %v, %v", value, result, err)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid input reached %d handlers", calls)
	}
}

func TestMCPGmailRPCExecutesFirstClassHandlers(t *testing.T) {
	var paths []string
	var modified gmail.BatchModifyMessagesRequest
	var deleted gmail.BatchDeleteMessagesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/settings/sendAs"):
			sendAsListHandler(w)
		case strings.HasSuffix(r.URL.Path, "/labels"):
			fmt.Fprint(w, `{"labels":[{"id":"Label_MiXeD","name":"Synthetic"}]}`)
		case strings.HasSuffix(r.URL.Path, "/messages/batchModify"):
			_ = json.NewDecoder(r.Body).Decode(&modified)
			fmt.Fprint(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/messages/batchDelete"):
			_ = json.NewDecoder(r.Body).Decode(&deleted)
			fmt.Fprint(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/drafts/send"):
			var draft gmail.Draft
			_ = json.NewDecoder(r.Body).Decode(&draft)
			if draft.Id != "d1" {
				t.Errorf("draft send ID: %q", draft.Id)
			}
			fmt.Fprint(w, `{"id":"sent1","threadId":"t1"}`)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/drafts/d1"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/drafts") || strings.HasSuffix(r.URL.Path, "/messages/send"):
			var msg gmail.Message
			if strings.HasSuffix(r.URL.Path, "/drafts") {
				var draft gmail.Draft
				_ = json.NewDecoder(r.Body).Decode(&draft)
				if draft.Message == nil {
					t.Error("missing draft message")
					return
				}
				msg = *draft.Message
			} else {
				_ = json.NewDecoder(r.Body).Decode(&msg)
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil || !strings.Contains(string(raw), "Subject: Synthetic") || !strings.Contains(string(raw), "synthetic body") {
				t.Errorf("invalid composed MIME: %q, %v", raw, err)
			}
			fmt.Fprint(w, `{"id":"d1","message":{"id":"m1"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	svc := newGmailServiceFromServer(t, srv)
	runtime := &app.Runtime{Services: app.Services{
		Gmail:       func(context.Context, string) (*gmail.Service, error) { return svc, nil },
		GmailDelete: func(context.Context, string) (*gmail.Service, error) { return svc, nil },
	}}
	s := newMCPServer()
	for _, tool := range mcpEnabledTools(McpCmd{AllowWrite: true, AllowGmailSend: true, AllowGmailDelete: true, AllowTool: []string{"gmail"}}, nil) {
		s.AddTool(newMCPTool(tool), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args, err := tool.BuildArgs(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			root := mcpParentRootArgs(&RootFlags{Home: t.TempDir(), Account: "me@example.com"})
			if tool.Capability == mcpCapabilityGmailDelete {
				root = append(root, "--force")
			}
			// The CLI entrypoint owns its root context, as it does in an MCP subprocess.
			result := executeWithTestRuntime(t, append(root, args...), runtime) //nolint:contextcheck
			if result.err != nil {
				return nil, result.err
			}
			return mcp.NewToolResultText(result.stdout), nil
		})
	}
	client := newMCPGmailTestClient(t, s)
	for _, req := range []mcp.CallToolRequest{
		mcpGmailRequest("gmail_create_draft", map[string]any{"to": "me@example.com", "subject": "Synthetic", "body": "synthetic body"}),
		mcpGmailRequest("gmail_send_message", map[string]any{"to": "me@example.com", "subject": "Synthetic", "body": "synthetic body"}),
		mcpGmailRequest("gmail_send_draft", map[string]any{"draft_id": "d1"}),
		mcpGmailRequest("gmail_modify_messages", map[string]any{"message_ids": []any{"m1", "m2"}, "add_labels": []any{"Label_MiXeD"}, "remove_labels": []any{"UNREAD"}}),
		mcpGmailRequest("gmail_delete_draft", map[string]any{"draft_id": "d1"}),
		mcpGmailRequest("gmail_delete_messages", map[string]any{"message_ids": []any{"m1"}}),
	} {
		result, err := client.CallTool(t.Context(), req)
		if err != nil || result.IsError {
			t.Fatalf("%s failed: %v, %v", req.Params.Name, result, err)
		}
	}
	if !reflect.DeepEqual(modified.Ids, []string{"m1", "m2"}) || !reflect.DeepEqual(modified.AddLabelIds, []string{"Label_MiXeD"}) || !reflect.DeepEqual(modified.RemoveLabelIds, []string{"UNREAD"}) {
		t.Fatalf("modified: %#v", modified)
	}
	if !reflect.DeepEqual(deleted.Ids, []string{"m1"}) {
		t.Fatalf("deleted: %#v", deleted)
	}
	if len(paths) != 9 {
		t.Fatalf("unexpected request paths: %v", paths)
	}
}

func TestMCPGmailChildProcess(t *testing.T) {
	if os.Getenv("GOG_MCP_GMAIL_TEST_CHILD") != "1" {
		return
	}
	delimiter := slices.Index(os.Args, "--")
	if delimiter < 0 {
		os.Exit(99)
	}
	os.Exit(ExitCode(Execute(os.Args[delimiter+1:])))
}

func TestMCPGmailSubprocessSafety(t *testing.T) {
	t.Setenv("GOG_MCP_GMAIL_TEST_CHILD", "1")
	t.Setenv("GOG_ACCOUNT", "")
	t.Setenv("GOG_ACCESS_TOKEN", "")
	t.Setenv("GOG_AUTH_MODE", "")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, tool, want string
		flags            RootFlags
		force            bool
	}{
		{name: "delete confirmation", tool: "gmail_delete_draft", want: "without --force"},
		{name: "operator force reaches account resolution", tool: "gmail_delete_draft", force: true, want: "missing --account"},
		{name: "forced delete dry-run", tool: "gmail_delete_draft", flags: RootFlags{DryRun: true}, force: true, want: `"dry_run":true`},
		{name: "no send", tool: "gmail_send_draft", flags: RootFlags{GmailNoSend: true}, want: "blocked by --gmail-no-send"},
		{name: "disabled", tool: "gmail_delete_draft", flags: RootFlags{DisableCommands: "gmail.drafts.delete"}, force: true, want: "is disabled"},
		{name: "exact allowlist", tool: "gmail_send_draft", flags: RootFlags{EnableCommandsExact: "gmail.drafts.create"}, want: "is not enabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.flags.Home = t.TempDir()
			tool := findMCPTool(t, tc.tool)
			args, err := tool.BuildArgs(mcpGmailRequest(tc.tool, map[string]any{"draft_id": "d1"}))
			if err != nil {
				t.Fatal(err)
			}
			base := append([]string{"-test.run=^TestMCPGmailChildProcess$", "--"}, mcpParentRootArgs(&tc.flags)...)
			result := mcpRunGogTool(t.Context(), mcpRunOptions{self: self, tool: tool, baseArgs: base, commandArgs: args, safetySuffix: mcpParentSafetyArgs(&tc.flags), timeout: 10 * time.Second, maxOutputBytes: 8192, force: tc.force})
			encoded, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), tc.want) {
				t.Fatalf("unexpected result: %s", encoded)
			}
			if tc.flags.DryRun == result.IsError {
				t.Fatalf("unexpected error state: %s", encoded)
			}
		})
	}
}
