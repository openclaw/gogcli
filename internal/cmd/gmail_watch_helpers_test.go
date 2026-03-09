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

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func setWatchTestConfigHome(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
}

func TestWriteWatchState_TextAndJSON(t *testing.T) {
	state := gmailWatchState{
		Account:              "a@b.com",
		Topic:                "projects/p/topics/t",
		Labels:               []string{"INBOX", "Label_1"},
		HistoryID:            "123",
		ExpirationMs:         1,
		ProviderExpirationMs: 2,
		RenewAfterMs:         3,
		UpdatedAtMs:          4,
		Hook: &gmailWatchHook{
			URL:         "http://example.com/hook",
			IncludeBody: true,
			MaxBytes:    12,
			Token:       "tok",
		},
		LastDeliveryStatus:     "ok",
		LastDeliveryAtMs:       5,
		LastDeliveryStatusNote: "note",
	}

	textOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		if err := writeWatchState(ctx, state, false); err != nil {
			t.Fatalf("writeWatchState: %v", err)
		}
	})
	if !strings.Contains(textOut, "account\ta@b.com") {
		t.Fatalf("expected account output")
	}
	if !strings.Contains(textOut, "hook_url\thttp://example.com/hook") {
		t.Fatalf("expected hook output")
	}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := writeWatchState(ctx, state, false); err != nil {
			t.Fatalf("writeWatchState json: %v", err)
		}
	})
	var parsed struct {
		Watch gmailWatchState `json:"watch"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.Watch.Hook == nil || parsed.Watch.Hook.URL == "" {
		t.Fatalf("expected hook in json")
	}
}

func TestHookFromFlags(t *testing.T) {
	t.Run("missing url with token", func(t *testing.T) {
		if _, err := hookFromFlags("", "tok", false, 0, false, false); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing url with hook opts", func(t *testing.T) {
		if _, err := hookFromFlags("", "", true, 0, true, false); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("allow no hook", func(t *testing.T) {
		hook, err := hookFromFlags("", "", false, 0, false, true)
		if err == nil || !errors.Is(err, errNoHookConfigured) {
			t.Fatalf("expected no hook error, got: %v", err)
		}
		if hook != nil {
			t.Fatalf("expected nil hook")
		}
	})

	t.Run("defaults max bytes", func(t *testing.T) {
		hook, err := hookFromFlags("http://example.com", "", true, 0, false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hook.MaxBytes != defaultHookMaxBytes {
			t.Fatalf("expected default max bytes")
		}
	})

	t.Run("invalid max bytes", func(t *testing.T) {
		if _, err := hookFromFlags("http://example.com", "", false, 0, true, false); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"":            true,
		"localhost":   true,
		"127.0.0.1":   true,
		"[::1]":       true,
		"example.com": false,
	}
	for host, want := range cases {
		if got := isLoopbackHost(host); got != want {
			t.Fatalf("isLoopbackHost(%q)=%v want %v", host, got, want)
		}
	}
}

func TestGmailWatchStore_StateHelpers(t *testing.T) {
	setWatchTestConfigHome(t)

	store, err := newGmailWatchStore("User+X@Example.COM")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if !strings.Contains(store.path, "user_x_example_com.json") {
		t.Fatalf("unexpected path: %s", store.path)
	}
	id, startErr := store.StartHistoryID("101")
	if startErr != nil {
		t.Fatalf("start history: %v", startErr)
	}
	if id != 101 {
		t.Fatalf("expected history id 101, got %d", id)
	}
	if store.state.HistoryID != "101" {
		t.Fatalf("expected history set")
	}
	id, startErr = store.StartHistoryID("")
	if startErr != nil {
		t.Fatalf("start history existing: %v", startErr)
	}
	if id != 101 {
		t.Fatalf("expected history id 101, got %d", id)
	}
	id, startErr = store.StartHistoryID("100")
	if startErr != nil {
		t.Fatalf("start history stale: %v", startErr)
	}
	if id != 0 {
		t.Fatalf("expected stale history ignored, got %d", id)
	}
	if store.state.HistoryID != "101" {
		t.Fatalf("expected history unchanged")
	}
	id, startErr = store.StartHistoryID("bad")
	if startErr != nil {
		t.Fatalf("start history invalid push: %v", startErr)
	}
	if id != 101 {
		t.Fatalf("expected history id 101, got %d", id)
	}

	if _, err := parseHistoryID(""); err == nil {
		t.Fatalf("expected parse error")
	}
	if got := formatHistoryID(0); got != "" {
		t.Fatalf("expected empty format")
	}
}

func TestGmailWatchStore_SaveMissingPath(t *testing.T) {
	store := &gmailWatchStore{}
	if err := store.Save(); err == nil {
		t.Fatalf("expected error")
	}
}
