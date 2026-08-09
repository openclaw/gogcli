package googleapi

import (
	"context"
	"errors"
	"testing"

	"github.com/openclaw/gogcli/internal/config"
)

func TestStoredAuthRequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := optionsForAccountScopes(context.Background(), "drive", "a@b.com", []string{"scope"})
	if !errors.Is(err, errAuthDependenciesRequired) {
		t.Fatalf("error = %v, want %v", err, errAuthDependenciesRequired)
	}
}

func TestADCRequiresTokenSourceFactory(t *testing.T) {
	t.Parallel()

	ctx := WithAuthDependencies(context.Background(), AuthDependencies{Mode: AuthModeADC})

	_, err := optionsForAccountScopes(ctx, "drive", "adc", []string{"scope"})
	if !errors.Is(err, errADCTokenSourceRequired) {
		t.Fatalf("error = %v, want %v", err, errADCTokenSourceRequired)
	}
}

func TestClientCredentialsRequireExplicitDependencies(t *testing.T) {
	t.Parallel()

	_, _, err := clientCredentialsForAccount(context.Background(), AuthDependencies{}, "a@b.com")
	if !errors.Is(err, errAuthClientResolverRequired) {
		t.Fatalf("resolver error = %v, want %v", err, errAuthClientResolverRequired)
	}

	dependencies := AuthDependencies{
		ResolveClient: func(string, string) (string, error) {
			return config.DefaultClientName, nil
		},
	}

	_, _, err = clientCredentialsForAccount(context.Background(), dependencies, "a@b.com")
	if !errors.Is(err, errAuthCredentialsReaderRequired) {
		t.Fatalf("credentials error = %v, want %v", err, errAuthCredentialsReaderRequired)
	}
}

func TestTokenSourceRequiresExplicitTokenStore(t *testing.T) {
	t.Parallel()

	_, err := tokenSourceForAccountScopesWithStoredScopeCheck(
		context.Background(),
		AuthDependencies{},
		"drive",
		"a@b.com",
		config.DefaultClientName,
		"id",
		"secret",
		[]string{"scope"},
		false,
	)
	if !errors.Is(err, errAuthTokenStoreOpenerRequired) {
		t.Fatalf("error = %v, want %v", err, errAuthTokenStoreOpenerRequired)
	}
}

func TestEmailReferenceUpdateRequiresDependency(t *testing.T) {
	t.Parallel()

	err := (AuthDependencies{}).updateEmailReferences("old@example.com", "new@example.com")
	if !errors.Is(err, errEmailReferenceUpdaterRequired) {
		t.Fatalf("error = %v, want %v", err, errEmailReferenceUpdaterRequired)
	}
}
