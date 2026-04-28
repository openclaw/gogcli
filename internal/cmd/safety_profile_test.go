package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withBakedSafetyProfile(t *testing.T, raw string) {
	t.Helper()
	prev := bakedSafetyProfileYAML
	bakedSafetyProfileYAML = raw
	t.Cleanup(func() { bakedSafetyProfileYAML = prev })
}

func TestParseSafetyProfileNestedAndAliases(t *testing.T) {
	profile, err := parseSafetyProfile(`
name: test
gmail:
  search: true
  send: false
aliases:
  send: false
allow:
  - version
deny:
  - auth.remove
`)
	if err != nil {
		t.Fatalf("parseSafetyProfile: %v", err)
	}
	for _, rule := range []string{"gmail.search", "version"} {
		if !profile.allow[rule] {
			t.Fatalf("expected allow rule %q in %#v", rule, profile.allow)
		}
	}
	for _, rule := range []string{"gmail.send", "send", "auth.remove"} {
		if !profile.deny[rule] {
			t.Fatalf("expected deny rule %q in %#v", rule, profile.deny)
		}
	}
}

func TestBakedSafetyProfileBlocksBeforeRuntimeAllowlist(t *testing.T) {
	setTestConfigHome(t)
	withBakedSafetyProfile(t, `
name: test
allow:
  - version
deny:
  - gmail.send
  - send
`)

	err := Execute([]string{"--enable-commands", "gmail.send", "gmail", "send", "--to", "a@example.com", "--subject", "S", "--body", "B"})
	if err == nil {
		t.Fatalf("expected baked safety profile block")
	}
	if got := err.Error(); !strings.Contains(got, "baked safety profile") || !strings.Contains(got, "gmail send") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBakedSafetyProfileFailsClosed(t *testing.T) {
	setTestConfigHome(t)
	withBakedSafetyProfile(t, `
name: readonly
allow:
  - version
`)

	err := Execute([]string{"tasks", "list", "task-list-1"})
	if err == nil {
		t.Fatalf("expected fail-closed safety profile block")
	}
	if got := err.Error(); !strings.Contains(got, "not included") || !strings.Contains(got, "tasks list") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBakedSafetyProfileAllowsListedCommand(t *testing.T) {
	setTestConfigHome(t)
	withBakedSafetyProfile(t, `
name: test
allow:
  - version
`)

	if err := Execute([]string{"version"}); err != nil {
		t.Fatalf("expected allowed command, got %v", err)
	}
}

func TestReadonlySafetyProfileBlocksNestedMutations(t *testing.T) {
	setTestConfigHome(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "safety-profiles", "readonly.yaml"))
	if err != nil {
		t.Fatalf("read readonly profile: %v", err)
	}
	withBakedSafetyProfile(t, string(raw))

	tests := [][]string{
		{"gmail", "messages", "modify", "msg-1", "--add", "Label_1"},
		{"calendar", "alias", "set", "work", "abc123@group.calendar.google.com"},
		{"calendar", "alias", "unset", "work"},
	}
	for _, args := range tests {
		err := Execute(args)
		if err == nil {
			t.Fatalf("expected readonly profile block for %v", args)
		}
		if got := err.Error(); !strings.Contains(got, "baked safety profile") {
			t.Fatalf("unexpected error for %v: %v", args, err)
		}
	}
}
