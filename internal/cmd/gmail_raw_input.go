package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/mail"
	"strings"

	"google.golang.org/api/gmail/v1"
)

// Gmail's discovery document limits draft and send media uploads to 35 MiB.
const maxGmailRawMessageBytes int64 = 35 * 1024 * 1024

// Only content-safe metadata is exported for offline dry runs.
type gmailRawMessagePlan struct {
	Source     string `json:"source"`
	Bytes      int    `json:"bytes"`
	SHA256     string `json:"sha256"`
	ThreadID   string `json:"thread_id,omitempty"`
	sender     *mail.Address
	from       string
	inReplyTo  string
	references string
}

func readRawGmailInput(ctx context.Context, source, threadID string, requireRecipients bool) ([]byte, gmailRawMessagePlan, error) {
	source = strings.TrimSpace(source)
	raw, message, err := readRFC822InputWithLimit(ctx, source, maxGmailRawMessageBytes)
	if err != nil {
		return nil, gmailRawMessagePlan{}, err
	}
	if !bytes.Contains(raw, []byte("\r\n\r\n")) && !bytes.Contains(raw, []byte("\n\n")) {
		return nil, gmailRawMessagePlan{}, usage("invalid RFC822 input: missing header/body separator")
	}
	fromHeaders := message.Header["From"]
	if len(fromHeaders) == 0 || strings.TrimSpace(fromHeaders[0]) == "" {
		return nil, gmailRawMessagePlan{}, usage("invalid RFC822 input: missing From header")
	}
	if len(fromHeaders) != 1 {
		return nil, gmailRawMessagePlan{}, usage("invalid RFC822 input: exactly one From header is required")
	}
	sender, addressErr := mail.ParseAddress(fromHeaders[0])
	if addressErr != nil {
		return nil, gmailRawMessagePlan{}, usage("invalid RFC822 input: invalid From header")
	}
	if requireRecipients && strings.TrimSpace(message.Header.Get("To")) == "" && strings.TrimSpace(message.Header.Get("Cc")) == "" && strings.TrimSpace(message.Header.Get("Bcc")) == "" {
		return nil, gmailRawMessagePlan{}, usage("invalid RFC822 input: missing recipient header")
	}
	for _, header := range []string{"To", "Cc", "Bcc"} {
		for _, value := range message.Header[header] {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if _, addressErr := mail.ParseAddressList(value); addressErr != nil {
				return nil, gmailRawMessagePlan{}, usagef("invalid RFC822 input: invalid %s header", header)
			}
		}
	}
	threadID = normalizeGmailThreadID(threadID)
	if strings.ContainsAny(threadID, " \t\r\n") {
		return nil, gmailRawMessagePlan{}, usage("invalid --thread-id")
	}
	digest := sha256.Sum256(raw)
	return raw, gmailRawMessagePlan{
		Source: source, Bytes: len(raw), SHA256: fmt.Sprintf("%x", digest), ThreadID: threadID,
		sender: sender, from: fromHeaders[0],
		inReplyTo: message.Header.Get("In-Reply-To"), references: message.Header.Get("References"),
	}, nil
}

// Validate the address without replacing the caller's display name or headers.
// No-send policy belongs to the send command, not to sender validation.
func validateRawGmailSender(ctx context.Context, svc *gmail.Service, account string, plan gmailRawMessagePlan) error {
	if account == accessTokenPlaceholderAccount || account == adcPlaceholderAccount {
		return usage("--raw-file requires an explicit --account with direct access tokens or ADC")
	}
	if !strings.EqualFold(account, plan.sender.Address) {
		_, err := resolveComposeSender(ctx, svc, account, plan.sender.Address)
		return err
	}
	return nil
}
