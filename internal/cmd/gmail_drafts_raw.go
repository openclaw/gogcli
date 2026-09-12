package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/ui"
)

func (c *GmailDraftsCreateCmd) AfterApply(kctx *kong.Context) error {
	return validateRawDraftFlagPresence(kctx, c.RawFile)
}

func (c *GmailDraftsUpdateCmd) AfterApply(kctx *kong.Context) error {
	return validateRawDraftFlagPresence(kctx, c.RawFile)
}

// Reject even explicitly empty compose inputs. Boolean false is allowed so a
// caller can disable automatic alias selection inherited from the environment.
func validateRawDraftFlagPresence(kctx *kong.Context, source string) error {
	if strings.TrimSpace(source) == "" {
		if flagProvided(kctx, "raw-file") {
			return usage("--raw-file requires a file path or '-' for stdin")
		}
		return nil
	}
	for _, flag := range []string{
		"to", "cc", "bcc", "subject", "body", "body-file", "body-html", "body-html-file",
		"attach", "from", "reply-to", "reply-to-message-id",
	} {
		if flagProvided(kctx, flag) {
			return usagef("--raw-file cannot be combined with --%s", flag)
		}
	}
	return nil
}

func (c *GmailDraftsCreateCmd) rawModeConflict() string {
	if c.AutoFromAddressedAlias {
		return "--auto-from-addressed-alias"
	}
	compose := GmailSendCmd{
		To: c.To, Cc: c.Cc, Bcc: c.Bcc, Subject: c.Subject,
		Body: c.Body, BodyFile: c.BodyFile, BodyHTML: c.BodyHTML, BodyHTMLFile: c.BodyHTMLFile,
		ReplyToMessageID: c.ReplyToMessageID, ReplyAll: c.ReplyAll, ReplyTo: c.ReplyTo,
		Attach: c.Attach, From: c.From, Quote: c.Quote,
	}
	return compose.rawModeConflict()
}

func (c *GmailDraftsUpdateCmd) rawModeConflict() string {
	if c.To != nil {
		return "--to"
	}
	if c.ClearAttachments {
		return "--clear-attachments"
	}
	if c.ClearReplyContext {
		return "--clear-reply-context"
	}
	compose := GmailDraftsCreateCmd{
		Cc: c.Cc, Bcc: c.Bcc, Subject: c.Subject,
		Body: c.Body, BodyFile: c.BodyFile, BodyHTML: c.BodyHTML, BodyHTMLFile: c.BodyHTMLFile,
		ReplyToMessageID: c.ReplyToMessageID, ReplyAll: c.ReplyAll, ReplyTo: c.ReplyTo,
		Attach: c.Attach, From: c.From, Quote: c.Quote, AutoFromAddressedAlias: c.AutoFromAddressedAlias,
	}
	return compose.rawModeConflict()
}

func (c *GmailDraftsUpdateCmd) runRaw(ctx context.Context, flags *RootFlags) error {
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}
	if conflict := c.rawModeConflict(); conflict != "" {
		return usagef("--raw-file cannot be combined with %s", conflict)
	}
	return runRawGmailDraft(ctx, flags, c.RawFile, c.ThreadID, draftID)
}

func runRawGmailDraft(ctx context.Context, flags *RootFlags, source, threadID, draftID string) error {
	raw, plan, err := readRawGmailInput(ctx, source, threadID, false)
	if err != nil {
		return err
	}
	op := "gmail.drafts.create"
	if draftID != "" {
		op = "gmail.drafts.update"
	}
	request := struct {
		gmailRawMessagePlan
		DraftID string `json:"draft_id,omitempty"`
	}{plan, draftID}
	if dryRunErr := dryRunExit(ctx, flags, op, request); dryRunErr != nil {
		return dryRunErr
	}
	if googleapi.ReadOnly(ctx) {
		return fmt.Errorf("%w: Gmail draft mutations are disabled", googleapi.ErrReadOnly)
	}
	// Deliberately not requireGmailSendService: staging a draft is permitted
	// under global and per-account no-send policies.
	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}
	if senderErr := validateRawGmailSender(ctx, svc, account, plan); senderErr != nil {
		return senderErr
	}
	input := &gmail.Draft{Message: &gmail.Message{
		Raw: base64.RawURLEncoding.EncodeToString(raw), ThreadId: plan.ThreadID,
	}}
	var draft *gmail.Draft
	if draftID == "" {
		draft, err = svc.Users.Drafts.Create("me", input).Context(ctx).Do()
	} else {
		// Raw update is a complete replacement. Do not fetch or merge the old
		// draft's recipients, attachments, thread, or reply headers.
		draft, err = svc.Users.Drafts.Update("me", draftID, input).Context(ctx).Do()
	}
	if err != nil {
		return err
	}
	threading := draftThreading{InReplyTo: plan.inReplyTo, References: plan.references}
	if strings.TrimSpace(plan.inReplyTo) != "" || strings.TrimSpace(plan.references) != "" {
		threading.Source = replyContextCaller
	}
	// Report the API's actual thread, which can differ from the requested one.
	return writeDraftResult(ctx, ui.FromContext(ctx), draft, threading, nil)
}
