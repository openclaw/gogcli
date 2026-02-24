package cmd

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"google.golang.org/api/idtoken"

	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailWatchServeCmd_UsesStoredHook(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.Hook = &gmailWatchHook{
			URL:         "http://example.com/hook",
			Token:       "tok",
			IncludeBody: true,
			MaxBytes:    123,
		}
		s.UpdatedAtMs = time.Now().UnixMilli()
		return nil
	})
	if updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	flags := &RootFlags{Account: "a@b.com"}
	var got *gmailWatchServer
	listenAndServe = func(srv *http.Server) error {
		if gs, ok := srv.Handler.(*gmailWatchServer); ok {
			got = gs
		}
		return nil
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook"}, ui.WithUI(context.Background(), u), flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if got == nil {
		t.Fatalf("expected server")
	}
	if got.cfg.HookURL != "http://example.com/hook" || got.cfg.HookToken != "tok" {
		t.Fatalf("unexpected hook config: %#v", got.cfg)
	}
	if !got.cfg.IncludeBody || got.cfg.MaxBodyBytes != 123 {
		t.Fatalf("unexpected hook flags: %#v", got.cfg)
	}
	if got.cfg.AllowNoHook {
		t.Fatalf("expected hook present")
	}
}

func TestGmailWatchServeCmd_DefaultMaxBytes(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		return nil
	})
	if updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	flags := &RootFlags{Account: "a@b.com"}
	var got *gmailWatchServer
	listenAndServe = func(srv *http.Server) error {
		if gs, ok := srv.Handler.(*gmailWatchServer); ok {
			got = gs
		}
		return nil
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook", "--max-bytes", "0"}, ui.WithUI(context.Background(), u), flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if got == nil {
		t.Fatalf("expected server")
	}
	if !got.cfg.AllowNoHook {
		t.Fatalf("expected allow no hook")
	}
	if got.cfg.MaxBodyBytes != defaultHookMaxBytes {
		t.Fatalf("expected default max bytes, got %d", got.cfg.MaxBodyBytes)
	}
	if len(got.cfg.ExcludeLabels) != 2 || got.cfg.ExcludeLabels[0] != "SPAM" || got.cfg.ExcludeLabels[1] != "TRASH" {
		t.Fatalf("unexpected exclude labels: %#v", got.cfg.ExcludeLabels)
	}
}

func TestGmailWatchServeCmd_ExcludeLabels_Disable(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		return nil
	})
	if updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	flags := &RootFlags{Account: "a@b.com"}
	var got *gmailWatchServer
	listenAndServe = func(srv *http.Server) error {
		if gs, ok := srv.Handler.(*gmailWatchServer); ok {
			got = gs
		}
		return nil
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook", "--exclude-labels", ""}, ui.WithUI(context.Background(), u), flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if got == nil {
		t.Fatalf("expected server")
	}
	if len(got.cfg.ExcludeLabels) != 0 {
		t.Fatalf("expected exclude labels disabled, got: %#v", got.cfg.ExcludeLabels)
	}
}

func TestGmailWatchServeCmd_SaveHookAndOIDC(t *testing.T) {
	origListen := listenAndServe
	origOIDC := newOIDCValidator
	t.Cleanup(func() {
		listenAndServe = origListen
		newOIDCValidator = origOIDC
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		return nil
	})
	if updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	flags := &RootFlags{Account: "a@b.com"}
	var got *gmailWatchServer
	listenAndServe = func(srv *http.Server) error {
		if gs, ok := srv.Handler.(*gmailWatchServer); ok {
			got = gs
		}
		return nil
	}
	newOIDCValidator = func(context.Context, ...idtoken.ClientOption) (*idtoken.Validator, error) {
		return &idtoken.Validator{}, nil
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{
		"--port", "9999",
		"--path", "/hook",
		"--verify-oidc",
		"--hook-url", "http://example.com/hook",
		"--hook-token", "tok",
		"--include-body",
		"--max-bytes", "10",
		"--save-hook",
	}, ui.WithUI(context.Background(), u), flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if got == nil || got.validator == nil || !got.cfg.VerifyOIDC {
		t.Fatalf("expected oidc validator")
	}

	loaded, err := loadGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Get().Hook == nil || loaded.Get().Hook.URL != "http://example.com/hook" {
		t.Fatalf("expected hook saved, got %#v", loaded.Get().Hook)
	}
}
