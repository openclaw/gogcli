package googleapi

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

// TestReauthFunctionFromContextResetsTokenSource verifies that the production
// closure returned by reauthFunctionFromContext actually calls
// ResetRefreshToken on the real persistingTokenSource chain after a
// successful reauth. This is the integration test for the glue code that
// was broken in the initial implementation.
func TestReauthFunctionFromContextResetsTokenSource(t *testing.T) {
	// Build a real resettableOAuthTokenSource → persistingTokenSource chain.
	// The oauth2.Config's TokenSource will use the refresh token we give it.
	// We simulate a token endpoint that returns invalid_grant for the old
	// token and success for the new one.

	var reauthRefreshToken string

	// Create the auth dependencies with a fake Reauth that returns a new
	// refresh token (simulating a successful browser flow).
	deps := AuthDependencies{
		ResolveClient: func(string, string) (string, error) { return "default", nil },
		ReadCredentials: func(string) (config.ClientCredentials, error) {
			return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		OpenTokens: func() (secrets.Store, error) {
			return &fakeStore{}, nil
		},
		Reauth: func(ctx context.Context, email, client string, services, scopes []string, storedToken *secrets.Token) (string, error) {
			reauthRefreshToken = "new-refresh-token"
			return "new-refresh-token", nil
		},
	}

	ctx := WithAuthDependencies(context.Background(), deps)

	// Build a real token source chain as the transport layer would.
	cfg := oauth2.Config{
		ClientID:     "id",
		ClientSecret: "secret",
		Endpoint:     google.Endpoint,
		Scopes:       []string{"https://www.googleapis.com/auth/calendar"},
	}

	// The transport construction captures the token source; we simulate
	// what tokenSourceForAccountScopesWithStoredScopeCheck does.
	baseSource := newResettableOAuthTokenSource(func(t *oauth2.Token) oauth2.TokenSource {
		return cfg.TokenSource(context.Background(), t)
	}, &oauth2.Token{
		RefreshToken: "revoked-token",
	})

	ts := newPersistingTokenSource(baseSource, &fakeStore{}, "default", "user@example.com", secrets.Token{
		Email:        "user@example.com",
		RefreshToken: "revoked-token",
		Scopes:       []string{"https://www.googleapis.com/auth/calendar"},
	}, "calendar", nil)

	// Get the reauth closure.
	reauthFn := reauthFunctionFromContext(ctx, "calendar", "user@example.com", []string{"https://www.googleapis.com/auth/calendar"}, ts)
	if reauthFn == nil {
		t.Fatal("expected non-nil reauth function")
	}

	// Call the reauth closure.
	if err := reauthFn(context.Background()); err != nil {
		t.Fatalf("reauth closure: %v", err)
	}

	if reauthRefreshToken != "new-refresh-token" {
		t.Fatalf("Reauth was not called or returned unexpected token: %q", reauthRefreshToken)
	}

	// Verify the token source was actually reset by checking the inner
	// resettableOAuthTokenSource's refresh token.
	persisting, ok := ts.(*persistingTokenSource)
	if !ok {
		t.Fatalf("expected *persistingTokenSource, got %T", ts)
	}

	resettable, ok := persisting.base.(*resettableOAuthTokenSource)
	if !ok {
		t.Fatalf("expected *resettableOAuthTokenSource, got %T", persisting.base)
	}

	if resettable.refreshToken != "new-refresh-token" {
		t.Fatalf("token source was not reset: expected new-refresh-token, got %q", resettable.refreshToken)
	}
}

// TestReauthFunctionFromContextNilWhenNoDeps verifies that the closure is
// nil when auth dependencies are not in the context.
func TestReauthFunctionFromContextNilWhenNoDeps(t *testing.T) {
	fn := reauthFunctionFromContext(context.Background(), "calendar", "user@example.com", []string{"scope"}, nil)
	if fn != nil {
		t.Fatal("expected nil reauth function when no deps in context")
	}
}

// TestReauthFunctionFromContextNilWhenNoReauthFunc verifies that the closure
// is nil when the Reauth field is not set on the dependencies.
func TestReauthFunctionFromContextNilWhenNoReauthFunc(t *testing.T) {
	deps := AuthDependencies{
		ResolveClient:   func(string, string) (string, error) { return "default", nil },
		ReadCredentials: func(string) (config.ClientCredentials, error) { return config.ClientCredentials{}, nil },
		OpenTokens:      func() (secrets.Store, error) { return &fakeStore{}, nil },
		// Reauth is nil
	}
	ctx := WithAuthDependencies(context.Background(), deps)
	fn := reauthFunctionFromContext(ctx, "calendar", "user@example.com", []string{"scope"}, nil)
	if fn != nil {
		t.Fatal("expected nil reauth function when Reauth is nil")
	}
}

// TestResettableOAuthTokenSourceResetRefreshToken verifies that
// ResetRefreshToken replaces the refresh token and rebuilds the source.
func TestResettableOAuthTokenSourceResetRefreshToken(t *testing.T) {
	var tokenRequests []string

	source := newResettableOAuthTokenSource(func(t *oauth2.Token) oauth2.TokenSource {
		// Track which refresh token is being used.
		tokenRequests = append(tokenRequests, t.RefreshToken)
		// Return a stub token source that always succeeds.
		return &stubTokenSource{token: &oauth2.Token{AccessToken: "access"}}
	}, &oauth2.Token{
		RefreshToken: "old-token",
	})

	// Verify initial state
	if source.refreshToken != "old-token" {
		t.Fatalf("expected old-token, got %q", source.refreshToken)
	}

	// Reset
	source.ResetRefreshToken("new-token")

	if source.refreshToken != "new-token" {
		t.Fatalf("expected new-token, got %q", source.refreshToken)
	}

	// Verify the source was rebuilt — calling Token() should use the new token
	_, _ = source.Token()

	if len(tokenRequests) == 0 {
		t.Fatal("Token() did not create a new source")
	}

	if tokenRequests[len(tokenRequests)-1] != "new-token" {
		t.Fatalf("expected new-token in request, got %q", tokenRequests[len(tokenRequests)-1])
	}
}

// TestPersistingTokenSourceResetRefreshToken verifies delegation.
func TestPersistingTokenSourceResetRefreshToken(t *testing.T) {
	base := newResettableOAuthTokenSource(func(t *oauth2.Token) oauth2.TokenSource {
		return (&oauth2.Config{
			ClientID:     "id",
			ClientSecret: "secret",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://example.com/auth",
				TokenURL: "https://example.com/token",
			},
			Scopes: []string{"scope"},
		}).TokenSource(context.Background(), t)
	}, &oauth2.Token{RefreshToken: "old"})

	persisting := newPersistingTokenSource(base, &fakeStore{}, "default", "user@example.com", secrets.Token{
		Email:        "user@example.com",
		RefreshToken: "old",
	}, "calendar", nil)

	// Reset via the persisting wrapper
	persisting.(*persistingTokenSource).ResetRefreshToken("new-refresh")

	// Verify the inner source was updated
	if base.refreshToken != "new-refresh" {
		t.Fatalf("expected new-refresh, got %q", base.refreshToken)
	}
}

