package cmd

import (
	"context"
	"errors"
	"io"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestGmailSendAsCmd_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailSendAsListCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected missing account error")
	}
	if err := (&GmailSendAsGetCmd{}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected missing email error")
	}
	if err := (&GmailSendAsCreateCmd{}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected missing email error")
	}
	if err := (&GmailSendAsVerifyCmd{}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected missing email error")
	}
	if err := (&GmailSendAsDeleteCmd{}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected missing email error")
	}
	if err := (&GmailSendAsUpdateCmd{}).Run(ctx, nil, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected missing email error")
	}
}

func TestGmailSendAsListCmd_ServiceError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		return nil, errors.New("service down")
	}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailSendAsListCmd{}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected service error")
	}
}
