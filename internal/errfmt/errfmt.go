package errfmt

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/99designs/keyring"
	"github.com/alecthomas/kong"
	ggoogleapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	gogapi "github.com/steipete/gogcli/internal/googleapi"
)

func Format(err error) string {
	if err == nil {
		return ""
	}

	// Handle Kong parse errors with better messaging
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) {
		return formatParseError(parseErr)
	}

	var authErr *gogapi.AuthRequiredError
	if errors.As(err, &authErr) {
		return fmt.Sprintf("No refresh token for %s %s. Run: gog auth add %s --services %s", authErr.Service, authErr.Email, authErr.Email, authErr.Service)
	}

	var credErr *config.CredentialsMissingError
	if errors.As(err, &credErr) {
		return fmt.Sprintf("OAuth credentials missing. Run: gog auth credentials <credentials.json> (expected at %s)", credErr.Path)
	}

	if errors.Is(err, keyring.ErrKeyNotFound) {
		return "Secret not found in keyring (refresh token missing). Run: gog auth add <email>"
	}

	if errors.Is(err, os.ErrNotExist) {
		return err.Error()
	}

	var gerr *ggoogleapi.Error
	if errors.As(err, &gerr) {
		reason := ""
		if len(gerr.Errors) > 0 && gerr.Errors[0].Reason != "" {
			reason = gerr.Errors[0].Reason
		}

		if reason != "" {
			return fmt.Sprintf("Google API error (%d %s): %s", gerr.Code, reason, gerr.Message)
		}

		return fmt.Sprintf("Google API error (%d): %s", gerr.Code, gerr.Message)
	}

	return err.Error()
}

// formatParseError enhances Kong parse errors with helpful hints.
func formatParseError(err *kong.ParseError) string {
	msg := err.Error()

	// If Kong already provided a suggestion, use it as-is
	if strings.Contains(msg, "did you mean") {
		return msg
	}

	// For unknown flag errors without suggestions, add a help hint
	if strings.HasPrefix(msg, "unknown flag") {
		return msg + "\nRun with --help to see available flags"
	}

	// For missing required flags
	if strings.Contains(msg, "missing") || strings.Contains(msg, "required") {
		return msg + "\nRun with --help to see usage"
	}

	return msg
}
