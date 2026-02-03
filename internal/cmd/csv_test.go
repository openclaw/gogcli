package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCSVCmdSubstitutionAndFilters(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "users.csv")
	data := "email,first,last,dept\n" +
		"alice@example.com,Alice,Example,Sales\n" +
		"bob@example.com,Bob,Example,HR\n"
	if err := os.WriteFile(csvPath, []byte(data), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	orig := executeSubcommand
	t.Cleanup(func() { executeSubcommand = orig })

	var mu sync.Mutex
	calls := [][]string{}
	executeSubcommand = func(_ context.Context, _ *RootFlags, args []string) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, append([]string{}, args...))
		return nil
	}

	cmd := &CSVCmd{
		File:    csvPath,
		Command: []string{"users", "create", "~email", "--first-name", "~~first~~", "--last-name", "~~last~~", "--alias", "~~email~!~@.*$~!~@alias.com~~"},
		Match:   []string{"dept:^Sales$"},
	}

	if err := cmd.Run(testContext(t), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected 1 command, got %d", len(calls))
	}
	got := strings.Join(calls[0], " ")
	if !strings.Contains(got, "alice@example.com") || !strings.Contains(got, "Alice") || !strings.Contains(got, "@alias.com") {
		t.Fatalf("unexpected args: %s", got)
	}
}

func TestBatchCmdParsing(t *testing.T) {
	batchPath := filepath.Join(t.TempDir(), "batch.txt")
	content := "# comment\n" +
		"users create \"user one@example.com\" --first-name \"User One\"\n" +
		"users create user2@example.com --first-name User2\n"
	if err := os.WriteFile(batchPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}

	orig := executeSubcommand
	t.Cleanup(func() { executeSubcommand = orig })

	var mu sync.Mutex
	calls := [][]string{}
	executeSubcommand = func(_ context.Context, _ *RootFlags, args []string) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, append([]string{}, args...))
		return nil
	}

	cmd := &BatchCmd{File: batchPath, Parallel: 2}
	if err := cmd.Run(testContext(t), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(calls))
	}

	foundQuoted := false
	for _, call := range calls {
		for _, arg := range call {
			if arg == "user one@example.com" {
				foundQuoted = true
				break
			}
		}
	}
	if !foundQuoted {
		t.Fatalf("expected quoted arg to be preserved: %v", calls)
	}
}
