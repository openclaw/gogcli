package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/mailmime"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailDraftsCmd struct {
	List   GmailDraftsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List drafts"`
	Get    GmailDraftsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get draft details"`
	Delete GmailDraftsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Permanently delete a draft (not recoverable; drafts are not moved to Trash)"`
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
	if err := validateGmailMaxResults(c.Max); err != nil {
		return err
	}
	_, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*gmail.Draft, string, error) {
		call := svc.Users.Drafts.List("me").MaxResults(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Drafts, resp.NextPageToken, nil
	}

	drafts, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
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
		return writePagedJSONResult(ctx, map[string]any{
			"drafts":        items,
			"nextPageToken": nextPageToken,
		}, len(items), c.FailEmpty)
	}
	if len(drafts) == 0 {
		u.Err().Println("No drafts")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactGmailRows(drafts),
		gmailDraftColumns(),
	); err != nil {
		return err
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
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	_, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Get("me", draftID).Format("full").Do()
	if err != nil {
		return err
	}
	if draft.Message == nil {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"draft": draft})
		}
		u.Err().Println("Empty draft")
		return nil
	}

	msg := draft.Message
	attachments := collectAttachments(msg.Payload)
	attachDir := ""
	if c.Download && msg.Id != "" && len(attachments) > 0 {
		layout, layoutErr := commandLayout(ctx, config.PathKindConfig)
		if layoutErr != nil {
			return layoutErr
		}
		attachDir = layout.GmailAttachmentsDir()
	}
	if outfmt.IsJSON(ctx) {
		out := map[string]any{"draft": draft}
		if c.Download {
			var downloads []attachmentDownloadOutput
			if attachDir != "" {
				downloads, err = downloadAttachmentOutputs(ctx, svc, msg.Id, attachments, attachDir)
				if err != nil {
					return err
				}
			}
			out["downloaded"] = attachmentDownloadDraftOutputs(downloads)
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), out)
	}

	u.Out().Linef("Draft-ID: %s", draft.Id)
	u.Out().Linef("Message-ID: %s", msg.Id)
	u.Out().Linef("To: %s", headerValue(msg.Payload, "To"))
	u.Out().Linef("Cc: %s", headerValue(msg.Payload, "Cc"))
	u.Out().Linef("Bcc: %s", headerValue(msg.Payload, "Bcc"))
	u.Out().Linef("Subject: %s", headerValue(msg.Payload, "Subject"))
	u.Out().Println("")

	body := bestBodyText(msg.Payload)
	if body != "" {
		u.Out().Println(body)
		u.Out().Println("")
	}

	printAttachmentSection(u.Out(), attachments)

	if attachDir != "" {
		downloads, err := downloadAttachmentOutputs(ctx, svc, msg.Id, attachments, attachDir)
		if err != nil {
			return err
		}
		for _, a := range downloads {
			if a.Cached {
				u.Out().Linef("Cached: %s", a.Path)
			} else {
				u.Out().Successf("Saved: %s", a.Path)
			}
		}
	}

	return nil
}

// GmailDraftsDeleteCmd permanently deletes a draft. The Gmail API's
// users.drafts.delete is irreversible — drafts are not moved to Trash and have
// no untrash path — so this cannot be undone.
type GmailDraftsDeleteCmd struct {
	DraftID string `arg:"" name:"draftId" help:"Draft ID"`
}

func (c *GmailDraftsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	draftID := strings.TrimSpace(c.DraftID)
	if draftID == "" {
		return usage("empty draftId")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "gmail.drafts.delete", map[string]any{
		"draft_id": draftID,
	}, fmt.Sprintf("permanently delete gmail draft %s (not recoverable; drafts are not moved to Trash)", draftID)); confirmErr != nil {
		return confirmErr
	}

	_, svc, err := requireGmailService(ctx, flags)
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

	_, svc, err := requireGmailSendService(ctx, flags)
	if err != nil {
		return err
	}

	msg, err := svc.Users.Drafts.Send("me", &gmail.Draft{Id: draftID}).Do()
	if err != nil {
		return err
	}
	return writeGmailMessageResults(ctx, u, []gmailMessageResult{{
		MessageID: msg.Id,
		ThreadID:  msg.ThreadId,
	}})
}

