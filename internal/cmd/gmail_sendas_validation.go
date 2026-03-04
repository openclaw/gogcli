package cmd

import (
	"strings"

	"google.golang.org/api/gmail/v1"
)

// Gmail-managed Workspace aliases can omit verificationStatus but are still valid
// From addresses when they do not rely on a custom SMTP relay.
func sendAsAllowedForFrom(sa *gmail.SendAs) bool {
	if sa == nil {
		return false
	}

	status := strings.TrimSpace(sa.VerificationStatus)
	if strings.EqualFold(status, gmailVerificationAccepted) {
		return true
	}

	return status == "" && sa.SmtpMsa == nil
}
