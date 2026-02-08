package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailGetCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID"`
	Format    string `name:"format" help:"Message format: full|metadata|raw" default:"full"`
	Headers   string `name:"headers" help:"Metadata headers (comma-separated; only for --format=metadata)"`
}

const (
	gmailFormatFull     = "full"
	gmailFormatMetadata = "metadata"
	gmailFormatRaw      = "raw"
)

func (c *GmailGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	messageID := strings.TrimSpace(c.MessageID)
	messageID = normalizeGmailMessageID(messageID)
	if messageID == "" {
		return usage("empty messageId")
	}

	format := strings.TrimSpace(c.Format)
	if format == "" {
		format = gmailFormatFull
	}
	switch format {
	case gmailFormatFull, gmailFormatMetadata, gmailFormatRaw:
	default:
		return fmt.Errorf("invalid --format: %q (expected full|metadata|raw)", format)
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Users.Messages.Get("me", messageID).Format(format).Context(ctx)
	if format == gmailFormatMetadata {
		headerList := splitCSV(c.Headers)
		if len(headerList) == 0 {
			headerList = []string{"From", "To", "Subject", "Date"}
		}
		if !hasHeaderName(headerList, "List-Unsubscribe") {
			headerList = append(headerList, "List-Unsubscribe")
		}
		call = call.MetadataHeaders(headerList...)
	}

	msg, err := call.Do()
	if err != nil {
		return err
	}

	unsubscribe := bestUnsubscribeLink(msg.Payload)
	if outfmt.IsJSON(ctx) {
		// Include a flattened headers map for easier querying
		// (e.g., jq '.headers.to' instead of complex nested queries)
		headers := map[string]string{
			"from":    headerValue(msg.Payload, "From"),
			"to":      headerValue(msg.Payload, "To"),
			"cc":      headerValue(msg.Payload, "Cc"),
			"bcc":     headerValue(msg.Payload, "Bcc"),
			"subject": headerValue(msg.Payload, "Subject"),
			"date":    headerValue(msg.Payload, "Date"),
		}
		payload := map[string]any{
			"message": msg,
			"headers": headers,
		}
		if unsubscribe != "" {
			payload["unsubscribe"] = unsubscribe
		}
		if format == gmailFormatFull {
			if body := bestBodyText(msg.Payload); body != "" {
				payload["body"] = body
			}
		}
		if format == gmailFormatFull || format == gmailFormatMetadata {
			attachments := collectAttachments(msg.Payload)
			if len(attachments) > 0 {
				payload["attachments"] = attachmentOutputs(attachments)
			}
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", msg.Id)
	u.Out().Printf("thread_id\t%s", msg.ThreadId)
	u.Out().Printf("label_ids\t%s", strings.Join(msg.LabelIds, ","))

	switch format {
	case gmailFormatRaw:
		if msg.Raw == "" {
			u.Err().Println("Empty raw message")
			return nil
		}
		decoded, err := base64.RawURLEncoding.DecodeString(msg.Raw)
		if err != nil {
			return err
		}
		u.Out().Println("")
		u.Out().Println(string(decoded))
		return nil
	case gmailFormatMetadata, gmailFormatFull:
		u.Out().Printf("from\t%s", headerValue(msg.Payload, "From"))
		u.Out().Printf("to\t%s", headerValue(msg.Payload, "To"))
		u.Out().Printf("subject\t%s", headerValue(msg.Payload, "Subject"))
		u.Out().Printf("date\t%s", headerValue(msg.Payload, "Date"))
		if unsubscribe != "" {
			u.Out().Printf("unsubscribe\t%s", unsubscribe)
		}
		attachments := attachmentOutputs(collectAttachments(msg.Payload))
		if len(attachments) > 0 {
			u.Out().Println("")
			printAttachmentLines(u.Out(), attachments)
		}
		if format == gmailFormatFull {
			body := bestBodyText(msg.Payload)
			if body != "" {
				u.Out().Println("")
				u.Out().Println(body)
			}
		}
		return nil
	default:
		return nil
	}
}
