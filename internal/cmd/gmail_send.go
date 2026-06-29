package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/mailmime"
	"github.com/openclaw/gogcli/internal/tracking"
	"github.com/openclaw/gogcli/internal/ui"
)

type GmailSendCmd struct {
	To                      string   `name:"to" help:"Recipients (comma-separated; required unless --reply-all is used)"`
	Cc                      string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc                     string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject                 string   `name:"subject" help:"Subject (required unless replying; inherited with Re: for replies)"`
	Body                    string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyFile                string   `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML                string   `name:"body-html" help:"Body (HTML; optional)"`
	BodyHTMLFile            string   `name:"body-html-file" help:"HTML body file path ('-' for stdin)"`
	ReplyToMessageID        string   `name:"reply-to-message-id" aliases:"in-reply-to" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ThreadID                string   `name:"thread-id" help:"Reply within a Gmail thread (uses latest message for headers)"`
	ReplyAll                bool     `name:"reply-all" help:"Auto-populate recipients from original message (requires --reply-to-message-id or --thread-id)"`
	ReplyTo                 string   `name:"reply-to" help:"Reply-To header address"`
	Attach                  []string `name:"attach" help:"Attachment file path (repeatable)"`
	From                    string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	composeSignatureOptions `embed:""`
	Track                   bool `name:"track" help:"Enable open tracking (requires tracking setup)"`
	TrackSplit              bool `name:"track-split" help:"Send tracked messages separately per recipient"`
	Quote                   bool `name:"quote" help:"Include quoted original message in reply (requires --reply-to-message-id or --thread-id)"`
}

type sendBatch struct {
	To  []string
	Cc  []string
	Bcc []string
	// TrackingRecipient, when set (only buildSendBatches sets it), is the bare
	// email address — never a formatted "Name" <a@x.com> mailbox — so the
	// identifier baked into the tracking pixel stays stable regardless of how
	// the user typed the recipient. Batches built by the reply/forward/drafts
	// paths leave it empty; sendGmailBatches normalizes defensively on read.
	TrackingRecipient string
}

type sendResult struct {
	To         string
	MessageID  string
	ThreadID   string
	TrackingID string
}

type sendMessageOptions struct {
	FromAddr    string
	ReplyTo     string
	Subject     string
	Body        string
	BodyHTML    string
	ReplyInfo   *replyInfo
	Headers     map[string]string
	Attachments []mailmime.Attachment
	Track       bool
	TrackingCfg *tracking.Config
}

