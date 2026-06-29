package cmd

import (
	"context"
	"fmt"
	"html"
	"net/mail"
	"strings"
	"time"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/gmailcontent"
	"github.com/openclaw/gogcli/internal/ui"
)

type GmailForwardCmd struct {
	MessageID string              `arg:"" name:"messageId" help:"Gmail message ID to forward"`
	Options   GmailForwardOptions `embed:""`
}

type GmailForwardOptions struct {
	To              string `name:"to" help:"Recipients (comma-separated; required when sending, optional when saving a draft)"`
	Cc              string `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc             string `name:"bcc" help:"BCC recipients (comma-separated)"`
	Note            string `name:"note" aliases:"intro" help:"Introductory text above the forwarded message"`
	NoteFile        string `name:"note-file" help:"Note file path (plain text; '-' for stdin)"`
	From            string `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	SkipAttachments bool   `name:"skip-attachments" help:"Do not include original attachments"`
}

// recipientRequirement records whether a compose path must have recipients. The
// send path requires them; a draft may be saved without any (like Gmail's UI).
type recipientRequirement bool

const (
	recipientsRequired recipientRequirement = true
	recipientsOptional recipientRequirement = false
)

// forwardComposeInputs holds the validated, service-free inputs for a forward
// compose. The note is resolved exactly once here because '-' reads stdin,
// which cannot be read twice.
type forwardComposeInputs struct {
	messageID     string
	note          string
	toRecipients  []string
	ccRecipients  []string
	bccRecipients []string
	// allowMissingTo carries the recipient requirement forward to the build
	// step: false on the send path (so buildGmailMessage keeps its missing-To
	// backstop), true on the draft path (which permits an addressless forward).
	allowMissingTo bool
}

// forwardComposeMessage carries the built forward message plus the metadata the
// caller needs to record results.
type forwardComposeMessage struct {
	message    *gmail.Message
	fromHeader string
}

func (c *GmailForwardCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	inputs, err := c.Options.resolveForwardInputs(ctx, c.MessageID, recipientsRequired)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.forward", c.Options.dryRunFields(inputs)); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailSendService(ctx, flags)
	if err != nil {
		return err
	}

	built, err := c.Options.buildForwardComposeMessage(ctx, svc, account, inputs)
	if err != nil {
		return err
	}

	sent, err := svc.Users.Messages.Send("me", built.message).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("send forward: %w", err)
	}

	return writeGmailMessageResults(ctx, u, []gmailMessageResult{{
		From:      built.fromHeader,
		MessageID: sent.Id,
		ThreadID:  sent.ThreadId,
	}})
}

// dryRunFields builds the dry-run request dictionary shared by the send-side
// forward and the draft-side forward, so both report the same fields and only
// the action name differs.
func (c *GmailForwardOptions) dryRunFields(inputs forwardComposeInputs) map[string]any {
	return map[string]any{
		"message_id":       inputs.messageID,
		"to":               inputs.toRecipients,
		"cc":               inputs.ccRecipients,
		"bcc":              inputs.bccRecipients,
		"from":             strings.TrimSpace(c.From),
		"note_len":         len(inputs.note),
		"skip_attachments": c.SkipAttachments,
	}
}

