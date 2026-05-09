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
		return fmt.Sprintf(
			"No auth for %s %s.\n\nOAuth (browser flow):\n  gog auth add %s --services %s\n\nWorkspace service account (domain-wide delegation):\n  gog auth service-account set %s --key <service-account.json>",
			authErr.Service,
			authErr.Email,
			authErr.Email,
			authErr.Service,
			authErr.Email,
		)
	}

	var credErr *config.CredentialsMissingError
	if errors.As(err, &credErr) {
		return fmt.Sprintf(
			"OAuth client credentials missing (OAuth client ID JSON).\nDownload from: https://console.cloud.google.com/apis/credentials (Create Credentials → OAuth client ID → Desktop app → Download JSON)\nThen run: gog auth credentials <credentials.json> (expected at %s)",
			credErr.Path,
		)
	}

	if errors.Is(err, keyring.ErrKeyNotFound) {
		return "Secret not found in keyring (refresh token missing). Run: gog auth add <email>"
	}

	if errors.Is(err, os.ErrNotExist) {
		return err.Error()
	}

	var userErr *UserFacingError
	if errors.As(err, &userErr) {
		return userErr.Message
	}

	var gerr *ggoogleapi.Error
	if errors.As(err, &gerr) {
		return formatGoogleAPIError(gerr)
	}

	return err.Error()
}

// UserFacingError forces a specific message, while preserving the underlying cause.
type UserFacingError struct {
	Message string
	Cause   error
}

func (e *UserFacingError) Error() string {
	if e == nil {
		return ""
	}

	return e.Message
}

func (e *UserFacingError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Cause
}

func NewUserFacingError(message string, cause error) error {
	return &UserFacingError{Message: message, Cause: cause}
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