func (c *GmailSendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	replyToMessageID := normalizeGmailMessageID(c.ReplyToMessageID)
	threadID := normalizeGmailThreadID(c.ThreadID)
	subject := strings.TrimSpace(c.Subject)

	body, htmlBodyInput, err := resolveComposeBodyInputs(ctx, c.Body, c.BodyFile, c.BodyHTML, c.BodyHTMLFile)
	if err != nil {
		return err
	}

	if replyToMessageID != "" && threadID != "" {
		return usage("use only one of --reply-to-message-id or --thread-id")
	}

	// Validate --reply-all requires a reply target
	if c.ReplyAll && replyToMessageID == "" && threadID == "" {
		return usage("--reply-all requires --reply-to-message-id or --thread-id")
	}

	// Validate --quote requires a reply target
	if c.Quote && replyToMessageID == "" && threadID == "" {
		return usage("--quote requires --reply-to-message-id or --thread-id")
	}

	// --to is required unless --reply-all is used
	if strings.TrimSpace(c.To) == "" && !c.ReplyAll {
		return usage("required: --to (or use --reply-all with --reply-to-message-id or --thread-id)")
	}
	if subject == "" && replyToMessageID == "" && threadID == "" {
		return usage("required: --subject")
	}
	if strings.TrimSpace(body) == "" && strings.TrimSpace(htmlBodyInput) == "" {
		return usage("required: --body, --body-file, --body-html, or --body-html-file")
	}
	if c.TrackSplit && !c.Track {
		return usage("--track-split requires --track")
	}
	if c.Track && strings.TrimSpace(htmlBodyInput) == "" {
		return usage("--track requires --body-html or --body-html-file (pixel must be in HTML)")
	}
	if sigErr := c.validateSignatureOptions(); sigErr != nil {
		return sigErr
	}
	if headerErr := validateComposeHeaderInputs(c.To, c.Cc, c.Bcc, c.ReplyTo, c.Subject, c.From); headerErr != nil {
		return headerErr
	}

	// Parse the recipient flags before the dry-run so it reports the lists the
	// built message will carry and bad input fails without any API call. Under
	// --reply-all the dry-run's to/cc report the explicit overrides only:
	// auto-populated recipients are resolved later, post-dry-run, by design.
	explicitTo, explicitCc, explicitBcc, err := parseComposeRecipients(c.To, c.Cc, c.Bcc)
	if err != nil {
		return err
	}

	attachPaths, err := expandComposeAttachmentPaths(c.Attach)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.send", map[string]any{
		"to":                  explicitTo,
		"cc":                  explicitCc,
		"bcc":                 explicitBcc,
		"subject":             subject,
		"reply_to_message_id": replyToMessageID,
		"thread_id":           threadID,
		"reply_all":           c.ReplyAll,
		"reply_to":            strings.TrimSpace(c.ReplyTo),
		"from":                strings.TrimSpace(c.From),
		"body_len":            len(body),
		"body_html_len":       len(htmlBodyInput),
		"attachments":         attachPaths,
		"signature":           c.Signature,
		"signature_from":      strings.TrimSpace(c.SignatureFrom),
		"signature_file":      strings.TrimSpace(c.SignatureFile),
		"track":               c.Track,
		"track_split":         c.TrackSplit,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailSendService(ctx, flags)
	if err != nil {
		return err
	}

	from, err := resolveComposeSender(ctx, svc, account, c.From)
	if err != nil {
		return err
	}
	if c.signatureRequested() {
		signature, source, sigErr := c.resolveComposeSignature(ctx, svc, from.sendingEmail)
		if sigErr != nil {
			return sigErr
		}
		if signature.empty() {
			u.Err().Linef("Warning: no signature configured for %s", source)
		} else {
			body, htmlBodyInput = appendComposeSignature(body, htmlBodyInput, signature)
		}
	}
	replyInfo, body, htmlBody, err := prepareComposeReply(ctx, svc, replyToMessageID, threadID, c.Quote, body, htmlBodyInput)
	if err != nil {
		return err
	}
	if subject == "" {
		subject = autoReplySubject("", replyInfo.Subject)
	}

	// Determine recipients
	var toRecipients, ccRecipients []string
	if c.ReplyAll {
		// Auto-populate recipients from original message
		toRecipients, ccRecipients = buildReplyAllRecipients(replyInfo, from.sendingEmail)
	}

	// Explicit --to and --cc override (not merge with) auto-populated
	// recipients. Non-empty flags always parse to at least one recipient
	// (parseMailboxValues rejects zero-parse input), so keying the override on
	// the raw flag matches keying on the parsed list; the raw flag states the
	// intent directly.
	if strings.TrimSpace(c.To) != "" {
		toRecipients = explicitTo
	}
	if strings.TrimSpace(c.Cc) != "" {
		ccRecipients = explicitCc
	}

	// Final validation: we must have at least one recipient
	if len(toRecipients) == 0 {
		return usage("no recipients: specify --to or use --reply-all with a message that has recipients")
	}

	atts, attachmentMetadata, err := mailmime.PrepareAttachments(attachmentsFromPaths(attachPaths), os.ReadFile)
	if err != nil {
		return err
	}
	atts = append(atts, replyInfo.InlineResources...)

	var trackingCfg *tracking.Config
	if c.Track {
		trackingCfg, err = c.resolveTrackingConfig(ctx, account, toRecipients, ccRecipients, explicitBcc, htmlBody)
		if err != nil {
			return err
		}
	}

	batches := buildSendBatches(toRecipients, ccRecipients, explicitBcc, c.Track, c.TrackSplit)
	results, err := sendGmailBatches(ctx, svc, sendMessageOptions{
		FromAddr:    from.header,
		ReplyTo:     c.ReplyTo,
		Subject:     subject,
		Body:        body,
		BodyHTML:    htmlBody,
		ReplyInfo:   replyInfo,
		Attachments: atts,
		Track:       c.Track,
		TrackingCfg: trackingCfg,
	}, batches)
	if err != nil {
		return err
	}

	return writeSendResults(ctx, u, from.header, results, attachmentMetadata)
}

func (c *GmailSendCmd) resolveTrackingConfig(ctx context.Context, account string, toRecipients, ccRecipients, bccRecipients []string, htmlBody string) (*tracking.Config, error) {
	totalRecipients := len(toRecipients) + len(ccRecipients) + len(bccRecipients)
	if totalRecipients != 1 && !c.TrackSplit {
		return nil, usage("--track requires exactly 1 recipient (no cc/bcc); use --track-split for per-recipient sends")
	}

	if strings.TrimSpace(htmlBody) == "" {
		return nil, usage("--track requires an HTML body (use --body-html or --quote)")
	}

	trackingCfg, _, _, err := loadTrackingConfig(ctx, account, true)
	if err != nil {
		return nil, fmt.Errorf("load tracking config: %w", err)
	}
	if !trackingCfg.IsConfigured() {
		return nil, trackingConfigError("tracking not configured; run 'gog gmail track setup' first")
	}

	return trackingCfg, nil
}

func listSendAs(ctx context.Context, svc *gmail.Service) ([]*gmail.SendAs, error) {
	if svc == nil {
		return nil, nil
	}
	resp, err := svc.Users.Settings.SendAs.List("me").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return resp.SendAs, nil
}

func findSendAsByEmail(sendAs []*gmail.SendAs, email string) *gmail.SendAs {
	needle := strings.ToLower(strings.TrimSpace(email))
	if needle == "" {
		return nil
	}
	for _, sa := range sendAs {
		if sa == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(sa.SendAsEmail)) == needle {
			return sa
		}
	}
	return nil
}

