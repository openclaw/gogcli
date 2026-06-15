package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSplitCommandPath_SplitsWhitespaceWithinArgs(t *testing.T) {
	got := splitCommandPath([]string{" drive ls ", "  "})
	want := []string{"drive", "ls"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected token at %d: got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestSchemaCmdUsesRuntimeOutput(t *testing.T) {
	result := executeWithTestRuntime(t, []string{"schema", "drive ls"}, nil)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var doc schemaDoc
	if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if doc.Command == nil || doc.Command.Name != "ls" {
		t.Fatalf("command = %#v", doc.Command)
	}
}

func TestExecute_Schema_QuotedCommandPathToken(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"schema", "drive ls"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var doc struct {
		Command struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"command"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v out=%q", err, out)
	}
	if doc.Command.Name != "ls" {
		t.Fatalf("expected command name ls, got %q", doc.Command.Name)
	}
	if doc.Command.Path == "" {
		t.Fatalf("expected non-empty command path")
	}
}

func TestExecute_SchemaIncludesAutomationContract(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--dry-run",
				"--no-input",
				"--wrap-untrusted",
				"--gmail-no-send",
				"--enable-commands-exact", "schema,gmail.search",
				"--disable-commands", "gmail.send",
				"schema",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var doc schemaDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v out=%q", err, out)
	}
	if doc.Automation.ExitCodes["auth_required"] != exitCodeAuthRequired {
		t.Fatalf("auth_required = %d", doc.Automation.ExitCodes["auth_required"])
	}
	if doc.Automation.ExitCodes["cancelled"] != exitCodeCancelled {
		t.Fatalf("cancelled = %d", doc.Automation.ExitCodes["cancelled"])
	}
	if doc.Command == nil || doc.Command.Name != "gog" {
		t.Fatalf("schema command metadata was transformed: %#v", doc.Command)
	}
	assertSchemaAliases(t, doc.Command)
	if !doc.Automation.Safety.DryRun || !doc.Automation.Safety.NoInput || !doc.Automation.Safety.WrapUntrusted || !doc.Automation.Safety.GmailNoSend {
		t.Fatalf("safety = %#v", doc.Automation.Safety)
	}
	if got := strings.Join(doc.Automation.Safety.CommandRules.EnabledExact, ","); got != "gmail.search,schema" {
		t.Fatalf("enabled exact = %q", got)
	}
	if got := strings.Join(doc.Automation.Safety.CommandRules.Disabled, ","); got != "gmail.send" {
		t.Fatalf("disabled = %q", got)
	}
	var accountFlag *schemaFlag
	for i := range doc.Command.Flags {
		if doc.Command.Flags[i].Name == "account" {
			accountFlag = &doc.Command.Flags[i]
			break
		}
	}
	if accountFlag == nil {
		t.Fatal("root schema is missing --account")
	}
	if accountFlag.Help != "Account email, alias, or auto for authenticated Google API commands" {
		t.Fatalf("account help = %q", accountFlag.Help)
	}
}

func TestExecute_Schema_GmailTruncationHelp(t *testing.T) {
	threadDoc := schemaForCommand(t, "gmail thread get")
	threadFull := schemaFlagByName(t, threadDoc.Command, "full")
	if threadFull.Help != "Show full message bodies without truncation" {
		t.Fatalf("thread get --full help = %q", threadFull.Help)
	}

	messagesDoc := schemaForCommand(t, "gmail messages search")
	messagesFull := schemaFlagByName(t, messagesDoc.Command, "full")
	if messagesFull.Help != "Show full message bodies without truncation (implies --include-body)" {
		t.Fatalf("messages search --full help = %q", messagesFull.Help)
	}
	includeBody := schemaFlagByName(t, messagesDoc.Command, "include-body")
	if includeBody.Help != "Include decoded message body (JSON is full; text output is truncated)" {
		t.Fatalf("messages search --include-body help = %q", includeBody.Help)
	}
}

func schemaForCommand(t *testing.T, command string) schemaDoc {
	t.Helper()
	result := executeWithTestRuntime(t, []string{"schema", command}, nil)
	if result.err != nil {
		t.Fatalf("Execute schema %q: %v", command, result.err)
	}
	var doc schemaDoc
	if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
		t.Fatalf("decode schema %q: %v", command, err)
	}
	if doc.Command == nil {
		t.Fatalf("schema %q missing command", command)
	}
	return doc
}

func schemaFlagByName(t *testing.T, node *schemaNode, name string) schemaFlag {
	t.Helper()
	for _, flag := range node.Flags {
		if flag.Name == name {
			return flag
		}
	}
	t.Fatalf("%s missing --%s", node.Path, name)
	return schemaFlag{}
}

func TestExecute_SchemaRejectsPlainMode(t *testing.T) {
	var runErr error
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			runErr = Execute([]string{"schema", "--plain"})
		})
	})
	if runErr == nil || ExitCode(runErr) != 2 {
		t.Fatalf("expected usage error, got %v", runErr)
	}
	if !strings.Contains(errText, "schema does not support --plain") {
		t.Fatalf("unexpected stderr: %q", errText)
	}
}

func assertSchemaAliases(t *testing.T, node *schemaNode) {
	t.Helper()
	seen := make(map[string]bool, len(node.Aliases))
	for _, alias := range node.Aliases {
		if alias == node.Name {
			t.Fatalf("%s repeats canonical name %q as an alias", node.Path, alias)
		}
		if seen[alias] {
			t.Fatalf("%s repeats alias %q", node.Path, alias)
		}
		seen[alias] = true
	}
	for _, child := range node.Subcommands {
		assertSchemaAliases(t, child)
	}
}

func TestExecute_SchemaResolvesAccountNoSendAliasFromEnvironment(t *testing.T) {
	setTestConfigHome(t)
	t.Setenv("GOG_ACCOUNT", "work")
	store := defaultConfigStoreForTest(t)
	if err := store.SetAccountAlias("work", "user@example.com"); err != nil {
		t.Fatalf("SetAccountAlias: %v", err)
	}
	if err := store.SetNoSendAccount("user@example.com", true); err != nil {
		t.Fatalf("SetNoSendAccount: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"schema"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var doc schemaDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v out=%q", err, out)
	}
	if !doc.Automation.Safety.GmailNoSend {
		t.Fatalf("expected account no-send policy in schema: %#v", doc.Automation.Safety)
	}
}
