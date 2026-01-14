package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestCompletionCmd(t *testing.T) {
	cases := map[string]string{
		"bash":       "complete -F _gog_complete gog",
		"zsh":        "bashcompinit",
		"fish":       "complete -c gog",
		"powershell": "Register-ArgumentCompleter",
	}
	for shell, marker := range cases {
		shell := shell
		marker := marker
		t.Run(shell, func(t *testing.T) {
			out := captureStdout(t, func() {
				cmd := &CompletionCmd{Shell: shell}
				if err := cmd.Run(context.Background()); err != nil {
					t.Fatalf("run: %v", err)
				}
			})
			if !strings.Contains(out, "__complete") {
				t.Fatalf("expected __complete hook, got %q", out)
			}
			if !strings.Contains(out, marker) {
				t.Fatalf("expected %q in output, got %q", marker, out)
			}
		})
	}
}
