package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
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
	if !doc.Automation.Safety.DryRun || !doc.Automation.Safety.NoInput || !doc.Automation.Safety.WrapUntrusted || !doc.Automation.Safety.GmailNoSend {
		t.Fatalf("safety = %#v", doc.Automation.Safety)
	}
	if got := strings.Join(doc.Automation.Safety.CommandRules.EnabledExact, ","); got != "gmail.search,schema" {
		t.Fatalf("enabled exact = %q", got)
	}
	if got := strings.Join(doc.Automation.Safety.CommandRules.Disabled, ","); got != "gmail.send" {
		t.Fatalf("disabled = %q", got)
	}
}

func TestExecute_SchemaResolvesAccountNoSendAliasFromEnvironment(t *testing.T) {
	setTestConfigHome(t)
	t.Setenv("GOG_ACCOUNT", "work")
	if err := config.SetAccountAlias("work", "user@example.com"); err != nil {
		t.Fatalf("SetAccountAlias: %v", err)
	}
	if err := config.SetNoSendAccount("user@example.com", true); err != nil {
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
