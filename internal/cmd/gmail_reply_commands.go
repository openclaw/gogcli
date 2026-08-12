package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/mailmime"
	"github.com/openclaw/gogcli/internal/ui"
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
	To                      []string `name:"to" sep:"none" help:"Add or move recipients to To (repeatable)"`
	Cc                      []string `name:"cc" sep:"none" help:"Add or move recipients to Cc (repeatable)"`
	Bcc                     []string `name:"bcc" sep:"none" help:"Add or move recipients to Bcc (repeatable)"`
	Remove                  []string `name:"remove" sep:"none" help:"Remove recipients from all fields (repeatable)"`
	Subject                 string   `name:"subject" help:"Override reply subject (a changed subject starts a new Gmail thread)"`
	Body                    string   `name:"body" help:"Body (plain text; required unless --body-html is set)"`
	BodyFile                string   `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML                string   `name:"body-html" help:"Body (HTML; optional)"`
	BodyHTMLFile            string   `name:"body-html-file" help:"HTML body file path ('-' for stdin)"`
	NoQuote                 bool     `name:"no-quote" help:"Do not include the original message below the reply"`
	Attach                  []string `name:"attach" sep:"none" help:"Attachment file path (repeatable)"`
	From                    string   `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	AutoFromAddressedAlias  bool     `name:"auto-from-addressed-alias" help:"When --from is omitted, reply from the verified send-as alias addressed by the original message" env:"GOG_GMAIL_AUTO_FROM_ADDRESSED_ALIAS"`
	composeSignatureOptions `embed:""`
}

func (c *GmailReplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return c.Options.run(ctx, flags, c.MessageID, false)
}

func (c *GmailReplyAllCmd) Run(ctx context.Context, flags *RootFlags) error {
	return c.Options.run(ctx, flags, c.MessageID, true)
}

// replyComposeInputs holds the validated, service-free inputs for a reply
// compose. Body/HTML inputs are resolved exactly once here because '-' reads
// stdin, which cannot be read twice.
type replyComposeInputs struct {
	messageID   string
	body        string
	htmlBody    string
	attachPaths []string
}

// replyComposeMessage carries the built reply message plus the metadata the
// caller needs to record results.
type replyComposeMessage struct {
	message            *gmail.Message
	fromHeader         string
	to                 []string
	attachmentMetadata []mailmime.AttachmentMetadata
	// threading records the reply headers the message was built with, for the
	// draft path's result report. The send path reports the sent message's
	// thread instead and ignores it.
	threading draftThreading
}

func (c *GmailReplyOptions) run(ctx context.Context, flags *RootFlags, messageID string, replyAll bool) error {
	u := ui.FromContext(ctx)

	inputs, err := c.resolveReplyInputs(ctx, messageID)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail."+replyModeName(replyAll), c.dryRunFields(inputs)); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailSendService(ctx, flags)
	if err != nil {
		return err
	}

	built, err := c.buildReplyComposeMessage(ctx, svc, account, inputs, replyAll)
	if err != nil {
		return err
	}

	sent, err := svc.Users.Messages.Send("me", built.message).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("send reply: %w", err)
	}

	return writeGmailMessageResults(ctx, u, []gmailMessageResult{{
		From:        built.fromHeader,
		To:          strings.Join(built.to, ", "),
		MessageID:   sent.Id,
		ThreadID:    sent.ThreadId,
		Attachments: built.attachmentMetadata,
	}})
}

// dryRunFields builds the dry-run request dictionary shared by the send-side
// reply/reply-all and the draft-side reply/reply-all, so both report the same
// fields and only the action name differs.
func (c *GmailReplyOptions) dryRunFields(inputs replyComposeInputs) map[string]any {
	return map[string]any{
		"message_id":                inputs.messageID,
		"to_add":                    c.To,
		"cc_add":                    c.Cc,
		"bcc_add":                   c.Bcc,
		"remove":                    c.Remove,
		"subject_override":          strings.TrimSpace(c.Subject),
		"quote":                     !c.NoQuote,
		"from":                      strings.TrimSpace(c.From),
		"auto_from_addressed_alias": c.AutoFromAddressedAlias,
		"body_len":                  len(inputs.body),
		"body_html_len":             len(inputs.htmlBody),
		"attachments":               inputs.attachPaths,
		"signature":                 c.Signature,
		"signature_from":            strings.TrimSpace(c.SignatureFrom),
		"signature_file":            strings.TrimSpace(c.SignatureFile),
	}
}

