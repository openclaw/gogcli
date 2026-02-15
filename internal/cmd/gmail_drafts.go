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
	List   GmailDraftsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List drafts"`
	Get    GmailDraftsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get draft details"`
	Delete GmailDraftsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a draft"`
	Send   GmailDraftsSendCmd   `cmd:"" name:"send" aliases:"post" help:"Send a draft"`
	Create GmailDraftsCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a draft"`
	Update GmailDraftsUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a draft"`
}

type GmailDraftsListCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"20"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*gmail.Draft, string, error) {
		call := svc.Users.Drafts.List("me").MaxResults(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Drafts, resp.NextPageToken, nil
	}

	var drafts []*gmail.Draft
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		drafts = all
	} else {
		var err error
		drafts, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			ID        string `json:"id"`
			MessageID string `json:"messageId,omitempty"`
			ThreadID  string `json:"threadId,omitempty"`
		}
		items := make([]item, 0, len(drafts))
		for _, d := range drafts {
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
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"drafts":        items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}
	if len(drafts) == 0 {
		u.Err().Println("No drafts")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tMESSAGE_ID")
	for _, d := range drafts {
		msgID := ""
		if d.Message != nil {
			msgID = d.Message.Id
		}
		fmt.Fprintf(w, "%s\t%s\n", d.Id, msgID)
	}
	printNextPageHint(u, nextPageToken)
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
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"draft": draft})
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
		return outfmt.WriteJSON(ctx, os.Stdout, out)
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
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete gmail draft %s", draftID)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Users.Drafts.Delete("me", draftID).Do(); err != nil {
		return err
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("draftId", draftID),
	)
}

type GmailDraftsSendCmd struct {
	DraftID string `arg:"" name:"draftId" help:"Draft ID"`
}

func (c *GmailDraftsSendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	if err := dryRunExit(ctx, flags, "gmail.drafts.send", map[string]any{
		"draft_id": draftID,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
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

	info, err := fetchReplyInfo(ctx, svc, input.ReplyToMessageID, input.ReplyToThreadID, false)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
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

	body, err := resolveBodyInput(c.Body, c.BodyFile)
	if err != nil {
		return err
	}
	replyToMessageID := normalizeGmailMessageID(c.ReplyToMessageID)

	attachPaths := make([]string, 0, len(c.Attach))
	for _, p := range c.Attach {
		expanded, expandErr := config.ExpandPath(p)
		if expandErr != nil {
			return expandErr
		}
		attachPaths = append(attachPaths, expanded)
	}

	input := draftComposeInput{
		To:               c.To,
		Cc:               c.Cc,
		Bcc:              c.Bcc,
		Subject:          c.Subject,
		Body:             body,
		BodyHTML:         c.BodyHTML,
		ReplyToMessageID: replyToMessageID,
		ReplyToThreadID:  "",
		ReplyTo:          c.ReplyTo,
		Attach:           attachPaths,
		From:             c.From,
	}
	if validateErr := input.validate(); validateErr != nil {
		return validateErr
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.drafts.create", map[string]any{
		"to":                  splitCSV(input.To),
		"cc":                  splitCSV(input.Cc),
		"bcc":                 splitCSV(input.Bcc),
		"subject":             strings.TrimSpace(input.Subject),
		"body_len":            len(strings.TrimSpace(input.Body)),
		"body_html_len":       len(strings.TrimSpace(input.BodyHTML)),
		"reply_to_message_id": strings.TrimSpace(input.ReplyToMessageID),
		"reply_to":            strings.TrimSpace(input.ReplyTo),
		"from":                strings.TrimSpace(input.From),
		"attachments":         attachPaths,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
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
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	to := ""
	toWasSet := false
	if c.To != nil {
		toWasSet = true
		to = *c.To
	}

	body, err := resolveBodyInput(c.Body, c.BodyFile)
	if err != nil {
		return err
	}
	replyToMessageID := normalizeGmailMessageID(c.ReplyToMessageID)

	attachPaths := make([]string, 0, len(c.Attach))
	for _, p := range c.Attach {
		expanded, expandErr := config.ExpandPath(p)
		if expandErr != nil {
			return expandErr
		}
		attachPaths = append(attachPaths, expanded)
	}

	input := draftComposeInput{
		To:               to,
		Cc:               c.Cc,
		Bcc:              c.Bcc,
		Subject:          c.Subject,
		Body:             body,
		BodyHTML:         c.BodyHTML,
		ReplyToMessageID: replyToMessageID,
		ReplyToThreadID:  "",
		ReplyTo:          c.ReplyTo,
		Attach:           attachPaths,
		From:             c.From,
	}
	if validateErr := input.validate(); validateErr != nil {
		return validateErr
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.drafts.update", map[string]any{
		"draft_id":            draftID,
		"to_keep_existing":    !toWasSet,
		"to":                  splitCSV(input.To),
		"cc":                  splitCSV(input.Cc),
		"bcc":                 splitCSV(input.Bcc),
		"subject":             strings.TrimSpace(input.Subject),
		"body_len":            len(strings.TrimSpace(input.Body)),
		"body_html_len":       len(strings.TrimSpace(input.BodyHTML)),
		"reply_to_message_id": strings.TrimSpace(input.ReplyToMessageID),
		"reply_to":            strings.TrimSpace(input.ReplyTo),
		"from":                strings.TrimSpace(input.From),
		"attachments":         attachPaths,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	existingThreadID := ""
	existingTo := ""
	if !toWasSet || strings.TrimSpace(replyToMessageID) == "" {
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

	replyToThreadID := ""
	if strings.TrimSpace(replyToMessageID) == "" {
		replyToThreadID = existingThreadID
	}

	input.To = to
	input.ReplyToThreadID = replyToThreadID

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
