package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestCompletionCmd(t *testing.T) {
	cases := map[string]string{
		"bash":       "complete -F _gog_complete gog",
		"zsh":        "compdef _gog gog",
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

func TestFishCompletionScript_IncludesCurrentToken(t *testing.T) {
	out := fishCompletionScript()
	if !strings.Contains(out, "set words $words $cur") {
		t.Fatalf("expected fish script to append current token, got %q", out)
	}
	if !strings.Contains(out, "set -l cword (math (count $words) - 1)") {
		t.Fatalf("expected fish script to compute cword from appended token, got %q", out)
	}
}
