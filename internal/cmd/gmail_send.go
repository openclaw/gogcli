package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"net/mail"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
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

	attachPaths := make([]string, 0, len(c.Attach))
	for _, p := range c.Attach {
		expanded, expandErr := config.ExpandPath(p)
		if expandErr != nil {
			return expandErr
		}
		attachPaths = append(attachPaths, expanded)
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
		"track":               c.Track,
		"track_split":         c.TrackSplit,
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

	sendAsList, sendAsListErr := listSendAs(ctx, svc)

	// Determine the From address
	fromAddr := account
	sendingEmail := account // The email we're sending from (without display name)
	if fromEmail := strings.TrimSpace(c.From); fromEmail != "" {
		// Validate that this is a configured and verified send-as alias.
		var sa *gmail.SendAs
		if sendAsListErr == nil {
			sa = findSendAsByEmail(sendAsList, fromEmail)
			if sa == nil {
				return fmt.Errorf("invalid --from address %q: not found in send-as settings", fromEmail)
			}
		} else {
			// Fallback: preserve legacy behavior if we cannot list settings.
			var getErr error
			sa, getErr = svc.Users.Settings.SendAs.Get("me", fromEmail).Context(ctx).Do()
			if getErr != nil {
				return fmt.Errorf("invalid --from address %q: %w", fromEmail, getErr)
			}
		}

		if sa.VerificationStatus != gmailVerificationAccepted {
			return fmt.Errorf("--from address %q is not verified (status: %s)", fromEmail, sa.VerificationStatus)
		}

		sendingEmail = fromEmail
		fromAddr = fromEmail

		if displayName := strings.TrimSpace(sa.DisplayName); displayName != "" {
			fromAddr = displayName + " <" + fromEmail + ">"
		}
	} else {
		// No --from specified: best-effort look up the primary account's display name.
		displayName := ""
		if sendAsListErr == nil {
			displayName = primaryDisplayNameFromSendAsList(sendAsList, account)
		}
		if displayName != "" {
			fromAddr = displayName + " <" + account + ">"
		}
		// If lookup fails, we just use the plain email address (no error)
	}

	// Fetch reply info (includes recipient headers for reply-all, and body for quoting)
	replyInfo, err := fetchReplyInfo(ctx, svc, replyToMessageID, threadID, c.Quote)
	if err != nil {
		return err
	}

	body, htmlBody := applyQuoteToBodies(body, c.BodyHTML, c.Quote, replyInfo)

	// Determine recipients
	var toRecipients, ccRecipients []string
	if c.ReplyAll {
		// Auto-populate recipients from original message
		toRecipients, ccRecipients = buildReplyAllRecipients(replyInfo, sendingEmail)
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

	atts := make([]mailAttachment, 0, len(attachPaths))
	for _, p := range attachPaths {
		atts = append(atts, mailAttachment{Path: p})
	}

	var trackingCfg *tracking.Config
	if c.Track {
		trackingCfg, err = c.resolveTrackingConfig(account, toRecipients, ccRecipients, bccRecipients, htmlBody)
		if err != nil {
			return err
		}
	}

	batches := buildSendBatches(toRecipients, ccRecipients, bccRecipients, c.Track, c.TrackSplit)
	results, err := sendGmailBatches(ctx, svc, sendMessageOptions{
		FromAddr:    fromAddr,
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

	return writeSendResults(ctx, u, fromAddr, results)
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
	reply := replyInfo{}
	if opts.ReplyInfo != nil {
		reply = *opts.ReplyInfo
	}

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

		raw, err := buildRFC822(mailOptions{
			From:        opts.FromAddr,
			To:          batch.To,
			Cc:          batch.Cc,
			Bcc:         batch.Bcc,
			ReplyTo:     opts.ReplyTo,
			Subject:     opts.Subject,
			Body:        opts.Body,
			BodyHTML:    htmlBody,
			InReplyTo:   reply.InReplyTo,
			References:  reply.References,
			Attachments: opts.Attachments,
		}, nil)
		if err != nil {
			return nil, err
		}

		msg := &gmail.Message{
			Raw: base64.RawURLEncoding.EncodeToString(raw),
		}
		if reply.ThreadID != "" {
			msg.ThreadId = reply.ThreadID
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
	if outfmt.IsJSON(ctx) {
		if len(results) == 1 {
			resp := map[string]any{
				"messageId": results[0].MessageID,
				"threadId":  results[0].ThreadID,
				"from":      fromAddr,
			}
			if results[0].TrackingID != "" {
				resp["tracking_id"] = results[0].TrackingID
			}
			return outfmt.WriteJSON(ctx, os.Stdout, resp)
		}

		items := make([]map[string]any, 0, len(results))
		for _, r := range results {
			item := map[string]any{
				"messageId": r.MessageID,
				"threadId":  r.ThreadID,
				"from":      fromAddr,
			}
			if r.To != "" {
				item["to"] = r.To
			}
			if r.TrackingID != "" {
				item["tracking_id"] = r.TrackingID
			}
			items = append(items, item)
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"messages": items})
	}

	if len(results) == 1 {
		u.Out().Printf("message_id\t%s", results[0].MessageID)
		if results[0].ThreadID != "" {
			u.Out().Printf("thread_id\t%s", results[0].ThreadID)
		}
		if results[0].TrackingID != "" {
			u.Out().Printf("tracking_id\t%s", results[0].TrackingID)
		}
		return nil
	}

	for i, r := range results {
		if i > 0 {
			u.Out().Println("")
		}
		if r.To != "" {
			u.Out().Printf("to\t%s", r.To)
		}
		u.Out().Printf("message_id\t%s", r.MessageID)
		if r.ThreadID != "" {
			u.Out().Printf("thread_id\t%s", r.ThreadID)
		}
		if r.TrackingID != "" {
			u.Out().Printf("tracking_id\t%s", r.TrackingID)
		}
	}

	return nil
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

// buildReplyAllRecipients constructs To and Cc lists for a reply-all.
// Per RFC 5322: if Reply-To header is present, use it instead of From.
// Reply-To (or From if no Reply-To) -> To
// Original To recipients -> To
// Original Cc recipients -> Cc
// Filters out self and deduplicates.
func buildReplyAllRecipients(info *replyInfo, selfEmail string) (to, cc []string) {
	// Collect To recipients: reply address (Reply-To if present, else From) + original To recipients
	toAddrs := make([]string, 0, 1+len(info.ToAddrs))

	// Per RFC 5322, Reply-To takes precedence over From for replies
	replyAddress := info.ReplyToAddr
	if replyAddress == "" {
		replyAddress = info.FromAddr
	}
	if replyAddrs := parseEmailAddresses(replyAddress); len(replyAddrs) > 0 {
		toAddrs = append(toAddrs, replyAddrs...)
	}
	toAddrs = append(toAddrs, info.ToAddrs...)

	// Filter out self and deduplicate
	toAddrs = filterOutSelf(toAddrs, selfEmail)
	toAddrs = deduplicateAddresses(toAddrs)

	// Cc recipients: original Cc, filtered
	ccAddrs := filterOutSelf(info.CcAddrs, selfEmail)
	ccAddrs = deduplicateAddresses(ccAddrs)

	// Remove any Cc addresses that are already in To
	toSet := make(map[string]bool)
	for _, addr := range toAddrs {
		toSet[strings.ToLower(addr)] = true
	}
	filteredCc := make([]string, 0, len(ccAddrs))
	for _, addr := range ccAddrs {
		if !toSet[strings.ToLower(addr)] {
			filteredCc = append(filteredCc, addr)
		}
	}

	return toAddrs, filteredCc
}

// replyInfo contains all information extracted from the original message for replying
type replyInfo struct {
	InReplyTo   string
	References  string
	ThreadID    string
	FromAddr    string   // Original sender
	ReplyToAddr string   // Original Reply-To header (per RFC 5322, use this instead of From if present)
	ToAddrs     []string // Original To recipients
	CcAddrs     []string // Original Cc recipients
	Date        string   // Original message date (for quoting)
	Body        string   // Original message plain text body (for quoting)
	BodyHTML    string   // Original message HTML body (for quoting with formatting)
}

func replyHeaders(ctx context.Context, svc *gmail.Service, replyToMessageID string) (inReplyTo string, references string, threadID string, err error) {
	info, err := fetchReplyInfo(ctx, svc, replyToMessageID, "", false)
	if err != nil {
		return "", "", "", err
	}
	return info.InReplyTo, info.References, info.ThreadID, nil
}

func fetchReplyInfo(ctx context.Context, svc *gmail.Service, replyToMessageID string, threadID string, includeQuoteBodies bool) (*replyInfo, error) {
	replyToMessageID = strings.TrimSpace(replyToMessageID)
	threadID = strings.TrimSpace(threadID)
	if replyToMessageID == "" && threadID == "" {
		return &replyInfo{}, nil
	}

	if replyToMessageID != "" {
		msg, err := fetchMessageForReplyInfo(ctx, svc, replyToMessageID, includeQuoteBodies)
		if err != nil {
			return nil, err
		}
		return replyInfoFromMessage(msg, includeQuoteBodies), nil
	}

	// For thread replies, we always need just headers to select the latest message.
	// If includeQuoteBodies is true (quoting), fetch that single message in "full" format afterwards
	// to avoid pulling entire thread bodies.
	thread, err := fetchThreadForReplyInfo(ctx, svc, threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil || len(thread.Messages) == 0 {
		return nil, fmt.Errorf("thread %s has no messages", threadID)
	}

	msg := selectLatestThreadMessage(thread.Messages)
	if msg == nil {
		return nil, fmt.Errorf("thread %s has no messages", threadID)
	}
	// If quoting, refetch the selected message in full format to get body parts.
	if includeQuoteBodies && msg.Id != "" {
		fullMsg, fullErr := fetchMessageForReplyInfo(ctx, svc, msg.Id, true)
		if fullErr == nil && fullMsg != nil {
			msg = fullMsg
		}
	}

	info := replyInfoFromMessage(msg, includeQuoteBodies)
	if info.ThreadID == "" {
		info.ThreadID = thread.Id
	}
	return info, nil
}

var replyInfoMetadataHeaders = []string{"Message-ID", "Message-Id", "References", "In-Reply-To", "From", "Reply-To", "To", "Cc", "Date"}

func fetchMessageForReplyInfo(ctx context.Context, svc *gmail.Service, messageID string, includeQuoteBodies bool) (*gmail.Message, error) {
	call := svc.Users.Messages.Get("me", messageID).Context(ctx)
	if includeQuoteBodies {
		call = call.Format(gmailFormatFull)
	} else {
		call = call.Format(gmailFormatMetadata).MetadataHeaders(replyInfoMetadataHeaders...)
	}
	return call.Do()
}

func fetchThreadForReplyInfo(ctx context.Context, svc *gmail.Service, threadID string) (*gmail.Thread, error) {
	return svc.Users.Threads.Get("me", threadID).
		Format(gmailFormatMetadata).
		MetadataHeaders(replyInfoMetadataHeaders...).
		Context(ctx).
		Do()
}

func replyInfoFromMessage(msg *gmail.Message, includeQuoteBodies bool) *replyInfo {
	if msg == nil {
		return &replyInfo{}
	}
	info := &replyInfo{
		ThreadID:    msg.ThreadId,
		FromAddr:    headerValue(msg.Payload, "From"),
		ReplyToAddr: headerValue(msg.Payload, "Reply-To"),
		ToAddrs:     parseEmailAddresses(headerValue(msg.Payload, "To")),
		CcAddrs:     parseEmailAddresses(headerValue(msg.Payload, "Cc")),
		Date:        headerValue(msg.Payload, "Date"),
	}

	// Include body if requested (for quoting)
	if includeQuoteBodies {
		plain := findPartBody(msg.Payload, "text/plain")
		// Some messages (or broken clients) put HTML into text/plain; never dump raw HTML into the plain quote.
		if plain != "" && !looksLikeHTML(plain) {
			info.Body = plain
		}
		info.BodyHTML = findPartBody(msg.Payload, "text/html")
	}

	// Prefer Message-ID and References from the original message.
	messageID := headerValue(msg.Payload, "Message-ID")
	if messageID == "" {
		messageID = headerValue(msg.Payload, "Message-Id")
	}
	info.InReplyTo = messageID
	info.References = strings.TrimSpace(headerValue(msg.Payload, "References"))
	if info.References == "" {
		info.References = messageID
	} else if messageID != "" && !strings.Contains(info.References, messageID) {
		info.References = info.References + " " + messageID
	}
	return info
}

func selectLatestThreadMessage(messages []*gmail.Message) *gmail.Message {
	var selected *gmail.Message
	var selectedDate int64
	hasDate := false
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.InternalDate <= 0 {
			if selected == nil && !hasDate {
				selected = msg
			}
			continue
		}
		if !hasDate || msg.InternalDate > selectedDate {
			selectedDate = msg.InternalDate
			selected = msg
			hasDate = true
		}
	}
	return selected
}

// parseEmailAddresses parses RFC 5322 email addresses from a header value.
// Returns just the email parts (lowercased for comparison).
func parseEmailAddresses(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(header)
	if err != nil {
		// Fallback: try splitting on comma and extracting addresses manually
		return parseEmailAddressesFallback(header)
	}
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if addr.Address != "" {
			result = append(result, strings.ToLower(addr.Address))
		}
	}
	return result
}

// parseEmailAddressesFallback handles cases where mail.ParseAddressList fails
func parseEmailAddressesFallback(header string) []string {
	parts := strings.Split(header, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Try to extract email from "Name <email>" format
		if start := strings.LastIndex(p, "<"); start != -1 {
			if end := strings.LastIndex(p, ">"); end > start {
				email := strings.TrimSpace(p[start+1 : end])
				if email != "" {
					result = append(result, strings.ToLower(email))
				}
				continue
			}
		}
		// Assume it's just an email address
		if strings.Contains(p, "@") {
			result = append(result, strings.ToLower(p))
		}
	}
	return result
}

// filterOutSelf removes the sending account from the address list
func filterOutSelf(addresses []string, selfEmail string) []string {
	selfLower := strings.ToLower(selfEmail)
	result := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if strings.ToLower(addr) != selfLower {
			result = append(result, addr)
		}
	}
	return result
}

// deduplicateAddresses removes duplicate email addresses (case-insensitive)
func deduplicateAddresses(addresses []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		lower := strings.ToLower(addr)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, addr)
		}
	}
	return result
}

