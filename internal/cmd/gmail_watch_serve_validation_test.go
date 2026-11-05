package cmd

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestResolveWatchToken(t *testing.T) {
	t.Setenv("TEST_WATCH_INPUT", " \topaque-token \n")
	for _, flag := range []string{"token", "hook-token"} {
		token, err := resolveWatchToken(nil, "", " TEST_WATCH_INPUT ", flag)
		if err != nil || token != " \topaque-token \n" {
			t.Fatalf("%s did not preserve environment token bytes: %v", flag, err)
		}
		literal, err := resolveWatchToken(nil, " literal-token ", "", flag)
		if err != nil || literal != " literal-token " {
			t.Fatalf("%s changed the literal token: %v", flag, err)
		}
		if _, err := resolveWatchToken(nil, "literal-token", "TEST_WATCH_INPUT", flag); ExitCode(err) != 2 {
			t.Fatalf("%s accepted conflicting sources: %v", flag, err)
		}
	}
}

func TestGmailWatchServeCmd_HookTokenSources(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		args        []string
		noStored    bool
		wantError   bool
	}{
		{name: "stored", token: "stored-token"},
		{name: "explicit empty", args: []string{"--hook-token="}},
		{name: "explicit literal", token: "literal-token", args: []string{"--hook-token", "literal-token"}},
		{name: "new URL", args: []string{"--hook-url", "https://example.com/new"}},
		{name: "env without URL", args: []string{"--hook-token-env", "TEST_WATCH_INPUT"}, noStored: true, wantError: true},
		{name: "invalid env with stored hook", args: []string{"--hook-token-env", "TEST_WATCH_EMPTY"}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setWatchTestConfigHome(t)
			t.Setenv("TEST_WATCH_INPUT", "env-token")
			t.Setenv("TEST_WATCH_EMPTY", "")
			store := newGmailWatchTestStore(t, "a@b.com")
			if err := store.Update(func(state *gmailWatchState) error {
				state.Account = "a@b.com"
				if !tc.noStored {
					state.Hook = &gmailWatchHook{URL: "https://example.com/stored", Token: "stored-token", MaxBytes: 123}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			origListen := listenAndServe
			t.Cleanup(func() { listenAndServe = origListen })
			listened := false
			listenAndServe = func(srv *http.Server) error {
				listened = true
				server := srv.Handler.(*gmailWatchServer)
				if server.cfg.HookToken != tc.token {
					t.Fatal("hook token source precedence changed")
				}
				return nil
			}
			ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &gmail.Service{})
			err := runKong(t, &GmailWatchServeCmd{}, tc.args, ctx, &RootFlags{Account: "a@b.com"})
			if tc.wantError {
				if ExitCode(err) != 2 || listened {
					t.Fatalf("invalid source did not fail before listening: %v", err)
				}
			} else if err != nil || !listened {
				t.Fatalf("serve failed: %v", err)
			}
		})
	}
}

func TestGmailWatchServeCmd_EnvTokenValidation(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })
	listenAndServe = func(*http.Server) error {
		t.Fatal("invalid token source started server")
		return nil
	}
	for _, flag := range []string{"token", "hook-token"} {
		for _, tc := range []struct {
			name, envName, value, want string
			literal                    bool
		}{
			{name: "unset", envName: "TEST_WATCH_MISSING", want: "requires a non-empty environment variable"},
			{name: "empty value", envName: "TEST_WATCH_INPUT", want: "requires a non-empty environment variable"},
			{name: "blank value", envName: "TEST_WATCH_INPUT", value: " \t\n", want: "requires a non-empty environment variable"},
			{name: "empty name", want: "requires a non-empty environment variable name"},
			{name: "blank name", envName: " \t", want: "requires a non-empty environment variable name"},
			{name: "literal conflict", envName: "TEST_WATCH_INPUT", value: "synthetic-env-secret", literal: true, want: "mutually exclusive"},
		} {
			t.Run(flag+"/"+tc.name, func(t *testing.T) {
				setWatchTestConfigHome(t)
				t.Setenv("TEST_WATCH_INPUT", tc.value)
				t.Setenv("TEST_WATCH_MISSING", "")
				if err := os.Unsetenv("TEST_WATCH_MISSING"); err != nil {
					t.Fatal(err)
				}
				args := []string{"--verify-oidc", "--" + flag + "-env=" + tc.envName}
				if tc.literal {
					args = append(args, "--"+flag+"=")
				}
				ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
				err := runKong(t, &GmailWatchServeCmd{}, args, ctx, &RootFlags{Account: "a@b.com", DryRun: true})
				if ExitCode(err) != 2 || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("expected usage error containing %q, got %v", tc.want, err)
				}
				if strings.Contains(err.Error(), "synthetic-env-secret") {
					t.Fatal("diagnostic exposed environment token")
				}
				layout := gmailWatchTestLayout(t)
				for _, path := range []string{layout.PrimaryGmailWatchDir(), layout.LegacyGmailWatchDir()} {
					if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
						t.Fatalf("invalid input touched watch state: %v", statErr)
					}
				}
			})
		}
	}
}

func TestGmailWatchServeCmd_ValidationErrors(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}

	t.Run("path missing slash", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--path", "hook", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("port missing", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "0"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("non-loopback without auth", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--bind", "0.0.0.0", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("oidc email requires verify", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--oidc-email", "svc@example.com", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("oidc audience requires verify", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--oidc-audience", "aud", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("fetch delay must be non-negative", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--fetch-delay", "-1", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("fetch delay must parse as duration", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--fetch-delay", "not-a-duration", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("dry-run validates explicit hook options", func(t *testing.T) {
		dryRunFlags := &RootFlags{Account: "a@b.com", DryRun: true}
		if err := runKong(t, &GmailWatchServeCmd{}, []string{
			"--hook-url", "https://example.com/hook",
			"--max-bytes", "0",
			"--port", "9999",
		}, context.Background(), dryRunFlags); err == nil {
			t.Fatalf("expected error")
		}
	})
}
