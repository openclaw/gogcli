package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/secrets"
)

const (
	rekeyTestOldPassword = "old-password"
	rekeyTestNewPassword = "new-password"
	rekeyTestPayload     = "private-refresh-token-must-not-appear"
)

// rekeyTestContext gives each test an isolated HOME/XDG layout so the keyring
// under test is never the ambient one.
func rekeyTestContext(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()

	ctx, stdout, _ := rekeyTestContextIO(t)

	return ctx, stdout
}

// rekeyTestContextIO is rekeyTestContext plus the stderr buffer, for tests that
// assert on what the command must *not* print.
func rekeyTestContextIO(t *testing.T) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))
	for _, name := range []string{"GOG_CONFIG_DIR", "GOG_DATA_DIR", "GOG_STATE_DIR", "GOG_CACHE_DIR"} {
		t.Setenv(name, "")
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))
	t.Setenv("GOG_HOME", "")
	t.Setenv("GOG_KEYRING_BACKEND", "file")

	var stdout, stderr bytes.Buffer

	// newCmdRuntimeIOContext wires the runtime's IO to these buffers, which is
	// what stdoutWriter reads — the ui alone is not enough.
	ctx := newCmdRuntimeIOContext(t, strings.NewReader(""), &stdout, &stderr)

	return ctx, &stdout, &stderr
}

// unsetEnv removes a variable for the duration of the test, restoring the
// original value on cleanup. t.Setenv alone cannot express "unset", because an
// empty value still counts as present.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	original, had := os.LookupEnv(name)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(name, original)

			return
		}
		_ = os.Unsetenv(name)
	})

	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
}

