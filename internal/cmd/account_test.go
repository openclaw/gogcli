package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

type fakeSecretsStore struct {
	defaultAccount string
	tokens         []secrets.Token
	errDefault     error
	errListTokens  error
}

func (s *fakeSecretsStore) Keys() ([]string, error) { return nil, errors.New("not implemented") }
func (s *fakeSecretsStore) SetToken(string, string, secrets.Token) error {
	return errors.New("not implemented")
}

func (s *fakeSecretsStore) GetToken(string, string) (secrets.Token, error) {
	return secrets.Token{}, errors.New("not implemented")
}
func (s *fakeSecretsStore) DeleteToken(string, string) error { return errors.New("not implemented") }
func (s *fakeSecretsStore) SetDefaultAccount(string, string) error {
	return errors.New("not implemented")
}

func (s *fakeSecretsStore) GetDefaultAccount(string) (string, error) {
	return s.defaultAccount, s.errDefault
}
func (s *fakeSecretsStore) ListTokens() ([]secrets.Token, error) { return s.tokens, s.errListTokens }

func TestRequireAccount_PrefersFlag(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "env@example.com")
	flags := &RootFlags{Account: "flag@example.com"}
	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "flag@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_UsesEnv(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "env@example.com")
	flags := &RootFlags{}
	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "env@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_ResolvesAliasFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	if err := config.WriteConfig(config.File{
		AccountAliases: map[string]string{"work": "alias@example.com"},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	flags := &RootFlags{Account: "work"}
	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "alias@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_ResolvesAliasEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	if err := config.WriteConfig(config.File{
		AccountAliases: map[string]string{"work": "alias@example.com"},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("GOG_ACCOUNT", "work")
	flags := &RootFlags{}
	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "alias@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_AutoUsesDefault(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	flags := rootFlagsWithAuthStore(
		&RootFlags{Account: "auto"},
		&fakeSecretsStore{defaultAccount: "default@example.com"},
	)

	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "default@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_ExplicitAutoIgnoresEnv(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "env@example.com")
	flags := rootFlagsWithAuthStore(
		&RootFlags{Account: "auto"},
		&fakeSecretsStore{defaultAccount: "default@example.com"},
	)

	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "default@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_ADCNilFlags(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")
	t.Setenv("GOG_ACCOUNT", "")

	got, err := requireAccount(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "adc" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_ADCEnvAutoUsesPlaceholder(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")

	for _, value := range []string{"auto", "default"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("GOG_ACCOUNT", value)

			got, err := requireAccount(nil)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != adcPlaceholderAccount {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestRequireAccount_Missing(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	flags := &RootFlags{}
	_, err := requireAccount(flags)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRequireAccount_UsesKeyringDefaultAccount(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	flags := rootFlagsWithAuthStore(nil, &fakeSecretsStore{defaultAccount: "default@example.com"})

	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "default@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_UsesSingleStoredToken(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	flags := rootFlagsWithAuthStore(
		nil,
		&fakeSecretsStore{
			tokens: []secrets.Token{{Email: "one@example.com", Client: config.DefaultClientName}},
		},
	)

	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "one@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireAccount_MissingWhenMultipleTokensAndNoDefault(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	flags := rootFlagsWithAuthStore(
		nil,
		&fakeSecretsStore{
			tokens: []secrets.Token{{Email: "a@example.com", Client: config.DefaultClientName}, {Email: "b@example.com", Client: config.DefaultClientName}},
		},
	)

	_, err := requireAccount(flags)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRequireAccount_AccessTokenNoAccount(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	var diagnostics strings.Builder
	flags := &RootFlags{AccessToken: "ya29.some-token", diagnostics: &diagnostics}
	flags.authOperations = app.AuthOperations{
		OpenSecretsStore: func() (secrets.Store, error) {
			t.Fatal("OpenSecretsStore should not be called when access token is provided")
			return nil, errors.New("unreachable")
		},
	}

	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != accessTokenPlaceholderAccount {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(diagnostics.String(), directAccessTokenWarning) {
		t.Fatalf("expected warning, got %q", diagnostics.String())
	}
}

func TestRequireAccount_AccessTokenWithExplicitAccount(t *testing.T) {
	var diagnostics strings.Builder
	flags := &RootFlags{
		AccessToken: "ya29.some-token",
		Account:     "explicit@example.com",
		diagnostics: &diagnostics,
	}

	got, err := requireAccount(flags)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "explicit@example.com" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(diagnostics.String(), directAccessTokenWarning) {
		t.Fatalf("expected warning, got %q", diagnostics.String())
	}
}

func TestExecuteAccountAliasUsesRuntimeConfigStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ambientStore := defaultConfigStoreForTest(t)
	if err := ambientStore.SetAccountAlias("work", "ambient@example.com"); err != nil {
		t.Fatalf("set ambient alias: %v", err)
	}
	runtimeStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := runtimeStore.SetAccountAlias("work", "runtime@example.com"); err != nil {
		t.Fatalf("set runtime alias: %v", err)
	}

	wantErr := errors.New("stop after account resolution")
	var gotAccount string
	result := executeWithTestRuntime(t, []string{
		"--account", "work",
		"calendar", "time",
	}, &app.Runtime{
		Config: runtimeStore,
		Services: app.Services{
			Calendar: func(_ context.Context, account string) (*calendar.Service, error) {
				gotAccount = account
				return nil, wantErr
			},
		},
	})
	if !errors.Is(result.err, wantErr) {
		t.Fatalf("error = %v, want %v", result.err, wantErr)
	}
	if gotAccount != "runtime@example.com" {
		t.Fatalf("account = %q, want runtime alias target", gotAccount)
	}
}
