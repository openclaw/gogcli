package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var autoReplyMetadataHeaders = []string{
	"Message-ID", "Message-Id", "References", "In-Reply-To",
	"From", "Reply-To", "To", "Cc", "Date", "Subject",
	"Auto-Submitted", "Precedence", "List-Id", "List-Unsubscribe",
}

const autoReplyActionSkipped = "skipped"

type GmailAutoReplyCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query"`
	Max       int64    `name:"max" aliases:"limit" help:"Max matching messages to inspect" default:"20"`
	Subject   string   `name:"subject" help:"Override reply subject (default: reply to original subject)"`
	Body      string   `name:"body" help:"Reply body (plain text; required unless --body-html is set)"`
	BodyFile  string   `name:"body-file" help:"Reply body file path (plain text; '-' for stdin)"`
	BodyHTML  string   `name:"body-html" help:"Reply body HTML"`
	From      string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	ReplyTo   string   `name:"reply-to" help:"Reply-To header address"`
	Label     string   `name:"label" help:"Label to add after replying (used for dedupe)" default:"AutoReplied"`
	Archive   bool     `name:"archive" help:"Archive threads after auto-replying"`
	MarkRead  bool     `name:"mark-read" help:"Mark threads as read after auto-replying"`
	SkipBulk  bool     `name:"skip-bulk" help:"Skip auto-generated/list mail" default:"true"`
	AllowSelf bool     `name:"allow-self" help:"Allow replying to messages sent by your own account/alias"`
}

type gmailAutoReplyInput struct {
	Query     string
	Max       int64
	Subject   string
	Body      string
	BodyHTML  string
	From      string
	ReplyTo   string
	Label     string
	Archive   bool
	MarkRead  bool
	SkipBulk  bool
	AllowSelf bool
}

