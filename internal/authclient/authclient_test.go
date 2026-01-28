package authclient

import (
	"context"
	"testing"
)

func TestWithAccessToken_EmptyToken(t *testing.T) {
	ctx := context.Background()
	ctx2 := WithAccessToken(ctx, "")

	if got := AccessTokenFromContext(ctx2); got != "" {
		t.Errorf("expected empty token, got %q", got)
	}
}

func TestWithAccessToken_WhitespaceOnlyToken(t *testing.T) {
	ctx := context.Background()
	ctx2 := WithAccessToken(ctx, "   ")

	if got := AccessTokenFromContext(ctx2); got != "" {
		t.Errorf("expected empty token for whitespace-only input, got %q", got)
	}
}

func TestWithAccessToken_ValidToken(t *testing.T) {
	ctx := context.Background()
	ctx2 := WithAccessToken(ctx, "ya29.test-token")

	got := AccessTokenFromContext(ctx2)
	if got != "ya29.test-token" {
		t.Errorf("expected 'ya29.test-token', got %q", got)
	}
}

func TestWithAccessToken_TrimsWhitespace(t *testing.T) {
	ctx := context.Background()
	ctx2 := WithAccessToken(ctx, "  ya29.test-token  ")

	got := AccessTokenFromContext(ctx2)
	if got != "ya29.test-token" {
		t.Errorf("expected 'ya29.test-token', got %q", got)
	}
}

func TestAccessTokenFromContext_NilContext(t *testing.T) {
	//nolint:staticcheck // intentionally passing nil for test
	got := AccessTokenFromContext(nil)
	if got != "" {
		t.Errorf("expected empty token for nil context, got %q", got)
	}
}

func TestAccessTokenFromContext_NoTokenSet(t *testing.T) {
	ctx := context.Background()

	got := AccessTokenFromContext(ctx)
	if got != "" {
		t.Errorf("expected empty token when not set, got %q", got)
	}
}
