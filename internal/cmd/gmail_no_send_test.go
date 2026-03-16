package cmd

import (
	"strings"
	"testing"
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
