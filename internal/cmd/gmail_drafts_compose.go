package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/ui"
)

// GmailDraftsReplyCmd saves a reply as a draft. It mirrors GmailReplyCmd exactly
// (same positional arg + embedded GmailReplyOptions) so it inherits every flag
// and ergonomic of the send-side reply; the only difference is that it creates a
// draft instead of sending.
type GmailDraftsReplyCmd struct {
	MessageID string            `arg:"" name:"messageId" help:"Gmail message ID to reply to"`
	Options   GmailReplyOptions `embed:""`
}

// GmailDraftsReplyAllCmd saves a reply-all as a draft. Mirrors GmailReplyAllCmd.
type GmailDraftsReplyAllCmd struct {
	MessageID string            `arg:"" name:"messageId" help:"Gmail message ID to reply to"`
	Options   GmailReplyOptions `embed:""`
}

// GmailDraftsForwardCmd saves a forward as a draft. Mirrors GmailForwardCmd.
type GmailDraftsForwardCmd struct {
	MessageID string              `arg:"" name:"messageId" help:"Gmail message ID to forward"`
	Options   GmailForwardOptions `embed:""`
}

func (c *GmailDraftsReplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return c.Options.runDraft(ctx, flags, c.MessageID, false)
}

func (c *GmailDraftsReplyAllCmd) Run(ctx context.Context, flags *RootFlags) error {
	return c.Options.runDraft(ctx, flags, c.MessageID, true)
}

func (c *GmailDraftsForwardCmd) Run(ctx context.Context, flags *RootFlags) error {
	return c.Options.runDraft(ctx, flags, c.MessageID)
}

// runDraft is the draft-saving counterpart to GmailReplyOptions.run. It reuses
// the shared resolve/build helpers verbatim and differs only in the dry-run
// action name, the service gate (the non-send gate, since saving a draft is not
// a send), and the finalize/report step (Drafts.Create + writeDraftResult
// instead of Messages.Send + writeGmailMessageResults).
func (c *GmailReplyOptions) runDraft(ctx context.Context, flags *RootFlags, messageID string, replyAll bool) error {
	u := ui.FromContext(ctx)

	inputs, err := c.resolveReplyInputs(ctx, messageID)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.drafts."+replyModeName(replyAll), c.dryRunFields(inputs)); dryRunErr != nil {
		return dryRunErr
	}

	// A draft is not a send, so drafts compose stays usable under no-send,
	// matching gmail drafts create: the --gmail-no-send flag and config keys
	// are enforced pre-dispatch by the gmailSendCommandPaths list (which omits
	// the drafts compose paths), and using requireGmailService here (not
	// requireGmailSendService) skips the per-account config no-send check.
	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	built, err := c.buildReplyComposeMessage(ctx, svc, account, inputs, replyAll)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Create("me", &gmail.Draft{Message: built.message}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create reply draft: %w", err)
	}

	return writeDraftResult(ctx, u, draft, built.threading, built.attachmentMetadata)
}

// runDraft is the draft-saving counterpart to GmailForwardCmd.Run. Like the
// reply draft path it reuses the shared resolve/build helpers verbatim and
// changes the dry-run action name, the service gate, the finalize/report step,
// and the recipient requirement: a draft may be addressless, so it resolves
// with recipientsOptional where the send path requires --to.
func (c *GmailForwardOptions) runDraft(ctx context.Context, flags *RootFlags, messageID string) error {
	u := ui.FromContext(ctx)

	inputs, err := c.resolveForwardInputs(ctx, messageID, recipientsOptional)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.drafts.forward", c.dryRunFields(inputs)); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	built, err := c.buildForwardComposeMessage(ctx, svc, account, inputs)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Create("me", &gmail.Draft{Message: built.message}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create forward draft: %w", err)
	}

	// Forward results intentionally omit attachment metadata, so pass nil here to
	// stay consistent with the send path. A forward starts a new thread and
	// carries no reply headers, so the threading is empty; writeDraftResult then
	// falls back to the thread id Gmail assigns on the Drafts.Create response.
	return writeDraftResult(ctx, u, draft, draftThreading{}, nil)
}
