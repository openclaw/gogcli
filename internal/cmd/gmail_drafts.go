package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailDraftsCmd struct {
	List   GmailDraftsListCmd   `cmd:"" name:"list" help:"List drafts"`
	Get    GmailDraftsGetCmd    `cmd:"" name:"get" help:"Get draft details"`
	Delete GmailDraftsDeleteCmd `cmd:"" name:"delete" help:"Delete a draft"`
	Send   GmailDraftsSendCmd   `cmd:"" name:"send" help:"Send a draft"`
	Create GmailDraftsCreateCmd `cmd:"" name:"create" help:"Create a draft"`
	Update GmailDraftsUpdateCmd `cmd:"" name:"update" help:"Update a draft"`
}

type GmailDraftsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"20"`
	Page string `name:"page" help:"Page token"`
}

func (c *GmailDraftsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Drafts.List("me").MaxResults(c.Max).PageToken(c.Page).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			ID        string `json:"id"`
			MessageID string `json:"messageId,omitempty"`
			ThreadID  string `json:"threadId,omitempty"`
		}
		items := make([]item, 0, len(resp.Drafts))
		for _, d := range resp.Drafts {
			if d == nil {
				continue
			}
			var msgID, threadID string
			if d.Message != nil {
				msgID = d.Message.Id
				threadID = d.Message.ThreadId
			}
			items = append(items, item{ID: d.Id, MessageID: msgID, ThreadID: threadID})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"drafts":        items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Drafts) == 0 {
		u.Err().Println("No drafts")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tMESSAGE_ID")
	for _, d := range resp.Drafts {
		msgID := ""
		if d.Message != nil {
			msgID = d.Message.Id
		}
		fmt.Fprintf(w, "%s\t%s\n", d.Id, msgID)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type GmailDraftsGetCmd struct {
	DraftID  string `arg:"" name:"draftId" help:"Draft ID"`
	Download bool   `name:"download" help:"Download draft attachments"`
}

func (c *GmailDraftsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Get("me", draftID).Format("full").Do()
	if err != nil {
		return err
	}
	if draft.Message == nil {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, map[string]any{"draft": draft})
		}
		u.Err().Println("Empty draft")
		return nil
	}

	msg := draft.Message
	if outfmt.IsJSON(ctx) {
		out := map[string]any{"draft": draft}
		if c.Download {
			attachDir, err := config.EnsureGmailAttachmentsDir()
			if err != nil {
				return err
			}
			downloads, err := downloadAttachmentOutputs(ctx, svc, msg.Id, collectAttachments(msg.Payload), attachDir)
			if err != nil {
				return err
			}
			out["downloaded"] = attachmentDownloadDraftOutputs(downloads)
		}
		return outfmt.WriteJSON(os.Stdout, out)
	}

	u.Out().Printf("Draft-ID: %s", draft.Id)
	u.Out().Printf("Message-ID: %s", msg.Id)
	u.Out().Printf("To: %s", headerValue(msg.Payload, "To"))
	u.Out().Printf("Cc: %s", headerValue(msg.Payload, "Cc"))
	u.Out().Printf("Bcc: %s", headerValue(msg.Payload, "Bcc"))
	u.Out().Printf("Subject: %s", headerValue(msg.Payload, "Subject"))
	u.Out().Println("")

	body := bestBodyText(msg.Payload)
	if body != "" {
		u.Out().Println(body)
		u.Out().Println("")
	}

	attachments := collectAttachments(msg.Payload)
	printAttachmentSection(u.Out(), attachments)

	if c.Download && msg.Id != "" && len(attachments) > 0 {
		attachDir, err := config.EnsureGmailAttachmentsDir()
		if err != nil {
			return err
		}
		downloads, err := downloadAttachmentOutputs(ctx, svc, msg.Id, attachments, attachDir)
		if err != nil {
			return err
		}
		for _, a := range downloads {
			if a.Cached {
				u.Out().Printf("Cached: %s", a.Path)
			} else {
				u.Out().Successf("Saved: %s", a.Path)
			}
		}
	}

	return nil
}

type GmailDraftsDeleteCmd struct {
	DraftID string `arg:"" name:"draftId" help:"Draft ID"`
}

func (c *GmailDraftsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete gmail draft %s", draftID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Users.Drafts.Delete("me", draftID).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"deleted": true, "draftId": draftID})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("draft_id\t%s", draftID)
	return nil
}

type GmailDraftsSendCmd struct {
	DraftID string `arg:"" name:"draftId" help:"Draft ID"`
}

func (c *GmailDraftsSendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	msg, err := svc.Users.Drafts.Send("me", &gmail.Draft{Id: draftID}).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"messageId": msg.Id,
			"threadId":  msg.ThreadId,
		})
	}
	u.Out().Printf("message_id\t%s", msg.Id)
	if msg.ThreadId != "" {
		u.Out().Printf("thread_id\t%s", msg.ThreadId)
	}
	return nil
}

type GmailDraftsCreateCmd struct {
	To               string   `name:"to" help:"Recipients (comma-separated)"`
	Cc               string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc              string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject          string   `name:"subject" help:"Subject (required)"`
	Body             string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyFile         string   `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML         string   `name:"body-html" help:"Body (HTML; optional)"`
	ReplyToMessageID string   `name:"reply-to-message-id" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ReplyTo          string   `name:"reply-to" help:"Reply-To header address"`
	Attach           []string `name:"attach" help:"Attachment file path (repeatable)"`
	From             string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
}

type draftComposeInput struct {
	To               string
	Cc               string
	Bcc              string
	Subject          string
	Body             string
	BodyHTML         string
	ReplyToMessageID string
	ReplyToThreadID  string
	ReplyTo          string
	Attach           []string
	From             string
}

func (c draftComposeInput) validate() error {
	if strings.TrimSpace(c.Subject) == "" {
		return usage("required: --subject")
	}
	if strings.TrimSpace(c.Body) == "" && strings.TrimSpace(c.BodyHTML) == "" {
		return usage("required: --body, --body-file, or --body-html")
	}
	return nil
}

func buildDraftMessage(ctx context.Context, svc *gmail.Service, account string, input draftComposeInput) (*gmail.Message, string, error) {
	fromAddr := account
	if strings.TrimSpace(input.From) != "" {
		sa, err := svc.Users.Settings.SendAs.Get("me", input.From).Context(ctx).Do()
		if err != nil {
			return nil, "", fmt.Errorf("invalid --from address %q: %w", input.From, err)
		}
		if sa.VerificationStatus != gmailVerificationAccepted {
			return nil, "", fmt.Errorf("--from address %q is not verified (status: %s)", input.From, sa.VerificationStatus)
		}
		fromAddr = input.From
		if sa.DisplayName != "" {
			fromAddr = sa.DisplayName + " <" + input.From + ">"
		}
	}

	info, err := fetchReplyInfo(ctx, svc, input.ReplyToMessageID, input.ReplyToThreadID)
	if err != nil {
		return nil, "", err
	}
	inReplyTo := info.InReplyTo
	references := info.References
	threadID := info.ThreadID

	atts := make([]mailAttachment, 0, len(input.Attach))
	for _, p := range input.Attach {
		expanded, expandErr := config.ExpandPath(p)
		if expandErr != nil {
			return nil, "", expandErr
		}
		atts = append(atts, mailAttachment{Path: expanded})
	}

	raw, err := buildRFC822(mailOptions{
		From:        fromAddr,
		To:          splitCSV(input.To),
		Cc:          splitCSV(input.Cc),
		Bcc:         splitCSV(input.Bcc),
		ReplyTo:     input.ReplyTo,
		Subject:     input.Subject,
		Body:        input.Body,
		BodyHTML:    input.BodyHTML,
		InReplyTo:   inReplyTo,
		References:  references,
		Attachments: atts,
	}, &rfc822Config{allowMissingTo: true})
	if err != nil {
		return nil, "", err
	}

	msg := &gmail.Message{
		Raw: base64.RawURLEncoding.EncodeToString(raw),
	}
	if threadID != "" {
		msg.ThreadId = threadID
	}

	return msg, threadID, nil
}

func writeDraftResult(ctx context.Context, u *ui.UI, draft *gmail.Draft, threadID string) error {
	if threadID == "" && draft != nil && draft.Message != nil {
		threadID = draft.Message.ThreadId
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"draftId":  draft.Id,
			"message":  draft.Message,
			"threadId": threadID,
		})
	}
	u.Out().Printf("draft_id\t%s", draft.Id)
	if draft.Message != nil && draft.Message.Id != "" {
		u.Out().Printf("message_id\t%s", draft.Message.Id)
	}
	if threadID != "" {
		u.Out().Printf("thread_id\t%s", threadID)
	}
	return nil
}

func (c *GmailDraftsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	body, err := resolveBodyInput(c.Body, c.BodyFile)
	if err != nil {
		return err
	}

	input := draftComposeInput{
		To:               c.To,
		Cc:               c.Cc,
		Bcc:              c.Bcc,
		Subject:          c.Subject,
		Body:             body,
		BodyHTML:         c.BodyHTML,
		ReplyToMessageID: c.ReplyToMessageID,
		ReplyToThreadID:  "",
		ReplyTo:          c.ReplyTo,
		Attach:           c.Attach,
		From:             c.From,
	}
	if validateErr := input.validate(); validateErr != nil {
		return validateErr
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	msg, threadID, err := buildDraftMessage(ctx, svc, account, input)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Create("me", &gmail.Draft{Message: msg}).Do()
	if err != nil {
		return err
	}
	return writeDraftResult(ctx, u, draft, threadID)
}

type GmailDraftsUpdateCmd struct {
	DraftID          string   `arg:"" name:"draftId" help:"Draft ID"`
	To               *string  `name:"to" help:"Recipients (comma-separated; omit to keep existing)"`
	Cc               string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc              string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject          string   `name:"subject" help:"Subject (required)"`
	Body             string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyFile         string   `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML         string   `name:"body-html" help:"Body (HTML; optional)"`
	ReplyToMessageID string   `name:"reply-to-message-id" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ReplyTo          string   `name:"reply-to" help:"Reply-To header address"`
	Attach           []string `name:"attach" help:"Attachment file path (repeatable)"`
	From             string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
}

func (c *GmailDraftsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	to := ""
	toWasSet := false
	if c.To != nil {
		toWasSet = true
		to = *c.To
	}

	existingThreadID := ""
	existingTo := ""
	if !toWasSet || strings.TrimSpace(c.ReplyToMessageID) == "" {
		existing, fetchErr := svc.Users.Drafts.Get("me", draftID).Format("full").Do()
		if fetchErr != nil {
			return fetchErr
		}
		if existing != nil && existing.Message != nil {
			existingThreadID = strings.TrimSpace(existing.Message.ThreadId)
			if !toWasSet {
				existingTo = strings.TrimSpace(headerValue(existing.Message.Payload, "To"))
			}
		}
	}
	if !toWasSet {
		to = existingTo
	}

	body, err := resolveBodyInput(c.Body, c.BodyFile)
	if err != nil {
		return err
	}

	replyToThreadID := ""
	if strings.TrimSpace(c.ReplyToMessageID) == "" {
		replyToThreadID = existingThreadID
	}

	input := draftComposeInput{
		To:               to,
		Cc:               c.Cc,
		Bcc:              c.Bcc,
		Subject:          c.Subject,
		Body:             body,
		BodyHTML:         c.BodyHTML,
		ReplyToMessageID: c.ReplyToMessageID,
		ReplyToThreadID:  replyToThreadID,
		ReplyTo:          c.ReplyTo,
		Attach:           c.Attach,
		From:             c.From,
	}
	if validateErr := input.validate(); validateErr != nil {
		return validateErr
	}

	msg, threadID, err := buildDraftMessage(ctx, svc, account, input)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Update("me", draftID, &gmail.Draft{Id: draftID, Message: msg}).Do()
	if err != nil {
		return err
	}
	return writeDraftResult(ctx, u, draft, threadID)
}
