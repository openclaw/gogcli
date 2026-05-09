package cmd

import (
	"context"
	"fmt"
	"html"
	"net/mail"
	"strings"

	"github.com/steipete/gogcli/internal/ui"
)

type GmailForwardCmd struct {
	MessageID       string `arg:"" name:"messageId" help:"Gmail message ID to forward"`
	To              string `name:"to" help:"Recipients (comma-separated; required)" required:""`
	Cc              string `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc             string `name:"bcc" help:"BCC recipients (comma-separated)"`
	Note            string `name:"note" aliases:"intro" help:"Introductory text above the forwarded message"`
	NoteFile        string `name:"note-file" help:"Note file path (plain text; '-' for stdin)"`
	From            string `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
	SkipAttachments bool   `name:"skip-attachments" help:"Do not include original attachments"`
}

func (c *GmailForwardCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	messageID := normalizeGmailMessageID(c.MessageID)
	if messageID == "" {
		return usage("required: messageId")
	}

	note, err := resolveBodyInput(c.Note, c.NoteFile)
	if err != nil {
		return err
	}

	toRecipients := splitCSV(c.To)
	if len(toRecipients) == 0 {
		return usage("required: --to")
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.forward", map[string]any{
		"message_id":       messageID,
		"to":               toRecipients,
		"cc":               splitCSV(c.Cc),
		"bcc":              splitCSV(c.Bcc),
		"from":             strings.TrimSpace(c.From),
		"note_len":         len(strings.TrimSpace(note)),
		"skip_attachments": c.SkipAttachments,
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

	// Fetch the original message in full format (headers + body + attachment metadata).
	origMsg, err := svc.Users.Messages.Get("me", messageID).Format(gmailFormatFull).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("fetch original message: %w", err)
	}

	origFrom := headerValue(origMsg.Payload, "From")
	origTo := headerValue(origMsg.Payload, "To")
	origCc := headerValue(origMsg.Payload, "Cc")
	origDate := headerValue(origMsg.Payload, "Date")
	origSubject := headerValue(origMsg.Payload, "Subject")
	origPlain := findPartBody(origMsg.Payload, "text/plain")
	origHTML := findPartBody(origMsg.Payload, "text/html")

	// Build forward subject (avoid stacking prefixes).
	fwdSubject := buildForwardSubject(origSubject)

	// Build forwarded body (plain text).
	fwdPlain := formatForwardedMessage(note, origFrom, origDate, origSubject, origTo, origCc, origPlain)

	// Build forwarded body (HTML) if original had HTML.
	var fwdHTML string
	if origHTML != "" {
		fwdHTML = formatForwardedMessageHTML(note, origFrom, origDate, origSubject, origTo, origCc, origHTML)
	}

	// Download and re-attach original attachments.
	var attachments []mailAttachment
	if !c.SkipAttachments {
		origAtts := collectAttachments(origMsg.Payload)
		for _, att := range origAtts {
			data, dlErr := fetchAttachmentBytes(ctx, svc, messageID, att.AttachmentID)
			if dlErr != nil {
				return fmt.Errorf("download attachment %q: %w", att.Filename, dlErr)
			}
			attachments = append(attachments, mailAttachment{
				Filename: att.Filename,
				MIMEType: att.MimeType,
				Data:     data,
			})
		}
	}

	// Build threading info to keep the forward in the sender's thread.
	info := replyInfoFromMessage(origMsg, false)

	ccRecipients := splitCSV(c.Cc)
	bccRecipients := splitCSV(c.Bcc)

	msg, err := buildGmailMessage(sendMessageOptions{
		FromAddr:    from.header,
		Subject:     fwdSubject,
		Body:        fwdPlain,
		BodyHTML:    fwdHTML,
		ReplyInfo:   info,
		Attachments: attachments,
	}, sendBatch{
		To:  toRecipients,
		Cc:  ccRecipients,
		Bcc: bccRecipients,
	}, nil)
	if err != nil {
		return fmt.Errorf("build message: %w", err)
	}

	sent, err := svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("send forward: %w", err)
	}

	return writeGmailMessageResults(ctx, u, []gmailMessageResult{{
		From:      from.header,
		MessageID: sent.Id,
		ThreadID:  sent.ThreadId,
	}})
}

type forwardedHeader struct {
	label string
	value string
}

func forwardedMessageHeaders(from, date, subject, to, cc string) []forwardedHeader {
	return []forwardedHeader{
		{"From", from},
		{"Date", date},
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
func formatForwardedMessage(note, from, date, subject, to, cc, body string) string {
	var sb strings.Builder

	if strings.TrimSpace(note) != "" {
		sb.WriteString(strings.TrimSpace(note))
		sb.WriteString("\n\n")
	}

	sb.WriteString("---------- Forwarded message ---------\n")
	for _, h := range forwardedMessageHeaders(from, date, subject, to, cc) {
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
func formatForwardedMessageHTML(note, from, date, subject, to, cc, htmlContent string) string {
	var sb strings.Builder

	if strings.TrimSpace(note) != "" {
		sb.WriteString("<div>")
		sb.WriteString(html.EscapeString(strings.TrimSpace(note)))
		sb.WriteString("</div><br>")
	}

	sb.WriteString(`<div class="gmail_quote">`)
	sb.WriteString(`<div style="margin:0 0 10px 0;color:#777">---------- Forwarded message ---------</div>`)
	sb.WriteString(`<div style="margin:0 0 10px 0;color:#777">`)

	for _, h := range forwardedMessageHeaders(from, date, subject, to, cc) {
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
