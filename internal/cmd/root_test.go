package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/app"
)

func TestEnvOr(t *testing.T) {
	t.Setenv("X_TEST", "")
	if got := envOr("X_TEST", "fallback"); got != "fallback" {
		t.Fatalf("unexpected: %q", got)
	}
	t.Setenv("X_TEST", "value")
	if got := envOr("X_TEST", "fallback"); got != "value" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestExecute_Help(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--help"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Google CLI") && !strings.Contains(out, "Usage:") {
		t.Fatalf("unexpected help output: %q", out)
	}
	if !strings.Contains(out, "config.json") || !strings.Contains(out, "keyring backend") {
		t.Fatalf("expected config info in help output: %q", out)
	}
	if strings.Contains(out, "gmail (mail,email) thread get") {
		t.Fatalf("expected collapsed help (no expanded subcommands), got: %q", out)
	}
	if strings.Contains(out, "Search Console/Ads/") || strings.Contains(out, "searchconsole/ads/") {
		t.Fatalf("root help must not advertise ads as a command service, got: %q", out)
	}
	if !strings.Contains(out, "Cloud Identity Groups (Workspace only)") {
		t.Fatalf("root help must identify Groups as Workspace-only, got: %q", out)
	}
	normalizedHelp := strings.Join(strings.Fields(out), " ")
	if !strings.Contains(normalizedHelp, "Account email, alias, or auto for authenticated Google API commands") {
		t.Fatalf("root help must describe account selection without a stale service list, got: %q", out)
	}
	if strings.Contains(out, "gmail/calendar/chat/classroom") {
		t.Fatalf("root help must not hard-code an incomplete account-scoped service list, got: %q", out)
	}
}

func TestExecute_NoArgsShowsHelp(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage output, got: %q", out)
	}
}

func TestExecute_HelpCommand(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"help", "drive", "ls"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Usage: gog drive (drv) ls") {
		t.Fatalf("unexpected command help: %q", out)
	}
}

func TestExecute_HelpIgnoresTrailingArguments(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--help", "nonsense"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Usage: gog <command>") {
		t.Fatalf("unexpected root help: %q", out)
	}
}

func TestExecute_EarlyUsageErrorsPrintToStderr(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "conflicting output modes", args: []string{"--json", "--plain", "version"}, want: "cannot combine --json and --plain"},
		{name: "invalid color", args: []string{"--color", "bogus", "version"}, want: "expected auto|always|never"},
		{name: "results only without json", args: []string{"--results-only", "version"}, want: "--results-only requires --json"},
		{name: "select without json", args: []string{"--select", "version", "version"}, want: "--select requires --json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runErr error
			errText := captureStderr(t, func() {
				_ = captureStdout(t, func() {
					runErr = Execute(tt.args)
				})
			})
			if runErr == nil || ExitCode(runErr) != 2 {
				t.Fatalf("expected usage error, got %v", runErr)
			}
			if !strings.Contains(errText, tt.want) {
				t.Fatalf("stderr = %q, want %q", errText, tt.want)
			}
		})
	}
}

func TestExecute_ExplicitOutputModeOverridesEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		envName  string
		args     []string
		wantJSON bool
	}{
		{name: "plain overrides json env", envName: "GOG_JSON", args: []string{"--plain", "version"}},
		{name: "tsv alias overrides json env", envName: "GOG_JSON", args: []string{"--tsv", "version"}},
		{name: "json overrides plain env", envName: "GOG_PLAIN", args: []string{"--json", "version"}, wantJSON: true},
		{name: "machine alias overrides plain env", envName: "GOG_PLAIN", args: []string{"--machine", "version"}, wantJSON: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOG_JSON", "")
			t.Setenv("GOG_PLAIN", "")
			t.Setenv(tt.envName, "1")

			var runErr error
			out := captureStdout(t, func() {
				_ = captureStderr(t, func() {
					runErr = Execute(tt.args)
				})
			})
			if runErr != nil {
				t.Fatalf("Execute: %v", runErr)
			}
			gotJSON := strings.HasPrefix(strings.TrimSpace(out), "{")
			if gotJSON != tt.wantJSON {
				t.Fatalf("output = %q, JSON = %v, want %v", out, gotJSON, tt.wantJSON)
			}
		})
	}
}

func TestExecute_Help_GmailHasGroupsAndRelativeCommands(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"gmail", "--help"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "\nRead\n") || !strings.Contains(out, "\nWrite\n") || !strings.Contains(out, "\nAdmin\n") {
		t.Fatalf("expected command groups in gmail help, got: %q", out)
	}
	if !strings.Contains(out, "\n  search") || !strings.Contains(out, "Search threads using Gmail query syntax") {
		t.Fatalf("expected relative command summaries in gmail help, got: %q", out)
	}
	if strings.Contains(out, "\n  gmail (mail,email) search <query>") {
		t.Fatalf("unexpected full command prefix in gmail help, got: %q", out)
	}
	if strings.Contains(out, "\n  watch <command>") {
		t.Fatalf("expected watch to be under gmail settings (not top-level gmail help), got: %q", out)
	}
	if !strings.Contains(out, "\n  settings <command>") {
		t.Fatalf("expected settings subgroup in gmail help, got: %q", out)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"no_such_cmd"}); err == nil {
				t.Fatalf("expected error")
			}
		})
	})
	if errText == "" {
		t.Fatalf("expected stderr output")
	}
}

func TestExecute_UnknownFlag(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--definitely-nope"}); err == nil {
				t.Fatalf("expected error")
			}
		})
	})
	if errText == "" {
		t.Fatalf("expected stderr output")
	}
}

func TestExecute_AccessTokenDoesNotWarnForVersion(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--access-token", "ya29.test-token", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if strings.Contains(errText, directAccessTokenWarning) {
		t.Fatalf("unexpected access-token warning for version command: %q", errText)
	}
}

func TestExecute_AccessTokenWarningUsesRuntimeStderr(t *testing.T) {
	stopErr := errors.New("stop after account resolution")
	result := executeWithTestRuntime(t, []string{
		"--access-token", "ya29.test-token",
		"drive", "ls",
	}, &app.Runtime{Services: app.Services{
		Drive: func(context.Context, string) (*drive.Service, error) {
			return nil, stopErr
		},
	}})
	if !errors.Is(result.err, stopErr) {
		t.Fatalf("Execute error = %v, want %v", result.err, stopErr)
	}
	if !strings.Contains(result.stderr, directAccessTokenWarning) {
		t.Fatalf("missing access-token warning: %q", result.stderr)
	}
}

func TestNewUsageError(t *testing.T) {
	if newUsageError(nil) != nil {
		t.Fatalf("expected nil for nil error")
	}

	err := errors.New("bad")
	wrapped := newUsageError(err)
	if wrapped == nil {
		t.Fatalf("expected wrapped error")
	}
	var exitErr *ExitError
	if !errors.As(wrapped, &exitErr) || exitErr.Code != 2 || !errors.Is(exitErr.Err, err) {
		t.Fatalf("unexpected wrapped error: %#v", wrapped)
	}
}
