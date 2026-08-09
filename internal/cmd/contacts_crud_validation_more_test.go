package cmd

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/openclaw/gogcli/internal/ui"
)

func parseContactsKong(t *testing.T, cmd any, args []string) *kong.Context {
	t.Helper()

	parser, err := kong.New(cmd)
	if err != nil {
		t.Fatalf("kong new: %v", err)
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("kong parse: %v", err)
	}
	return kctx
}

func TestContactsValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&ContactsGetCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected get missing identifier")
	}
	if err := (&ContactsCreateCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected create missing given")
	}
	if err := (&ContactsCreateCmd{Given: "Ada", Email: "nope"}).Run(ctx, &RootFlags{Account: "a@b.com", DryRun: true}); err == nil || !strings.Contains(err.Error(), "invalid --email") {
		t.Fatalf("expected create invalid email, got %v", err)
	}

	{
		cmd := &ContactsUpdateCmd{}
		kctx := parseContactsKong(t, cmd, []string{"people/123"})
		cmd.ResourceName = "nope"
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected update invalid resourceName")
		}
	}
	{
		cmd := &ContactsUpdateCmd{}
		kctx := parseContactsKong(t, cmd, []string{"people/123", "--email", "nope"})
		if err := cmd.Run(ctx, kctx, &RootFlags{Account: "a@b.com", DryRun: true}); err == nil || !strings.Contains(err.Error(), "invalid --email") {
			t.Fatalf("expected update invalid email, got %v", err)
		}
	}

	if err := (&ContactsDeleteCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected delete invalid resourceName")
	}
}
