package googleauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

// mockSecretStore is a minimal secrets.Store for testing Reauth.
type mockSecretStore struct {
	tokens map[string]secrets.Token
}

func (s *mockSecretStore) GetToken(client string, email string) (secrets.Token, error) {
	key := client + ":" + email
	if tok, ok := s.tokens[key]; ok {
		return tok, nil
	}
	return secrets.Token{}, errors.New("not found")
}

func (s *mockSecretStore) SetToken(client string, email string, token secrets.Token) error {
	key := client + ":" + email
	s.tokens[key] = token
	return nil
}

func (s *mockSecretStore) DeleteToken(client string, email string) error {
	key := client + ":" + email
	delete(s.tokens, key)
	return nil
}



func (s *mockSecretStore) ListTokens() ([]secrets.Token, error) {
	var out []secrets.Token
	for _, tok := range s.tokens {
		out = append(out, tok)
	}
	return out, nil
}

func (s *mockSecretStore) Keys() ([]string, error) {
	keys := make([]string, 0, len(s.tokens))
	for k := range s.tokens {
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *mockSecretStore) GetDefaultAccount(string) (string, error) {
	return "", errors.New("not implemented")
}

func (s *mockSecretStore) SetDefaultAccount(string, string) error {
	return errors.New("not implemented")
}

func (s *mockSecretStore) Close() error { return nil }

func TestReauthSuccess(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	store := &mockSecretStore{tokens: make(map[string]secrets.Token)}

	authorizeCalled := false
	identityCalled := false

	opts := ReauthOptions{
		Email:    "user@example.com",
		Client:   "default",
		Services: []string{"calendar"},
		Scopes:   []string{"https://www.googleapis.com/auth/calendar"},
		OpenSecretsStore: func() (secrets.Store, error) {
			return store, nil
		},
		EnsureKeychainAccess: func(context.Context) error { return nil },
		AuthorizeFunc: func(ctx context.Context, authOpts AuthorizeOptions) (string, error) {
			authorizeCalled = true
			if authOpts.ForceConsent != true {
				t.Fatalf("expected ForceConsent=true")
			}
			if authOpts.Client != "default" {
				t.Fatalf("expected client 'default', got %q", authOpts.Client)
			}
			return "new-refresh-token", nil
		},
		FetchIdentityFunc: func(ctx context.Context, client string, refreshToken string, scopes []string, timeout time.Duration) (Identity, error) {
			identityCalled = true
			if refreshToken != "new-refresh-token" {
				t.Fatalf("expected new-refresh-token, got %q", refreshToken)
			}
			return Identity{Subject: "sub123", Email: "user@example.com"}, nil
		},
		Stderr: &bytesBuffer{},
	}

	if _, err := Reauth(context.Background(), opts); err != nil {
		t.Fatalf("Reauth: %v", err)
	}

	if !authorizeCalled {
		t.Fatal("AuthorizeFunc was not called")
	}

	if !identityCalled {
		t.Fatal("FetchIdentityFunc was not called")
	}

	// Verify token was persisted
	tok, err := store.GetToken("default", "user@example.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	if tok.RefreshToken != "new-refresh-token" {
		t.Fatalf("expected new-refresh-token, got %q", tok.RefreshToken)
	}

	if tok.Email != "user@example.com" {
		t.Fatalf("expected user@example.com, got %q", tok.Email)
	}

	if tok.Subject != "sub123" {
		t.Fatalf("expected sub123, got %q", tok.Subject)
	}
}

func TestReauthAuthorizeFailure(t *testing.T) {
	opts := ReauthOptions{
		Email:    "user@example.com",
		Client:   "default",
		Services: []string{"calendar"},
		Scopes:   []string{"https://www.googleapis.com/auth/calendar"},
		OpenSecretsStore: func() (secrets.Store, error) {
			return &mockSecretStore{tokens: make(map[string]secrets.Token)}, nil
		},
		EnsureKeychainAccess: func(context.Context) error { return nil },
		AuthorizeFunc: func(ctx context.Context, authOpts AuthorizeOptions) (string, error) {
			return "", errors.New("user denied access")
		},
		FetchIdentityFunc: func(context.Context, string, string, []string, time.Duration) (Identity, error) {
			t.Fatal("FetchIdentityFunc should not be called on authorize failure")
			return Identity{}, nil
		},
		Stderr: &bytesBuffer{},
	}

	_, err := Reauth(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "authorization failed") {
		t.Fatalf("error should mention authorization failed: %v", err)
	}
}

func TestReauthMissingEmail(t *testing.T) {
	opts := ReauthOptions{
		Client:   "default",
		Scopes:   []string{"scope"},
		OpenSecretsStore: func() (secrets.Store, error) {
			return &mockSecretStore{}, nil
		},
	}

	_, err := Reauth(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing email")
	}
}

func TestReauthMissingScopes(t *testing.T) {
	opts := ReauthOptions{
		Email:  "user@example.com",
		Client: "default",
		OpenSecretsStore: func() (secrets.Store, error) {
			return &mockSecretStore{}, nil
		},
	}

	_, err := Reauth(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing scopes")
	}
}

func TestReauthKeychainAccessFailure(t *testing.T) {
	opts := ReauthOptions{
		Email:    "user@example.com",
		Client:   "default",
		Scopes:   []string{"scope"},
		OpenSecretsStore: func() (secrets.Store, error) {
			return &mockSecretStore{}, nil
		},
		EnsureKeychainAccess: func(context.Context) error {
			return errors.New("keychain locked")
		},
	}

	_, err := Reauth(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "keychain") {
		t.Fatalf("error should mention keychain: %v", err)
	}
}

func TestReauthEmailMismatch(t *testing.T) {
	opts := ReauthOptions{
		Email:    "user@example.com",
		Client:   "default",
		Services: []string{"calendar"},
		Scopes:   []string{"https://www.googleapis.com/auth/calendar"},
		OpenSecretsStore: func() (secrets.Store, error) {
			return &mockSecretStore{tokens: make(map[string]secrets.Token)}, nil
		},
		EnsureKeychainAccess: func(context.Context) error { return nil },
		AuthorizeFunc: func(ctx context.Context, authOpts AuthorizeOptions) (string, error) {
			return "new-refresh-token", nil
		},
		FetchIdentityFunc: func(context.Context, string, string, []string, time.Duration) (Identity, error) {
			// Return a *different* email than requested
			return Identity{Subject: "sub456", Email: "other@example.com"}, nil
		},
		Stderr: &bytesBuffer{},
	}

	_, err := Reauth(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for email mismatch")
	}

	if !strings.Contains(err.Error(), "authorized as other@example.com") {
		t.Fatalf("error should mention email mismatch: %v", err)
	}
}

func TestReauthPreservesStoredScopes(t *testing.T) {
	store := &mockSecretStore{tokens: map[string]secrets.Token{
		"default:user@example.com": {
			Scopes: []string{
				"https://www.googleapis.com/auth/calendar",
				"https://www.googleapis.com/auth/gmail.modify",
				"https://www.googleapis.com/auth/drive",
			},
			Services: []string{"calendar", "gmail", "drive"},
		},
	}}

	var requestedScopes []string
	var requestedServices []string

	opts := ReauthOptions{
		Email:    "user@example.com",
		Client:   "default",
		Services: []string{"calendar"}, // narrowed — only the triggering request's service
		Scopes:   []string{"https://www.googleapis.com/auth/calendar"}, // narrowed
		StoredToken: &secrets.Token{
			Scopes: []string{
				"https://www.googleapis.com/auth/calendar",
				"https://www.googleapis.com/auth/gmail.modify",
				"https://www.googleapis.com/auth/drive",
			},
			Services: []string{"calendar", "gmail", "drive"},
		},
		OpenSecretsStore: func() (secrets.Store, error) {
			return store, nil
		},
		EnsureKeychainAccess: func(context.Context) error { return nil },
		AuthorizeFunc: func(ctx context.Context, authOpts AuthorizeOptions) (string, error) {
			requestedScopes = authOpts.Scopes
			requestedServices = make([]string, len(authOpts.Services))
			for i, svc := range authOpts.Services {
				requestedServices[i] = string(svc)
			}
			return "new-refresh-token", nil
		},
		FetchIdentityFunc: func(context.Context, string, string, []string, time.Duration) (Identity, error) {
			return Identity{Subject: "sub123", Email: "user@example.com"}, nil
		},
		Stderr: &bytesBuffer{},
	}

	if _, err := Reauth(context.Background(), opts); err != nil {
		t.Fatalf("Reauth: %v", err)
	}

	// Should request the stored token's full scopes, not the narrowed set
	if len(requestedScopes) != 3 {
		t.Fatalf("expected 3 scopes (preserved from stored token), got %d: %v", len(requestedScopes), requestedScopes)
	}

	// Should request the stored token's full services
	if len(requestedServices) != 3 {
		t.Fatalf("expected 3 services (preserved from stored token), got %d: %v", len(requestedServices), requestedServices)
	}

	// Verify persisted token has the full scope set
	tok, err := store.GetToken("default", "user@example.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if len(tok.Scopes) != 3 {
		t.Fatalf("persisted token should have 3 scopes, got %d: %v", len(tok.Scopes), tok.Scopes)
	}
}

func TestServicesFromScopes(t *testing.T) {
	// Calendar scope should match calendar service
	services := servicesFromScopes([]string{"https://www.googleapis.com/auth/calendar"})
	if len(services) != 1 || services[0] != ServiceCalendar {
		t.Fatalf("expected [calendar], got %v", services)
	}

	// Gmail scopes should match gmail service
	services = servicesFromScopes([]string{
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/gmail.settings.basic",
		"https://www.googleapis.com/auth/gmail.settings.sharing",
	})
	if len(services) != 1 || services[0] != ServiceGmail {
		t.Fatalf("expected [gmail], got %v", services)
	}

	// Unknown scopes should return empty
	services = servicesFromScopes([]string{"https://unknown.example.com/scope"})
	if len(services) != 0 {
		t.Fatalf("expected empty, got %v", services)
	}
}

// bytesBuffer is a minimal io.Writer for capturing stderr output in tests.
type bytesBuffer struct {
	data []byte
}

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *bytesBuffer) String() string {
	return string(b.data)
}
