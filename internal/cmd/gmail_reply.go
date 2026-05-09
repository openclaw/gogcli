package cmd

import (
	"context"
	"fmt"
	"html"
	"net/mail"
	"strings"

	"google.golang.org/api/gmail/v1"
)

// buildReplyAllRecipients constructs To and Cc lists for a reply-all.
// Per RFC 5322: if Reply-To header is present, use it instead of From.
func buildReplyAllRecipients(info *replyInfo, selfEmail string) (to, cc []string) {
	toAddrs := make([]string, 0, 1+len(info.ToAddrs))

	replyAddress := info.ReplyToAddr
	if replyAddress == "" {
		replyAddress = info.FromAddr
	}
	if replyAddrs := parseEmailAddresses(replyAddress); len(replyAddrs) > 0 {
		toAddrs = append(toAddrs, replyAddrs...)
	}
	toAddrs = append(toAddrs, info.ToAddrs...)

	toAddrs = filterOutSelf(toAddrs, selfEmail)
	toAddrs = deduplicateAddresses(toAddrs)

	ccAddrs := filterOutSelf(info.CcAddrs, selfEmail)
	ccAddrs = deduplicateAddresses(ccAddrs)

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

// replyInfo contains all information extracted from the original message for replying.
type replyInfo struct {
	InReplyTo   string
	References  string
	ThreadID    string
	FromAddr    string
	ReplyToAddr string
	ToAddrs     []string
	CcAddrs     []string
	Date        string
	Subject     string
	Body        string
	BodyHTML    string
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
		info := replyInfoFromMessage(msg, includeQuoteBodies)
		if info.InReplyTo == "" {
			return nil, fmt.Errorf("reply target message %s has no Message-ID header; cannot set In-Reply-To/References", replyToMessageID)
		}
		return info, nil
	}

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

func fetchMessageForReplyInfo(ctx context.Context, svc *gmail.Service, messageID string, includeQuoteBodies bool) (*gmail.Message, error) {
	call := svc.Users.Messages.Get("me", messageID).Context(ctx)
	if includeQuoteBodies {
		call = call.Format(gmailFormatFull)
	} else {
		call = call.Format(gmailFormatMetadata).MetadataHeaders(gmailReplyMetadataHeaders...)
	}
	return call.Do()
}

func fetchThreadForReplyInfo(ctx context.Context, svc *gmail.Service, threadID string) (*gmail.Thread, error) {
	return svc.Users.Threads.Get("me", threadID).
		Format(gmailFormatMetadata).
		MetadataHeaders(gmailReplyMetadataHeaders...).
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
		Subject:     headerValue(msg.Payload, "Subject"),
	}

	if includeQuoteBodies {
		plain := findPartBody(msg.Payload, "text/plain")
		// Some messages put HTML into text/plain; never dump raw HTML into the plain quote.
		if plain != "" && !looksLikeHTML(plain) {
			info.Body = plain
		}
		info.BodyHTML = findPartBody(msg.Payload, "text/html")
	}

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

func parseEmailAddresses(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(header)
	if err != nil {
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

func parseEmailAddressesFallback(header string) []string {
	parts := strings.Split(header, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if start := strings.LastIndex(p, "<"); start != -1 {
			if end := strings.LastIndex(p, ">"); end > start {
				email := strings.TrimSpace(p[start+1 : end])
				if email != "" {
					result = append(result, strings.ToLower(email))
				}
				continue
			}
		}
		if strings.Contains(p, "@") {
			result = append(result, strings.ToLower(p))
		}
	}
	return result
}

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
func formatQuotedMessage(from, date, body string) string {
	if body == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n")

	switch {
	case date != "" && from != "":
		fmt.Fprintf(&sb, "On %s, %s wrote:\n", date, from)
	case from != "":
		fmt.Fprintf(&sb, "%s wrote:\n", from)
	default:
		sb.WriteString("Original message:\n")
	}

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		sb.WriteString("> ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

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
