package googleapi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

var (
	errReauthTestMissingRefreshToken = errors.New("missing refresh token")
	errReauthTestTokenNotFound       = errors.New("token not found")
)

func TestReauthFunctionFromContextPersistsAndResetsTokenSource(t *testing.T) {
	store := &fakeStore{token: &secrets.Token{
		Email:        "user@example.com",
		RefreshToken: "revoked-token",
		Scopes:       []string{"scope"},
	}}
	base := reauthTestTokenSource()
	persisting := newPersistingTokenSource(base, store, "default", "user@example.com", *store.token, "calendar", nil).(*persistingTokenSource)

	deps := AuthDependencies{
		ResolveClient: func(string, string) (string, error) { return "default", nil },
		ReadCredentials: func(string) (config.ClientCredentials, error) {
			return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		OpenTokens: func() (secrets.Store, error) { return store, nil },
		Reauth: func(context.Context, string, string, []string, []string, *secrets.Token) (secrets.Token, error) {
			return secrets.Token{
				Email:        "user@example.com",
				RefreshToken: "new-refresh-token",
				Scopes:       []string{"scope"},
			}, nil
		},
	}
	ctx := WithAuthDependencies(context.Background(), deps)

	reauthFn := reauthFunctionFromContext(ctx, "calendar", "user@example.com", []string{"scope"}, persisting)
	if reauthFn == nil {
		t.Fatal("expected non-nil reauth function")
	}

	if err := reauthFn(context.Background()); err != nil {
		t.Fatalf("reauth closure: %v", err)
	}

	if got := base.refreshToken; got != "new-refresh-token" {
		t.Fatalf("in-memory refresh token = %q, want new-refresh-token", got)
	}

	if got := store.token.RefreshToken; got != "new-refresh-token" {
		t.Fatalf("stored refresh token = %q, want new-refresh-token", got)
	}

	// A normal refresh response commonly omits RefreshToken. Persisting its
	// access-token metadata must retain the replacement refresh token.
	if _, err := persisting.Token(); err != nil {
		t.Fatalf("post-reauth token: %v", err)
	}

	if got := store.token.RefreshToken; got != "new-refresh-token" {
		t.Fatalf("stored refresh token after refresh = %q, want new-refresh-token", got)
	}
}

func TestPersistingTokenSourceReauthorizeCoalescesConcurrentCalls(t *testing.T) {
	store := &fakeStore{token: &secrets.Token{Email: "user@example.com", RefreshToken: "revoked-token"}}
	original := *store.token
	firstBase := reauthTestTokenSource()
	secondBase := reauthTestTokenSource()
	first := newPersistingTokenSource(
		firstBase,
		store,
		"default",
		"user@example.com",
		original,
		"calendar",
		nil,
	).(*persistingTokenSource)
	second := newPersistingTokenSource(
		secondBase,
		store,
		"default",
		"user@example.com",
		original,
		"calendar",
		nil,
	).(*persistingTokenSource)

	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	deps := AuthDependencies{
		ReauthCoordinator: NewReauthCoordinator(),
		Reauth: func(ctx context.Context, _ string, _ string, _ []string, _ []string, stored *secrets.Token) (secrets.Token, error) {
			mu.Lock()
			calls++

			if calls == 1 {
				close(started)
			}

			mu.Unlock()
			<-release

			if err := ctx.Err(); err != nil {
				return secrets.Token{}, fmt.Errorf("reauthorization context: %w", err)
			}

			updated := *stored
			updated.RefreshToken = "new-refresh-token"

			return updated, nil
		},
	}
	ctx := WithAuthDependencies(context.Background(), deps)
	firstReauth := reauthFunctionFromContext(ctx, "calendar", "user@example.com", []string{"scope"}, first)
	secondReauth := reauthFunctionFromContext(ctx, "drive", "user@example.com", []string{"scope"}, second)

	errs := make(chan error, 2)
	go func() { errs <- firstReauth(context.Background()) }()

	<-started

	go func() { errs <- secondReauth(context.Background()) }()

	close(release)

	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("reauthorize: %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if calls != 1 {
		t.Fatalf("browser reauthorization calls = %d, want 1", calls)
	}

	if got := firstBase.refreshToken; got != "new-refresh-token" {
		t.Fatalf("first source refresh token = %q, want new-refresh-token", got)
	}

	if got := secondBase.refreshToken; got != "new-refresh-token" {
		t.Fatalf("second source refresh token = %q, want new-refresh-token", got)
	}
}

func TestReauthClosureLoadsStoredToken(t *testing.T) {
	store := &fakeStore{token: &secrets.Token{
		Email:        "user@example.com",
		RefreshToken: "revoked-token",
		Scopes:       []string{"scope1", "scope2", "scope3"},
		Services:     []string{"calendar", "gmail", "drive"},
	}}
	persisting := newPersistingTokenSource(
		reauthTestTokenSource(),
		store,
		"default",
		"user@example.com",
		*store.token,
		"calendar",
		nil,
	).(*persistingTokenSource)

	var passed *secrets.Token
	deps := AuthDependencies{
		Reauth: func(_ context.Context, _, _ string, _, _ []string, stored *secrets.Token) (secrets.Token, error) {
			snapshot := *stored
			passed = &snapshot
			snapshot.RefreshToken = "new-refresh-token"

			return snapshot, nil
		},
	}
	fn := reauthFunctionFromContext(WithAuthDependencies(context.Background(), deps), "calendar", "user@example.com", []string{"scope1"}, persisting)

	if err := fn(context.Background()); err != nil {
		t.Fatalf("reauth: %v", err)
	}

	if passed == nil || len(passed.Scopes) != 3 {
		t.Fatalf("stored token scopes = %#v, want 3 preserved scopes", passed)
	}
}

func TestReauthFunctionFromContextUnavailable(t *testing.T) {
	if fn := reauthFunctionFromContext(context.Background(), "calendar", "user@example.com", []string{"scope"}, nil); fn != nil {
		t.Fatal("expected nil without auth dependencies")
	}

	ctx := WithAuthDependencies(context.Background(), AuthDependencies{})
	if fn := reauthFunctionFromContext(ctx, "calendar", "user@example.com", []string{"scope"}, nil); fn != nil {
		t.Fatal("expected nil without reauth dependency")
	}
}

func reauthTestTokenSource() *resettableOAuthTokenSource {
	return newResettableOAuthTokenSource(func(token *oauth2.Token) oauth2.TokenSource {
		refreshToken := token.RefreshToken

		return tokenSourceFunc(func() (*oauth2.Token, error) {
			if refreshToken == "revoked-token" {
				return nil, &oauth2.RetrieveError{ErrorCode: "invalid_grant"}
			}

			if refreshToken == "" {
				return nil, errReauthTestMissingRefreshToken
			}

			return &oauth2.Token{AccessToken: "fresh-access-token"}, nil
		})
	}, &oauth2.Token{RefreshToken: "revoked-token"})
}

type fakeStore struct {
	token *secrets.Token
}

func (s *fakeStore) Keys() ([]string, error) { return nil, nil }
func (s *fakeStore) GetToken(string, string) (secrets.Token, error) {
	if s.token == nil {
		return secrets.Token{}, errReauthTestTokenNotFound
	}

	return *s.token, nil
}

func (s *fakeStore) SetToken(_ string, _ string, token secrets.Token) error {
	tokenCopy := token
	s.token = &tokenCopy

	return nil
}
func (s *fakeStore) DeleteToken(string, string) error         { return nil }
func (s *fakeStore) ListTokens() ([]secrets.Token, error)     { return nil, nil }
func (s *fakeStore) GetDefaultAccount(string) (string, error) { return "", nil }
func (s *fakeStore) SetDefaultAccount(string, string) error   { return nil }
