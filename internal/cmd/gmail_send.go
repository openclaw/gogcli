package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/tracking"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailSendCmd struct {
	To               string   `name:"to" help:"Recipients (comma-separated; required unless --reply-all is used)"`
	Cc               string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc              string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject          string   `name:"subject" help:"Subject (required)"`
	Body             string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyFile         string   `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML         string   `name:"body-html" help:"Body (HTML; optional)"`
	ReplyToMessageID string   `name:"reply-to-message-id" aliases:"in-reply-to" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ThreadID         string   `name:"thread-id" help:"Reply within a Gmail thread (uses latest message for headers)"`
	ReplyAll         bool     `name:"reply-all" help:"Auto-populate recipients from original message (requires --reply-to-message-id or --thread-id)"`
	ReplyTo          string   `name:"reply-to" help:"Reply-To header address"`
	Attach           []string `name:"attach" help:"Attachment file path (repeatable)"`
	From             string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	Signature        bool     `name:"signature" help:"Append the Gmail signature from the active send-as address"`
	SignatureFrom    string   `name:"signature-from" help:"Append the Gmail signature from this send-as email address"`
	SignatureFile    string   `name:"signature-file" help:"Append a local signature file (plain text or HTML)"`
	Track            bool     `name:"track" help:"Enable open tracking (requires tracking setup)"`
	TrackSplit       bool     `name:"track-split" help:"Send tracked messages separately per recipient"`
	Quote            bool     `name:"quote" help:"Include quoted original message in reply (requires --reply-to-message-id or --thread-id)"`
}

type sendBatch struct {
	To                []string
	Cc                []string
	Bcc               []string
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
	Attachments []mailAttachment
	Track       bool
	TrackingCfg *tracking.Config
}