func escapeTextToHTML(value string) string {
	value = html.EscapeString(value)
	return strings.ReplaceAll(value, "\n", "<br>\n")
}

func applyQuoteToBodies(plainBody string, htmlBody string, quote bool, info *replyInfo) (string, string) {
	if !quote || info == nil {
		return plainBody, htmlBody
	}
	if info.Body == "" && info.BodyHTML == "" {
		return plainBody, htmlBody
	}

	userPlain := plainBody
	outPlain := plainBody
	if info.Body != "" {
		outPlain += formatQuotedMessage(info.FromAddr, info.Date, info.Body)
	}

	quoteContent := info.BodyHTML
	if quoteContent == "" && info.Body != "" {
		quoteContent = escapeTextToHTML(info.Body)
	}
	if quoteContent == "" {
		return outPlain, htmlBody
	}

	quoteHTML := formatQuotedMessageHTMLWithContent(info.FromAddr, info.Date, quoteContent)

	outHTML := htmlBody
	if strings.TrimSpace(outHTML) == "" {
		outHTML = escapeTextToHTML(strings.TrimSpace(userPlain)) + quoteHTML
	} else {
		outHTML += quoteHTML
	}

	return outPlain, outHTML
}

// formatQuotedMessage formats the original message as a quoted reply.
// It adds an attribution line and prefixes each line with "> ".
func formatQuotedMessage(from, date, body string) string {
	if body == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n")

	// Attribution line
	switch {
	case date != "" && from != "":
		sb.WriteString(fmt.Sprintf("On %s, %s wrote:\n", date, from))
	case from != "":
		sb.WriteString(fmt.Sprintf("%s wrote:\n", from))
	default:
		sb.WriteString("Original message:\n")
	}

	// Quote each line with "> " prefix
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		sb.WriteString("> ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatQuotedMessageHTMLWithContent wraps pre-formatted HTML content in a blockquote.
// Use this when the content is already HTML (preserves original formatting).
func formatQuotedMessageHTMLWithContent(from, date, htmlContent string) string {
	senderName := from
	if addr, err := mail.ParseAddress(from); err == nil && addr.Name != "" {
		senderName = addr.Name
	}

	dateStr := date
	if date == "" {
		dateStr = "an earlier date"
	}

	return fmt.Sprintf(`<br><br><div class="gmail_quote"><div class="gmail_attr">On %s, %s wrote:</div><blockquote class="gmail_quote" style="margin:0 0 0 .8ex;border-left:1px #ccc solid;padding-left:1ex">%s</blockquote></div>`,
		html.EscapeString(dateStr),
		html.EscapeString(senderName),
		htmlContent)
}
