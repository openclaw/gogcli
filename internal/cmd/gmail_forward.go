package cmd

import (
	"context"
	"fmt"
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
	To                      string `name:"to" help:"Recipients (comma-separated; required when sending, optional when saving a draft)"`
	Cc                      string `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc                     string `name:"bcc" help:"BCC recipients (comma-separated)"`
	Note                    string `name:"note" aliases:"intro" help:"Introductory text above the forwarded message"`
	NoteFile                string `name:"note-file" help:"Note file path (plain text; '-' for stdin)"`
	NoteHTML                string `name:"note-html" help:"Introductory HTML above the forwarded message"`
	NoteHTMLFile            string `name:"note-html-file" help:"HTML note file path ('-' for stdin)"`
	From                    string `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	SkipAttachments         bool   `name:"skip-attachments" help:"Do not include original attachments"`
	composeSignatureOptions `embed:""`
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
	noteHTML      string
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
		"note_html_len":    len(inputs.noteHTML),
		"skip_attachments": c.SkipAttachments,
		"signature":        c.Signature,
		"signature_from":   strings.TrimSpace(c.SignatureFrom),
		"signature_file":   strings.TrimSpace(c.SignatureFile),
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

	if signatureErr := c.validateSignatureOptions(); signatureErr != nil {
		return forwardComposeInputs{}, signatureErr
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
	// the send path fails fast without consuming stdin (--note-file - or
	// --note-html-file -).
	note, noteHTML, err := resolveTextAndHTMLInputs(ctx, "--note", c.Note, c.NoteFile, c.NoteHTML, c.NoteHTMLFile)
	if err != nil {
		return forwardComposeInputs{}, err
	}

	return forwardComposeInputs{
		messageID:      messageID,
		note:           note,
		noteHTML:       noteHTML,
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

	signature, err := c.requestedSignature(ctx, svc, from.sendingEmail)
	if err != nil {
		return forwardComposeMessage{}, err
	}

	// Fetch the original message in full format (headers + body + attachment metadata).
	origMsg, err := svc.Users.Messages.Get("me", inputs.messageID).Format(gmailFormatFull).Context(ctx).Do()
	if err != nil {
		return forwardComposeMessage{}, fmt.Errorf("fetch original message: %w", err)
	}
	if origMsg == nil {
		return forwardComposeMessage{}, fmt.Errorf("fetch original message %s: empty response", inputs.messageID)
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

	// The note fills in its missing form the way a reply body does.
	note := composeText{plain: inputs.note, html: directedHTML(inputs.noteHTML)}

	// The original's header lines, rendered once for both parts.
	headers := forwardedMessageHeaders(origFrom, origDate, origSubject, origTo, origCc, loc)

	// Build forwarded body (plain text). A note with no plain rendering (an
	// image, say) leaves the plain part out rather than send one without the
	// note, as an image-only reply does. A reply's body already includes its
	// signature when it makes that check; the note doesn't, so a signature with
	// text keeps the plain part here.
	var fwdPlain string
	if !note.htmlOnlyWithoutText() || signature.plainBlock() != "" {
		fwdPlain = formatForwardedMessage(note.plainVersion(), signature, headers, origPlain)
	}

	// Build forwarded body (HTML) if the original had HTML. An original with
	// only plain text, or no body text at all, still gets one when the
	// signature or the note has HTML, so neither is lost.
	var fwdHTML string
	if origHTML != "" || signature.htmlBlock() != "" || note.hasHTML() {
		htmlContent := origHTML
		if htmlContent == "" {
			htmlContent = escapeTextToHTML(origPlain)
		}
		fwdHTML = formatForwardedMessageHTML(note.htmlVersion(), signature, headers, htmlContent)
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

// forwardedSeparator opens the forwarded-message header block in both parts,
// in Gmail's words.
const forwardedSeparator = "---------- Forwarded message ---------"

// forwardedHeader is one line of the forwarded-message header block, with its
// value rendered for each MIME part.
type forwardedHeader struct {
	label string
	plain string
	html  string
}

// forwardedMessageHeaders renders the original's headers in line with Gmail
// Web's forwards: the sender and each recipient as "Name <email>" or
// "<email>", with each address linked and the sender's name in bold in HTML; a
// value that doesn't parse is shown as is, unlinked. In HTML every value taken
// from the original keeps its own direction (see isolateDirection). A blank
// header gets no line. A To or Cc that renders empty keeps an empty line, as
// Gmail Web does for a To of only "undisclosed-recipients:;".
func forwardedMessageHeaders(from, date, subject, to, cc string, loc *time.Location) []forwardedHeader {
	headers := make([]forwardedHeader, 0, 5)
	add := func(label, value string, render func(string) (string, string)) {
		if strings.TrimSpace(value) != "" {
			plain, htmlValue := render(value)
			headers = append(headers, forwardedHeader{label, plain, htmlValue})
		}
	}
	add("From", from, forwardedSender)
	add("Date", formatQuoteDate(date, loc), forwardedText)
	add("Subject", subject, forwardedText)
	add("To", to, forwardedRecipients)
	add("Cc", cc, forwardedRecipients)
	return headers
}

// forwardedSender renders the From value, with the name in bold in HTML.
func forwardedSender(from string) (plain, htmlValue string) {
	return formatQuoteMailbox(from), formatQuoteMailboxHTML(from, boldName)
}

// forwardedRecipients renders a To or Cc value.
func forwardedRecipients(value string) (plain, htmlValue string) {
	return formatQuoteMailboxList(value), formatQuoteMailboxListHTML(value)
}

// forwardedText renders the Date or Subject value: as is, and escaped for
// HTML, where it keeps its own direction (see isolateDirection). A subject can
// be in any script, and a Date that doesn't parse is shown as it came.
func forwardedText(value string) (plain, htmlValue string) {
	return value, isolateTextDirection(value)
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
func formatForwardedMessage(note string, signature composeSignature, headers []forwardedHeader, body string) string {
	var sb strings.Builder

	if strings.TrimSpace(note) != "" {
		sb.WriteString(strings.TrimSpace(note))
		sb.WriteString("\n\n")
	}
	if block := signature.plainBlock(); block != "" {
		sb.WriteString(block)
		sb.WriteString("\n\n")
	}

	sb.WriteString(forwardedSeparator + "\n")
	for _, h := range headers {
		fmt.Fprintf(&sb, "%s: %s\n", h.label, h.plain)
	}

	// Gmail leaves two blank lines between the headers and the body.
	if body != "" {
		sb.WriteString("\n\n")
		sb.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// formatForwardedMessageHTML builds the HTML forwarded body. noteHTML is the
// note as composeText.htmlVersion returns it, written trimmed and with no
// wrapper of its own.
func formatForwardedMessageHTML(noteHTML string, signature composeSignature, headers []forwardedHeader, htmlContent string) string {
	var sb strings.Builder

	if note := strings.TrimSpace(noteHTML); note != "" {
		sb.WriteString(note)
		sb.WriteString(breakAfter(note))
	}
	if block := signature.htmlBlock(); block != "" {
		sb.WriteString(block)
		sb.WriteString("<br>")
	}

	// Like Gmail's, the header block has no inline styles and its labels aren't
	// bold; only the sender's name is (see nameStyle). Bold labels would come
	// out as *From:* in the text part Gmail rebuilds on send.
	sb.WriteString(gmailQuoteOpen + forwardedSeparator + "<br>")
	for _, h := range headers {
		fmt.Fprintf(&sb, "%s: %s<br>", h.label, h.html)
	}
	sb.WriteString("</div>")

	// Gmail leaves two blank lines between the headers and the body.
	if htmlContent != "" {
		sb.WriteString("<br><br>")
		sb.WriteString(htmlContent)
	}
	sb.WriteString("</div>")

	return sb.String()
}
