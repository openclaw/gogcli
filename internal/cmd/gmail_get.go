package cmd

import (
	"context"
	"strings"

	"github.com/openclaw/gogcli/internal/gmailcontent"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type GmailGetCmd struct {
	MessageID               string `arg:"" name:"messageId" help:"Message ID"`
	UseIndexedAttachmentIDs bool   `name:"use-indexed-attachment-ids" help:"Use 0-based indexes as attachment ids everywhere (output, the download argument, and saved filenames)" env:"GOG_GMAIL_USE_INDEXED_ATTACHMENT_IDS"`
	Format                  string `name:"format" help:"Message format: full|metadata|raw" default:"full"`
	Headers                 string `name:"headers" help:"Metadata headers (comma-separated; only for --format=metadata)"`
	SanitizeContent         bool   `name:"sanitize-content" aliases:"sanitize,safe" help:"Emit agent-oriented sanitized content: strip HTML, remove HTTP(S) URLs, and omit raw Gmail payloads from JSON"`
}

const (
	gmailFormatFull     = "full"
	gmailFormatMetadata = "metadata"
	gmailFormatMinimal  = "minimal"
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
		return usagef("invalid --format: %q (expected full|metadata|raw)", format)
	}
	if c.SanitizeContent && format == gmailFormatRaw {
		return usage("--sanitize-content cannot be used with --format raw")
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Users.Messages.Get("me", messageID).Format(format).Context(ctx)
	if format == gmailFormatMetadata {
		headerList := splitCSV(c.Headers)
		if len(headerList) == 0 {
			headerList = defaultGmailGetMetadataHeaders()
		} else if !hasHeaderName(headerList, "List-Unsubscribe") {
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
		if c.SanitizeContent {
			output := sanitizedGmailMessage(msg, format == gmailFormatFull, c.UseIndexedAttachmentIDs)
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"message": output})
		}
		// Include a flattened headers map for easier querying
		// (e.g., jq '.headers.to' instead of complex nested queries)
		headers := map[string]string{
			"from":        headerValue(msg.Payload, "From"),
			"to":          headerValue(msg.Payload, "To"),
			"cc":          headerValue(msg.Payload, "Cc"),
			"bcc":         headerValue(msg.Payload, "Bcc"),
			"subject":     headerValue(msg.Payload, "Subject"),
			"date":        headerValue(msg.Payload, "Date"),
			"message_id":  headerValue(msg.Payload, "Message-ID"),
			"in_reply_to": headerValue(msg.Payload, "In-Reply-To"),
			"references":  headerValue(msg.Payload, "References"),
		}
		payload := map[string]any{
			"message": msg,
			"headers": headers,
		}
		if unsubscribe != "" {
			payload["unsubscribe"] = unsubscribe
		}
		if format == gmailFormatFull {
			if body := gmailcontent.BestBodyText(msg.Payload); body != "" {
				payload["body"] = body
			}
		}
		if format == gmailFormatFull || format == gmailFormatMetadata {
			attachments := collectAttachments(msg.Payload)
			if len(attachments) > 0 {
				payload["attachments"] = attachmentOutputs(attachments, c.UseIndexedAttachmentIDs)
			}
		}
		// The raw message dump carries the opaque attachmentIds too; in indexed
		// mode strip them so the dump omits the long ids (the index is in the
		// curated attachments output).
		if c.UseIndexedAttachmentIDs {
			stripAttachmentIDs(msg.Payload)
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), outfmt.PrimaryResult(payload))
	}

	u.Out().Linef("id\t%s%s", msg.Id, gmailHumanMessageStatusMarker(ctx, msg.LabelIds))
	u.Out().Linef("thread_id\t%s", msg.ThreadId)
	u.Out().Linef("label_ids\t%s", strings.Join(msg.LabelIds, ","))

	switch format {
	case gmailFormatRaw:
		if msg.Raw == "" {
			u.Err().Println("Empty raw message")
			return nil
		}
		decoded, err := decodeGmailRaw(msg.Raw)
		if err != nil {
			return err
		}
		u.Out().Println("")
		u.Out().Println(string(decoded))
		return nil
	case gmailFormatMetadata, gmailFormatFull:
		header := func(name string) string {
			value := headerValue(msg.Payload, name)
			if c.SanitizeContent {
				return sanitizeGmailText(value)
			}
			return value
		}
		u.Out().Linef("from\t%s", header("From"))
		u.Out().Linef("to\t%s", header("To"))
		u.Out().Linef("cc\t%s", header("Cc"))
		u.Out().Linef("bcc\t%s", header("Bcc"))
		u.Out().Linef("subject\t%s", header("Subject"))
		u.Out().Linef("date\t%s", header("Date"))
		if unsubscribe != "" && !c.SanitizeContent {
			u.Out().Linef("unsubscribe\t%s", unsubscribe)
		}
		attachments := attachmentOutputs(collectAttachments(msg.Payload), c.UseIndexedAttachmentIDs)
		if len(attachments) > 0 {
			u.Out().Println("")
			printAttachmentLines(u.Out(), attachments)
		}
		if format == gmailFormatFull {
			body := gmailcontent.BestBodyText(msg.Payload)
			if body != "" {
				if c.SanitizeContent {
					displayBody, isHTML := gmailcontent.BestBodyForDisplay(msg.Payload)
					body = sanitizeGmailBody(displayBody, isHTML)
				}
				u.Out().Println("")
				u.Out().Println(body)
			}
		}
		return nil
	default:
		return nil
	}
}
