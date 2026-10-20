package errfmt

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"golang.org/x/oauth2"
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
				want := "OAuth token request failed."
				if code == "invalid_grant" || code == " INVALID_GRANT " {
					want = "OAuth token expired or revoked (invalid_grant); run 'gog auth add' to re-authorize."
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