type GmailDraftsCreateCmd struct {
	To               string   `name:"to" help:"Recipients (comma-separated)"`
	Cc               string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc              string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject          string   `name:"subject" help:"Subject (required)"`
	Body             string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyFile         string   `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML         string   `name:"body-html" help:"Body (HTML; optional)"`
	BodyHTMLFile     string   `name:"body-html-file" help:"HTML body file path ('-' for stdin)"`
	ReplyToMessageID string   `name:"reply-to-message-id" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ThreadID         string   `name:"thread-id" help:"Reply within a Gmail thread (uses latest message for headers)"`
	ReplyTo          string   `name:"reply-to" help:"Reply-To header address"`
	Quote            bool     `name:"quote" help:"Include quoted original message in reply (requires --reply-to-message-id or --thread-id)"`
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
	Quote            bool
	Attach           []string
	// PrebuiltAttachments carry already-resolved attachment bytes (e.g. existing
	// draft attachments preserved across an update) alongside any --attach paths.
	PrebuiltAttachments []mailmime.Attachment
	From                string
}

func (c draftComposeInput) validate() error {
	if strings.TrimSpace(c.Subject) == "" &&
		strings.TrimSpace(c.ReplyToMessageID) == "" &&
		strings.TrimSpace(c.ReplyToThreadID) == "" {
		return usage("required: --subject")
	}
	if strings.TrimSpace(c.Body) == "" && strings.TrimSpace(c.BodyHTML) == "" {
		return usage("required: --body, --body-file, --body-html, or --body-html-file")
	}
	return nil
}

func buildDraftMessage(ctx context.Context, svc *gmail.Service, account string, input draftComposeInput) (*gmail.Message, string, []mailmime.AttachmentMetadata, error) {
	from, err := resolveComposeSender(ctx, svc, account, input.From)
	if err != nil {
		return nil, "", nil, err
	}

	info, body, htmlBody, err := prepareComposeReply(ctx, svc, input.ReplyToMessageID, input.ReplyToThreadID, input.Quote, input.Body, input.BodyHTML)
	if err != nil {
		return nil, "", nil, err
	}
	threadID := info.ThreadID
	atts := attachmentsFromPaths(input.Attach)
	atts = append(atts, input.PrebuiltAttachments...)
	atts, attachmentMetadata, err := mailmime.PrepareAttachments(atts, os.ReadFile)
	if err != nil {
		return nil, "", nil, err
	}
	subject := input.Subject
	if strings.TrimSpace(subject) == "" {
		subject = autoReplySubject("", info.Subject)
	}

	msg, err := buildGmailMessage(ctx, sendMessageOptions{
		FromAddr:    from.header,
		ReplyTo:     input.ReplyTo,
		Subject:     subject,
		Body:        body,
		BodyHTML:    htmlBody,
		ReplyInfo:   info,
		Attachments: atts,
	}, sendBatch{
		To:  splitCSV(input.To),
		Cc:  splitCSV(input.Cc),
		Bcc: splitCSV(input.Bcc),
	}, true)
	if err != nil {
		return nil, "", nil, err
	}

	return msg, threadID, attachmentMetadata, nil
}

