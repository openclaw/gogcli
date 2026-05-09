package cmd

import (
	"context"
	"fmt"
	"strings"
)

// GmailRawCmd dumps the full Users.Messages.Get response as JSON. Note the
// naming collision: "raw" is both the gog-side subcommand name (meaning
// "dump the full API response") and a Gmail API `format=raw` value meaning
// "base64url-encoded RFC822 source". This command defaults to
// `format=full` (the structured parsed message). Pass `--format raw` to
// get Gmail's native RAW — the base64url blob will still appear as the
// `raw` field inside the JSON response.
//
// REST reference: https://developers.google.com/gmail/api/reference/rest/v1/users.messages/get
// Go type: https://pkg.go.dev/google.golang.org/api/gmail/v1#Message
type GmailRawCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID"`
	Format    string `name:"format" help:"Gmail format: full|metadata|minimal|raw (default: full; note: 'raw' here means Gmail's base64url RFC822 blob, NOT the gog raw subcommand sense)" default:"full"`
	Pretty    bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *GmailRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	messageID := normalizeGmailMessageID(strings.TrimSpace(c.MessageID))
	if messageID == "" {
		return usage("empty messageId")
	}

	format := strings.TrimSpace(c.Format)
	if format == "" {
		format = gmailFormatFull
	}

	switch format {
	case gmailFormatFull, gmailFormatMetadata, gmailFormatMinimal, gmailFormatRaw:
	default:
		return fmt.Errorf("invalid --format: %q (expected full|metadata|minimal|raw)", format)
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	msg, err := svc.Users.Messages.Get("me", messageID).Format(format).Context(ctx).Do()
	if err != nil {
		return err
	}
	msg, err = requireRawResponse(msg, "message not found")
	if err != nil {
		return err
	}

	return writeRawJSON(ctx, msg, c.Pretty)
}
