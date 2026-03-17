package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestIsGmailSendCommand(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		// Blocked send paths
		{"send", true},
		{"send --to x@y.com --subject S --body B", true},
		{"gmail send", true},
		{"gmail send --to x@y.com", true},
		{"gmail drafts send <draftId>", true},
		{"gmail autoreply <query>", true},

		// Non-send paths (must NOT match)
		{"gmail search <query>", false},
		{"gmail drafts create", false},
		{"gmail drafts list", false},
		{"gmail drafts get <draftId>", false},
		{"gmail drafts update <draftId>", false},
		{"gmail labels ls", false},
		{"gmail get <messageId>", false},
		{"gmail thread get <threadId>", false},
		{"gmail batch", false},
		{"gmail watch serve", false},
		{"drive ls", false},
		{"calendar events", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isGmailSendCommand(tc.command); got != tc.want {
			t.Errorf("isGmailSendCommand(%q) = %v, want %v", tc.command, got, tc.want)
		}
	}
}

func TestGmailNoSendBlocksViaCLI(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"gmail send", []string{
			"--gmail-no-send", "gmail", "send",
			"--to", "x@y.com", "--subject", "S", "--body", "B",
		}},
		{"top-level send alias", []string{
			"--gmail-no-send", "send",
			"--to", "x@y.com", "--subject", "S", "--body", "B",
		}},
		{"gmail drafts send", []string{"--gmail-no-send", "gmail", "drafts", "send", "DRAFT123"}},
		{"gmail autoreply", []string{
			"--gmail-no-send", "gmail", "autoreply",
			"from:someone", "--body", "reply",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Execute(tc.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "send blocked") {
				t.Fatalf("expected 'send blocked' error, got: %v", err)
			}
		})
	}
}

func TestGmailNoSendAllowsNonSendCommands(t *testing.T) {
	err := Execute([]string{"--gmail-no-send", "--account", "a@b.com", "gmail", "search", "test"})
	if err == nil {
		return // unlikely without auth, but acceptable
	}
	if strings.Contains(err.Error(), "send blocked") {
		t.Fatalf("non-send command should not be blocked: %v", err)
	}
}

func TestGmailNoSendEnvVar(t *testing.T) {
	t.Setenv("GOG_GMAIL_NO_SEND", "1")
	err := Execute([]string{
		"gmail", "send",
		"--to", "x@y.com", "--subject", "S", "--body", "B",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "send blocked") {
		t.Fatalf("expected 'send blocked' error, got: %v", err)
	}
}

func TestGmailNoSendNotSetAllowsSend(t *testing.T) {
	// Ensure the env var is not set (may be set in the shell).
	t.Setenv("GOG_GMAIL_NO_SEND", "")
	// Without --gmail-no-send, send should NOT be blocked
	// (it will fail for other reasons like missing auth)
	err := Execute([]string{
		"gmail", "send",
		"--to", "x@y.com", "--subject", "S", "--body", "B",
	})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "send blocked") {
		t.Fatalf("send should not be blocked without --gmail-no-send: %v", err)
	}
}

// writeNoSendConfig writes a config.json to a temp dir and sets
// XDG_CONFIG_HOME so config.ReadConfig() picks it up.
func writeNoSendConfig(t *testing.T, accounts map[string]bool) {
	t.Helper()

	writeNoSendConfigFull(t, false, accounts)
}

func writeNoSendConfigFull(t *testing.T, globalNoSend bool, accounts map[string]bool) {
	t.Helper()

	xdgHome := t.TempDir()
	configDir := filepath.Join(xdgHome, "gogcli")

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.File{
		GmailNoSend:    globalNoSend,
		NoSendAccounts: accounts,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdgHome)
}

func TestCheckAccountNoSend_Blocked(t *testing.T) {
	writeNoSendConfig(t, map[string]bool{"blocked@gmail.com": true})

	err := checkAccountNoSend("blocked@gmail.com")
	if err == nil {
		t.Fatal("expected error for blocked account")
	}
	if !strings.Contains(err.Error(), "send blocked") {
		t.Fatalf("expected 'send blocked' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "blocked@gmail.com") {
		t.Fatalf("expected error to mention account, got: %v", err)
	}
}

func TestCheckAccountNoSend_Allowed(t *testing.T) {
	writeNoSendConfig(t, map[string]bool{"blocked@gmail.com": true})

	err := checkAccountNoSend("allowed@gmail.com")
	if err != nil {
		t.Fatalf("expected no error for allowed account, got: %v", err)
	}
}

func TestCheckAccountNoSend_NoConfig(t *testing.T) {
	// Point to empty config dir — no config.json exists.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := checkAccountNoSend("any@gmail.com")
	if err != nil {
		t.Fatalf("expected no error when config is absent, got: %v", err)
	}
}

func TestGmailNoSendGlobalOverridesPerAccount(t *testing.T) {
	// Global flag should block even if the account is not in no_send_accounts.
	writeNoSendConfig(t, nil)

	err := Execute([]string{
		"--gmail-no-send", "gmail", "send",
		"--to", "x@y.com", "--subject", "S", "--body", "B",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "send blocked") {
		t.Fatalf("expected 'send blocked' error, got: %v", err)
	}
}

func TestGmailNoSendConfigGlobal(t *testing.T) {
	// gmail_no_send: true in config.json should block send without the CLI flag.
	t.Setenv("GOG_GMAIL_NO_SEND", "")
	writeNoSendConfigFull(t, true, nil)

	err := Execute([]string{
		"gmail", "send",
		"--to", "x@y.com", "--subject", "S", "--body", "B",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "send blocked") {
		t.Fatalf("expected 'send blocked' error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "gmail_no_send config") {
		t.Fatalf("expected error to mention config source, got: %v", err)
	}
}

func TestGmailNoSendConfigGlobalAllowsNonSend(t *testing.T) {
	t.Setenv("GOG_GMAIL_NO_SEND", "")
	writeNoSendConfigFull(t, true, nil)

	err := Execute([]string{
		"--account", "a@b.com", "gmail", "search", "test",
	})
	if err == nil {
		return
	}

	if strings.Contains(err.Error(), "send blocked") {
		t.Fatalf("non-send command should not be blocked: %v", err)
	}
}
