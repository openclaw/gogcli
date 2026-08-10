package googleauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/secrets"
)

var (
	errReauthTestUserDenied     = errors.New("user denied access")
	errReauthTestKeychainLocked = errors.New("keychain locked")
)

func TestReauthSuccess(t *testing.T) {
	authorizeCalled := false
	identityCalled := false

	opts := ReauthOptions{
		Email:                "user@example.com",
		Client:               "default",
		Services:             []string{"calendar"},
		Scopes:               []string{"https://www.googleapis.com/auth/calendar"},
		EnsureKeychainAccess: func(context.Context) error { return nil },
		Confirm:              func(context.Context, string) (bool, error) { return true, nil },
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

	tok, err := Reauth(context.Background(), opts)
	if err != nil {
		t.Fatalf("Reauth: %v", err)
	}

	if !authorizeCalled {
		t.Fatal("AuthorizeFunc was not called")
	}

	if !identityCalled {
		t.Fatal("FetchIdentityFunc was not called")
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
		Email:                "user@example.com",
		Client:               "default",
		Services:             []string{"calendar"},
		Scopes:               []string{"https://www.googleapis.com/auth/calendar"},
		EnsureKeychainAccess: func(context.Context) error { return nil },
		Confirm:              func(context.Context, string) (bool, error) { return true, nil },
		AuthorizeFunc: func(ctx context.Context, authOpts AuthorizeOptions) (string, error) {
			return "", errReauthTestUserDenied
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

func TestReauthRequiresConfirmation(t *testing.T) {
	opts := ReauthOptions{
		Email:  "user@example.com",
		Client: "default",
		Scopes: []string{"scope"},
	}
	if _, err := Reauth(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "confirmation callback is required") {
		t.Fatalf("Reauth() error = %v, want missing confirmation error", err)
	}

	authorized := false
	opts.Confirm = func(context.Context, string) (bool, error) { return false, nil }

	opts.AuthorizeFunc = func(context.Context, AuthorizeOptions) (string, error) {
		authorized = true
		return "token", nil
	}
	if _, err := Reauth(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("Reauth() error = %v, want cancellation", err)
	}

	if authorized {
		t.Fatal("authorization started after confirmation was declined")
	}
}

func TestReauthMissingEmail(t *testing.T) {
	opts := ReauthOptions{
		Client: "default",
		Scopes: []string{"scope"},
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
	}

	_, err := Reauth(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing scopes")
	}
}

func TestReauthKeychainAccessFailure(t *testing.T) {
	opts := ReauthOptions{
		Email:   "user@example.com",
		Client:  "default",
		Scopes:  []string{"scope"},
		Confirm: func(context.Context, string) (bool, error) { return true, nil },
		EnsureKeychainAccess: func(context.Context) error {
			return errReauthTestKeychainLocked
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
		Email:                "user@example.com",
		Client:               "default",
		Services:             []string{"calendar"},
		Scopes:               []string{"https://www.googleapis.com/auth/calendar"},
		EnsureKeychainAccess: func(context.Context) error { return nil },
		Confirm:              func(context.Context, string) (bool, error) { return true, nil },
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

func TestReauthRejectsIdentityWithoutEmail(t *testing.T) {
	opts := ReauthOptions{
		Email:    "user@example.com",
		Client:   "default",
		Services: []string{"calendar"},
		Scopes:   []string{"https://www.googleapis.com/auth/calendar"},
		Confirm:  func(context.Context, string) (bool, error) { return true, nil },
		AuthorizeFunc: func(context.Context, AuthorizeOptions) (string, error) {
			return "new-refresh-token", nil
		},
		FetchIdentityFunc: func(context.Context, string, string, []string, time.Duration) (Identity, error) {
			return Identity{Subject: "sub123"}, nil
		},
		Stderr: &bytesBuffer{},
	}

	if _, err := Reauth(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "did not include an email") {
		t.Fatalf("Reauth() error = %v, want missing email error", err)
	}
}

func TestReauthPreservesStoredScopes(t *testing.T) {
	var requestedScopes []string
	var requestedServices []string

	opts := ReauthOptions{
		Email:    "user@example.com",
		Client:   "default",
		Services: []string{"calendar"},                                 // narrowed — only the triggering request's service
		Scopes:   []string{"https://www.googleapis.com/auth/calendar"}, // narrowed
		StoredToken: &secrets.Token{
			Scopes: []string{
				"https://www.googleapis.com/auth/calendar",
				"https://www.googleapis.com/auth/gmail.modify",
				"https://www.googleapis.com/auth/drive",
			},
			Services: []string{"calendar", "gmail", "drive"},
		},
		EnsureKeychainAccess: func(context.Context) error { return nil },
		Confirm:              func(context.Context, string) (bool, error) { return true, nil },
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

	tok, err := Reauth(context.Background(), opts)
	if err != nil {
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

	if len(tok.Scopes) != 3 {
		t.Fatalf("returned token should have 3 scopes, got %d: %v", len(tok.Scopes), tok.Scopes)
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
