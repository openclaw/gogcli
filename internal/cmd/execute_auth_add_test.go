package cmd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleauth"
	"github.com/openclaw/gogcli/internal/secrets"
)

func TestExecute_AuthAdd_JSON(t *testing.T) {
	store := newMemSecretsStore()

	var gotOpts googleauth.AuthorizeOptions
	runtime := &app.Runtime{Auth: app.AuthOperations{
		OpenSecretsStore:     func() (secrets.Store, error) { return store, nil },
		EnsureKeychainAccess: func(context.Context) error { return nil },
		AuthorizeGoogle: func(_ context.Context, opts googleauth.AuthorizeOptions) (string, error) {
			gotOpts = opts
			gotOpts.Services = append([]googleauth.Service{}, opts.Services...)
			gotOpts.Scopes = append([]string{}, opts.Scopes...)
			return "rt", nil
		},
		FetchAuthorizedIdentity: func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
			return googleauth.Identity{Email: "a@b.com"}, nil
		},
	}}

	result := executeWithTestRuntime(t, []string{
		"--json", "auth", "add", "a@b.com", "--services", "calendar,gmail",
	}, runtime)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Stored   bool     `json:"stored"`
		Email    string   `json:"email"`
		Services []string `json:"services"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
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