// resolveForwardInputs normalizes the message ID, runs the service-free
// validation, and resolves the note input. It reads the note exactly once so
// '-' (stdin) is consumed a single time. When req is recipientsRequired (the
// send path) an empty --to is rejected; a draft may have no recipients, so the
// draft path passes recipientsOptional.
func (c *GmailForwardOptions) resolveForwardInputs(ctx context.Context, messageID string, req recipientRequirement) (forwardComposeInputs, error) {
	messageID = normalizeGmailMessageID(messageID)
	if messageID == "" {
		return forwardComposeInputs{}, usage("required: messageId")
	}

	// Parsed before the dry-run so it reports the lists the build will use.
	toRecipients, ccRecipients, bccRecipients, err := parseComposeRecipients(c.To, c.Cc, c.Bcc)
	if err != nil {
		return forwardComposeInputs{}, err
	}
	if req == recipientsRequired && len(toRecipients) == 0 {
		return forwardComposeInputs{}, usage("required: --to")
	}

	// Resolve the note after the required-recipient check so a missing --to on
	// the send path fails fast without consuming stdin (--note-file -).
	note, err := resolveBodyInput(ctx, c.Note, c.NoteFile)
	if err != nil {
		return forwardComposeInputs{}, err
	}

	return forwardComposeInputs{
		messageID:      messageID,
		note:           note,
		toRecipients:   toRecipients,
		ccRecipients:   ccRecipients,
		bccRecipients:  bccRecipients,
		allowMissingTo: req == recipientsOptional,
	}, nil
}

// buildForwardComposeMessage assembles the forwarded message from already-validated
// inputs and an already-acquired service. It resolves the sender, fetches the
// original message, and returns the message without sending so the caller
// controls how it is dispatched.
func (c *GmailForwardOptions) buildForwardComposeMessage(ctx context.Context, svc *gmail.Service, account string, inputs forwardComposeInputs) (forwardComposeMessage, error) {
	from, err := resolveComposeSender(ctx, svc, account, c.From)
	if err != nil {
		return forwardComposeMessage{}, err
	}

	// Fetch the original message in full format (headers + body + attachment metadata).
	origMsg, err := svc.Users.Messages.Get("me", inputs.messageID).Format(gmailFormatFull).Context(ctx).Do()
	if err != nil {
		return forwardComposeMessage{}, fmt.Errorf("fetch original message: %w", err)
	}

	origFrom := headerValue(origMsg.Payload, "From")
	origTo := headerValue(origMsg.Payload, "To")
	origCc := headerValue(origMsg.Payload, "Cc")
	origDate := headerValue(origMsg.Payload, "Date")
	origSubject := headerValue(origMsg.Payload, "Subject")
	origPlain := gmailcontent.FindPartBody(origMsg.Payload, "text/plain")
	origHTML := gmailcontent.FindPartBody(origMsg.Payload, "text/html")

	// Build forward subject (avoid stacking prefixes).
	fwdSubject := buildForwardSubject(origSubject)

	// Resolve the timezone for the forwarded Date header from the same
	// configured/local source as the reply quote and the outgoing Date header.
	loc, err := mailDateLocation(ctx, stderrWriter(ctx))
	if err != nil {
		return forwardComposeMessage{}, err
	}

	// Build forwarded body (plain text).
	fwdPlain := formatForwardedMessage(inputs.note, origFrom, origDate, origSubject, origTo, origCc, origPlain, loc)

	// Build forwarded body (HTML) if original had HTML.
	var fwdHTML string
	if origHTML != "" {
		fwdHTML = formatForwardedMessageHTML(inputs.note, origFrom, origDate, origSubject, origTo, origCc, origHTML, loc)
	}

	// Preserve CID-backed inline resources required by the forwarded HTML and,
	// unless disabled, ordinary attachments.
	attachments, err := preserveForwardMessageParts(ctx, svc, inputs.messageID, origMsg.Payload, origHTML, !c.SkipAttachments)
	if err != nil {
		return forwardComposeMessage{}, fmt.Errorf("preserve forwarded message parts: %w", err)
	}

	// allowMissingTo comes from the recipient requirement resolved up front: the
	// send path keeps buildGmailMessage's missing-To backstop (false), while the
	// draft path opts out (true) to permit an addressless forward like Gmail's UI.
	msg, err := buildGmailMessage(ctx, sendMessageOptions{
		FromAddr:    from.header,
		Subject:     fwdSubject,
		Body:        fwdPlain,
		BodyHTML:    fwdHTML,
		Attachments: attachments,
	}, sendBatch{
		To:  inputs.toRecipients,
		Cc:  inputs.ccRecipients,
		Bcc: inputs.bccRecipients,
	}, inputs.allowMissingTo)
	if err != nil {
		return forwardComposeMessage{}, fmt.Errorf("build message: %w", err)
	}

	return forwardComposeMessage{
		message:    msg,
		fromHeader: from.header,
	}, nil
}

