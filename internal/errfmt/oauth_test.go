package errfmt

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/zoom"
)

func TestFormat_OAuthProviderDetails(t *testing.T) {
	for _, code := range []string{"invalid_grant", " INVALID_GRANT ", "invalid_client", "synthetic-secret-code", ""} {
		for _, wrap := range []string{"direct", "wrapped", "joined", "user-facing"} {
			t.Run(code+"/"+wrap, func(t *testing.T) {
				cause := &oauth2.RetrieveError{
					Response: &http.Response{StatusCode: 400, Status: "400 synthetic-secret-status"},
					Body:     []byte("synthetic-secret-body"), ErrorCode: code,
					ErrorDescription: "synthetic-secret-description", ErrorURI: "https://example.invalid/synthetic-secret-uri",
				}
				var err error = cause
				switch wrap {
				case "wrapped":
					err = fmt.Errorf("synthetic-secret-context: %w", cause)
				case "joined":
					err = errors.Join(errors.New("synthetic-secret-sibling"), cause)
				case "user-facing":
					err = NewUserFacingError(cause.Error(), cause)
				}
				want := "OAuth token request failed. Run 'gog auth doctor' to check client configuration, then 'gog auth add <email>' to re-authorize."
				if code == "invalid_grant" || code == " INVALID_GRANT " {
					want = "OAuth token expired or revoked (invalid_grant); run 'gog auth add <email>' to re-authorize."
				}
				if code == "invalid_client" {
					want = "OAuth token request failed (invalid_client); check client credentials with 'gog auth credentials <credentials.json>', then run 'gog auth add <email>' to re-authorize."
				}
				if got := Format(err); got != want {
					t.Fatalf("Format() = %q, want %q", got, want)
				}
				var retrieved *oauth2.RetrieveError
				if !errors.As(err, &retrieved) || retrieved != cause || !errors.Is(err, cause) {
					t.Fatal("formatting must preserve original cause")
				}
			})
		}
	}
}

func TestFormat_ZoomOAuthProviderDetails(t *testing.T) {
	cause := fmt.Errorf("%w: synthetic-provider-status: synthetic-provider-body", zoom.ErrZoomTokenRequestFailed)
	for _, err := range []error{cause, fmt.Errorf("request: %w", cause), errors.Join(errors.New("synthetic-sibling"), cause), NewUserFacingError(cause.Error(), cause)} {
		if got := Format(err); got != "Zoom OAuth token request failed. Verify the Server-to-Server OAuth app is active and its account ID, client ID, client secret, and scopes; run 'gog zoom auth setup --force' to update credentials, then 'gog zoom auth doctor'." {
			t.Fatalf("Format() = %q", got)
		}
		if !errors.Is(err, cause) || !errors.Is(err, zoom.ErrZoomTokenRequestFailed) {
			t.Fatal("formatting must preserve the Zoom OAuth cause and sentinel")
		}
	}
}

func TestFormat_ContextErrorOAuthCauses(t *testing.T) {
	googleErr := &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: "synthetic-secret-description"}
	zoomErr := fmt.Errorf("%w: synthetic-secret-body", zoom.ErrZoomTokenRequestFailed)
	err := NewContextError("calendar write and Zoom rollback failed", fmt.Errorf("synthetic-secret-wrapper: %w", googleErr), zoomErr)
	for _, wrapped := range []error{err, fmt.Errorf("synthetic-secret-outer: %w", err), NewUserFacingError("synthetic-secret-message", err)} {
		got := Format(wrapped)
		for _, want := range []string{"calendar write and Zoom rollback failed", "invalid_grant", "gog auth add <email>", "Zoom OAuth token request failed.", "gog zoom auth setup"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing context/cause %q: %s", want, got)
			}
		}
		if strings.Contains(got, "synthetic-secret") {
			t.Fatalf("unsafe context diagnostic: %s", got)
		}
		if !errors.Is(wrapped, googleErr) || !errors.Is(wrapped, zoom.ErrZoomTokenRequestFailed) {
			t.Fatal("context formatting must retain both original causes")
		}
	}
}