// carryForwardDraftAttachments fetches the bytes of an existing draft message's
// attachments so they can be re-attached to a rebuilt draft on update. Returns
// nil when the draft has no attachments. Mirrors the gmail forward reattach path.
func carryForwardDraftAttachments(ctx context.Context, svc *gmail.Service, messageID string, payload *gmail.MessagePart) ([]mailmime.Attachment, error) {
	var out []mailmime.Attachment
	var walk func(*gmail.MessagePart) error
	walk = func(part *gmail.MessagePart) error {
		if part == nil {
			return nil
		}
		if part.Body != nil {
			filename := strings.TrimSpace(part.Filename)
			if filename == "" && part.Body.AttachmentId != "" {
				filename = defaultAttachmentFilename
			}
			switch {
			case part.Body.AttachmentId != "":
				data, dlErr := fetchDraftAttachmentBytes(ctx, svc, messageID, part.Body.AttachmentId, part.Body.Size)
				if dlErr != nil {
					return fmt.Errorf("preserve attachment %q: %w", filename, dlErr)
				}
				out = append(out, mailmime.Attachment{
					Filename: filename,
					MIMEType: part.MimeType,
					Data:     data,
					DataSet:  true,
				})
			case filename != "" && (part.Body.Data != "" || part.Body.Size == 0):
				var data []byte
				if part.Body.Data != "" {
					decoded, decErr := decodeBase64URLBytes(part.Body.Data)
					if decErr != nil {
						return fmt.Errorf("preserve attachment %q: %w", filename, decErr)
					}
					data = decoded
				}
				out = append(out, mailmime.Attachment{
					Filename: filename,
					MIMEType: part.MimeType,
					Data:     data,
					DataSet:  true,
				})
			}
		}
		for _, child := range part.Parts {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(payload); err != nil {
		return nil, err
	}
	return out, nil
}

func fetchDraftAttachmentBytes(ctx context.Context, svc *gmail.Service, messageID, attachmentID string, expectedSize int64) ([]byte, error) {
	body, err := svc.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("empty attachment data")
	}
	if body.Data == "" {
		if expectedSize == 0 {
			return []byte{}, nil
		}
		return nil, errors.New("empty attachment data")
	}
	return decodeBase64URLBytes(body.Data)
}

func writeDraftResult(ctx context.Context, u *ui.UI, draft *gmail.Draft, threadID string, attachments []mailmime.AttachmentMetadata) error {
	if threadID == "" && draft != nil && draft.Message != nil {
		threadID = draft.Message.ThreadId
	}
	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"draftId":  draft.Id,
			"message":  draft.Message,
			"threadId": threadID,
		}
		if len(attachments) > 0 {
			result["attachments"] = attachments
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	u.Out().Linef("draft_id\t%s", draft.Id)
	if draft.Message != nil && draft.Message.Id != "" {
		u.Out().Linef("message_id\t%s", draft.Message.Id)
	}
	if threadID != "" {
		u.Out().Linef("thread_id\t%s", threadID)
	}
	return nil
}

func resolveQuoteReplyTargetMessageID(ctx context.Context, svc *gmail.Service, threadID string, account string, excludeMessageID string) (string, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return "", usage("--quote requires --reply-to-message-id or existing draft thread")
	}

	thread, err := fetchThreadForReplyInfo(ctx, svc, threadID)
	if err != nil {
		return "", err
	}
	if thread == nil || len(thread.Messages) == 0 {
		return "", usage("--quote requires --reply-to-message-id or existing draft thread")
	}

	msg := selectLatestThreadReplyTarget(thread.Messages, account, excludeMessageID)
	if msg == nil || strings.TrimSpace(msg.Id) == "" {
		return "", usage("--quote requires --reply-to-message-id or existing draft thread with a non-draft, non-self message")
	}
	return msg.Id, nil
}

func selectLatestThreadReplyTarget(messages []*gmail.Message, account string, excludeMessageID string) *gmail.Message {
	account = strings.ToLower(strings.TrimSpace(account))
	excludeMessageID = strings.TrimSpace(excludeMessageID)

	var selected *gmail.Message
	var selectedDate int64
	hasDate := false

	for _, msg := range messages {
		if msg == nil || strings.TrimSpace(msg.Id) == "" {
			continue
		}
		if excludeMessageID != "" && strings.TrimSpace(msg.Id) == excludeMessageID {
			continue
		}
		if hasLabel(msg.LabelIds, "DRAFT") {
			continue
		}
		if account != "" && messageFromMatchesAccount(msg, account) {
			continue
		}

		if msg.InternalDate <= 0 {
			if selected == nil && !hasDate {
				selected = msg
			}
			continue
		}
		if !hasDate || msg.InternalDate > selectedDate {
			selected = msg
			selectedDate = msg.InternalDate
			hasDate = true
		}
	}
	return selected
}

