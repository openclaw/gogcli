package authclient

import (
	"context"
	"testing"
)

func TestWithAccessToken_EmptyToken(t *testing.T) {
	ctx := context.Background()
	if got := AccessTokenFromContext(WithAccessToken(ctx, "")); got != "" {
		t.Fatalf("expected empty token, got %q", got)
	}
}

func TestWithAccessToken_TrimsWhitespace(t *testing.T) {
	ctx := context.Background()
	if got := AccessTokenFromContext(WithAccessToken(ctx, "  ya29.test-token  ")); got != "ya29.test-token" {
		t.Fatalf("expected trimmed token, got %q", got)
	}
}

func TestAccessTokenFromContext_NilContext(t *testing.T) {
	//nolint:staticcheck // intentional nil for regression coverage
	if got := AccessTokenFromContext(nil); got != "" {
		t.Fatalf("expected empty token, got %q", got)
	}
}
