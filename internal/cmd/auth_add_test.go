package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestAuthAddCmd_JSON(t *testing.T) {
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
	})

	ensureKeychainAccess = func() error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"gmail,drive,gmail",
				"--manual",
				"--force-consent",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !gotOpts.Manual || !gotOpts.ForceConsent {
		t.Fatalf("expected options set, got %+v", gotOpts)
	}
	if len(gotOpts.Services) != 2 {
		t.Fatalf("expected deduped services, got %v", gotOpts.Services)
	}

	var parsed struct {
		Stored   bool     `json:"stored"`
		Email    string   `json:"email"`
		Services []string `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Stored || parsed.Email != "user@example.com" || len(parsed.Services) != 2 {
		t.Fatalf("unexpected response: %#v", parsed)
	}
	tok, err := store.GetToken("user@example.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.RefreshToken != "rt" || !strings.Contains(strings.Join(tok.Services, ","), "gmail") {
		t.Fatalf("unexpected token: %#v", tok)
	}
}

func TestAuthAddCmd_KeychainError(t *testing.T) {
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
	})

	// Simulate keychain locked error
	ensureKeychainAccess = func() error {
		return errors.New("keychain is locked")
	}

	authCalled := false
	authorizeGoogle = func(_ context.Context, _ googleauth.AuthorizeOptions) (string, error) {
		authCalled = true
		return "rt", nil
	}

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	cmd := &AuthAddCmd{Email: "test@example.com", ServicesCSV: "gmail"}
	err := cmd.Run(context.Background())

	if err == nil {
		t.Fatal("expected error when keychain is locked")
	}
	if !strings.Contains(err.Error(), "keychain") {
		t.Errorf("expected error to mention keychain, got: %v", err)
	}
	if authCalled {
		t.Error("authorizeGoogle should not be called when keychain check fails")
	}
}
