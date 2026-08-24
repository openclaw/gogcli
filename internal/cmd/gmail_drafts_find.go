package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// GmailDraftsFindCmd resolves the single reply draft addressed to an existing
// message, without needing the draft id. It exists so a caller that created a
// reply draft but lost the returned id (a crash between create and recording
// the id) can recover the exact draft from Gmail — the owner side, not a local
// cache — instead of creating a second one.
//
// It is bounded: it reads at most --max drafts (default 20, hard ceiling
// enforced by validateGmailMaxResults) and returns the draft whose In-Reply-To
// matches the original message's RFC Message-ID and whose thread matches. The
// result mirrors `drafts get` (a `draft` object with the full message payload)
// plus a `matched` count, so exactly-one recovery is deterministic.
type GmailDraftsFindCmd struct {
	ReplyToMessageID string `name:"reply-to-message-id" help:"Original Gmail message ID the draft replies to" required:""`
	Max              int64  `name:"max" aliases:"limit" help:"Max drafts to scan" default:"20"`
}

func (c *GmailDraftsFindCmd) Run(ctx context.Context, flags *RootFlags) error {
	_ = ui.FromContext(ctx)
	replyToID := strings.TrimSpace(c.ReplyToMessageID)
	if replyToID == "" {
		return usage("empty reply-to-message-id")
	}
	if err := validateGmailMaxResults(c.Max); err != nil {
		return err
	}
	_, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	original, err := svc.Users.Messages.Get("me", replyToID).Format("metadata").
		MetadataHeaders("Message-ID").Context(ctx).Do()
	if err != nil {
		return err
	}
	originalRfcID := ""
	originalThread := ""
	if original != nil {
		originalThread = original.ThreadId
		originalRfcID = strings.TrimSpace(headerValue(original.Payload, "Message-ID"))
	}
	if originalRfcID == "" {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"matched": 0, "draft": nil})
	}

	listResp, err := svc.Users.Drafts.List("me").MaxResults(c.Max).Context(ctx).Do()
	if err != nil {
		return err
	}

	var matched *gmail.Draft
	count := 0
	for _, listed := range listResp.Drafts {
		if listed == nil || listed.Id == "" {
			continue
		}
		full, getErr := svc.Users.Drafts.Get("me", listed.Id).Format("full").Context(ctx).Do()
		if getErr != nil {
			return getErr
		}
		if full.Message == nil {
			continue
		}
		if full.Message.ThreadId != "" && originalThread != "" && full.Message.ThreadId != originalThread {
			continue
		}
		inReplyTo := strings.TrimSpace(headerValue(full.Message.Payload, "In-Reply-To"))
		if inReplyTo == "" || inReplyTo != originalRfcID {
			continue
		}
		count++
		if matched == nil {
			matched = full
		}
	}

	if matched == nil || count != 1 {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"matched": count, "draft": nil})
	}
	return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"matched": count, "draft": matched})
}
