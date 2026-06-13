package authclient

import (
	"context"
	"errors"
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

func TestResolveClientUsesContextResolver(t *testing.T) {
	t.Parallel()

	ctx := WithClientResolver(context.Background(), func(email string, override string) (string, error) {
		if email != "user@example.com" || override != "work" {
			t.Fatalf("resolver args = %q, %q", email, override)
		}

		return "resolved", nil
	})
	ctx = WithClient(ctx, "work")

	client, err := ResolveClient(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("ResolveClient: %v", err)
	}

	if client != "resolved" {
		t.Fatalf("client = %q, want resolved", client)
	}
}

func TestResolveClientWithOverrideUsesResolver(t *testing.T) {
	t.Parallel()

	ctx := WithClientResolver(context.Background(), func(_ string, override string) (string, error) {
		return override, nil
	})

	client, err := ResolveClientWithOverride(ctx, "user@example.com", "custom")
	if err != nil {
		t.Fatalf("ResolveClientWithOverride: %v", err)
	}

	if client != "custom" {
		t.Fatalf("client = %q, want custom", client)
	}
}

func TestResolveClientRequiresResolver(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		call func() (string, error)
	}{
		{
			name: "context override",
			call: func() (string, error) {
				return ResolveClient(context.Background(), "user@example.com")
			},
		},
		{
			name: "explicit override",
			call: func() (string, error) {
				return ResolveClientWithOverride(context.Background(), "user@example.com", "work")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := test.call(); !errors.Is(err, errClientResolverRequired) {
				t.Fatalf("error = %v, want resolver-required", err)
			}
		})
	}
}

func TestUpdateEmailReferencesUsesContextUpdater(t *testing.T) {
	t.Parallel()

	var gotOld, gotNew string
	ctx := WithEmailReferenceUpdater(context.Background(), func(oldEmail, newEmail string) error {
		gotOld = oldEmail
		gotNew = newEmail

		return nil
	})

	if err := UpdateEmailReferences(ctx, "old@example.com", "new@example.com"); err != nil {
		t.Fatalf("UpdateEmailReferences: %v", err)
	}

	if gotOld != "old@example.com" || gotNew != "new@example.com" {
		t.Fatalf("updater args = %q, %q", gotOld, gotNew)
	}
}

func TestUpdateEmailReferencesRequiresUpdater(t *testing.T) {
	t.Parallel()

	if err := UpdateEmailReferences(context.Background(), "old@example.com", "new@example.com"); !errors.Is(err, errEmailReferenceUpdaterRequired) {
		t.Fatalf("error = %v, want updater-required", err)
	}
}
