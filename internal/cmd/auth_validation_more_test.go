package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

func TestAuthCredentialsCmd_ErrorsAndStdin(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	if err := (&AuthCredentialsSetCmd{Path: "/nope/credentials.json"}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected read error")
	}

	tmp := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(tmp, []byte("nope"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (&AuthCredentialsSetCmd{Path: tmp}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected parse error")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	creds := `{"installed":{"client_id":"id","client_secret":"secret"}}`
	out := captureStdout(t, func() {
		withStdin(t, creds, func() {
			if err := (&AuthCredentialsSetCmd{Path: "-"}).Run(ctx, &RootFlags{}); err != nil {
				t.Fatalf("stdin run: %v", err)
			}
		})
	})
	if !strings.Contains(out, "\"saved\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAuthTokensList_ErrorsAndEmpty(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	openSecretsStore = func() (secrets.Store, error) { return nil, errors.New("boom") }
	if err := (&AuthTokensListCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected open error")
	}

	openSecretsStore = func() (secrets.Store, error) {
		return &memStoreErr{keysErr: errors.New("keys")}, nil
	}
	if err := (&AuthTokensListCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected keys error")
	}

	store := newMemStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }
	if err := (&AuthTokensListCmd{}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("empty list: %v", err)
	}
}

type memStoreErr struct {
	keysErr   error
	deleteErr error
}

func (m *memStoreErr) Keys() ([]string, error)                      { return nil, m.keysErr }
func (m *memStoreErr) SetToken(string, string, secrets.Token) error { return nil }
func (m *memStoreErr) GetToken(string, string) (secrets.Token, error) {
	return secrets.Token{}, errors.New("missing")
}
func (m *memStoreErr) DeleteToken(string, string) error         { return m.deleteErr }
func (m *memStoreErr) ListTokens() ([]secrets.Token, error)     { return nil, m.keysErr }
func (m *memStoreErr) GetDefaultAccount(string) (string, error) { return "", nil }
func (m *memStoreErr) SetDefaultAccount(string, string) error   { return nil }

func TestAuthTokensDelete_Errors(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&AuthTokensDeleteCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected empty email error")
	}

	if err := (&AuthTokensDeleteCmd{Email: "a@b.com"}).Run(ctx, &RootFlags{NoInput: true}); err == nil {
		t.Fatalf("expected confirm error")
	}

	openSecretsStore = func() (secrets.Store, error) { return nil, errors.New("open") }
	if err := (&AuthTokensDeleteCmd{Email: "a@b.com"}).Run(ctx, &RootFlags{Force: true}); err == nil {
		t.Fatalf("expected open error")
	}

	openSecretsStore = func() (secrets.Store, error) {
		return &memStoreErr{deleteErr: errors.New("delete")}, nil
	}
	if err := (&AuthTokensDeleteCmd{Email: "a@b.com"}).Run(ctx, &RootFlags{Force: true}); err == nil {
		t.Fatalf("expected delete error")
	}
}

func TestAuthTokensExport_UsageAndErrors(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&AuthTokensExportCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected missing email error")
	}
	if err := (&AuthTokensExportCmd{Email: "a@b.com"}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected missing outPath error")
	}

	openSecretsStore = func() (secrets.Store, error) { return nil, errors.New("open") }
	if err := (&AuthTokensExportCmd{Email: "a@b.com", Output: OutputPathRequiredFlag{Path: "out"}}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected open error")
	}

	openSecretsStore = func() (secrets.Store, error) {
		return &memStoreErr{}, nil
	}
	if err := (&AuthTokensExportCmd{Email: "a@b.com", Output: OutputPathRequiredFlag{Path: "out"}}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected get token error")
	}

	store := newMemStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{Email: "a@b.com", RefreshToken: "rt"})

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (&AuthTokensExportCmd{Email: "a@b.com", Output: OutputPathRequiredFlag{Path: filepath.Join(blocker, "out.json")}}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected mkdir error")
	}

	dir := t.TempDir()
	if err := (&AuthTokensExportCmd{Email: "a@b.com", Output: OutputPathRequiredFlag{Path: dir}}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected open error")
	}
}

func TestAuthTokensImport_ErrorsAndStdin(t *testing.T) {
	origOpen := openSecretsStore
	origEnsure := ensureKeychainAccess
	t.Cleanup(func() {
		openSecretsStore = origOpen
		ensureKeychainAccess = origEnsure
	})

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&AuthTokensImportCmd{InPath: "/nope/token.json"}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected read error")
	}

	tmp := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(tmp, []byte("nope"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (&AuthTokensImportCmd{InPath: tmp}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected unmarshal error")
	}

	missing := filepath.Join(t.TempDir(), "missing.json")
	if err := os.WriteFile(missing, []byte(`{"email":""}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (&AuthTokensImportCmd{InPath: missing}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected missing fields error")
	}

	badDate := filepath.Join(t.TempDir(), "bad-date.json")
	if err := os.WriteFile(badDate, []byte(`{"email":"a@b.com","refresh_token":"rt","created_at":"bad"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (&AuthTokensImportCmd{InPath: badDate}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected date parse error")
	}

	ensureKeychainAccess = func() error { return nil }
	store := newMemStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	in := `{"email":"a@b.com","refresh_token":"rt"}`
	withStdin(t, in, func() {
		if err := (&AuthTokensImportCmd{InPath: "-"}).Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("stdin import: %v", err)
		}
	})
}

func TestAuthAdd_TextOutput(t *testing.T) {
	origOpen := openSecretsStore
	origAuth := authorizeGoogle
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		openSecretsStore = origOpen
		authorizeGoogle = origAuth
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	store := newMemStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }
	authorizeGoogle = func(context.Context, googleauth.AuthorizeOptions) (string, error) { return "rt", nil }
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) { return "a@b.com", nil }
	ensureKeychainAccess = func() error { return nil }

	var outBuf strings.Builder
	u, uiErr := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&AuthAddCmd{Email: "a@b.com", ServicesCSV: "gmail"}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(outBuf.String(), "email") || !strings.Contains(outBuf.String(), "services") {
		t.Fatalf("unexpected output: %q", outBuf.String())
	}
}

func TestAuthKeep_Errors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&AuthKeepCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected missing email error")
	}
	if err := (&AuthKeepCmd{Email: "a@b.com"}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected missing key path error")
	}

	tmp := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(tmp, []byte("nope"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (&AuthKeepCmd{Email: "a@b.com", Key: tmp}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected invalid json error")
	}

	wrong := filepath.Join(t.TempDir(), "wrong.json")
	if err := os.WriteFile(wrong, []byte(`{"type":"user"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := (&AuthKeepCmd{Email: "a@b.com", Key: wrong}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected wrong type error")
	}
}

func TestAuthTokensExport_UsesCreatedAt(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{
		Email:        "a@b.com",
		RefreshToken: "rt",
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	outPath := filepath.Join(t.TempDir(), "tok.json")
	if err := (&AuthTokensExportCmd{Email: "a@b.com", Output: OutputPathRequiredFlag{Path: outPath}, Overwrite: true}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["created_at"] == "" {
		t.Fatalf("expected created_at")
	}
}