// resolveReplyInputs normalizes the message ID, resolves body/HTML inputs, and
// runs all validation that does not require a Gmail service. It reads body
// inputs exactly once so '-' (stdin) is consumed a single time.
func (c *GmailReplyOptions) resolveReplyInputs(ctx context.Context, messageID string) (replyComposeInputs, error) {
	messageID = normalizeGmailMessageID(messageID)
	if messageID == "" {
		return replyComposeInputs{}, usage("required: messageId")
	}

	body, htmlBody, err := resolveComposeBodyInputs(ctx, c.Body, c.BodyFile, c.BodyHTML, c.BodyHTMLFile)
	if err != nil {
		return replyComposeInputs{}, err
	}
	if strings.TrimSpace(body) == "" && strings.TrimSpace(htmlBody) == "" {
		return replyComposeInputs{}, usage("required: --body, --body-file, --body-html, or --body-html-file")
	}
	if validationErr := mailmime.ValidateHeaderValue(c.Subject); validationErr != nil {
		return replyComposeInputs{}, usagef("invalid --subject: %v", validationErr)
	}
	if validationErr := mailmime.ValidateHeaderValue(c.From); validationErr != nil {
		return replyComposeInputs{}, usagef("invalid --from: %v", validationErr)
	}
	if _, parseErr := parseExplicitRecipientFields(c.To, c.Cc, c.Bcc); parseErr != nil {
		return replyComposeInputs{}, parseErr
	}
	if _, parseErr := parseMailboxValues("--remove", c.Remove); parseErr != nil {
		return replyComposeInputs{}, parseErr
	}

	if signatureErr := c.validateSignatureOptions(); signatureErr != nil {
		return replyComposeInputs{}, signatureErr
	}

	attachPaths, err := expandComposeAttachmentPaths(c.Attach)
	if err != nil {
		return replyComposeInputs{}, err
	}

	return replyComposeInputs{
		messageID:   messageID,
		body:        body,
		htmlBody:    htmlBody,
		attachPaths: attachPaths,
	}, nil
}

// buildReplyComposeMessage assembles the outgoing reply from already-validated
// inputs and an already-acquired service. It resolves the sender and signature,
// builds the reply recipients and body, and returns the message without sending
// so the caller controls how it is dispatched.
func (c *GmailReplyOptions) buildReplyComposeMessage(ctx context.Context, svc *gmail.Service, account string, inputs replyComposeInputs, replyAll bool) (replyComposeMessage, error) {
	u := ui.FromContext(ctx)
	body, htmlBody := inputs.body, inputs.htmlBody

	sendAs, sendAsErr := listSendAs(ctx, svc)
	from, err := resolveComposeFrom(ctx, svc, account, c.From, sendAs, sendAsErr)
	if err != nil {
		return replyComposeMessage{}, err
	}
	info, err := fetchReplyInfo(ctx, svc, inputs.messageID, "", !c.NoQuote)
	if err != nil {
		return replyComposeMessage{}, err
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
	if c.signatureRequested() {
		signature, source, sigErr := c.resolveComposeSignature(ctx, svc, from.sendingEmail)
		if sigErr != nil {
			return replyComposeMessage{}, sigErr
		}
		if signature.empty() {
			u.Err().Linef("Warning: no signature configured for %s", source)
		} else {
			body, htmlBody = appendComposeSignature(body, htmlBody, signature)
		}
	}
	body, htmlBody, err = applyReplyQuote(ctx, !c.NoQuote, info, body, htmlBody)
	if err != nil {
		return replyComposeMessage{}, err
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
		return replyComposeMessage{}, err
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

	userAttachments, attachmentMetadata, err := mailmime.PrepareAttachments(attachmentsFromPaths(inputs.attachPaths), os.ReadFile)
	if err != nil {
		return replyComposeMessage{}, err
	}
	attachments := append([]mailmime.Attachment{}, userAttachments...)
	attachments = append(attachments, info.InlineResources...)

	toRecipients := formatMailboxes(recipients.To)
	msg, err := buildGmailMessage(ctx, sendMessageOptions{
		FromAddr:    from.header,
		Subject:     subject,
		Body:        body,
		BodyHTML:    htmlBody,
		ReplyInfo:   info,
		Attachments: attachments,
	}, sendBatch{
		To:  toRecipients,
		Cc:  formatMailboxes(recipients.Cc),
		Bcc: formatMailboxes(recipients.Bcc),
	}, true)
	if err != nil {
		return replyComposeMessage{}, fmt.Errorf("build reply: %w", err)
	}

	threading := draftThreading{
		ThreadID:   info.ThreadID,
		InReplyTo:  strings.TrimSpace(info.InReplyTo),
		References: strings.TrimSpace(info.References),
	}
	if threading.InReplyTo != "" {
		threading.Source = replyContextCaller
	}

	return replyComposeMessage{
		message:            msg,
		fromHeader:         from.header,
		to:                 toRecipients,
		attachmentMetadata: attachmentMetadata,
		threading:          threading,
	}, nil
}
