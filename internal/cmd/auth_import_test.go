package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
)

func newImportTestContext(t *testing.T, store secrets.Store) context.Context {
	t.Helper()
	return newImportTestContextWithInput(t, strings.NewReader(""), store)
}

func newImportTestContextWithInput(t *testing.T, stdin io.Reader, store secrets.Store) context.Context {
	t.Helper()
	ctx := outfmt.WithMode(newCmdRuntimeIOContext(t, stdin, io.Discard, io.Discard), outfmt.Mode{})
	return withAuthOperations(ctx, app.AuthOperations{
		OpenSecretsStore:     func() (secrets.Store, error) { return store, nil },
		EnsureKeychainAccess: func(context.Context) error { return nil },
	})
}

func authImportCmdWithEnvToken(t *testing.T, email string, token string) *AuthImportCmd {
	t.Helper()
	t.Setenv("GOG_TEST_REFRESH_TOKEN", token)
	return &AuthImportCmd{
		Email:           email,
		RefreshTokenEnv: "GOG_TEST_REFRESH_TOKEN",
	}
}

func TestAuthImportCmd_RejectsEmptyRefreshToken(t *testing.T) {
	store := newMemSecretsStore()

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "   ")
	cmd.ServicesCSV = "gmail"
	err := cmd.Run(newImportTestContext(t, store), &RootFlags{})
	if err == nil {
		t.Fatal("expected error for empty refresh token")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected ExitError code=2, got %#v", err)
	}
	if !strings.Contains(err.Error(), "refresh token") {
		t.Fatalf("expected refresh token in error, got %q", err.Error())
	}
}

