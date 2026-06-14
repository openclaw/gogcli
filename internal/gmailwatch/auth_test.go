package gmailwatch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizerOIDCAndSharedTokenFallback(t *testing.T) {
	t.Parallel()

	verified := false
	authorizer := &Authorizer{
		Config: AuthConfig{
			VerifyOIDC:  true,
			OIDCEmail:   "service@example.com",
			SharedToken: "shared",
		},
		Verify: func(_ context.Context, token, audience, email string) (bool, error) {
			verified = true

			if token != "oidc" || audience != "https://example.com/hook" || email != "service@example.com" {
				t.Fatalf("verify args = %q %q %q", token, audience, email)
			}

			return true, nil
		},
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/hook", nil)
	request.Header.Set("Authorization", "Bearer oidc")

	if !authorizer.Authorize(request) || !verified {
		t.Fatal("OIDC request was not authorized")
	}

	authorizer.Verify = func(context.Context, string, string, string) (bool, error) {
		return false, errors.New("invalid token") //nolint:err113 // Test-only verifier failure.
	}

	request = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/hook?token=shared", nil)
	if !authorizer.Authorize(request) {
		t.Fatal("shared-token fallback was not authorized")
	}
}

func TestAudienceUsesForwardedHeaders(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/hook", nil)
	request.Header.Set("X-Forwarded-Proto", "https, http")
	request.Header.Set("X-Forwarded-Host", "proxy.example.com, example.com")

	if got := Audience(request, ""); got != "https://proxy.example.com/hook" {
		t.Fatalf("audience = %q", got)
	}

	if got := Audience(request, "https://explicit.example/hook"); got != "https://explicit.example/hook" {
		t.Fatalf("explicit audience = %q", got)
	}
}

func TestBearerAndSharedTokens(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/hook?token=query", nil)
	request.Header.Set("Authorization", "Bearer oidc")

	if BearerToken(request) != "oidc" {
		t.Fatalf("bearer = %q", BearerToken(request))
	}

	if !SharedTokenMatches(request, "query") {
		t.Fatal("query token did not match")
	}

	request.Header.Set("x-gog-token", "header")

	if !SharedTokenMatches(request, "header") || SharedTokenMatches(request, "query") {
		t.Fatal("header token precedence mismatch")
	}
}
