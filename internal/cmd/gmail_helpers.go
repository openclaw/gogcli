package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"
)

// defaultAttachmentFilename is used when an attachment has no filename.
const defaultAttachmentFilename = "attachment"

// resolveSendFrom determines the From address to use when sending/forwarding.
// If from is specified, validates it as a verified send-as alias and formats with display name.
// If from is empty, uses the account and looks up its display name from send-as settings.
// Returns the formatted "From" address (e.g., "Display Name <email@example.com>" or "email@example.com").
func resolveSendFrom(ctx context.Context, svc *gmail.Service, account, from string) (string, error) {
	fromAddr := account
	from = strings.TrimSpace(from)
	if from != "" {
		sa, err := svc.Users.Settings.SendAs.Get("me", from).Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("invalid --from address %q: %w", from, err)
		}
		if sa.VerificationStatus != gmailVerificationAccepted {
			return "", fmt.Errorf("--from address %q is not verified (status: %s)", from, sa.VerificationStatus)
		}
		fromAddr = from
		if sa.DisplayName != "" {
			fromAddr = sa.DisplayName + " <" + from + ">"
		}
		return fromAddr, nil
	}

	// No --from specified: look up the primary account's send-as settings
	// to get the display name
	sa, saErr := svc.Users.Settings.SendAs.Get("me", account).Context(ctx).Do()
	if saErr == nil && sa.DisplayName != "" {
		fromAddr = sa.DisplayName + " <" + account + ">"
	}
	return fromAddr, nil
}