type forwardedHeader struct {
	label string
	value string
}

func forwardedMessageHeaders(from, date, subject, to, cc string, loc *time.Location) []forwardedHeader {
	return []forwardedHeader{
		{"From", from},
		{"Date", formatQuoteDate(date, loc)},
		{"Subject", subject},
		{"To", to},
		{"Cc", cc},
	}
}

// buildForwardSubject prepends "Fwd: " to the subject, avoiding duplication.
func buildForwardSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "Fwd: (no subject)"
	}
	stripped := stripForwardPrefix(subject)
	return "Fwd: " + stripped
}

// stripForwardPrefix removes existing Fwd:/Fw:/FWD: prefixes from a subject.
func stripForwardPrefix(subject string) string {
	for {
		lower := strings.ToLower(strings.TrimSpace(subject))
		switch {
		case strings.HasPrefix(lower, "fwd: "):
			subject = strings.TrimSpace(subject[5:])
		case strings.HasPrefix(lower, "fwd:"):
			subject = strings.TrimSpace(subject[4:])
		case strings.HasPrefix(lower, "fw: "):
			subject = strings.TrimSpace(subject[4:])
		case strings.HasPrefix(lower, "fw:"):
			subject = strings.TrimSpace(subject[3:])
		default:
			return subject
		}
	}
}

// formatForwardedMessage builds the plain-text forwarded body.
func formatForwardedMessage(note, from, date, subject, to, cc, body string, loc *time.Location) string {
	var sb strings.Builder

	if strings.TrimSpace(note) != "" {
		sb.WriteString(strings.TrimSpace(note))
		sb.WriteString("\n\n")
	}

	sb.WriteString("---------- Forwarded message ---------\n")
	for _, h := range forwardedMessageHeaders(from, date, subject, to, cc, loc) {
		if h.value != "" {
			fmt.Fprintf(&sb, "%s: %s\n", h.label, h.value)
		}
	}
	sb.WriteString("\n")

	if body != "" {
		sb.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// formatForwardedMessageHTML builds the HTML forwarded body.
func formatForwardedMessageHTML(note, from, date, subject, to, cc, htmlContent string, loc *time.Location) string {
	var sb strings.Builder

	if strings.TrimSpace(note) != "" {
		sb.WriteString("<div>")
		sb.WriteString(html.EscapeString(strings.TrimSpace(note)))
		sb.WriteString("</div><br>")
	}

	sb.WriteString(`<div class="gmail_quote">`)
	sb.WriteString(`<div style="margin:0 0 10px 0;color:#777">---------- Forwarded message ---------</div>`)
	sb.WriteString(`<div style="margin:0 0 10px 0;color:#777">`)

	for _, h := range forwardedMessageHeaders(from, date, subject, to, cc, loc) {
		if h.value != "" {
			displayName := html.EscapeString(h.value)
			// Format the From address more nicely if it has a name part.
			if h.label == "From" {
				if addr, err := mail.ParseAddress(h.value); err == nil && addr.Name != "" {
					displayName = html.EscapeString(addr.Name) + " &lt;" + html.EscapeString(addr.Address) + "&gt;"
				}
			}
			fmt.Fprintf(&sb, "<b>%s:</b> %s<br>", h.label, displayName)
		}
	}
	sb.WriteString("</div>")

	sb.WriteString(`<div style="margin:10px 0 0 0">`)
	sb.WriteString(htmlContent)
	sb.WriteString("</div></div>")

	return sb.String()
}