// seedRekeyKeyring writes one token into the keyring the command will rekey.
func seedRekeyKeyring(t *testing.T, ctx context.Context) {
	t.Helper()

	layout, err := commandLayout(ctx, config.PathKindConfig, config.PathKindData)
	if err != nil {
		t.Fatalf("layout: %v", err)
	}

	store, err := secrets.Open(secrets.OpenOptions{
		Layout:      layout,
		Backend:     "file",
		Password:    rekeyTestOldPassword,
		PasswordSet: true,
		GOOS:        runtime.GOOS,
	})
	if err != nil {
		t.Fatalf("open keyring: %v", err)
	}

	if err := store.SetToken("default", "a@example.com", secrets.Token{
		Client:       "default",
		Email:        "a@example.com",
		RefreshToken: rekeyTestPayload,
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
}

// keyringReadable reports whether the isolated keyring can be opened and read
// with the given password.
func keyringReadable(t *testing.T, ctx context.Context, password string) bool {
	t.Helper()

	layout, err := commandLayout(ctx, config.PathKindConfig, config.PathKindData)
	if err != nil {
		t.Fatalf("layout: %v", err)
	}

	store, err := secrets.Open(secrets.OpenOptions{
		Layout:      layout,
		Backend:     "file",
		Password:    password,
		PasswordSet: true,
		GOOS:        runtime.GOOS,
	})
	if err != nil {
		return false
	}

	tokens, err := store.ListTokens()

	return err == nil && len(tokens) > 0
}

func TestAuthRekey_OutputAndNextStep(t *testing.T) {
	for _, mode := range []string{"human", "plain", "json"} {
		t.Run(mode, func(t *testing.T) {
			ctx, stdout, stderr := rekeyTestContextIO(t)
			seedRekeyKeyring(t, ctx)
			t.Setenv(rekeyPasswordEnv, rekeyTestOldPassword)
			t.Setenv(rekeyNewPasswordEnv, rekeyTestNewPassword)
			if mode == "json" {
				ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			}
			if mode == "plain" {
				ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
			}
			out := filepath.Join(t.TempDir(), "output")
			if err := runKong(t, &AuthRekeyCmd{}, []string{out}, ctx, &RootFlags{NoInput: true}); err != nil {
				t.Fatal(err)
			}
			resolvedOut, err := filepath.EvalSymlinks(out)
			if err != nil {
				t.Fatal(err)
			}
			layout, err := commandLayout(ctx, config.PathKindData)
			if err != nil {
				t.Fatal(err)
			}
			live, err := filepath.EvalSymlinks(layout.KeyringDir())
			if err != nil {
				t.Fatal(err)
			}
			quote := func(path string) string {
				if runtime.GOOS == "windows" {
					return "'" + strings.ReplaceAll(path, "'", "''") + "'"
				}
				return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
			}
			move := "mv -- " + quote(live) + " " + quote(live+".old") + " && mv -- " + quote(resolvedOut) + " " + quote(live)
			if runtime.GOOS == "windows" {
				move = "Move-Item -LiteralPath " + quote(live) + " -Destination " + quote(live+".old") + " -ErrorAction Stop; Move-Item -LiteralPath " + quote(resolvedOut) + " -Destination " + quote(live) + " -ErrorAction Stop"
			}
			combined := stdout.String() + stderr.String()
			requiredText := []string{".old", "proven", "GOG_KEYRING_PASSWORD", "token:default:a@example.com", "token:a@example.com"}
			if mode != "json" {
				requiredText = append(requiredText, resolvedOut, move)
			}
			for _, required := range requiredText {
				if !strings.Contains(combined, required) {
					t.Fatalf("missing %q in %s", required, combined)
				}
			}
			for _, secret := range []string{rekeyTestOldPassword, rekeyTestNewPassword, rekeyTestPayload} {
				if strings.Contains(combined, secret) {
					t.Fatalf("output disclosed secret %q", secret)
				}
			}
			if !keyringReadable(t, ctx, rekeyTestOldPassword) || keyringReadable(t, ctx, rekeyTestNewPassword) {
				t.Fatal("command changed live password")
			}
			switch mode {
			case "json":
				var payload map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload["password_changed"] != false || payload["items"] != float64(2) || payload["output_dir"] != resolvedOut || payload["move_command"] == "" {
					t.Fatalf("dishonest result: %v", payload)
				}
				if command, ok := payload["move_command"].(string); !ok || !strings.Contains(command, move) {
					t.Fatal("JSON omitted the exact move command")
				}
			case "plain":
				for _, required := range []string{"password_changed\tfalse", "items\t2", "move_command\t"} {
					if !strings.Contains(stdout.String(), required) {
						t.Fatalf("plain output missing %q", required)
					}
				}
			default:
				if !strings.Contains(stdout.String(), "Live keyring untouched") || !strings.Contains(stdout.String(), "Install with:") {
					t.Fatal("human output omitted live status or installation command")
				}
			}
		})
	}
}

func TestAuthRekey_PromptPreservesWhitespace(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := newCmdRuntimeIOContext(t, strings.NewReader("  private password  \n"), &stdout, &stderr)
	password, err := rekeyPromptSecret(ctx, "Password: ")
	if err != nil || password != "  private password  " {
		t.Fatalf("prompt changed the password: %v", err)
	}
	if stdout.Len() != 0 || stderr.String() != "Password: \n" {
		t.Fatal("prompt printed a password or wrote to stdout")
	}
}

func TestAuthRekey_RejectsBackendBeforePasswords(t *testing.T) {
	ctx, stdout, stderr := rekeyTestContextIO(t)
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")
	// The test runtime normally overrides backend selection with "file".
	ctx = withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.KeyringOptions.Backend = "keychain"
	})
	unsetEnv(t, rekeyPasswordEnv)
	unsetEnv(t, rekeyNewPasswordEnv)
	layout, err := commandLayout(ctx, config.PathKindData)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	err = runKong(t, &AuthRekeyCmd{}, []string{out}, ctx, &RootFlags{NoInput: true})
	if !errors.Is(err, secrets.ErrRekeyNotFileBackend) {
		t.Fatalf("backend guard did not run first: %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("backend rejection prompted or reported success")
	}
	for _, path := range []string{layout.KeyringDir(), out} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("backend rejection created a directory")
		}
	}
}

