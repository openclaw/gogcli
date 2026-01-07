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
			type dl struct {
				MessageID    string `json:"messageId"`
				AttachmentID string `json:"attachmentId"`
				Filename     string `json:"filename"`
				Path         string `json:"path"`
				Cached       bool   `json:"cached"`
			}
			downloaded := make([]dl, 0)
			for _, a := range collectAttachments(msg.Payload) {
				outPath, cached, err := downloadAttachment(ctx, svc, msg.Id, a, attachDir)
				if err != nil {
					return err
				}
				downloaded = append(downloaded, dl{
					MessageID:    msg.Id,
					AttachmentID: a.AttachmentID,
					Filename:     a.Filename,
					Path:         outPath,
					Cached:       cached,
				})
			}
			out["downloaded"] = downloaded
		}
		return outfmt.WriteJSON(os.Stdout, out)
	}

	u.Out().Printf("Draft-ID: %s", draft.Id)
	u.Out().Printf("Message-ID: %s", msg.Id)
	u.Out().Printf("To: %s", headerValue(msg.Payload, "To"))
	u.Out().Printf("Cc: %s", headerValue(msg.Payload, "Cc"))
	u.Out().Printf("Subject: %s", headerValue(msg.Payload, "Subject"))
	u.Out().Println("")

	body := bestBodyText(msg.Payload)
	if body != "" {
		u.Out().Println(body)
		u.Out().Println("")
	}

	attachments := collectAttachments(msg.Payload)
	if len(attachments) > 0 {
		u.Out().Println("Attachments:")
		for _, a := range attachments {
			u.Out().Printf("  - %s (%d bytes)", a.Filename, a.Size)
		}
		u.Out().Println("")
	}

	if c.Download && msg.Id != "" && len(attachments) > 0 {
		attachDir, err := config.EnsureGmailAttachmentsDir()
		if err != nil {
			return err
		}
		for _, a := range attachments {
			outPath, cached, err := downloadAttachment(ctx, svc, msg.Id, a, attachDir)
			if err != nil {
				return err
			}
			if cached {
				u.Out().Printf("Cached: %s", outPath)
			} else {
				u.Out().Successf("Saved: %s", outPath)
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
	To               string   `name:"to" help:"Recipients (comma-separated, required)"`
	Cc               string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc              string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject          string   `name:"subject" help:"Subject (required)"`
	Body             string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyHTML         string   `name:"body-html" help:"Body (HTML; optional)"`
	ReplyToMessageID string   `name:"reply-to-message-id" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ReplyTo          string   `name:"reply-to" help:"Reply-To header address"`
	Attach           []string `name:"attach" help:"Attachment file path (repeatable)"`
	From             string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
}

func (c *GmailDraftsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.To) == "" || strings.TrimSpace(c.Subject) == "" {
		return usage("required: --to, --subject")
	}
	if strings.TrimSpace(c.Body) == "" && strings.TrimSpace(c.BodyHTML) == "" {
		return usage("required: --body or --body-html")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Determine the From address
	fromAddr := account
	if strings.TrimSpace(c.From) != "" {
		// Validate that this is a configured send-as alias
		var sa *gmail.SendAs
		sa, err = svc.Users.Settings.SendAs.Get("me", c.From).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("invalid --from address %q: %w", c.From, err)
		}
		if sa.VerificationStatus != gmailVerificationAccepted {
			return fmt.Errorf("--from address %q is not verified (status: %s)", c.From, sa.VerificationStatus)
		}
		fromAddr = c.From
		// Include display name if set
		if sa.DisplayName != "" {
			fromAddr = sa.DisplayName + " <" + c.From + ">"
		}
	}

	inReplyTo, references, threadID, err := replyHeaders(ctx, svc, c.ReplyToMessageID)
	if err != nil {
		return err
	}

	atts := make([]mailAttachment, 0, len(c.Attach))
	for _, p := range c.Attach {
		atts = append(atts, mailAttachment{Path: p})
	}

	raw, err := buildRFC822(mailOptions{
		From:        fromAddr,
		To:          splitCSV(c.To),
		Cc:          splitCSV(c.Cc),
		Bcc:         splitCSV(c.Bcc),
		ReplyTo:     c.ReplyTo,
		Subject:     c.Subject,
		Body:        c.Body,
		BodyHTML:    c.BodyHTML,
		InReplyTo:   inReplyTo,
		References:  references,
		Attachments: atts,
	})
	if err != nil {
		return err
	}

	msg := &gmail.Message{
		Raw: base64.RawURLEncoding.EncodeToString(raw),
	}
	if threadID != "" {
		msg.ThreadId = threadID
	}

	draft, err := svc.Users.Drafts.Create("me", &gmail.Draft{Message: msg}).Do()
	if err != nil {
		return err
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
