package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/mailmime"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailReplyCmd struct {
	MessageID string            `arg:"" name:"messageId" help:"Gmail message ID to reply to"`
	Options   GmailReplyOptions `embed:""`
}

type GmailReplyAllCmd struct {
	MessageID string            `arg:"" name:"messageId" help:"Gmail message ID to reply to"`
	Options   GmailReplyOptions `embed:""`
}

type GmailReplyOptions struct {
	To                     []string `name:"to" sep:"none" help:"Add or move recipients to To (repeatable)"`
	Cc                     []string `name:"cc" sep:"none" help:"Add or move recipients to Cc (repeatable)"`
	Bcc                    []string `name:"bcc" sep:"none" help:"Add or move recipients to Bcc (repeatable)"`
	Remove                 []string `name:"remove" sep:"none" help:"Remove recipients from all fields (repeatable)"`
	Subject                string   `name:"subject" help:"Override reply subject (a changed subject starts a new Gmail thread)"`
	Body                   string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyFile               string   `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML               string   `name:"body-html" help:"Body (HTML; optional)"`
	BodyHTMLFile           string   `name:"body-html-file" help:"HTML body file path ('-' for stdin)"`
	NoQuote                bool     `name:"no-quote" help:"Do not include the original message below the reply"`
	Attach                 []string `name:"attach" sep:"none" help:"Attachment file path (repeatable)"`
	From                   string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	AutoFromAddressedAlias bool     `name:"auto-from-addressed-alias" help:"When --from is omitted, reply from the verified send-as alias addressed by the original message"`
	Signature              bool     `name:"signature" help:"Append the Gmail signature from the active send-as address"`
	SignatureFrom          string   `name:"signature-from" help:"Append the Gmail signature from this send-as email address"`
	SignatureFile          string   `name:"signature-file" help:"Append a local signature file (plain text or HTML)"`
}

func (c *GmailReplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return c.Options.run(ctx, flags, c.MessageID, false)
}

func (c *GmailReplyAllCmd) Run(ctx context.Context, flags *RootFlags) error {
	return c.Options.run(ctx, flags, c.MessageID, true)
}

func (c *GmailReplyOptions) run(ctx context.Context, flags *RootFlags, messageID string, replyAll bool) error {
	u := ui.FromContext(ctx)
	messageID = normalizeGmailMessageID(messageID)
	if messageID == "" {
		return usage("required: messageId")
	}

	body, htmlBody, err := resolveComposeBodyInputs(ctx, c.Body, c.BodyFile, c.BodyHTML, c.BodyHTMLFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" && strings.TrimSpace(htmlBody) == "" {
		return usage("required: --body, --body-file, --body-html, or --body-html-file")
	}
	if validationErr := mailmime.ValidateHeaderValue(c.Subject); validationErr != nil {
		return usagef("invalid --subject: %v", validationErr)
	}
	if validationErr := mailmime.ValidateHeaderValue(c.From); validationErr != nil {
		return usagef("invalid --from: %v", validationErr)
	}
	if _, parseErr := parseExplicitRecipientFields(c.To, c.Cc, c.Bcc); parseErr != nil {
		return parseErr
	}
	if _, parseErr := parseMailboxValues("--remove", c.Remove); parseErr != nil {
		return parseErr
	}

	signatureCmd := GmailSendCmd{
		Signature:     c.Signature,
		SignatureFrom: c.SignatureFrom,
		SignatureFile: c.SignatureFile,
	}
	if signatureErr := signatureCmd.validateSignatureOptions(); signatureErr != nil {
		return signatureErr
	}

	attachPaths, err := expandComposeAttachmentPaths(c.Attach)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail."+replyModeName(replyAll), map[string]any{
		"message_id":                messageID,
		"to_add":                    c.To,
		"cc_add":                    c.Cc,
		"bcc_add":                   c.Bcc,
		"remove":                    c.Remove,
		"subject_override":          strings.TrimSpace(c.Subject),
		"quote":                     !c.NoQuote,
		"from":                      strings.TrimSpace(c.From),
		"auto_from_addressed_alias": c.AutoFromAddressedAlias,
		"body_len":                  len(body),
		"body_html_len":             len(htmlBody),
		"attachments":               attachPaths,
		"signature":                 c.Signature,
		"signature_from":            strings.TrimSpace(c.SignatureFrom),
		"signature_file":            strings.TrimSpace(c.SignatureFile),
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailSendService(ctx, flags)
	if err != nil {
		return err
	}

	sendAs, sendAsErr := listSendAs(ctx, svc)
	from, err := resolveComposeFrom(ctx, svc, account, c.From, sendAs, sendAsErr)
	if err != nil {
		return err
	}
	info, err := fetchReplyInfo(ctx, svc, messageID, "", !c.NoQuote)
	if err != nil {
		return err
	}
	// When requested, reply as the verified alias the original was addressed to. Do this
	// before signature resolution so the signature matches the identity actually sending.
	if c.AutoFromAddressedAlias && strings.TrimSpace(c.From) == "" && sendAsErr == nil {
		if alias := pickSendAsFromRecipients(info.ToAddrs, info.CcAddrs, sendAs); alias != "" {
			if picked, pickErr := resolveComposeFrom(ctx, svc, account, alias, sendAs, sendAsErr); pickErr == nil {
				from = picked
			}
		}
	}
	if signatureCmd.signatureRequested() {
		signature, source, sigErr := signatureCmd.resolveComposeSignature(ctx, svc, from.sendingEmail)
		if sigErr != nil {
			return sigErr
		}
		if signature.empty() {
			u.Err().Linef("Warning: no signature configured for %s", source)
		} else {
			body, htmlBody = appendComposeSignature(body, htmlBody, signature)
		}
	}
	body, htmlBody, err = applyReplyQuote(ctx, !c.NoQuote, info, body, htmlBody)
	if err != nil {
		return err
	}
	recipients, err := buildReplyRecipients(
		info,
		selfEmailsForReply(account, from.sendingEmail, sendAs),
		replyAll,
		c.To,
		c.Cc,
		c.Bcc,
		c.Remove,
	)
	if err != nil {
		return err
	}

	defaultSubject := autoReplySubject("", info.Subject)
	subject := strings.TrimSpace(c.Subject)
	if subject == "" {
		subject = defaultSubject
	}
	if strings.TrimSpace(c.Subject) != "" && subject != defaultSubject {
		// Gmail requires matching subjects for an explicit threadId. Keep reply
		// headers, but let Gmail create a new thread for an edited subject.
		info.ThreadID = ""
	}

	userAttachments, attachmentMetadata, err := mailmime.PrepareAttachments(attachmentsFromPaths(attachPaths), os.ReadFile)
	if err != nil {
		return err
	}
	attachments := append([]mailmime.Attachment{}, userAttachments...)
	attachments = append(attachments, info.InlineResources...)

	msg, err := buildGmailMessage(ctx, sendMessageOptions{
		FromAddr:    from.header,
		Subject:     subject,
		Body:        body,
		BodyHTML:    htmlBody,
		ReplyInfo:   info,
		Attachments: attachments,
	}, sendBatch{
		To:  formatMailboxes(recipients.To),
		Cc:  formatMailboxes(recipients.Cc),
		Bcc: formatMailboxes(recipients.Bcc),
	}, true)
	if err != nil {
		return fmt.Errorf("build reply: %w", err)
	}

	sent, err := svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("send reply: %w", err)
	}

	return writeGmailMessageResults(ctx, u, []gmailMessageResult{{
		From:        from.header,
		To:          strings.Join(formatMailboxes(recipients.To), ", "),
		MessageID:   sent.Id,
		ThreadID:    sent.ThreadId,
		Attachments: attachmentMetadata,
	}})
}
