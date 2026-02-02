package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/ui"
)

type GmailForwardCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID to forward"`
	To        string `arg:"" name:"to" help:"Recipients (comma-separated)"`
	Cc        string `name:"cc" help:"CC recipients (comma-separated)"`
	Bcc       string `name:"bcc" help:"BCC recipients (comma-separated)"`
	Subject   string `name:"subject" help:"Override subject (default: Fwd: <original subject>)"`
	Body      string `name:"body" help:"Body preface (plain text)"`
	BodyFile  string `name:"body-file" help:"Body file path (plain text; '-' for stdin)"`
	BodyHTML  string `name:"body-html" help:"Body preface (HTML)"`
	From      string `name:"from" help:"Send from this email address (must be a verified send-as alias)"`
}

func (c *GmailForwardCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	messageID := strings.TrimSpace(c.MessageID)
	toArg := strings.TrimSpace(c.To)
	if messageID == "" || toArg == "" {
		return usage("messageId/to required")
	}

	prefacePlain, err := resolveBodyInput(c.Body, c.BodyFile)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
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

	fromAddr, err := resolveSendFrom(ctx, svc, account, c.From)
	if err != nil {
		return err
	}

	subject := strings.TrimSpace(c.Subject)
	if subject == "" {
		subject = forwardSubject(headerValue(msg.Payload, "Subject"))
	}

	plainBody, htmlBody := buildForwardBodies(msg.Payload, prefacePlain, c.BodyHTML)

	attachments, err := collectForwardAttachments(ctx, svc, messageID, msg.Payload)
	if err != nil {
		return err
	}

	toRecipients := splitCSV(toArg)
	ccRecipients := splitCSV(c.Cc)
	bccRecipients := splitCSV(c.Bcc)
	if len(toRecipients) == 0 {
		return usage("missing recipients")
	}

	raw, err := buildRFC822(mailOptions{
		From:        fromAddr,
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
		Raw: base64.RawURLEncoding.EncodeToString(raw),
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	results := []sendResult{{
		To:        strings.TrimSpace(firstRecipient(toRecipients, ccRecipients, bccRecipients)),
		MessageID: sent.Id,
		ThreadID:  sent.ThreadId,
	}}
	return writeSendResults(ctx, u, fromAddr, results)
}

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

type forwardHeaderField struct {
	Label string
	Value string
}

func forwardHeaderFields(p *gmail.MessagePart) []forwardHeaderField {
	fields := []string{"From", "Date", "Subject", "To", "Cc"}
	out := make([]forwardHeaderField, 0, len(fields))
	for _, name := range fields {
		value := strings.TrimSpace(headerValue(p, name))
		if value != "" {
			out = append(out, forwardHeaderField{Label: name, Value: value})
		}
	}
	return out
}

func forwardHeaderPlain(p *gmail.MessagePart) string {
	lines := make([]string, 0, 6)
	lines = append(lines, "-------- Forwarded message --------")
	for _, field := range forwardHeaderFields(p) {
		lines = append(lines, fmt.Sprintf("%s: %s", field.Label, field.Value))
	}
	return strings.Join(lines, "\n")
}

func forwardHeaderHTML(p *gmail.MessagePart) string {
	lines := make([]string, 0, 6)
	lines = append(lines, html.EscapeString("-------- Forwarded message --------"))
	for _, field := range forwardHeaderFields(p) {
		lines = append(lines, fmt.Sprintf("%s: %s", html.EscapeString(field.Label), html.EscapeString(field.Value)))
	}
	return strings.Join(lines, "<br>")
}

func joinSections(sep string, parts ...string) string {
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

func buildForwardBodies(payload *gmail.MessagePart, prefacePlain, prefaceHTML string) (string, string) {
	plainOriginal := findPartBody(payload, "text/plain")
	htmlOriginal := findPartBody(payload, "text/html")
	if plainOriginal == "" && htmlOriginal != "" {
		plainOriginal = stripHTMLTags(htmlOriginal)
	}

	plainHeader := forwardHeaderPlain(payload)
	plainBody := joinSections("\n\n", prefacePlain, plainHeader, plainOriginal)

	useHTML := strings.TrimSpace(htmlOriginal) != "" || strings.TrimSpace(prefaceHTML) != ""
	if !useHTML {
		return plainBody, ""
	}

	if strings.TrimSpace(prefaceHTML) == "" && strings.TrimSpace(prefacePlain) != "" {
		prefaceHTML = plainToHTML(prefacePlain)
	}
	if strings.TrimSpace(htmlOriginal) == "" && strings.TrimSpace(plainOriginal) != "" {
		htmlOriginal = plainToHTML(plainOriginal)
	}

	htmlHeader := forwardHeaderHTML(payload)
	htmlBody := joinSections("<br><br>", prefaceHTML, htmlHeader, htmlOriginal)
	return plainBody, htmlBody
}

func plainToHTML(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	escaped := html.EscapeString(value)
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

func collectForwardAttachments(ctx context.Context, svc *gmail.Service, messageID string, part *gmail.MessagePart) ([]mailAttachment, error) {
	if part == nil {
		return nil, nil
	}
	attachments := []mailAttachment{}
	var walk func(p *gmail.MessagePart) error
	walk = func(p *gmail.MessagePart) error {
		if p == nil {
			return nil
		}
		if p.Body != nil {
			filename := strings.TrimSpace(p.Filename)
			hasAttachmentID := strings.TrimSpace(p.Body.AttachmentId) != ""
			hasInlineData := strings.TrimSpace(p.Body.Data) != "" && filename != ""
			if hasAttachmentID || hasInlineData {
				if filename == "" {
					filename = defaultAttachmentFilename
				}
				att := mailAttachment{Filename: filename, MIMEType: p.MimeType}
				var data []byte
				if strings.TrimSpace(p.Body.Data) != "" {
					decoded, err := decodeBase64URLBytes(p.Body.Data)
					if err != nil {
						return err
					}
					data = decoded
				} else {
					body, err := svc.Users.Messages.Attachments.Get("me", messageID, p.Body.AttachmentId).Context(ctx).Do()
					if err != nil {
						return err
					}
					if body == nil || body.Data == "" {
						return fmt.Errorf("empty attachment data for %s", filename)
					}
					decoded, err := decodeBase64URLBytes(body.Data)
					if err != nil {
						return err
					}
					data = decoded
				}
				att.Data = data
				attachments = append(attachments, att)
			}
		}
		for _, child := range p.Parts {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(part); err != nil {
		return nil, err
	}
	return attachments, nil
}
