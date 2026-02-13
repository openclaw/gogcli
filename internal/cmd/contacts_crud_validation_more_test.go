package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/degree-analytics/ratatosk/internal/ui"
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

	{
		cmd := &ContactsUpdateCmd{}
		kctx := parseContactsKong(t, cmd, []string{"people/123"})
		cmd.ResourceName = "nope"
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected update invalid resourceName")
		}
	}

	if err := (&ContactsDeleteCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected delete invalid resourceName")
	}
}
