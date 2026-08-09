package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func executeGmailSettingsValidation(t *testing.T, args []string) error {
	t.Helper()
	result := executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Gmail: func(context.Context, string) (*gmail.Service, error) {
			t.Fatal("expected validation to fail before creating gmail service")
			return nil, errors.New("unexpected gmail service call")
		},
	}})
	return result.err
}

func TestGmailSettingsEmailValidation(t *testing.T) {
	testCases := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{name: "plain", email: "user@example.com"},
		{name: "missing at", email: "nope", wantErr: true},
		{name: "display name", email: "Tester <user@example.com>", wantErr: true},
		{name: "two addresses", email: "a@example.com,b@example.com", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGmailSettingsEmail("email", tc.email)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "invalid email") {
					t.Fatalf("expected invalid email error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateGmailSettingsEmail: %v", err)
			}
		})
	}
}

func TestGmailForwarding_InvalidEmailFailsBeforeDryRun(t *testing.T) {
	testCases := [][]string{
		{"--account", "a@b.com", "--dry-run", "gmail", "forwarding", "create", "nope"},
		{"--account", "a@b.com", "--dry-run", "gmail", "forwarding", "delete", "nope"},
		{"--account", "a@b.com", "--dry-run", "gmail", "forwarding", "create", "Tester <x@example.com>"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args[4:], "_"), func(t *testing.T) {
			err := executeGmailSettingsValidation(t, args)
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 || !strings.Contains(err.Error(), "invalid forwardingEmail") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestGmailDelegates_InvalidEmailFailsBeforeDryRun(t *testing.T) {
	testCases := [][]string{
		{"--account", "a@b.com", "--dry-run", "gmail", "delegates", "add", "nope"},
		{"--account", "a@b.com", "--dry-run", "gmail", "delegates", "remove", "nope"},
		{"--account", "a@b.com", "--dry-run", "gmail", "delegates", "add", "Tester <x@example.com>"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args[4:], "_"), func(t *testing.T) {
			err := executeGmailSettingsValidation(t, args)
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 || !strings.Contains(err.Error(), "invalid delegateEmail") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestGmailSendAs_InvalidEmailFailsBeforeDryRun(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{name: "create email", args: []string{"--account", "a@b.com", "--dry-run", "gmail", "sendas", "create", "nope"}, want: "invalid email"},
		{name: "verify email", args: []string{"--account", "a@b.com", "--dry-run", "gmail", "sendas", "verify", "nope"}, want: "invalid email"},
		{name: "delete email", args: []string{"--account", "a@b.com", "--dry-run", "gmail", "sendas", "delete", "nope"}, want: "invalid email"},
		{name: "update email", args: []string{"--account", "a@b.com", "--dry-run", "gmail", "sendas", "update", "nope", "--make-default"}, want: "invalid email"},
		{name: "create reply-to", args: []string{"--account", "a@b.com", "--dry-run", "gmail", "sendas", "create", "alias@example.com", "--reply-to", "nope"}, want: "invalid --reply-to"},
		{name: "update reply-to", args: []string{"--account", "a@b.com", "--dry-run", "gmail", "sendas", "update", "alias@example.com", "--reply-to", "Tester <x@example.com>"}, want: "invalid --reply-to"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := executeGmailSettingsValidation(t, tc.args)
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}
