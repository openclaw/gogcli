package cmd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestExecute_AuthAdd_JSON(t *testing.T) {
	origOpen := openSecretsStore
	origAuth := authorizeGoogle
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedIdentity
	t.Cleanup(func() {
		openSecretsStore = origOpen
		authorizeGoogle = origAuth
		ensureKeychainAccess = origKeychain
		fetchAuthorizedIdentity = origFetch
	})

	ensureKeychainAccess = func() error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(_ context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service{}, opts.Services...)
		gotOpts.Scopes = append([]string{}, opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "a@b.com"}, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "add", "a@b.com", "--services", "calendar,gmail"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Stored   bool     `json:"stored"`
		Email    string   `json:"email"`
		Services []string `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Stored || parsed.Email != "a@b.com" || len(parsed.Services) != 2 {
		t.Fatalf("unexpected: %#v", parsed)
	}

	tok, err := store.GetToken(config.DefaultClientName, "a@b.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.RefreshToken != "rt" {
		t.Fatalf("unexpected token: %#v", tok)
	}

	_ = gotOpts // keep for future assertions; ensures auth add actually called authorizeGoogle.
}
