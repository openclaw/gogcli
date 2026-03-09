package googleapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/config"
)

func TestServiceAccountSubject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		subject             string
		serviceAccountEmail string
		want                string
	}{
		{
			name:                "empty subject stays empty",
			subject:             "",
			serviceAccountEmail: "sa@test-project.iam.gserviceaccount.com",
			want:                "",
		},
		{
			name:                "same subject becomes pure service account mode",
			subject:             "sa@test-project.iam.gserviceaccount.com",
			serviceAccountEmail: "sa@test-project.iam.gserviceaccount.com",
			want:                "",
		},
		{
			name:                "different subject keeps impersonation target",
			subject:             "user@example.com",
			serviceAccountEmail: "sa@test-project.iam.gserviceaccount.com",
			want:                "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := serviceAccountSubject(tt.subject, tt.serviceAccountEmail)
			if got != tt.want {
				t.Fatalf("serviceAccountSubject(%q, %q) = %q, want %q", tt.subject, tt.serviceAccountEmail, got, tt.want)
			}
		})
	}
}

func TestTokenSourceForServiceAccountScopes_NonKeepIgnoresKeepFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	keepSAPath, err := config.KeepServiceAccountPath("a@b.com")
	if err != nil {
		t.Fatalf("KeepServiceAccountPath: %v", err)
	}

	if _, ensureErr := config.EnsureDir(); ensureErr != nil {
		t.Fatalf("EnsureDir: %v", ensureErr)
	}

	if writeErr := os.WriteFile(keepSAPath, []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write keep sa: %v", writeErr)
	}

	origSA := newServiceAccountTokenSource

	t.Cleanup(func() { newServiceAccountTokenSource = origSA })

	called := false
	newServiceAccountTokenSource = func(context.Context, []byte, string, []string) (oauth2.TokenSource, error) {
		called = true
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}), nil
	}

	ts, path, ok, err := tokenSourceForServiceAccountScopes(context.Background(), "gmail", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if ok {
		t.Fatalf("expected keep-only fallback to be ignored, got ok=true path=%q ts=%v", path, ts)
	}

	if called {
		t.Fatalf("expected keep-only fallback not to initialize a token source")
	}
}

func TestTokenSourceForServiceAccountScopes_KeepUsesKeepFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	keepSAPath, err := config.KeepServiceAccountPath("a@b.com")
	if err != nil {
		t.Fatalf("KeepServiceAccountPath: %v", err)
	}

	if _, ensureErr := config.EnsureDir(); ensureErr != nil {
		t.Fatalf("EnsureDir: %v", ensureErr)
	}

	if writeErr := os.WriteFile(keepSAPath, []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write keep sa: %v", writeErr)
	}

	origSA := newServiceAccountTokenSource

	t.Cleanup(func() { newServiceAccountTokenSource = origSA })

	called := false
	newServiceAccountTokenSource = func(_ context.Context, keyJSON []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
		called = true

		if subject != "a@b.com" {
			t.Fatalf("unexpected subject: %q", subject)
		}

		if len(scopes) != 1 || scopes[0] != "s1" {
			t.Fatalf("unexpected scopes: %#v", scopes)
		}

		if string(keyJSON) == "" {
			t.Fatalf("expected key JSON")
		}

		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}), nil
	}

	ts, path, ok, err := tokenSourceForServiceAccountScopes(context.Background(), "keep", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !ok || ts == nil {
		t.Fatalf("expected keep fallback token source, got ok=%v ts=%v", ok, ts)
	}

	if path != keepSAPath {
		t.Fatalf("unexpected keep fallback path: %q", path)
	}

	if !called {
		t.Fatalf("expected keep fallback token source initialization")
	}
}