func (c *GmailSendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	replyToMessageID := normalizeGmailMessageID(c.ReplyToMessageID)
	threadID := normalizeGmailThreadID(c.ThreadID)

	body, err := resolveBodyInput(c.Body, c.BodyFile)
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
	if strings.TrimSpace(c.Subject) == "" {
		return usage("required: --subject")
	}
	if strings.TrimSpace(body) == "" && strings.TrimSpace(c.BodyHTML) == "" {
		return usage("required: --body, --body-file, or --body-html")
	}
	if c.TrackSplit && !c.Track {
		return usage("--track-split requires --track")
	}
	if c.Track && strings.TrimSpace(c.BodyHTML) == "" {
		return fmt.Errorf("--track requires --body-html (pixel must be in HTML)")
	}
	if sigErr := c.validateSignatureOptions(); sigErr != nil {
		return sigErr
	}

	attachPaths, err := expandComposeAttachmentPaths(c.Attach)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.send", map[string]any{
		"to":                  splitCSV(c.To),
		"cc":                  splitCSV(c.Cc),
		"bcc":                 splitCSV(c.Bcc),
		"subject":             strings.TrimSpace(c.Subject),
		"reply_to_message_id": replyToMessageID,
		"thread_id":           threadID,
		"reply_all":           c.ReplyAll,
		"reply_to":            strings.TrimSpace(c.ReplyTo),
		"from":                strings.TrimSpace(c.From),
		"body_len":            len(strings.TrimSpace(body)),
		"body_html_len":       len(strings.TrimSpace(c.BodyHTML)),
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
	htmlBodyInput := c.BodyHTML
	if c.signatureRequested() {
		signature, source, sigErr := c.resolveComposeSignature(ctx, svc, from.sendingEmail)
		if sigErr != nil {
			return sigErr
		}
		if signature.empty() {
			u.Err().Printf("Warning: no signature configured for %s", source)
		} else {
			body, htmlBodyInput = appendComposeSignature(body, htmlBodyInput, signature)
		}
	}
	replyInfo, body, htmlBody, err := prepareComposeReply(ctx, svc, replyToMessageID, threadID, c.Quote, body, htmlBodyInput)
	if err != nil {
		return err
	}

	// Determine recipients
	var toRecipients, ccRecipients []string
	if c.ReplyAll {
		// Auto-populate recipients from original message
		toRecipients, ccRecipients = buildReplyAllRecipients(replyInfo, from.sendingEmail)
	}

	// Explicit --to and --cc override (not merge with) auto-populated recipients
	if strings.TrimSpace(c.To) != "" {
		toRecipients = splitCSV(c.To)
	}
	if strings.TrimSpace(c.Cc) != "" {
		ccRecipients = splitCSV(c.Cc)
	}

	// Final validation: we must have at least one recipient
	if len(toRecipients) == 0 {
		return usage("no recipients: specify --to or use --reply-all with a message that has recipients")
	}

	bccRecipients := splitCSV(c.Bcc)

	atts := attachmentsFromPaths(attachPaths)

	var trackingCfg *tracking.Config
	if c.Track {
		trackingCfg, err = c.resolveTrackingConfig(account, toRecipients, ccRecipients, bccRecipients, htmlBody)
		if err != nil {
			return err
		}
	}

	batches := buildSendBatches(toRecipients, ccRecipients, bccRecipients, c.Track, c.TrackSplit)
	results, err := sendGmailBatches(ctx, svc, sendMessageOptions{
		FromAddr:    from.header,
		ReplyTo:     c.ReplyTo,
		Subject:     c.Subject,
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

	return writeSendResults(ctx, u, from.header, results)
}

func (c *GmailSendCmd) resolveTrackingConfig(account string, toRecipients, ccRecipients, bccRecipients []string, htmlBody string) (*tracking.Config, error) {
	totalRecipients := len(toRecipients) + len(ccRecipients) + len(bccRecipients)
	if totalRecipients != 1 && !c.TrackSplit {
		return nil, usage("--track requires exactly 1 recipient (no cc/bcc); use --track-split for per-recipient sends")
	}

	if strings.TrimSpace(htmlBody) == "" {
		return nil, fmt.Errorf("--track requires an HTML body (use --body-html or --quote)")
	}

	trackingCfg, err := tracking.LoadConfig(account)
	if err != nil {
		return nil, fmt.Errorf("load tracking config: %w", err)
	}
	if !trackingCfg.IsConfigured() {
		return nil, fmt.Errorf("tracking not configured; run 'gog gmail track setup' first")
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

func buildSendBatches(toRecipients, ccRecipients, bccRecipients []string, track, trackSplit bool) []sendBatch {
	totalRecipients := len(toRecipients) + len(ccRecipients) + len(bccRecipients)
	if track && trackSplit && totalRecipients > 1 {
		recipients := append(append(append([]string{}, toRecipients...), ccRecipients...), bccRecipients...)
		recipients = deduplicateAddresses(recipients)

		batches := make([]sendBatch, 0, len(recipients))
		for _, recipient := range recipients {
			batches = append(batches, sendBatch{
				To:                []string{recipient},
				TrackingRecipient: recipient,
			})
		}

		return batches
	}

	trackingRecipient := firstRecipient(toRecipients, ccRecipients, bccRecipients)
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
			recipient := strings.TrimSpace(batch.TrackingRecipient)
			if recipient == "" {
				recipient = strings.TrimSpace(firstRecipient(batch.To, batch.Cc, batch.Bcc))
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
		msg, err := buildGmailMessage(messageOpts, batch, nil)
		if err != nil {
			return nil, err
		}

		sent, err := svc.Users.Messages.Send("me", msg).Context(ctx).Do()
		if err != nil {
			return nil, err
		}

		resultRecipient := strings.TrimSpace(batch.TrackingRecipient)
		if resultRecipient == "" {
			resultRecipient = strings.TrimSpace(firstRecipient(batch.To, batch.Cc, batch.Bcc))
		}
		results = append(results, sendResult{
			To:         resultRecipient,
			MessageID:  sent.Id,
			ThreadID:   sent.ThreadId,
			TrackingID: trackingID,
		})
	}

	return results, nil
}

func writeSendResults(ctx context.Context, u *ui.UI, fromAddr string, results []sendResult) error {
	items := make([]gmailMessageResult, 0, len(results))
	for _, r := range results {
		items = append(items, gmailMessageResult{
			From:       fromAddr,
			To:         r.To,
			MessageID:  r.MessageID,
			ThreadID:   r.ThreadID,
			TrackingID: r.TrackingID,
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