type gmailAutoReplyResult struct {
	Action         string `json:"action"`
	MessageID      string `json:"messageId"`
	ThreadID       string `json:"threadId,omitempty"`
	ReplyTo        string `json:"replyTo,omitempty"`
	ReplyMessageID string `json:"replyMessageId,omitempty"`
	ReplyThreadID  string `json:"replyThreadId,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Subject        string `json:"subject,omitempty"`
}

type gmailAutoReplySummary struct {
	Query   string                 `json:"query"`
	Label   string                 `json:"label"`
	Matched int                    `json:"matched"`
	Replied int                    `json:"replied"`
	Skipped int                    `json:"skipped"`
	Results []gmailAutoReplyResult `json:"results"`
}

func (c *GmailAutoReplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	body, err := resolveBodyInput(c.Body, c.BodyFile)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("missing query")
	}
	if strings.TrimSpace(body) == "" && strings.TrimSpace(c.BodyHTML) == "" {
		return usage("required: --body, --body-file, or --body-html")
	}
	label := strings.TrimSpace(c.Label)
	if label == "" {
		return usage("--label must not be empty")
	}
	resultLimit := c.Max
	if resultLimit <= 0 {
		return usage("--max must be > 0")
	}

	input := gmailAutoReplyInput{
		Query:     query,
		Max:       resultLimit,
		Subject:   strings.TrimSpace(c.Subject),
		Body:      body,
		BodyHTML:  c.BodyHTML,
		From:      strings.TrimSpace(c.From),
		ReplyTo:   strings.TrimSpace(c.ReplyTo),
		Label:     label,
		Archive:   c.Archive,
		MarkRead:  c.MarkRead,
		SkipBulk:  c.SkipBulk,
		AllowSelf: c.AllowSelf,
	}
	if dryRunErr := dryRunExit(ctx, flags, "gmail.autoreply", map[string]any{
		"query":         input.Query,
		"max":           input.Max,
		"subject":       input.Subject,
		"body_len":      len(strings.TrimSpace(input.Body)),
		"body_html_len": len(strings.TrimSpace(input.BodyHTML)),
		"from":          input.From,
		"reply_to":      input.ReplyTo,
		"label":         input.Label,
		"archive":       input.Archive,
		"mark_read":     input.MarkRead,
		"skip_bulk":     input.SkipBulk,
		"allow_self":    input.AllowSelf,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}
	summary, err := runGmailAutoReply(ctx, svc, account, input)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"autoReply": summary})
	}
	if len(summary.Results) == 0 {
		u.Out().Println("No matching messages")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ACTION\tMESSAGE\tTHREAD\tREPLY_TO\tREPLY_MESSAGE\tREASON\tSUBJECT")
	for _, item := range summary.Results {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			item.Action,
			item.MessageID,
			item.ThreadID,
			item.ReplyTo,
			item.ReplyMessageID,
			item.Reason,
			sanitizeTab(item.Subject),
		)
	}
	u.Out().Printf("matched\t%d", summary.Matched)
	u.Out().Printf("replied\t%d", summary.Replied)
	u.Out().Printf("skipped\t%d", summary.Skipped)
	return nil
}

func runGmailAutoReply(ctx context.Context, svc *gmail.Service, account string, input gmailAutoReplyInput) (gmailAutoReplySummary, error) {
	summary := gmailAutoReplySummary{
		Query: input.Query,
		Label: input.Label,
	}

	sendAsList, sendAsListErr := listSendAs(ctx, svc)
	from, err := resolveComposeFrom(ctx, svc, account, input.From, sendAsList, sendAsListErr)
	if err != nil {
		return summary, err
	}
	labelID, err := ensureLabelExists(ctx, svc, input.Label)
	if err != nil {
		return summary, err
	}

	messageIDs, err := searchMessageIDs(ctx, svc, input.Query, input.Max)
	if err != nil {
		return summary, err
	}
	summary.Matched = len(messageIDs)
	summary.Results = make([]gmailAutoReplyResult, 0, len(messageIDs))
	if len(messageIDs) == 0 {
		return summary, nil
	}

	selfAddrs := []string{account}
	if from.sendingEmail != "" && !strings.EqualFold(from.sendingEmail, account) {
		selfAddrs = append(selfAddrs, from.sendingEmail)
	}

	for _, messageID := range messageIDs {
		msg, err := fetchMessageForAutoReply(ctx, svc, messageID)
		if err != nil {
			return summary, err
		}
		result := gmailAutoReplyResult{
			MessageID: messageID,
		}
		if msg != nil {
			result.ThreadID = msg.ThreadId
			result.Subject = headerValue(msg.Payload, "Subject")
		}
		if msg == nil {
			result.Action = autoReplyActionSkipped
			result.Reason = "missing_message"
			summary.Results = append(summary.Results, result)
			summary.Skipped++
			continue
		}
		if hasMessageLabel(msg, labelID) {
			result.Action = autoReplyActionSkipped
			result.Reason = "already_labeled"
			summary.Results = append(summary.Results, result)
			summary.Skipped++
			continue
		}
		if input.SkipBulk {
			if skip, reason := shouldSkipAutoReplyMessage(msg); skip {
				result.Action = autoReplyActionSkipped
				result.Reason = reason
				summary.Results = append(summary.Results, result)
				summary.Skipped++
				continue
			}
		}

		replyMeta := replyInfoFromMessage(msg, false)
		recipients := autoReplyRecipients(replyMeta, selfAddrs)
		if len(recipients) == 0 && input.AllowSelf {
			recipients = autoReplyRecipients(replyMeta, nil)
		}
		if len(recipients) == 0 {
			result.Action = autoReplyActionSkipped
			result.Reason = "no_reply_recipient"
			summary.Results = append(summary.Results, result)
			summary.Skipped++
			continue
		}
		result.ReplyTo = recipients[0]

		sendResults, err := sendGmailBatches(ctx, svc, sendMessageOptions{
			FromAddr:  from.header,
			ReplyTo:   input.ReplyTo,
			Subject:   autoReplySubject(input.Subject, headerValue(msg.Payload, "Subject")),
			Body:      input.Body,
			BodyHTML:  input.BodyHTML,
			ReplyInfo: replyMeta,
			Headers: map[string]string{
				"Auto-Submitted":           "auto-replied",
				"X-Auto-Response-Suppress": "All",
			},
		}, []sendBatch{{To: recipients}})
		if err != nil {
			return summary, err
		}
		if len(sendResults) > 0 {
			result.ReplyMessageID = sendResults[0].MessageID
			result.ReplyThreadID = sendResults[0].ThreadID
		}

		if err := modifyAutoReplyThread(ctx, svc, msg.ThreadId, labelID, input.Archive, input.MarkRead); err != nil {
			return summary, err
		}
		result.Action = "replied"
		summary.Results = append(summary.Results, result)
		summary.Replied++
	}

	return summary, nil
}

func fetchMessageForAutoReply(ctx context.Context, svc *gmail.Service, messageID string) (*gmail.Message, error) {
	return svc.Users.Messages.Get("me", messageID).
		Format(gmailFormatMetadata).
		MetadataHeaders(autoReplyMetadataHeaders...).
		Context(ctx).
		Do()
}

func ensureLabelExists(ctx context.Context, svc *gmail.Service, name string) (string, error) {
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return "", err
	}
	if id, ok := idMap[strings.ToLower(strings.TrimSpace(name))]; ok {
		return id, nil
	}
	label, err := createLabel(ctx, svc, name)
	if err != nil {
		return "", mapLabelCreateError(err, name)
	}
	return label.Id, nil
}

func hasMessageLabel(msg *gmail.Message, labelID string) bool {
	if msg == nil || strings.TrimSpace(labelID) == "" {
		return false
	}
	for _, id := range msg.LabelIds {
		if id == labelID {
			return true
		}
	}
	return false
}

func shouldSkipAutoReplyMessage(msg *gmail.Message) (bool, string) {
	if msg == nil {
		return true, "missing_message"
	}
	autoSubmitted := strings.ToLower(strings.TrimSpace(headerValue(msg.Payload, "Auto-Submitted")))
	if autoSubmitted != "" && autoSubmitted != "no" {
		return true, "auto_submitted"
	}
	precedence := strings.ToLower(strings.TrimSpace(headerValue(msg.Payload, "Precedence")))
	switch precedence {
	case "bulk", "list", "junk":
		return true, "bulk_precedence"
	}
	if strings.TrimSpace(headerValue(msg.Payload, "List-Id")) != "" {
		return true, "list_id"
	}
	if strings.TrimSpace(headerValue(msg.Payload, "List-Unsubscribe")) != "" {
		return true, "list_unsubscribe"
	}
	return false, ""
}

func autoReplyRecipients(info *replyInfo, self []string) []string {
	replyAddress := strings.TrimSpace(info.ReplyToAddr)
	if replyAddress == "" {
		replyAddress = strings.TrimSpace(info.FromAddr)
	}
	recipients := parseEmailAddresses(replyAddress)
	if len(self) > 0 {
		selfSet := make(map[string]struct{}, len(self))
		for _, addr := range self {
			trimmed := strings.ToLower(strings.TrimSpace(addr))
			if trimmed != "" {
				selfSet[trimmed] = struct{}{}
			}
		}
		filtered := make([]string, 0, len(recipients))
		for _, addr := range recipients {
			if _, ok := selfSet[strings.ToLower(strings.TrimSpace(addr))]; ok {
				continue
			}
			filtered = append(filtered, addr)
		}
		recipients = filtered
	}
	return deduplicateAddresses(recipients)
}

func autoReplySubject(override, original string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	subject := strings.TrimSpace(original)
	if subject == "" {
		return "Re:"
	}
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

func modifyAutoReplyThread(ctx context.Context, svc *gmail.Service, threadID, labelID string, archive, markRead bool) error {
	if strings.TrimSpace(threadID) == "" {
		return usage("missing threadId")
	}
	addIDs := []string{labelID}
	removeIDs := []string{}
	if archive {
		removeIDs = append(removeIDs, "INBOX")
	}
	if markRead {
		removeIDs = append(removeIDs, "UNREAD")
	}
	_, err := svc.Users.Threads.Modify("me", threadID, &gmail.ModifyThreadRequest{
		AddLabelIds:    addIDs,
		RemoveLabelIds: removeIDs,
	}).Context(ctx).Do()
	return err
}