func hasLabel(labels []string, target string) bool {
	target = strings.ToUpper(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, l := range labels {
		if strings.ToUpper(strings.TrimSpace(l)) == target {
			return true
		}
	}
	return false
}

func messageFromMatchesAccount(msg *gmail.Message, account string) bool {
	if msg == nil {
		return false
	}
	fromHeader := headerValue(msg.Payload, "From")
	if strings.TrimSpace(fromHeader) == "" {
		return false
	}
	for _, addr := range parseEmailAddresses(fromHeader) {
		if strings.EqualFold(addr, account) {
			return true
		}
	}
	return false
}

func (c *GmailDraftsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	body, htmlBody, err := resolveComposeBodyInputs(ctx, c.Body, c.BodyFile, c.BodyHTML, c.BodyHTMLFile)
	if err != nil {
		return err
	}
	replyToMessageID := normalizeGmailMessageID(c.ReplyToMessageID)
	threadID := normalizeGmailThreadID(c.ThreadID)
	if replyToMessageID != "" && threadID != "" {
		return usage("use only one of --reply-to-message-id or --thread-id")
	}
	if c.Quote && replyToMessageID == "" && threadID == "" {
		return usage("--quote requires --reply-to-message-id or --thread-id")
	}

	attachPaths, err := expandComposeAttachmentPaths(c.Attach)
	if err != nil {
		return err
	}

	input := draftComposeInput{
		To:               c.To,
		Cc:               c.Cc,
		Bcc:              c.Bcc,
		Subject:          c.Subject,
		Body:             body,
		BodyHTML:         htmlBody,
		ReplyToMessageID: replyToMessageID,
		ReplyToThreadID:  threadID,
		ReplyTo:          c.ReplyTo,
		Quote:            c.Quote,
		Attach:           attachPaths,
		From:             c.From,
	}
	if validateErr := input.validate(); validateErr != nil {
		return validateErr
	}
	if headerErr := validateComposeHeaderInputs(input.To, input.Cc, input.Bcc, input.ReplyTo, input.Subject, input.From); headerErr != nil {
		return headerErr
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.drafts.create", map[string]any{
		"to":                  splitCSV(input.To),
		"cc":                  splitCSV(input.Cc),
		"bcc":                 splitCSV(input.Bcc),
		"subject":             strings.TrimSpace(input.Subject),
		"body_len":            len(strings.TrimSpace(input.Body)),
		"body_html_len":       len(strings.TrimSpace(input.BodyHTML)),
		"reply_to_message_id": strings.TrimSpace(input.ReplyToMessageID),
		"thread_id":           strings.TrimSpace(input.ReplyToThreadID),
		"reply_to":            strings.TrimSpace(input.ReplyTo),
		"quote":               input.Quote,
		"from":                strings.TrimSpace(input.From),
		"attachments":         attachPaths,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	msg, threadID, attachmentMetadata, err := buildDraftMessage(ctx, svc, account, input)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Create("me", &gmail.Draft{Message: msg}).Do()
	if err != nil {
		return err
	}
	return writeDraftResult(ctx, u, draft, threadID, attachmentMetadata)
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
	BodyHTMLFile     string   `name:"body-html-file" help:"HTML body file path ('-' for stdin)"`
	ReplyToMessageID string   `name:"reply-to-message-id" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ThreadID         string   `name:"thread-id" help:"Reply within a Gmail thread (uses latest message for headers); overrides the draft's existing thread"`
	ReplyTo          string   `name:"reply-to" help:"Reply-To header address"`
	Quote            bool     `name:"quote" help:"Include quoted original message in reply"`
	Attach           []string `name:"attach" help:"Attachment file path (repeatable). Replaces existing attachments; omit to preserve them, or use --clear-attachments to remove all."`
	ClearAttachments bool     `name:"clear-attachments" help:"Remove all attachments from the draft. By default, omitting --attach preserves the draft's existing attachments."`
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

	body, htmlBody, err := resolveComposeBodyInputs(ctx, c.Body, c.BodyFile, c.BodyHTML, c.BodyHTMLFile)
	if err != nil {
		return err
	}
	replyToMessageID := normalizeGmailMessageID(c.ReplyToMessageID)
	threadID := normalizeGmailThreadID(c.ThreadID)
	if replyToMessageID != "" && threadID != "" {
		return usage("use only one of --reply-to-message-id or --thread-id")
	}

	attachPaths, err := expandComposeAttachmentPaths(c.Attach)
	if err != nil {
		return err
	}
	if c.ClearAttachments && len(attachPaths) > 0 {
		return usage("use only one of --attach or --clear-attachments")
	}
	// gmail drafts update rebuilds the whole message, so without intervention an
	// omitted --attach would silently drop the draft's existing attachments.
	// Preserve them by default; --attach replaces, --clear-attachments removes.
	preserveAttachments := len(attachPaths) == 0 && !c.ClearAttachments

	input := draftComposeInput{
		To:               to,
		Cc:               c.Cc,
		Bcc:              c.Bcc,
		Subject:          c.Subject,
		Body:             body,
		BodyHTML:         htmlBody,
		ReplyToMessageID: replyToMessageID,
		ReplyToThreadID:  threadID,
		ReplyTo:          c.ReplyTo,
		Quote:            c.Quote,
		Attach:           attachPaths,
		From:             c.From,
	}
	if validateErr := input.validate(); validateErr != nil {
		return validateErr
	}
	if headerErr := validateComposeHeaderInputs(input.To, input.Cc, input.Bcc, input.ReplyTo, input.Subject, input.From); headerErr != nil {
		return headerErr
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.drafts.update", map[string]any{
		"draft_id":             draftID,
		"to_keep_existing":     !toWasSet,
		"to":                   splitCSV(input.To),
		"cc":                   splitCSV(input.Cc),
		"bcc":                  splitCSV(input.Bcc),
		"subject":              strings.TrimSpace(input.Subject),
		"body_len":             len(strings.TrimSpace(input.Body)),
		"body_html_len":        len(strings.TrimSpace(input.BodyHTML)),
		"reply_to_message_id":  strings.TrimSpace(input.ReplyToMessageID),
		"reply_to":             strings.TrimSpace(input.ReplyTo),
		"quote":                input.Quote,
		"from":                 strings.TrimSpace(input.From),
		"attachments":          attachPaths,
		"clear_attachments":    c.ClearAttachments,
		"preserve_attachments": preserveAttachments,
		"thread_id":            strings.TrimSpace(threadID),
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	existingThreadID := ""
	existingMessageID := ""
	existingTo := ""
	var existingPayload *gmail.MessagePart
	if !toWasSet || strings.TrimSpace(replyToMessageID) == "" || preserveAttachments {
		existing, fetchErr := svc.Users.Drafts.Get("me", draftID).Format("full").Do()
		if fetchErr != nil {
			return fetchErr
		}
		if existing != nil && existing.Message != nil {
			existingThreadID = strings.TrimSpace(existing.Message.ThreadId)
			existingMessageID = strings.TrimSpace(existing.Message.Id)
			existingPayload = existing.Message.Payload
			if !toWasSet {
				existingTo = strings.TrimSpace(headerValue(existing.Message.Payload, "To"))
			}
		}
	}
	if !toWasSet {
		to = existingTo
	}

	// A caller-provided --thread-id targets that thread for reply headers,
	// overriding the draft's own existing thread; otherwise fall back to the
	// existing draft thread as before.
	targetThreadID := existingThreadID
	if threadID != "" {
		targetThreadID = threadID
	}

	// Carry the existing draft's attachments forward unless the caller replaced
	// (--attach) or explicitly cleared (--clear-attachments) them.
	if preserveAttachments && existingMessageID != "" {
		carried, attErr := carryForwardDraftAttachments(ctx, svc, existingMessageID, existingPayload)
		if attErr != nil {
			return attErr
		}
		input.PrebuiltAttachments = carried
	}

	replyToThreadID := ""
	if c.Quote && strings.TrimSpace(replyToMessageID) == "" {
		resolvedMessageID, resolveErr := resolveQuoteReplyTargetMessageID(ctx, svc, targetThreadID, account, existingMessageID)
		if resolveErr != nil {
			return resolveErr
		}
		replyToMessageID = resolvedMessageID
	}
	if strings.TrimSpace(replyToMessageID) == "" {
		replyToThreadID = targetThreadID
	}
	if c.Quote && strings.TrimSpace(replyToMessageID) == "" && strings.TrimSpace(replyToThreadID) == "" {
		return usage("--quote requires --reply-to-message-id or --thread-id or existing draft thread")
	}

	input.To = to
	input.ReplyToMessageID = replyToMessageID
	input.ReplyToThreadID = replyToThreadID

	msg, threadID, attachmentMetadata, err := buildDraftMessage(ctx, svc, account, input)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Update("me", draftID, &gmail.Draft{Id: draftID, Message: msg}).Do()
	if err != nil {
		return err
	}
	return writeDraftResult(ctx, u, draft, threadID, attachmentMetadata)
}