func primaryDisplayNameFromSendAsList(sendAs []*gmail.SendAs, account string) string {
	account = strings.TrimSpace(account)
	if account == "" {
		return ""
	}

	if sa := findSendAsByEmail(sendAs, account); sa != nil {
		if displayName := strings.TrimSpace(sa.DisplayName); displayName != "" {
			return displayName
		}
	}

	for _, sa := range sendAs {
		if sa == nil || !sa.IsPrimary {
			continue
		}
		if displayName := strings.TrimSpace(sa.DisplayName); displayName != "" {
			return displayName
		}
	}

	return ""
}

// buildSendBatches splits a tracked multi-recipient send into per-recipient
// batches (--track-split) or returns a single batch carrying all recipients.
func buildSendBatches(toRecipients, ccRecipients, bccRecipients []string, track, trackSplit bool) []sendBatch {
	totalRecipients := len(toRecipients) + len(ccRecipients) + len(bccRecipients)
	if track && trackSplit && totalRecipients > 1 {
		recipients := append(append(append([]string{}, toRecipients...), ccRecipients...), bccRecipients...)
		recipients = deduplicateAddresses(recipients)

		batches := make([]sendBatch, 0, len(recipients))
		for _, recipient := range recipients {
			batches = append(batches, sendBatch{
				To:                []string{recipient},
				TrackingRecipient: bareEmail(recipient),
			})
		}

		return batches
	}

	trackingRecipient := bareEmail(firstRecipient(toRecipients, ccRecipients, bccRecipients))
	return []sendBatch{{
		To:                toRecipients,
		Cc:                ccRecipients,
		Bcc:               bccRecipients,
		TrackingRecipient: trackingRecipient,
	}}
}

func sendGmailBatches(ctx context.Context, svc *gmail.Service, opts sendMessageOptions, batches []sendBatch) ([]sendResult, error) {
	results := make([]sendResult, 0, len(batches))
	for _, batch := range batches {
		htmlBody := opts.BodyHTML
		trackingID := ""
		if opts.Track {
			recipient := bareEmail(batch.TrackingRecipient)
			if recipient == "" {
				recipient = bareEmail(firstRecipient(batch.To, batch.Cc, batch.Bcc))
			}
			if recipient == "" {
				return nil, errors.New("tracking requires a recipient")
			}
			pixelURL, blob, pixelErr := tracking.GeneratePixelURL(opts.TrackingCfg, recipient, opts.Subject)
			if pixelErr != nil {
				return nil, fmt.Errorf("generate tracking pixel: %w", pixelErr)
			}
			trackingID = blob

			// Inject pixel into HTML body (prefer before </body> / </html>)
			pixelHTML := tracking.GeneratePixelHTML(pixelURL)
			htmlBody = injectTrackingPixelHTML(htmlBody, pixelHTML)
		}

		messageOpts := opts
		messageOpts.BodyHTML = htmlBody
		msg, err := buildGmailMessage(ctx, messageOpts, batch, false)
		if err != nil {
			return nil, err
		}

		sent, err := svc.Users.Messages.Send("me", msg).Context(ctx).Do()
		if err != nil {
			return nil, err
		}

		// Results report the mailbox as actually sent (possibly a formatted
		// "Name" <a@x.com>); only the tracking identity above is bare.
		resultRecipient := strings.TrimSpace(firstRecipient(batch.To, batch.Cc, batch.Bcc))
		results = append(results, sendResult{
			To:         resultRecipient,
			MessageID:  sent.Id,
			ThreadID:   sent.ThreadId,
			TrackingID: trackingID,
		})
	}

	return results, nil
}

func writeSendResults(ctx context.Context, u *ui.UI, fromAddr string, results []sendResult, attachments []mailmime.AttachmentMetadata) error {
	items := make([]gmailMessageResult, 0, len(results))
	for _, r := range results {
		items = append(items, gmailMessageResult{
			From:        fromAddr,
			To:          r.To,
			MessageID:   r.MessageID,
			ThreadID:    r.ThreadID,
			TrackingID:  r.TrackingID,
			Attachments: attachments,
		})
	}
	return writeGmailMessageResults(ctx, u, items)
}

func firstRecipient(toRecipients, ccRecipients, bccRecipients []string) string {
	if len(toRecipients) > 0 {
		return toRecipients[0]
	}
	if len(ccRecipients) > 0 {
		return ccRecipients[0]
	}
	if len(bccRecipients) > 0 {
		return bccRecipients[0]
	}
	return ""
}

func injectTrackingPixelHTML(htmlBody, pixelHTML string) string {
	lower := strings.ToLower(htmlBody)
	if i := strings.LastIndex(lower, "</body>"); i != -1 {
		return htmlBody[:i] + pixelHTML + htmlBody[i:]
	}
	if i := strings.LastIndex(lower, "</html>"); i != -1 {
		return htmlBody[:i] + pixelHTML + htmlBody[i:]
	}
	return htmlBody + pixelHTML
}
