package errfmt

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/99designs/keyring"
	"github.com/alecthomas/kong"
	"golang.org/x/oauth2"
	ggoogleapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/config"
	gogapi "github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/zoom"
)

func Format(err error) string {
	if err == nil {
		return ""
	}

	// Structured application context is safe to retain because each cause is
	// formatted separately; arbitrary wrapper messages below are still omitted.
	var contextErr *contextError
	if errors.As(err, &contextErr) {
		return contextErr.Error()
	}

	// OAuth errors can contain provider-controlled descriptions, URIs, and
	// response bodies, including echoed credentials. Never render their text
	// or wrapper messages, which may already include those details. Keep the
	// original error untouched so callers can still classify its cause.
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		switch strings.ToLower(strings.TrimSpace(retrieveErr.ErrorCode)) {
		case "invalid_grant":
			return "OAuth token expired or revoked (invalid_grant); run 'gog auth add <email>' to re-authorize."
		case "invalid_client":
			return "OAuth token request failed (invalid_client); check client credentials with 'gog auth credentials <credentials.json>', then run 'gog auth add <email>' to re-authorize."
		default:
			return "OAuth token request failed. Run 'gog auth doctor' to check client configuration, then 'gog auth add <email>' to re-authorize."
		}
	}
	if errors.Is(err, zoom.ErrZoomTokenRequestFailed) {
		return "Zoom OAuth token request failed. Verify the Server-to-Server OAuth app is active and its account ID, client ID, client secret, and scopes; run 'gog zoom auth setup --alias <alias> --force' to update credentials, then 'gog zoom auth doctor --alias <alias>'. Replace <alias> with the failing alias (default if none was selected)."
	}

	var userErr *UserFacingError
	if errors.As(err, &userErr) {
		return userErr.Message
	}

	// Handle Kong parse errors with better messaging
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) {
		return formatParseError(parseErr)
	}

	var authErr *gogapi.AuthRequiredError
	if errors.As(err, &authErr) {
		if isServiceAccountOnlyAuthService(authErr.Service) {
			return fmt.Sprintf(
				"No auth for %s %s.\n\nWorkspace service account (domain-wide delegation):\n  gog auth service-account set %s --key <service-account.json>",
				authErr.Service,
				authErr.Email,
				authErr.Email,
			)
		}

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

	var gerr *ggoogleapi.Error
	if errors.As(err, &gerr) {
		return formatGoogleAPIError(gerr)
	}

	return err.Error()
}

func isServiceAccountOnlyAuthService(service string) bool {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "admin", "admin directory", "admin orgunits", "groups", "keep":
		return true
	default:
		return false
	}
}

// NewContextError retains application-authored context and formats each cause
// separately. The message must not contain provider error text; pass errors as
// causes instead. Unwrap preserves every original cause for classification.
func NewContextError(message string, causes ...error) error {
	return &contextError{message: message, causes: causes}
}

type contextError struct {
	message string
	causes  []error
}

func (e *contextError) Error() string {
	parts := []string{e.message}
	for _, cause := range e.causes {
		if cause != nil {
			parts = append(parts, Format(cause))
		}
	}

	return strings.Join(parts, "\n")
}

func (e *contextError) Unwrap() []error { return e.causes }

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
