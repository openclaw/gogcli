package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/gmailcontent"
	"github.com/openclaw/gogcli/internal/mailmime"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type GmailDraftsCmd struct {
	List     GmailDraftsListCmd     `cmd:"" name:"list" aliases:"ls" help:"List drafts"`
	Get      GmailDraftsGetCmd      `cmd:"" name:"get" aliases:"info,show" help:"Get draft details"`
	Delete   GmailDraftsDeleteCmd   `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Permanently delete a draft (not recoverable; drafts are not moved to Trash)"`
	Send     GmailDraftsSendCmd     `cmd:"" name:"send" aliases:"post" help:"Send a draft"`
	Create   GmailDraftsCreateCmd   `cmd:"" name:"create" aliases:"add,new" help:"Create a draft"`
	Update   GmailDraftsUpdateCmd   `cmd:"" name:"update" aliases:"edit,set" help:"Update a draft"`
	Reply    GmailDraftsReplyCmd    `cmd:"" name:"reply" help:"Save a reply as a draft"`
	ReplyAll GmailDraftsReplyAllCmd `cmd:"" name:"reply-all" aliases:"replyall" help:"Save a reply-all as a draft"`
	Forward  GmailDraftsForwardCmd  `cmd:"" name:"forward" aliases:"fwd" help:"Save a forward as a draft"`
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
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type GmailDraftsGetCmd struct {
	DraftID                 string `arg:"" name:"draftId" help:"Draft ID"`
	UseIndexedAttachmentIDs bool   `name:"use-indexed-attachment-ids" help:"Use 0-based indexes as attachment ids everywhere (output, the download argument, and saved filenames)" env:"GOG_GMAIL_USE_INDEXED_ATTACHMENT_IDS"`
	Download                bool   `name:"download" help:"Download draft attachments"`
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
				downloads, err = downloadAttachmentOutputs(ctx, svc, msg.Id, attachments, attachDir, c.UseIndexedAttachmentIDs)
				if err != nil {
					return err
				}
			}
			out["downloaded"] = attachmentDownloadDraftOutputs(downloads)
		}
		// The raw draft dump carries the opaque attachmentIds too; in indexed mode
		// expose a curated index mapping, then strip the long ids from the raw dump.
		if c.UseIndexedAttachmentIDs {
			out["attachments"] = attachmentOutputs(attachments, true)
			stripAttachmentIDs(msg.Payload)
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

	body := gmailcontent.BestBodyText(msg.Payload)
	if body != "" {
		u.Out().Println(body)
		u.Out().Println("")
	}

	printAttachmentSection(u.Out(), attachments, c.UseIndexedAttachmentIDs)

	if attachDir != "" {
		downloads, err := downloadAttachmentOutputs(ctx, svc, msg.Id, attachments, attachDir, c.UseIndexedAttachmentIDs)
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
	To                     string   `name:"to" help:"Recipients (comma-separated)"`
	Cc                     string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc                    string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject                string   `name:"subject" help:"Subject (required)"`
	Body                   string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyFile               string   `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML               string   `name:"body-html" help:"Body (HTML; optional)"`
	BodyHTMLFile           string   `name:"body-html-file" help:"HTML body file path ('-' for stdin)"`
	ReplyToMessageID       string   `name:"reply-to-message-id" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ThreadID               string   `name:"thread-id" help:"Reply within a Gmail thread (uses latest message for headers)"`
	ReplyAll               bool     `name:"reply-all" help:"Auto-populate recipients from original message (requires --reply-to-message-id or --thread-id)"`
	ReplyTo                string   `name:"reply-to" help:"Reply-To header address"`
	Quote                  bool     `name:"quote" help:"Include quoted original message in reply (requires --reply-to-message-id or --thread-id)"`
	Attach                 []string `name:"attach" help:"Attachment file path (repeatable)"`
	From                   string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	AutoFromAddressedAlias bool     `name:"auto-from-addressed-alias" help:"When --from is omitted, reply from the verified send-as alias addressed by the original message" env:"GOG_GMAIL_AUTO_FROM_ADDRESSED_ALIAS"`
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
	// ThreadContinuityID keeps a rebuilt message in the thread it already
	// belongs to. It is deliberately NOT a reply target: where a message lives
	// says nothing about what it replies to. Only used when no reply target
	// resolved a thread of its own.
	ThreadContinuityID string
	// InReplyTo/References carry a reply lineage that has already been
	// resolved elsewhere — on update, the lineage the draft itself already
	// stores. Applied verbatim, so repeated updates are idempotent.
	InReplyTo  string
	References string
	ReplyAll   bool
	ReplyTo    string
	Quote      bool
	Attach     []string
	// PrebuiltAttachments carry already-resolved attachment bytes (e.g. existing
	// draft attachments preserved across an update) alongside any --attach paths.
	PrebuiltAttachments []mailmime.Attachment
	// KeptToRecipients carries the existing draft's To recipients when an
	// update omits --to (kept-existing). They bypass the strict flag parse:
	// the header was already resolved by keptDraftRecipients, so a
	// legacy-malformed header cannot fail a body-only edit.
	KeptToRecipients       []string
	From                   string
	AutoFromAddressedAlias bool
}

func (c draftComposeInput) validate() error {
	if c.ReplyAll &&
		strings.TrimSpace(c.ReplyToMessageID) == "" &&
		strings.TrimSpace(c.ReplyToThreadID) == "" {
		return usage("--reply-all requires --reply-to-message-id or --thread-id")
	}
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

// keptDraftRecipients converts an existing draft's To header into the
// recipient list for a rebuild when an update omits --to. It prefers the
// address-aware parse (display-name commas stay one recipient) but must not
// reject a draft that predates strict parsing: on any parse failure it falls
// back to splitCSV's verbatim fragments, so a body-only edit of a
// legacy-malformed header (e.g. "alice@", or Outlook-style semicolon
// separators) cannot fail. The kept fragments then render exactly as they
// did before this parser: the MIME writer passes an unextractable value
// through verbatim and normalizes one it can extract addresses from.
func keptDraftRecipients(header string) []string {
	if recipients, err := parseRecipientCSV("--to", header); err == nil {
		return recipients
	}
	return splitCSV(header)
}

func buildDraftMessage(ctx context.Context, svc *gmail.Service, account string, input draftComposeInput) (*gmail.Message, draftThreading, []mailmime.AttachmentMetadata, error) {
	sendAs, sendAsErr := listSendAs(ctx, svc)
	from, err := resolveComposeFrom(ctx, svc, account, input.From, sendAs, sendAsErr)
	if err != nil {
		return nil, draftThreading{}, nil, err
	}

	info, body, htmlBody, err := prepareComposeReply(ctx, svc, input.ReplyToMessageID, input.ReplyToThreadID, input.Quote, input.Body, input.BodyHTML)
	if err != nil {
		return nil, draftThreading{}, nil, err
	}
	replyContextSource := ""
	if strings.TrimSpace(info.InReplyTo) != "" {
		replyContextSource = replyContextCaller
	}
	// No reply target resolved a thread, so fall back to the thread the message
	// already lives in. Thread continuity only — this must not populate
	// In-Reply-To/References.
	if strings.TrimSpace(info.ThreadID) == "" {
		info.ThreadID = strings.TrimSpace(input.ThreadContinuityID)
	}
	// No reply target resolved a lineage either, so re-apply whatever lineage
	// the caller carried in (an update preserving the draft's own headers).
	if strings.TrimSpace(info.InReplyTo) == "" && strings.TrimSpace(input.InReplyTo) != "" {
		info.InReplyTo = input.InReplyTo
		info.References = input.References
		if strings.TrimSpace(info.References) == "" {
			info.References = input.InReplyTo
		}
		replyContextSource = replyContextCarried
	}
	threading := draftThreading{
		ThreadID:   info.ThreadID,
		InReplyTo:  strings.TrimSpace(info.InReplyTo),
		References: strings.TrimSpace(info.References),
		Source:     replyContextSource,
	}
	// When requested, reply as the verified alias the original was addressed to.
	if input.AutoFromAddressedAlias && strings.TrimSpace(input.From) == "" && sendAsErr == nil {
		if alias := pickSendAsFromRecipients(info.ToAddrs, info.CcAddrs, sendAs); alias != "" {
			if picked, pickErr := resolveComposeFrom(ctx, svc, account, alias, sendAs, sendAsErr); pickErr == nil {
				from = picked
			}
		}
	}
	atts := attachmentsFromPaths(input.Attach)
	atts = append(atts, input.PrebuiltAttachments...)
	atts = append(atts, info.InlineResources...)
	atts, attachmentMetadata, err := mailmime.PrepareAttachments(atts, os.ReadFile)
	if err != nil {
		return nil, draftThreading{}, nil, err
	}
	subject := input.Subject
	if strings.TrimSpace(subject) == "" {
		subject = autoReplySubject("", info.Subject)
	}

	toRecipients, ccRecipients, bccRecipients, err := parseComposeRecipients(input.To, input.Cc, input.Bcc)
	if err != nil {
		return nil, draftThreading{}, nil, err
	}
	// A kept-existing To (update without --to) was resolved leniently by
	// keptDraftRecipients; input.To is empty then, so the parse above yields
	// nil and the kept recipients take over.
	if len(input.KeptToRecipients) > 0 {
		toRecipients = input.KeptToRecipients
	}
	if input.ReplyAll {
		recipients, recipientErr := buildReplyRecipients(
			info,
			selfEmailsForReply(account, from.sendingEmail, sendAs),
			true,
			nil,
			nil,
			nil,
			nil,
		)
		if recipientErr != nil {
			return nil, draftThreading{}, nil, recipientErr
		}
		// --reply-all auto-populates from the original message, but an explicit
		// flag (parsed above) overrides the corresponding auto-populated field.
		if strings.TrimSpace(input.To) == "" {
			toRecipients = formatMailboxes(recipients.To)
		}
		if strings.TrimSpace(input.Cc) == "" {
			ccRecipients = formatMailboxes(recipients.Cc)
		}
		if strings.TrimSpace(input.Bcc) == "" {
			bccRecipients = formatMailboxes(recipients.Bcc)
		}
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
		To:  toRecipients,
		Cc:  ccRecipients,
		Bcc: bccRecipients,
	}, true)
	if err != nil {
		return nil, draftThreading{}, nil, err
	}

	return msg, threading, attachmentMetadata, nil
}

// draftHasHTMLBodyPart reports whether a draft message's stored payload carries
// a text/html body part (a rich-text draft). Attachment parts (non-empty
// filename) don't count, so an attached .html file is not a rich body.
func draftHasHTMLBodyPart(payload *gmail.MessagePart) bool {
	if payload == nil {
		return false
	}
	// A named MIME subtree is an attachment, including an attached message whose
	// nested body may itself contain text/html.
	if strings.TrimSpace(payload.Filename) != "" {
		return false
	}
	if strings.EqualFold(payload.MimeType, "text/html") {
		return true
	}
	for _, part := range payload.Parts {
		if draftHasHTMLBodyPart(part) {
			return true
		}
	}
	return false
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
					decoded, decErr := gmailcontent.DecodeBase64URLBytes(part.Body.Data)
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
	return gmailcontent.DecodeBase64URLBytes(body.Data)
}

// draftThreading is the threading actually written to a draft: where it lives
// (ThreadID) and what it replies to (InReplyTo/References), reported back so a
// caller can verify without re-fetching raw headers. Source records where the
// reply lineage came from — "caller" (an explicit reply target) or "carried"
// (preserved from the draft's own stored headers); empty when the draft has no
// reply context at all, which reports as null alongside the headers themselves.
type draftThreading struct {
	ThreadID   string
	InReplyTo  string
	References string
	Source     string
}

const (
	replyContextCaller  = "caller"
	replyContextCarried = "carried"
)

// nilIfEmpty reports a header as an explicit JSON null when unset, so callers
// can distinguish "no reply context" from "field not reported".
func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func writeDraftResult(ctx context.Context, u *ui.UI, draft *gmail.Draft, threading draftThreading, attachments []mailmime.AttachmentMetadata) error {
	threadID := threading.ThreadID
	if threadID == "" && draft != nil && draft.Message != nil {
		threadID = draft.Message.ThreadId
	}
	source := threading.Source
	if source == replyContextCarried {
		u.Err().Linef("Warning: reply headers preserved from the existing draft (In-Reply-To %s); pass --clear-reply-context to drop them", threading.InReplyTo)
	}
	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"draftId":            draft.Id,
			"message":            draft.Message,
			"threadId":           threadID,
			"inReplyTo":          nilIfEmpty(threading.InReplyTo),
			"references":         nilIfEmpty(threading.References),
			"replyContextSource": nilIfEmpty(source),
		}
		if len(attachments) > 0 {
			result["attachments"] = attachments
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), outfmt.PrimaryResult(result))
	}
	u.Out().Linef("draft_id\t%s", draft.Id)
	if draft.Message != nil && draft.Message.Id != "" {
		u.Out().Linef("message_id\t%s", draft.Message.Id)
	}
	if threadID != "" {
		u.Out().Linef("thread_id\t%s", threadID)
	}
	if threading.InReplyTo != "" {
		u.Out().Linef("in_reply_to\t%s", threading.InReplyTo)
	}
	if threading.References != "" {
		u.Out().Linef("references\t%s", threading.References)
	}
	if source != "" {
		u.Out().Linef("reply_context_source\t%s", source)
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
		To:                     c.To,
		Cc:                     c.Cc,
		Bcc:                    c.Bcc,
		Subject:                c.Subject,
		Body:                   body,
		BodyHTML:               htmlBody,
		ReplyToMessageID:       replyToMessageID,
		ReplyToThreadID:        threadID,
		ReplyAll:               c.ReplyAll,
		ReplyTo:                c.ReplyTo,
		Quote:                  c.Quote,
		Attach:                 attachPaths,
		From:                   c.From,
		AutoFromAddressedAlias: c.AutoFromAddressedAlias,
	}
	if validateErr := input.validate(); validateErr != nil {
		return validateErr
	}
	if headerErr := validateComposeHeaderInputs(input.To, input.Cc, input.Bcc, input.ReplyTo, input.Subject, input.From); headerErr != nil {
		return headerErr
	}

	// Parsed identically in buildDraftMessage, so the dry-run reports the built lists.
	toRecipients, ccRecipients, bccRecipients, err := parseComposeRecipients(input.To, input.Cc, input.Bcc)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.drafts.create", map[string]any{
		"to":                        toRecipients,
		"cc":                        ccRecipients,
		"bcc":                       bccRecipients,
		"subject":                   strings.TrimSpace(input.Subject),
		"body_len":                  len(input.Body),
		"body_html_len":             len(input.BodyHTML),
		"reply_to_message_id":       strings.TrimSpace(input.ReplyToMessageID),
		"thread_id":                 strings.TrimSpace(input.ReplyToThreadID),
		"reply_all":                 input.ReplyAll,
		"reply_to":                  strings.TrimSpace(input.ReplyTo),
		"quote":                     input.Quote,
		"from":                      strings.TrimSpace(input.From),
		"auto_from_addressed_alias": input.AutoFromAddressedAlias,
		"attachments":               attachPaths,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	msg, threading, attachmentMetadata, err := buildDraftMessage(ctx, svc, account, input)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Create("me", &gmail.Draft{Message: msg}).Do()
	if err != nil {
		return err
	}
	return writeDraftResult(ctx, u, draft, threading, attachmentMetadata)
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
	ReplyAll         bool     `name:"reply-all" help:"Auto-populate recipients from original message (requires --reply-to-message-id or --thread-id)"`
	ReplyTo          string   `name:"reply-to" help:"Reply-To header address"`
	Quote            bool     `name:"quote" help:"Include quoted original message in reply"`
	Attach           []string `name:"attach" help:"Attachment file path (repeatable). Replaces existing attachments; omit to preserve them, or use --clear-attachments to remove all."`
	ClearAttachments bool     `name:"clear-attachments" help:"Remove all attachments from the draft. By default, omitting --attach preserves the draft's existing attachments."`
	//nolint:lll // flag help text
	ClearReplyContext      bool   `name:"clear-reply-context" help:"Strip In-Reply-To/References from the draft, making it a standalone message. By default an update preserves the draft's existing reply headers."`
	From                   string `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	AutoFromAddressedAlias bool   `name:"auto-from-addressed-alias" help:"When --from is omitted, reply from the verified send-as alias addressed by the original message" env:"GOG_GMAIL_AUTO_FROM_ADDRESSED_ALIAS"`
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
	if c.ClearReplyContext && (replyToMessageID != "" || threadID != "" || c.Quote) {
		return usage("use only one of --clear-reply-context or --reply-to-message-id/--thread-id/--quote")
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
		To:                     to,
		Cc:                     c.Cc,
		Bcc:                    c.Bcc,
		Subject:                c.Subject,
		Body:                   body,
		BodyHTML:               htmlBody,
		ReplyToMessageID:       replyToMessageID,
		ReplyToThreadID:        threadID,
		ReplyAll:               c.ReplyAll,
		ReplyTo:                c.ReplyTo,
		Quote:                  c.Quote,
		Attach:                 attachPaths,
		From:                   c.From,
		AutoFromAddressedAlias: c.AutoFromAddressedAlias,
	}
	if validateErr := input.validate(); validateErr != nil {
		return validateErr
	}
	if headerErr := validateComposeHeaderInputs(input.To, input.Cc, input.Bcc, input.ReplyTo, input.Subject, input.From); headerErr != nil {
		return headerErr
	}

	// Parsed identically in buildDraftMessage, so the dry-run reports the built
	// lists. (A kept-existing To is empty here; it is resolved later from the
	// draft's own header, leniently, via keptDraftRecipients.)
	toRecipients, ccRecipients, bccRecipients, err := parseComposeRecipients(input.To, input.Cc, input.Bcc)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.drafts.update", map[string]any{
		"draft_id":                  draftID,
		"to_keep_existing":          !toWasSet && !input.ReplyAll,
		"to":                        toRecipients,
		"cc":                        ccRecipients,
		"bcc":                       bccRecipients,
		"subject":                   strings.TrimSpace(input.Subject),
		"body_len":                  len(input.Body),
		"body_html_len":             len(input.BodyHTML),
		"reply_to_message_id":       strings.TrimSpace(input.ReplyToMessageID),
		"reply_all":                 input.ReplyAll,
		"reply_to":                  strings.TrimSpace(input.ReplyTo),
		"quote":                     input.Quote,
		"from":                      strings.TrimSpace(input.From),
		"auto_from_addressed_alias": input.AutoFromAddressedAlias,
		"attachments":               attachPaths,
		"clear_attachments":         c.ClearAttachments,
		"preserve_attachments":      preserveAttachments,
		"thread_id":                 strings.TrimSpace(threadID),
		"clear_reply_context":       c.ClearReplyContext,
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
	existingInReplyTo := ""
	existingReferences := ""
	var existingPayload *gmail.MessagePart
	// Recipients, reply lineage and attachment carry-forward are built from the
	// stored draft, so those updates cannot proceed without it.
	requireExistingDraft := (!toWasSet && !c.ReplyAll) || strings.TrimSpace(replyToMessageID) == "" || preserveAttachments
	// An update carrying no HTML body can silently drop the draft's stored one,
	// so the warning below needs the existing payload. Without this the fetch is
	// skipped whenever --to, --reply-to-message-id and --attach are all supplied,
	// and that path would downgrade a rich-text draft unwarned. --quote is exempt
	// because quoting regenerates an HTML part.
	inspectForHTMLDowngrade := strings.TrimSpace(htmlBody) == "" && !c.Quote
	if requireExistingDraft || inspectForHTMLDowngrade {
		existing, fetchErr := svc.Users.Drafts.Get("me", draftID).Format("full").Do()
		// This read is advisory whenever nothing but the warning wants it: a
		// failed inspection costs the warning, never the update the caller asked
		// for. Only a read the rebuilt message actually depends on is fatal.
		if fetchErr != nil && requireExistingDraft {
			return fetchErr
		}
		if fetchErr == nil && existing != nil && existing.Message != nil {
			existingThreadID = strings.TrimSpace(existing.Message.ThreadId)
			existingMessageID = strings.TrimSpace(existing.Message.Id)
			existingPayload = existing.Message.Payload
			existingInReplyTo = strings.TrimSpace(headerValue(existing.Message.Payload, "In-Reply-To"))
			existingReferences = strings.TrimSpace(headerValue(existing.Message.Payload, "References"))
			if !toWasSet && !c.ReplyAll {
				existingTo = strings.TrimSpace(headerValue(existing.Message.Payload, "To"))
			}
		}
	}
	if !toWasSet && !c.ReplyAll {
		input.KeptToRecipients = keptDraftRecipients(existingTo)
	}

	// gmail drafts update rebuilds the whole message, so updating a rich-text
	// draft with only a plain body silently drops its text/html part — Gmail
	// then renders the stored hard-wrapped plain text literally. Warn on stderr.
	if inspectForHTMLDowngrade && draftHasHTMLBodyPart(existingPayload) {
		u.Err().Println("Warning: draft has an HTML body; this update replaces it with plain text only. Pass --body-html/--body-html-file to keep rich-text formatting.")
	}

	// The thread the draft already lives in. Used for --quote target discovery
	// (which skips drafts) and for thread continuity — never as a reply target.
	// A caller-provided --thread-id overrides it and moves the draft.
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
	// Only a caller-supplied thread is a reply target. Anchoring to the draft's
	// own thread would resolve In-Reply-To/References from the newest message
	// in it — which, for a draft, is the draft's own previous revision: a
	// message that was never sent and that no recipient can thread against.
	if strings.TrimSpace(replyToMessageID) == "" {
		replyToThreadID = threadID
	}

	// Reply lineage survives an update because it is carried forward from the
	// draft's own stored headers, not re-derived. Applying the same values
	// every time keeps repeated updates idempotent and stops References from
	// accumulating. An explicit target re-resolves instead; --clear-reply-context
	// drops the lineage entirely.
	carriedInReplyTo := ""
	carriedReferences := ""
	if !c.ClearReplyContext && strings.TrimSpace(replyToMessageID) == "" && strings.TrimSpace(replyToThreadID) == "" {
		carriedInReplyTo = existingInReplyTo
		carriedReferences = existingReferences
	}

	input.ReplyToMessageID = replyToMessageID
	input.ReplyToThreadID = replyToThreadID
	input.ThreadContinuityID = targetThreadID
	input.InReplyTo = carriedInReplyTo
	input.References = carriedReferences

	msg, threading, attachmentMetadata, err := buildDraftMessage(ctx, svc, account, input)
	if err != nil {
		return err
	}

	draft, err := svc.Users.Drafts.Update("me", draftID, &gmail.Draft{Id: draftID, Message: msg}).Do()
	if err != nil {
		return err
	}
	return writeDraftResult(ctx, u, draft, threading, attachmentMetadata)
}
