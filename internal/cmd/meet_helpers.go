package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/meet/v2"

	"github.com/steipete/gogcli/internal/errfmt"
)

// wrapMeetError provides helpful error messages for common Meet API issues.
func wrapMeetError(err error) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	if strings.Contains(errStr, "accessNotConfigured") ||
		strings.Contains(errStr, "Meet REST API has not been used") {
		return errfmt.NewUserFacingError(
			"Meet REST API is not enabled; enable it at: https://console.developers.google.com/apis/api/meet.googleapis.com/overview",
			err,
		)
	}

	if strings.Contains(errStr, "insufficientPermissions") ||
		strings.Contains(errStr, "insufficient authentication scopes") {
		return errfmt.NewUserFacingError(
			"Insufficient permissions for Meet API; re-authenticate with: gog auth add <account> --services meet",
			err,
		)
	}

	return err
}

func meetSpaceNameFilter(spaceName string) string {
	return fmt.Sprintf("space.name = %q", spaceName)
}

// participantDisplayName extracts a human-readable name from a participant.
func participantDisplayName(p *meet.Participant) string {
	if p == nil {
		return ""
	}

	if p.SignedinUser != nil && p.SignedinUser.DisplayName != "" {
		return p.SignedinUser.DisplayName
	}

	if p.AnonymousUser != nil && p.AnonymousUser.DisplayName != "" {
		return p.AnonymousUser.DisplayName
	}

	if p.PhoneUser != nil && p.PhoneUser.DisplayName != "" {
		return p.PhoneUser.DisplayName
	}

	return p.Name
}