// TestReauthClosureLoadsStoredToken verifies the closure loads the stored
// token and passes it through to Reauth for scope preservation.
func TestReauthClosureLoadsStoredToken(t *testing.T) {
	store := &fakeStore{
		token: &secrets.Token{
			Email:    "user@example.com",
			Scopes:   []string{"scope1", "scope2", "scope3"},
			Services: []string{"calendar", "gmail", "drive"},
		},
	}

	var passedStoredToken *secrets.Token

	deps := AuthDependencies{
		ResolveClient: func(string, string) (string, error) { return "default", nil },
		ReadCredentials: func(string) (config.ClientCredentials, error) {
			return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		OpenTokens: func() (secrets.Store, error) { return store, nil },
		Reauth: func(ctx context.Context, email, client string, services, scopes []string, storedToken *secrets.Token) (string, error) {
			passedStoredToken = storedToken
			return "new-token", nil
		},
	}

	ctx := WithAuthDependencies(context.Background(), deps)

	// Build a real token source so the reset path works
	baseSource := newResettableOAuthTokenSource(func(t *oauth2.Token) oauth2.TokenSource {
		return (&oauth2.Config{
			ClientID:     "id",
			ClientSecret: "secret",
			Endpoint:     google.Endpoint,
			Scopes:       []string{"scope"},
		}).TokenSource(context.Background(), t)
	}, &oauth2.Token{RefreshToken: "old"})

	ts := newPersistingTokenSource(baseSource, store, "default", "user@example.com", secrets.Token{
		Email:        "user@example.com",
		RefreshToken: "old",
	}, "calendar", nil)

	reauthFn := reauthFunctionFromContext(ctx, "calendar", "user@example.com", []string{"scope1"}, ts)
	if reauthFn == nil {
		t.Fatal("expected non-nil reauth function")
	}

	if err := reauthFn(context.Background()); err != nil {
		t.Fatalf("reauth: %v", err)
	}

	if passedStoredToken == nil {
		t.Fatal("expected stored token to be passed to Reauth")
	}

	if len(passedStoredToken.Scopes) != 3 {
		t.Fatalf("expected 3 scopes from stored token, got %d: %v", len(passedStoredToken.Scopes), passedStoredToken.Scopes)
	}
}

// stubTokenSource returns a fixed token without any network calls.
type stubTokenSource struct {
	token *oauth2.Token
}

func (s *stubTokenSource) Token() (*oauth2.Token, error) {
	return s.token, nil
}

// fakeStore is a minimal secrets.Store for testing.
type fakeStore struct {
	token *secrets.Token
}

func (s *fakeStore) Keys() ([]string, error)                       { return nil, nil }
func (s *fakeStore) GetToken(client, email string) (secrets.Token, error) {
	if s.token != nil {
		return *s.token, nil
	}
	return secrets.Token{}, errors.New("not found")
}
func (s *fakeStore) SetToken(client, email string, tok secrets.Token) error  { s.token = &tok; return nil }
func (s *fakeStore) DeleteToken(client, email string) error                   { return nil }
func (s *fakeStore) ListTokens() ([]secrets.Token, error)                     { return nil, nil }
func (s *fakeStore) GetDefaultAccount(string) (string, error)                 { return "", nil }
func (s *fakeStore) SetDefaultAccount(string, string) error                   { return nil }