func TestAuthRekey_Guards(t *testing.T) {
	for _, scenario := range []string{"missing-output", "missing-old", "missing-new", "wrong", "nonempty"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, stdout := rekeyTestContext(t)
			seedRekeyKeyring(t, ctx)
			t.Setenv(rekeyPasswordEnv, rekeyTestOldPassword)
			t.Setenv(rekeyNewPasswordEnv, rekeyTestNewPassword)
			out := filepath.Join(t.TempDir(), "output")
			args := make([]string, 0, 2)
			wantText := "outdir"
			if scenario != "missing-output" {
				args = append(args, out)
			}
			switch scenario {
			case "missing-old":
				unsetEnv(t, rekeyPasswordEnv)
				wantText = "no current password:"
			case "missing-new":
				unsetEnv(t, rekeyNewPasswordEnv)
				wantText = "no new password:"
			case "wrong":
				t.Setenv(rekeyPasswordEnv, "wrong")
				args = append(args, "--skip-unreadable")
				wantText = "does not open any keyring item"
			case "nonempty":
				wantText = "output directory is not empty"
				if err := os.Mkdir(out, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, "keep"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := runKong(t, &AuthRekeyCmd{}, args, ctx, &RootFlags{NoInput: true}); err == nil || !strings.Contains(err.Error(), wantText) {
				t.Fatalf("want %q error, got %v", wantText, err)
			}
			if stdout.Len() != 0 {
				t.Fatal("failure printed success")
			}
			if !keyringReadable(t, ctx, rekeyTestOldPassword) {
				t.Fatal("guard changed live keyring")
			}
		})
	}
}

func TestAuthRekey_PositionalRequired(t *testing.T) {
	for _, args := range [][]string{nil, {"--outdir", "output"}, {"output"}} {
		command := &AuthRekeyCmd{}
		parser, err := kong.New(command)
		if err != nil {
			t.Fatal(err)
		}
		_, err = parser.Parse(args)
		if len(args) == 1 {
			if err != nil || command.OutDir != "output" {
				t.Fatalf("required positional not accepted: %v", err)
			}
		} else if err == nil {
			t.Fatalf("accepted invalid positional interface: %v", args)
		}
	}
}

func TestAuthRekey_PasswordEnvironment(t *testing.T) {
	for _, value := range []string{"", "  whitespace matters  "} {
		ctx, _, stderr := rekeyTestContextIO(t)
		t.Setenv(rekeyPasswordEnv, value)
		t.Setenv(rekeyNewPasswordEnv, value)
		old, oldSet, err := rekeyReadPassword(ctx, rekeyPasswordEnv, "Current: ", true)
		if err != nil || !oldSet || old != value {
			t.Fatalf("old password changed or lost: %v", err)
		}
		newPassword, newSet, err := rekeyReadNewPassword(ctx, true)
		if err != nil || !newSet || newPassword != value {
			t.Fatalf("new password changed or lost: %v", err)
		}
		if stderr.Len() != 0 {
			t.Fatal("environment passwords prompted or were printed")
		}
	}
}

func TestAuthRekey_ForceReportsReplacement(t *testing.T) {
	ctx, stdout := rekeyTestContext(t)
	seedRekeyKeyring(t, ctx)
	t.Setenv(rekeyPasswordEnv, rekeyTestOldPassword)
	t.Setenv(rekeyNewPasswordEnv, rekeyTestNewPassword)
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "stale"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runKong(t, &AuthRekeyCmd{}, []string{out}, ctx, &RootFlags{Force: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Replaced") || !strings.Contains(stdout.String(), "stale") {
		t.Fatal("replacement not reported")
	}
}

func TestAuthRekey_EmptyHonest(t *testing.T) {
	ctx, stdout, stderr := rekeyTestContextIO(t)
	layout, err := commandLayout(ctx, config.PathKindData, config.PathKindConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.KeyringDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(rekeyPasswordEnv, rekeyTestOldPassword)
	t.Setenv(rekeyNewPasswordEnv, rekeyTestNewPassword)
	if err := runKong(t, &AuthRekeyCmd{}, []string{filepath.Join(t.TempDir(), "out")}, ctx, &RootFlags{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String()+stderr.String(), "no password was changed or established") {
		t.Fatal("empty keyring claimed password rotation")
	}
}

func TestAuthRekey_DryRunChangesNothing(t *testing.T) {
	ctx, _ := rekeyTestContext(t)
	// No passwords or source directory; dry-run must not try to open anything.
	unsetEnv(t, rekeyPasswordEnv)
	unsetEnv(t, rekeyNewPasswordEnv)
	layout, err := commandLayout(ctx, config.PathKindData, config.PathKindConfig)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "output")
	err = runKong(t, &AuthRekeyCmd{}, []string{out}, ctx, &RootFlags{DryRun: true})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 0 {
		t.Fatalf("dry run: %v", err)
	}
	for _, path := range []string{layout.KeyringDir(), out} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry run created %s", path)
		}
	}
}
