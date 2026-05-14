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

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

func newImportTestContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{})
}

func withImportOverrides(t *testing.T, store secrets.Store) {
	t.Helper()
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origStdin := readAuthImportStdin
	t.Cleanup(func() {
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		readAuthImportStdin = origStdin
	})
	openSecretsStore = func() (secrets.Store, error) { return store, nil }
	ensureKeychainAccess = func() error { return nil }
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
	withImportOverrides(t, store)

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "   ")
	cmd.ServicesCSV = "gmail"
	err := cmd.Run(newImportTestContext(t), &RootFlags{})
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
	withImportOverrides(t, store)

	cmd := &AuthImportCmd{
		Email: "a@b.com",
	}
	err := cmd.Run(newImportTestContext(t), &RootFlags{})
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
}

func TestAuthImportCmd_RejectsMultipleRefreshTokenSources(t *testing.T) {
	store := newMemSecretsStore()
	withImportOverrides(t, store)

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt")
	cmd.RefreshTokenStdin = true
	err := cmd.Run(newImportTestContext(t), &RootFlags{})
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
	withImportOverrides(t, store)

	cmd := authImportCmdWithEnvToken(t, "   ", "rt")
	err := cmd.Run(newImportTestContext(t), &RootFlags{})
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
	withImportOverrides(t, store)
	t.Setenv("GOG_TEST_REFRESH_TOKEN", "rt-1\n")

	cmd := &AuthImportCmd{
		Email:           "A@B.com",
		RefreshTokenEnv: "GOG_TEST_REFRESH_TOKEN",
		ServicesCSV:     "gmail, drive",
	}
	if err := cmd.Run(newImportTestContext(t), &RootFlags{}); err != nil {
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
	withImportOverrides(t, store)
	path := t.TempDir() + "/token.txt"
	if err := os.WriteFile(path, []byte("rt-file\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	cmd := &AuthImportCmd{
		Email:            "a@b.com",
		RefreshTokenFile: path,
	}
	if err := cmd.Run(newImportTestContext(t), &RootFlags{}); err != nil {
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
	withImportOverrides(t, store)
	path := filepath.Join(home, "token.txt")
	if err := os.WriteFile(path, []byte("rt-home\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	cmd := &AuthImportCmd{
		Email:            "a@b.com",
		RefreshTokenFile: "~/token.txt",
	}
	if err := cmd.Run(newImportTestContext(t), &RootFlags{}); err != nil {
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
	withImportOverrides(t, store)
	readAuthImportStdin = func() ([]byte, error) {
		return io.ReadAll(bytes.NewBufferString("rt-stdin\n"))
	}

	cmd := &AuthImportCmd{
		Email:             "a@b.com",
		RefreshTokenStdin: true,
	}
	if err := cmd.Run(newImportTestContext(t), &RootFlags{}); err != nil {
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

func TestAuthImportCmd_RejectsExistingEntryWithoutForce(t *testing.T) {
	store := newMemSecretsStore()
	withImportOverrides(t, store)
	if err := store.SetToken("default", "a@b.com", secrets.Token{RefreshToken: "rt-old"}); err != nil {
		t.Fatalf("seed SetToken: %v", err)
	}

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt-new")
	err := cmd.Run(newImportTestContext(t), &RootFlags{})
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
	withImportOverrides(t, store)
	if err := store.SetToken("default", "a@b.com", secrets.Token{RefreshToken: "rt-old"}); err != nil {
		t.Fatalf("seed SetToken: %v", err)
	}

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt-new")
	if err := cmd.Run(newImportTestContext(t), &RootFlags{Force: true}); err != nil {
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
	withImportOverrides(t, store)

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt-new")
	if err := cmd.Run(newImportTestContext(t), &RootFlags{Force: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAuthImportCmd_CustomClientNamespace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	store := newMemSecretsStore()
	withImportOverrides(t, store)

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt")
	if err := cmd.Run(newImportTestContext(t), &RootFlags{Client: "org"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := store.GetToken("org", "a@b.com"); err != nil {
		t.Fatalf("expected token under custom client: %v", err)
	}
	if _, err := store.GetToken("default", "a@b.com"); err == nil {
		t.Fatalf("expected no token under default client")
	}
	cfg, err := config.ReadConfig()
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
	if err := config.WriteConfig(config.File{ClientDomains: map[string]string{
		"example.com": "work",
	}}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	store := newMemSecretsStore()
	withImportOverrides(t, store)

	cmd := authImportCmdWithEnvToken(t, "user@example.com", "rt")
	if err := cmd.Run(newImportTestContext(t), &RootFlags{}); err != nil {
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
	withImportOverrides(t, store)

	cmd := authImportCmdWithEnvToken(t, "a@b.com", "rt")
	err := cmd.Run(newImportTestContext(t), &RootFlags{DryRun: true})
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 0 {
		t.Fatalf("expected dry-run ExitError code=0, got %#v", err)
	}
	if _, err := store.GetToken("default", "a@b.com"); err == nil {
		t.Fatal("expected no token after dry-run")
	}
}
