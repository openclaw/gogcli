package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailForwardCmd struct {
	MessageID     string   `arg:"" name:"messageId" help:"Message ID to forward"`
	To            string   `name:"to" help:"Recipients (comma-separated; required)"`
	Cc            string   `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc           string   `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject       string   `name:"subject" help:"Override subject (default: Fwd: <original>)"`
	Body          string   `name:"body" help:"Body preface (plain text)"`
	BodyFile      string   `name:"body-file" help:"Body preface file ('-' for stdin)"`
	From          string   `name:"from" help:"Send from verified send-as alias"`
	Attach        []string `name:"attach" help:"Additional local file (repeatable)"`
	NoAttachments bool     `name:"no-attachments" help:"Strip original attachments"`
}

func (c *GmailForwardCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	messageID := normalizeGmailMessageID(strings.TrimSpace(c.MessageID))
	if messageID == "" {
		return usage("messageId required")
	}

	toArg := strings.TrimSpace(c.To)
	if toArg == "" {
		return usage("required: --to")
	}
	toRecipients := splitCSV(toArg)
	if len(toRecipients) == 0 {
		return usage("required: --to")
	}
	ccRecipients := splitCSV(c.Cc)
	bccRecipients := splitCSV(c.Bcc)

	prefacePlain, err := resolveBodyInput(c.Body, c.BodyFile)
	if err != nil {
		return err
	}

	// Expand local attachment paths early so we fail fast on bad paths.
	var localAttachPaths []string
	if len(c.Attach) > 0 {
		localAttachPaths, err = expandComposeAttachmentPaths(c.Attach)
		if err != nil {
			return err
		}
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.forward", map[string]any{
		"message_id":     messageID,
		"to":             toRecipients,
		"cc":             ccRecipients,
		"bcc":            bccRecipients,
		"subject":        strings.TrimSpace(c.Subject),
		"from":           strings.TrimSpace(c.From),
		"body_len":       len(strings.TrimSpace(prefacePlain)),
		"attachments":    localAttachPaths,
		"no_attachments": c.NoAttachments,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return err
	}

	msg, err := svc.Users.Messages.Get("me", messageID).Format("full").Context(ctx).Do()
	if err != nil {
		return err
	}
	if msg == nil || msg.Payload == nil {
		return fmt.Errorf("message %s has no payload", messageID)
	}

	sendAsList, sendAsListErr := listSendAs(ctx, svc)
	from, err := resolveComposeFrom(ctx, svc, account, c.From, sendAsList, sendAsListErr)
	if err != nil {
		return err
	}

	subject := strings.TrimSpace(c.Subject)
	if subject == "" {
		subject = forwardSubject(headerValue(msg.Payload, "Subject"))
	}

	plainBody, htmlBody := buildForwardBodies(msg.Payload, prefacePlain)

	var attachments []mailAttachment

	if !c.NoAttachments {
		attachments, err = collectForwardAttachments(ctx, svc, messageID, msg.Payload)
		if err != nil {
			return err
		}
	}

	if len(localAttachPaths) > 0 {
		attachments = append(attachments, attachmentsFromPaths(localAttachPaths)...)
	}

	raw, err := buildRFC822(mailOptions{
		From:        from.header,
		To:          toRecipients,
		Cc:          ccRecipients,
		Bcc:         bccRecipients,
		Subject:     subject,
		Body:        plainBody,
		BodyHTML:    htmlBody,
		Attachments: attachments,
	}, nil)
	if err != nil {
		return err
	}

	sent, err := svc.Users.Messages.Send("me", &gmail.Message{
		Raw:      base64.RawURLEncoding.EncodeToString(raw),
		ThreadId: msg.ThreadId,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"messageId": sent.Id,
			"threadId":  sent.ThreadId,
			"from":      from.header,
		})
	}

	u.Out().Printf("message_id\t%s", sent.Id)
	if sent.ThreadId != "" {
		u.Out().Printf("thread_id\t%s", sent.ThreadId)
	}
	return nil
}

// forwardSubject prepends "Fwd: " to a subject unless it already has a forward prefix.
func forwardSubject(original string) string {
	subject := strings.TrimSpace(original)
	if subject == "" {
		return "Fwd: (no subject)"
	}
	lower := strings.ToLower(subject)
	if strings.HasPrefix(lower, "fwd:") || strings.HasPrefix(lower, "fw:") {
		return subject
	}
	return "Fwd: " + subject
}

func forwardHeaderPlain(p *gmail.MessagePart) string {
	fields := []string{"From", "Date", "Subject", "To", "Cc"}
	lines := make([]string, 0, len(fields)+1)
	lines = append(lines, "---------- Forwarded message ----------")
	for _, name := range fields {
		value := strings.TrimSpace(headerValue(p, name))
		if value != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", name, value))
		}
	}
	return strings.Join(lines, "\n")
}

func forwardHeaderHTML(p *gmail.MessagePart) string {
	fields := []string{"From", "Date", "Subject", "To", "Cc"}
	lines := make([]string, 0, len(fields)+1)
	lines = append(lines, html.EscapeString("---------- Forwarded message ----------"))
	for _, name := range fields {
		value := strings.TrimSpace(headerValue(p, name))
		if value != "" {
			lines = append(lines, fmt.Sprintf("<b>%s:</b> %s", html.EscapeString(name), html.EscapeString(value)))
		}
	}
	return strings.Join(lines, "<br>")
}

func buildForwardBodies(payload *gmail.MessagePart, prefacePlain string) (string, string) {
	plainOriginal := findPartBody(payload, "text/plain")
	htmlOriginal := findPartBody(payload, "text/html")

	// Generate plain text fallback from HTML when no text/plain part exists.
	if plainOriginal == "" && htmlOriginal != "" {
		plainOriginal = stripHTMLTags(htmlOriginal)
	}

	plainHeader := forwardHeaderPlain(payload)
	plainBody := joinForwardSections("\n\n", prefacePlain, plainHeader, plainOriginal)

	// Only produce an HTML body if the original has HTML or the result would benefit from it.
	useHTML := strings.TrimSpace(htmlOriginal) != ""
	if !useHTML {
		return plainBody, ""
	}

	htmlPreface := ""
	if strings.TrimSpace(prefacePlain) != "" {
		htmlPreface = plainToHTML(prefacePlain)
	}
	if strings.TrimSpace(htmlOriginal) == "" && strings.TrimSpace(plainOriginal) != "" {
		htmlOriginal = plainToHTML(plainOriginal)
	}

	htmlHeader := forwardHeaderHTML(payload)
	htmlBody := joinForwardSections("<br><br>", htmlPreface, htmlHeader, htmlOriginal)
	return plainBody, htmlBody
}

func plainToHTML(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	escaped := html.EscapeString(s)
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

// joinForwardSections joins non-empty parts with the given separator.
func joinForwardSections(sep string, parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, sep)
}

func collectForwardAttachments(ctx context.Context, svc *gmail.Service, messageID string, payload *gmail.MessagePart) ([]mailAttachment, error) {
	infos := collectAttachments(payload)
	if len(infos) == 0 {
		return nil, nil
	}

	attachments := make([]mailAttachment, 0, len(infos))
	for _, info := range infos {
		data, err := fetchAttachmentBytes(ctx, svc, messageID, info.AttachmentID)
		if err != nil {
			return nil, fmt.Errorf("fetching attachment %q: %w", info.Filename, err)
		}
		attachments = append(attachments, mailAttachment{
			Filename: info.Filename,
			MIMEType: info.MimeType,
			Data:     data,
		})
	}
	return attachments, nil
}
