package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"
	ggoogleapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/config"
	gogapi "github.com/openclaw/gogcli/internal/googleapi"
)

func TestStableExitCode_PreservesExistingExitError(t *testing.T) {
	in := &ExitError{Code: 3, Err: errors.New("no results")}
	out := stableExitCode(in)
	if !errors.Is(out, in) {
		t.Fatalf("expected same error instance")
	}
	if got := ExitCode(out); got != 3 {
		t.Fatalf("expected exit code 3, got %d", got)
	}
}

func TestStableExitCode_AuthRequired(t *testing.T) {
	in := &gogapi.AuthRequiredError{Service: "gmail", Email: "a@b.com", Cause: keyring.ErrKeyNotFound}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeAuthRequired {
		t.Fatalf("expected exit code %d, got %d", exitCodeAuthRequired, got)
	}
}

func TestStableExitCode_OAuthRetrieveError(t *testing.T) {
	invalidGrant := &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: "Token has been expired or revoked."}
	wrappedGrant := &url.Error{
		Op:  "Get",
		URL: "https://www.googleapis.com/calendar/v3/users/me/calendarList",
		Err: fmt.Errorf("read-only transport: refresh token expired or revoked: %w; run 'gog auth add' to re-authorize", invalidGrant),
	}
	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid grant", err: invalidGrant, want: exitCodeAuthRequired},
		{name: "wrapped invalid grant", err: wrappedGrant, want: exitCodeAuthRequired},
		{name: "normalized code", err: &oauth2.RetrieveError{ErrorCode: " INVALID_GRANT "}, want: exitCodeAuthRequired},
		{name: "other oauth error", err: &oauth2.RetrieveError{ErrorCode: "invalid_client"}, want: 1},
		{name: "provider failure", err: &oauth2.RetrieveError{Response: &http.Response{StatusCode: 503, Status: "503 Service Unavailable"}}, want: 1},
		{name: "description only", err: &oauth2.RetrieveError{Response: &http.Response{StatusCode: 400, Status: "400 Bad Request"}, ErrorDescription: "invalid_grant"}, want: 1},
		{name: "untyped message", err: errors.New(`oauth2: "invalid_grant" "Bad Request"`), want: 1},
		{name: "cancelled reauth", err: errors.Join(invalidGrant, context.Canceled), want: exitCodeCancelled},
		{name: "explicit exit code", err: &ExitError{Code: 10, Err: invalidGrant}, want: exitCodeConfig},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := stableExitCode(tt.err)
			if got := ExitCode(out); got != tt.want {
				t.Fatalf("exit code = %d, want %d", got, tt.want)
			}
			if !errors.Is(out, tt.err) {
				t.Fatal("original error is missing from the error chain")
			}
			if out.Error() != tt.err.Error() {
				t.Fatalf("error text changed: %q", out.Error())
			}
		})
	}
}

func TestStableExitCode_InsufficientScope(t *testing.T) {
	in := &gogapi.InsufficientScopeError{
		Service:        "gmail",
		Email:          "a@b.com",
		RequiredScopes: []string{"https://mail.google.com/"},
	}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeAuthRequired {
		t.Fatalf("expected exit code %d, got %d", exitCodeAuthRequired, got)
	}
}

func TestStableExitCode_CredentialsMissing(t *testing.T) {
	in := &config.CredentialsMissingError{Path: "/tmp/credentials.json", Cause: errors.New("missing")}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeConfig {
		t.Fatalf("expected exit code %d, got %d", exitCodeConfig, got)
	}
}

func TestStableExitCode_GoogleAPINotFound(t *testing.T) {
	in := &ggoogleapi.Error{Code: 404, Message: "not found"}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeNotFound {
		t.Fatalf("expected exit code %d, got %d", exitCodeNotFound, got)
	}
}

func TestStableExitCode_HTTPStatusNotFound(t *testing.T) {
	in := &gogapi.HTTPStatusError{
		Code:   404,
		Status: "NOT_FOUND",
		Err:    errors.New("photos API error"),
	}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeNotFound {
		t.Fatalf("expected exit code %d, got %d", exitCodeNotFound, got)
	}
}

func TestStableExitCode_HTTPStatusResourceExhausted(t *testing.T) {
	in := &gogapi.HTTPStatusError{
		Code:   403,
		Status: "RESOURCE_EXHAUSTED",
		Err:    errors.New("places API error"),
	}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeRateLimited {
		t.Fatalf("expected exit code %d, got %d", exitCodeRateLimited, got)
	}
}

func TestStableExitCode_HTTPStatusRetryable(t *testing.T) {
	in := &gogapi.HTTPStatusError{
		Code:   503,
		Status: "UNAVAILABLE",
		Err:    errors.New("photos API error"),
	}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeRetryable {
		t.Fatalf("expected exit code %d, got %d", exitCodeRetryable, got)
	}
}

func TestStableExitCode_GoogleAPIRateLimited(t *testing.T) {
	in := &ggoogleapi.Error{Code: 429, Message: "too many requests"}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeRateLimited {
		t.Fatalf("expected exit code %d, got %d", exitCodeRateLimited, got)
	}
}

func TestStableExitCode_GoogleAPIQuotaExceeded(t *testing.T) {
	in := &ggoogleapi.Error{
		Code:    403,
		Message: "quota exceeded",
		Errors:  []ggoogleapi.ErrorItem{{Reason: "quotaExceeded"}},
	}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeRateLimited {
		t.Fatalf("expected exit code %d, got %d", exitCodeRateLimited, got)
	}
}

func TestStableExitCode_GoogleAPIRetryable(t *testing.T) {
	in := &ggoogleapi.Error{Code: 503, Message: "backend error"}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeRetryable {
		t.Fatalf("expected exit code %d, got %d", exitCodeRetryable, got)
	}
}

func TestStableExitCode_Cancelled(t *testing.T) {
	out := stableExitCode(context.Canceled)
	if got := ExitCode(out); got != exitCodeCancelled {
		t.Fatalf("expected exit code %d, got %d", exitCodeCancelled, got)
	}
}

func TestStableExitCode_ReadOnly(t *testing.T) {
	out := stableExitCode(gogapi.ErrReadOnly)
	if got := ExitCode(out); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
}

func TestStableExitCode_DeadlineExceeded(t *testing.T) {
	out := stableExitCode(context.DeadlineExceeded)
	if got := ExitCode(out); got != exitCodeRetryable {
		t.Fatalf("expected exit code %d, got %d", exitCodeRetryable, got)
	}
}

func TestStableExitCode_GenericErrorUnchanged(t *testing.T) {
	in := errors.New("boom")
	out := stableExitCode(in)
	if !errors.Is(out, in) {
		t.Fatalf("expected stableExitCode to return original error for generic errors")
	}
	if got := ExitCode(out); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
}
