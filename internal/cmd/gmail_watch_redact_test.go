package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

func TestWriteWatchState_TokenRedaction(t *testing.T) {
	makeState := func(token string) gmailWatchState {
		return gmailWatchState{
			Account:   "a@b.com",
			Topic:     "projects/p/topics/t",
			HistoryID: "1",
			Hook: &gmailWatchHook{
				URL:   "http://example.com/hook",
				Token: token,
			},
		}
	}

	run := func(t *testing.T, state gmailWatchState, showSecrets bool) string {
		t.Helper()
		return captureStdout(t, func() {
			u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
			if err != nil {
				t.Fatalf("ui.New: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			if err := writeWatchState(ctx, state, showSecrets); err != nil {
				t.Fatalf("writeWatchState: %v", err)
			}
		})
	}

	t.Run("long token is redacted by default", func(t *testing.T) {
		out := run(t, makeState("supersecrettoken123"), false)
		if strings.Contains(out, "supersecrettoken123") {
			t.Fatal("token should be redacted but was shown in full")
		}
		if !strings.Contains(out, "supe...(19 chars)") {
			t.Fatalf("expected masked token, got: %s", out)
		}
	})

	t.Run("short token is fully redacted", func(t *testing.T) {
		out := run(t, makeState("ab"), false)
		if strings.Contains(out, "hook_token\tab") {
			t.Fatal("short token should be fully redacted")
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Fatalf("expected [REDACTED], got: %s", out)
		}
	})

	t.Run("4-char token is fully redacted", func(t *testing.T) {
		out := run(t, makeState("abcd"), false)
		if !strings.Contains(out, "[REDACTED]") {
			t.Fatalf("expected [REDACTED] for 4-char token, got: %s", out)
		}
	})

	t.Run("show-secrets reveals full token", func(t *testing.T) {
		out := run(t, makeState("supersecrettoken123"), true)
		if !strings.Contains(out, "hook_token\tsupersecrettoken123") {
			t.Fatalf("expected full token with --show-secrets, got: %s", out)
		}
	})

	t.Run("empty token not shown", func(t *testing.T) {
		out := run(t, makeState(""), false)
		if strings.Contains(out, "hook_token") {
			t.Fatal("empty token should not appear in output")
		}
	})

	t.Run("json output redacts token by default", func(t *testing.T) {
		out := captureStdout(t, func() {
			u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
			if err != nil {
				t.Fatalf("ui.New: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			if err := writeWatchState(ctx, makeState("supersecrettoken123"), false); err != nil {
				t.Fatalf("writeWatchState json: %v", err)
			}
		})
		if strings.Contains(out, "supersecrettoken123") {
			t.Fatal("JSON output should not contain plaintext token")
		}
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("json parse: %v", err)
		}
		if !strings.Contains(out, `"[REDACTED]"`) {
			t.Fatalf("expected [REDACTED] in JSON, got: %s", out)
		}
	})

	t.Run("json output shows token with show-secrets", func(t *testing.T) {
		out := captureStdout(t, func() {
			u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
			if err != nil {
				t.Fatalf("ui.New: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			if err := writeWatchState(ctx, makeState("supersecrettoken123"), true); err != nil {
				t.Fatalf("writeWatchState json: %v", err)
			}
		})
		if !strings.Contains(out, "supersecrettoken123") {
			t.Fatalf("JSON with --show-secrets should contain token, got: %s", out)
		}
	})
}

func TestWriteWatchState_HookURLCredentialRedaction(t *testing.T) {
	password := "example-" + "password"
	queryToken := "example-" + "query-token"
	pathToken := "example-" + "path-token"
	basicAuthURL := "https://alice:" + password + "@example.com/hook"
	credentialURL := "https://alice:" + password + "@example.com/hooks/" + pathToken + "?token=" + queryToken

	makeState := func(hookURL string) gmailWatchState {
		return gmailWatchState{
			Account:   "a@b.com",
			Topic:     "projects/p/topics/t",
			HistoryID: "1",
			Hook: &gmailWatchHook{
				URL: hookURL,
			},
		}
	}

	run := func(t *testing.T, state gmailWatchState, showSecrets, jsonOut bool) string {
		t.Helper()
		return captureStdout(t, func() {
			u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
			if err != nil {
				t.Fatalf("ui.New: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			if jsonOut {
				ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			}
			if err := writeWatchState(ctx, state, showSecrets); err != nil {
				t.Fatalf("writeWatchState: %v", err)
			}
		})
	}

	t.Run("userinfo password redacted by default", func(t *testing.T) {
		out := run(t, makeState(basicAuthURL), false, false)
		if strings.Contains(out, password) {
			t.Fatalf("basic-auth password leaked in hook URL: %s", out)
		}
		if !strings.Contains(out, "hook_url\thttps://example.com/[REDACTED]") {
			t.Fatalf("expected recognizable redacted origin, got: %s", out)
		}
	})

	t.Run("query token redacted by default", func(t *testing.T) {
		out := run(t, makeState("https://example.com/hook?token="+queryToken), false, false)
		if strings.Contains(out, queryToken) {
			t.Fatalf("query token leaked in hook URL: %s", out)
		}
	})

	t.Run("path credential redacted by default", func(t *testing.T) {
		out := run(t, makeState("https://example.com/hooks/"+pathToken), false, false)
		if strings.Contains(out, pathToken) || strings.Contains(out, "/hooks/") {
			t.Fatalf("path credential leaked in hook URL: %s", out)
		}
	})

	t.Run("origin-only url unchanged", func(t *testing.T) {
		out := run(t, makeState("https://example.com"), false, false)
		if !strings.Contains(out, "hook_url\thttps://example.com") {
			t.Fatalf("origin-only hook URL should be shown unchanged, got: %s", out)
		}
	})

	t.Run("malformed url fails closed", func(t *testing.T) {
		out := run(t, makeState("opaque-secret-without-an-origin"), false, false)
		if strings.Contains(out, "opaque-secret") || !strings.Contains(out, "hook_url\t[REDACTED]") {
			t.Fatalf("malformed hook URL was not fully redacted: %s", out)
		}
	})

	t.Run("show-secrets reveals full url", func(t *testing.T) {
		out := run(t, makeState(credentialURL), true, false)
		if !strings.Contains(out, password) || !strings.Contains(out, pathToken) || !strings.Contains(out, queryToken) {
			t.Fatalf("--show-secrets should reveal full hook URL, got: %s", out)
		}
	})

	t.Run("json output redacts url credentials by default", func(t *testing.T) {
		out := run(t, makeState(credentialURL), false, true)
		if strings.Contains(out, password) || strings.Contains(out, pathToken) || strings.Contains(out, queryToken) {
			t.Fatalf("JSON output leaked hook URL credentials: %s", out)
		}
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("json parse: %v", err)
		}
	})
}