func TestAuthImportCmd_RejectsMissingRefreshTokenSource(t *testing.T) {
	store := newMemSecretsStore()

	cmd := &AuthImportCmd{
		Email: "a@b.com",
	}
	err := cmd.Run(newImportTestContext(t, store), &RootFlags{})
	if err == nil {
		t.Fatal("expected error for missing refresh token source")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected ExitError code=2, got %#v", err)
	}
	if !strings.Contains(err.Error(), "--refresh-token-stdin") {
		t.Fatalf("expected safe source hint in error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "required refresh token") || !strings.Contains(err.Error(), "cannot replace it") {
		t.Fatalf("expected refresh/access dependency guidance, got %q", err.Error())
	}
}

func TestAuthImportCmd_RejectsMultipleRefreshTokenSources(t *testing.T) {
	store := newMemSecretsStore()

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt")
	cmd.RefreshTokenStdin = true
	err := cmd.Run(newImportTestContext(t, store), &RootFlags{})
	if err == nil {
		t.Fatal("expected error for multiple refresh token sources")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected ExitError code=2, got %#v", err)
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected exactly-one source error, got %q", err.Error())
	}
}

func TestAuthImportCmd_RejectsEmptyEmail(t *testing.T) {
	store := newMemSecretsStore()

	cmd := authImportCmdWithEnvToken(t, "   ", "rt")
	err := cmd.Run(newImportTestContext(t, store), &RootFlags{})
	if err == nil {
		t.Fatal("expected error for empty email")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected ExitError code=2, got %#v", err)
	}
}

func TestAuthImportCmd_DefaultsClientToDefault(t *testing.T) {
	store := newMemSecretsStore()
	t.Setenv("GOG_TEST_REFRESH_TOKEN", "rt-1\n")

	cmd := &AuthImportCmd{
		Email:           "A@B.com",
		RefreshTokenEnv: "GOG_TEST_REFRESH_TOKEN",
		ServicesCSV:     "gmail, drive",
	}
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tok, err := store.GetToken(config.DefaultClientName, "a@b.com")
	if err != nil {
		t.Fatalf("GetToken default client: %v", err)
	}
	if tok.RefreshToken != "rt-1" {
		t.Fatalf("unexpected refresh token: %q", tok.RefreshToken)
	}
	if tok.Email != "a@b.com" {
		t.Fatalf("unexpected email: %q", tok.Email)
	}
	if strings.Join(tok.Services, ",") != "gmail,drive" {
		t.Fatalf("unexpected services: %v", tok.Services)
	}
}

func TestAuthImportCmd_ReadsRefreshTokenFromFile(t *testing.T) {
	store := newMemSecretsStore()
	path := t.TempDir() + "/token.txt"
	if err := os.WriteFile(path, []byte("rt-file\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	cmd := &AuthImportCmd{
		Email:            "a@b.com",
		RefreshTokenFile: path,
	}
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tok, err := store.GetToken("default", "a@b.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.RefreshToken != "rt-file" {
		t.Fatalf("expected file token, got %q", tok.RefreshToken)
	}
}

func TestAuthImportCmd_ExpandsRefreshTokenFilePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store := newMemSecretsStore()
	path := filepath.Join(home, "token.txt")
	if err := os.WriteFile(path, []byte("rt-home\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	cmd := &AuthImportCmd{
		Email:            "a@b.com",
		RefreshTokenFile: "~/token.txt",
	}
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tok, err := store.GetToken("default", "a@b.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.RefreshToken != "rt-home" {
		t.Fatalf("expected expanded file token, got %q", tok.RefreshToken)
	}
}

func TestAuthImportCmd_ReadsRefreshTokenFromStdin(t *testing.T) {
	store := newMemSecretsStore()

	cmd := &AuthImportCmd{
		Email:             "a@b.com",
		RefreshTokenStdin: true,
	}
	if err := cmd.Run(newImportTestContextWithInput(t, bytes.NewBufferString("rt-stdin\n"), store), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tok, err := store.GetToken("default", "a@b.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.RefreshToken != "rt-stdin" {
		t.Fatalf("expected stdin token, got %q", tok.RefreshToken)
	}
}

func TestAuthImportCmd_StoresAccessTokenFromEnv(t *testing.T) {
	store := newMemSecretsStore()
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	origNow := authImportNow
	t.Cleanup(func() { authImportNow = origNow })
	authImportNow = func() time.Time { return now }
	t.Setenv("GOG_TEST_REFRESH_TOKEN", "rt")
	t.Setenv("GOG_TEST_ACCESS_TOKEN", "at\n")

	cmd := &AuthImportCmd{
		Email:           "a@b.com",
		RefreshTokenEnv: "GOG_TEST_REFRESH_TOKEN",
		AccessTokenEnv:  "GOG_TEST_ACCESS_TOKEN",
	}
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tok, err := store.GetToken("default", "a@b.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.AccessToken != "at" {
		t.Fatalf("expected access token, got %q", tok.AccessToken)
	}
	wantExpiry := now.Add(time.Hour)
	if !tok.AccessTokenExpiresAt.Equal(wantExpiry) {
		t.Fatalf("expected default expiry %s, got %s", wantExpiry, tok.AccessTokenExpiresAt)
	}
}

func TestAuthImportCmd_StoresAccessTokenExpiry(t *testing.T) {
	store := newMemSecretsStore()
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	origNow := authImportNow
	t.Cleanup(func() { authImportNow = origNow })
	authImportNow = func() time.Time { return now }
	t.Setenv("GOG_TEST_REFRESH_TOKEN", "rt")
	t.Setenv("GOG_TEST_ACCESS_TOKEN", "at")

	cmd := &AuthImportCmd{
		Email:                "a@b.com",
		RefreshTokenEnv:      "GOG_TEST_REFRESH_TOKEN",
		AccessTokenEnv:       "GOG_TEST_ACCESS_TOKEN",
		AccessTokenExpiresAt: "2026-05-22T14:30:00Z",
	}
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tok, err := store.GetToken("default", "a@b.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got := tok.AccessTokenExpiresAt.UTC().Format(time.RFC3339); got != "2026-05-22T14:30:00Z" {
		t.Fatalf("unexpected expiry: %s", got)
	}
}

func TestAuthImportCmd_RejectsAccessTokenExpiryWithoutToken(t *testing.T) {
	store := newMemSecretsStore()
	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt")
	cmd.AccessTokenExpiresAt = "2026-05-22T14:30:00Z"

	err := cmd.Run(newImportTestContext(t, store), &RootFlags{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires an access token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthImportCmd_RejectsBothTokenStdinSources(t *testing.T) {
	store := newMemSecretsStore()
	cmd := &AuthImportCmd{
		Email:             "a@b.com",
		RefreshTokenStdin: true,
		AccessTokenStdin:  true,
	}

	err := cmd.Run(newImportTestContext(t, store), &RootFlags{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthImportCmd_RejectsExistingEntryWithoutForce(t *testing.T) {
	store := newMemSecretsStore()
	if err := store.SetToken("default", "a@b.com", secrets.Token{RefreshToken: "rt-old"}); err != nil {
		t.Fatalf("seed SetToken: %v", err)
	}

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt-new")
	err := cmd.Run(newImportTestContext(t, store), &RootFlags{})
	if err == nil {
		t.Fatal("expected error when entry exists without --force")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected ExitError code=2, got %#v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force in error, got %q", err.Error())
	}
	tok, _ := store.GetToken("default", "a@b.com")
	if tok.RefreshToken != "rt-old" {
		t.Fatalf("expected unchanged token, got %q", tok.RefreshToken)
	}
}

func TestAuthImportCmd_ForceOverwritesExistingEntry(t *testing.T) {
	store := newMemSecretsStore()
	if err := store.SetToken("default", "a@b.com", secrets.Token{RefreshToken: "rt-old"}); err != nil {
		t.Fatalf("seed SetToken: %v", err)
	}

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt-new")
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{Force: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tok, err := store.GetToken("default", "a@b.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.RefreshToken != "rt-new" {
		t.Fatalf("expected overwritten token, got %q", tok.RefreshToken)
	}
}

func TestAuthImportCmd_ForceOverwritesUnreadableEntry(t *testing.T) {
	store := &errorTokenStore{err: errors.New("decode token")}

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt-new")
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{Force: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAuthImportCmd_CustomClientNamespace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	store := newMemSecretsStore()

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt")
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{Client: "org"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := store.GetToken("org", "a@b.com"); err != nil {
		t.Fatalf("expected token under custom client: %v", err)
	}
	if _, err := store.GetToken("default", "a@b.com"); err == nil {
		t.Fatalf("expected no token under default client")
	}
	cfg, err := defaultConfigStoreForTest(t).Read()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if got := cfg.AccountClients["a@b.com"]; got != "org" {
		t.Fatalf("expected account client mapping org, got %q", got)
	}
}

func TestAuthImportCmd_UsesConfiguredClientMapping(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	if err := defaultConfigStoreForTest(t).Write(config.File{ClientDomains: map[string]string{
		"example.com": "work",
	}}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	store := newMemSecretsStore()

	cmd := authImportCmdWithEnvToken(t, "user@example.com", "rt")
	if err := cmd.Run(newImportTestContext(t, store), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := store.GetToken("work", "user@example.com"); err != nil {
		t.Fatalf("expected token under mapped client: %v", err)
	}
	if _, err := store.GetToken("default", "user@example.com"); err == nil {
		t.Fatalf("expected no token under default client")
	}
}

func TestAuthImportCmd_DryRunDoesNotWrite(t *testing.T) {
	store := newMemSecretsStore()

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt")
	err := cmd.Run(newImportTestContext(t, store), &RootFlags{DryRun: true})
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 0 {
		t.Fatalf("expected dry-run ExitError code=0, got %#v", err)
	}
	if _, err := store.GetToken("default", "a@b.com"); err == nil {
		t.Fatal("expected no token after dry-run")
	}
}
