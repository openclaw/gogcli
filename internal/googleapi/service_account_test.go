package googleapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/config"
)

func TestTokenSourceForServiceAccountScopesRequiresStore(t *testing.T) {
	t.Parallel()

	_, _, _, err := tokenSourceForServiceAccountScopes(context.Background(), AuthDependencies{}, "gmail", "a@b.com", []string{"scope"})
	if !errors.Is(err, errServiceAccountStoreRequired) {
		t.Fatalf("error = %v, want %v", err, errServiceAccountStoreRequired)
	}
}

func TestTokenSourceForServiceAccountScopesPropagatesResolverError(t *testing.T) {
	t.Parallel()

	want := errBoom
	dependencies := AuthDependencies{
		ServiceAccounts: func() (*config.ServiceAccountStore, error) {
			return nil, want
		},
	}

	_, _, _, err := tokenSourceForServiceAccountScopes(context.Background(), dependencies, "gmail", "a@b.com", []string{"scope"})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}

func TestTokenSourceForServiceAccountScopesUsesInjectedStore(t *testing.T) {
	ambientHome := t.TempDir()
	t.Setenv("HOME", ambientHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(ambientHome, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(ambientHome, "xdg-data"))

	ambientLayout, err := config.NewSystemResolver("").Resolve(config.PathKindData)
	if err != nil {
		t.Fatalf("resolve ambient layout: %v", err)
	}
	ambientPath := ambientLayout.ServiceAccountPath("a@b.com")

	if ensureErr := os.MkdirAll(ambientLayout.DataDir, 0o700); ensureErr != nil {
		t.Fatalf("ensure data dir: %v", ensureErr)
	}

	if writeErr := os.WriteFile(ambientPath, []byte("ambient"), 0o600); writeErr != nil {
		t.Fatalf("write ambient service account: %v", writeErr)
	}

	ctx, injected := testServiceAccountContext(t, context.Background())
	dependencies, _ := authDependenciesFromContext(ctx)

	injectedPath, err := injected.Write("a@b.com", []byte("injected"))
	if err != nil {
		t.Fatalf("write injected service account: %v", err)
	}

	var gotData string
	dependencies.ServiceAccountTokenSource = func(_ context.Context, data []byte, _ string, _ []string) (oauth2.TokenSource, error) {
		gotData = string(data)
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"}), nil
	}

	_, path, ok, err := tokenSourceForServiceAccountScopes(ctx, dependencies, "gmail", "a@b.com", []string{"scope"})
	if err != nil {
		t.Fatalf("tokenSourceForServiceAccountScopes: %v", err)
	}

	if !ok || path != injectedPath || gotData != "injected" {
		t.Fatalf("ok=%t path=%q data=%q, want injected path=%q", ok, path, gotData, injectedPath)
	}
}

func TestTokenSourceForServiceAccountScopesRequiresTokenSourceFactory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	store := config.NewServiceAccountStore(config.Layout{
		ConfigDir:      filepath.Join(root, "config"),
		DataDir:        filepath.Join(root, "data"),
		ExplicitConfig: true,
		ExplicitData:   true,
	})
	if _, err := store.Write("a@b.com", []byte(`{"type":"service_account"}`)); err != nil {
		t.Fatalf("write service account: %v", err)
	}
	dependencies := AuthDependencies{
		ServiceAccounts: func() (*config.ServiceAccountStore, error) {
			return store, nil
		},
	}

	_, _, _, err := tokenSourceForServiceAccountScopes(context.Background(), dependencies, "gmail", "a@b.com", []string{"scope"})
	if !errors.Is(err, errServiceAccountTokenSourceRequired) {
		t.Fatalf("error = %v, want %v", err, errServiceAccountTokenSourceRequired)
	}
}

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
			name:                "same subject ignores case and whitespace",
			subject:             " SA@Test-Project.iam.gserviceaccount.com ",
			serviceAccountEmail: "sa@test-project.iam.gserviceaccount.com",
			want:                "",
		},
		{
			name:                "different subject keeps impersonation target",
			subject:             " user@example.com ",
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
	ctx, serviceAccounts := testServiceAccountContext(t, context.Background())
	dependencies, _ := authDependenciesFromContext(ctx)

	if _, err := serviceAccounts.WriteKeep("a@b.com", []byte(`{"type":"service_account"}`)); err != nil {
		t.Fatalf("write Keep service account: %v", err)
	}

	called := false
	dependencies.ServiceAccountTokenSource = func(context.Context, []byte, string, []string) (oauth2.TokenSource, error) {
		called = true
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}), nil
	}

	ts, path, ok, err := tokenSourceForServiceAccountScopes(ctx, dependencies, "gmail", "a@b.com", []string{"s1"})
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
	ctx, serviceAccounts := testServiceAccountContext(t, context.Background())
	dependencies, _ := authDependenciesFromContext(ctx)

	keepSAPath, err := serviceAccounts.WriteKeep("a@b.com", []byte(`{"type":"service_account"}`))
	if err != nil {
		t.Fatalf("write Keep service account: %v", err)
	}

	called := false
	dependencies.ServiceAccountTokenSource = func(_ context.Context, keyJSON []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
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

	ts, path, ok, err := tokenSourceForServiceAccountScopes(ctx, dependencies, "keep", "a@b.com", []string{"s1"})
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

func TestTokenSourceForServiceAccountScopes_ExplicitDataSkipsRawKeepLegacyFallback(t *testing.T) {
	root := t.TempDir()
	layout := config.Layout{
		ConfigDir:      filepath.Join(root, "config"),
		DataDir:        filepath.Join(root, "isolated-data"),
		ExplicitConfig: true,
		ExplicitData:   true,
	}
	serviceAccounts := config.NewServiceAccountStore(layout)
	dependencies := AuthDependencies{
		ServiceAccounts: func() (*config.ServiceAccountStore, error) {
			return serviceAccounts, nil
		},
	}
	legacyPath := layout.KeepServiceAccountLegacyPath("a@b.com")

	if mkdirErr := os.MkdirAll(filepath.Dir(legacyPath), 0o700); mkdirErr != nil {
		t.Fatalf("mkdir legacy keep sa: %v", mkdirErr)
	}

	if writeErr := os.WriteFile(legacyPath, []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write legacy keep sa: %v", writeErr)
	}

	dependencies.ServiceAccountTokenSource = func(context.Context, []byte, string, []string) (oauth2.TokenSource, error) {
		t.Fatal("legacy keep service account should not initialize with explicit data dir")
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "unexpected"}), nil
	}

	ts, path, ok, err := tokenSourceForServiceAccountScopes(context.Background(), dependencies, "keep", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if ok || ts != nil || path != "" {
		t.Fatalf("expected no token source, got ok=%v ts=%v path=%q", ok, ts, path)
	}
}
