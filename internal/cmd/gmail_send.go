package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/mail"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailSendCmd struct {
	To               string   `name:"to" help:"Recipients (comma-separated; required unless --reply-all is used)"`
	Cc               string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc              string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject          string   `name:"subject" help:"Subject (required)"`
	Body             string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyHTML         string   `name:"body-html" help:"Body (HTML; optional)"`
	ReplyToMessageID string   `name:"reply-to-message-id" aliases:"in-reply-to" help:"Reply to Gmail message ID (sets In-Reply-To/References and thread)"`
	ThreadID         string   `name:"thread-id" help:"Reply within a Gmail thread (uses latest message for headers)"`
	ReplyAll         bool     `name:"reply-all" help:"Auto-populate recipients from original message (requires --reply-to-message-id or --thread-id)"`
	ReplyTo          string   `name:"reply-to" help:"Reply-To header address"`
	Attach           []string `name:"attach" help:"Attachment file path (repeatable)"`
	From             string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
}

func (c *GmailSendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	replyToMessageID := strings.TrimSpace(c.ReplyToMessageID)
	threadID := strings.TrimSpace(c.ThreadID)

	if replyToMessageID != "" && threadID != "" {
		return usage("use only one of --reply-to-message-id or --thread-id")
	}

	// Validate --reply-all requires a reply target
	if c.ReplyAll && replyToMessageID == "" && threadID == "" {
		return usage("--reply-all requires --reply-to-message-id or --thread-id")
	}

	// --to is required unless --reply-all is used
	if strings.TrimSpace(c.To) == "" && !c.ReplyAll {
		return usage("required: --to (or use --reply-all with --reply-to-message-id or --thread-id)")
	}
	if strings.TrimSpace(c.Subject) == "" {
		return usage("required: --subject")
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
	sendingEmail := account // The email we're sending from (without display name)
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
		sendingEmail = c.From
		fromAddr = c.From
		// Include display name if set
		if sa.DisplayName != "" {
			fromAddr = sa.DisplayName + " <" + c.From + ">"
		}
	}

	// Fetch reply info (includes recipient headers for reply-all)
	replyInfo, err := fetchReplyInfo(ctx, svc, replyToMessageID, threadID)
	if err != nil {
		return err
	}

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

	atts := make([]mailAttachment, 0, len(c.Attach))
	for _, p := range c.Attach {
		atts = append(atts, mailAttachment{Path: p})
	}

	raw, err := buildRFC822(mailOptions{
		From:        fromAddr,
		To:          toRecipients,
		Cc:          ccRecipients,
		Bcc:         splitCSV(c.Bcc),
		ReplyTo:     c.ReplyTo,
		Subject:     c.Subject,
		Body:        c.Body,
		BodyHTML:    c.BodyHTML,
		InReplyTo:   replyInfo.InReplyTo,
		References:  replyInfo.References,
		Attachments: atts,
	})
	if err != nil {
		return err
	}

	msg := &gmail.Message{
		Raw: base64.RawURLEncoding.EncodeToString(raw),
	}
	if replyInfo.ThreadID != "" {
		msg.ThreadId = replyInfo.ThreadID
	}

	sent, err := svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"messageId": sent.Id,
			"threadId":  sent.ThreadId,
			"from":      fromAddr,
		})
	}
	u.Out().Printf("message_id\t%s", sent.Id)
	if sent.ThreadId != "" {
		u.Out().Printf("thread_id\t%s", sent.ThreadId)
	}
	return nil
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
}

func replyHeaders(ctx context.Context, svc *gmail.Service, replyToMessageID string) (inReplyTo string, references string, threadID string, err error) {
	info, err := fetchReplyInfo(ctx, svc, replyToMessageID, "")
	if err != nil {
		return "", "", "", err
	}
	return info.InReplyTo, info.References, info.ThreadID, nil
}

func fetchReplyInfo(ctx context.Context, svc *gmail.Service, replyToMessageID string, threadID string) (*replyInfo, error) {
	replyToMessageID = strings.TrimSpace(replyToMessageID)
	threadID = strings.TrimSpace(threadID)
	if replyToMessageID == "" && threadID == "" {
		return &replyInfo{}, nil
	}

	if replyToMessageID != "" {
		msg, err := svc.Users.Messages.Get("me", replyToMessageID).
			Format("metadata").
			MetadataHeaders("Message-ID", "Message-Id", "References", "In-Reply-To", "From", "Reply-To", "To", "Cc").
			Context(ctx).
			Do()
		if err != nil {
			return nil, err
		}
		return replyInfoFromMessage(msg), nil
	}

	thread, err := svc.Users.Threads.Get("me", threadID).
		Format("metadata").
		MetadataHeaders("Message-ID", "Message-Id", "References", "In-Reply-To", "From", "Reply-To", "To", "Cc").
		Context(ctx).
		Do()
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
	info := replyInfoFromMessage(msg)
	if info.ThreadID == "" {
		info.ThreadID = thread.Id
	}
	return info, nil
}

func replyInfoFromMessage(msg *gmail.Message) *replyInfo {
	if msg == nil {
		return &replyInfo{}
	}
	info := &replyInfo{
		ThreadID:    msg.ThreadId,
		FromAddr:    headerValue(msg.Payload, "From"),
		ReplyToAddr: headerValue(msg.Payload, "Reply-To"),
		ToAddrs:     parseEmailAddresses(headerValue(msg.Payload, "To")),
		CcAddrs:     parseEmailAddresses(headerValue(msg.Payload, "Cc")),
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
