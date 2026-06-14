package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/idtoken"

	"github.com/steipete/gogcli/internal/authclient"
)

func TestGmailWatchServeCmd_DryRunDoesNotTouchStateOrListen(t *testing.T) {
	origListen := listenAndServe
	origOIDC := newOIDCValidator
	t.Cleanup(func() {
		listenAndServe = origListen
		newOIDCValidator = origOIDC
	})

	setWatchTestConfigHome(t)
	layout := gmailWatchTestLayout(t)
	listenAndServe = func(*http.Server) error {
		t.Fatal("dry-run started server")
		return nil
	}
	newOIDCValidator = func(context.Context, ...idtoken.ClientOption) (*idtoken.Validator, error) {
		t.Fatal("dry-run created OIDC validator")
		return nil, errors.New("dry-run created OIDC validator")
	}

	var stdout bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
	err := runKong(t, &GmailWatchServeCmd{}, []string{
		"--bind", "0.0.0.0",
		"--port", "9999",
		"--path", "/hook",
		"--fetch-delay", "750ms",
		"--timezone", "UTC",
		"--verify-oidc",
		"--oidc-email", "push@example.com",
		"--oidc-audience", "https://example.com/hook?secret=value",
		"--token", "shared-secret",
		"--hook-url", "https://example.com/downstream?secret=value",
		"--hook-token", "hook-secret",
		"--include-body",
		"--max-bytes", "42",
		"--history-types", "messageAdded,labelRemoved",
		"--exclude-labels", "SPAM,Label_123",
		"--save-hook",
	}, ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
	if ExitCode(err) != 0 {
		t.Fatalf("exit code = %d, want 0: %v", ExitCode(err), err)
	}

	for _, path := range []string{layout.PrimaryGmailWatchDir(), layout.LegacyGmailWatchDir()} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("dry-run touched watch state path %q: %v", path, statErr)
		}
	}

	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Account string `json:"account"`
			Listen  string `json:"listen"`
			Path    string `json:"path"`
			Auth    struct {
				VerifyOIDC      bool `json:"verify_oidc"`
				OIDCEmailSet    bool `json:"oidc_email_set"`
				OIDCAudienceSet bool `json:"oidc_audience_set"`
				SharedTokenSet  bool `json:"shared_token_set"`
			} `json:"auth"`
			Hook struct {
				Source      string `json:"source"`
				URLSet      bool   `json:"url_set"`
				TokenSet    bool   `json:"token_set"`
				IncludeBody bool   `json:"include_body"`
				MaxBytes    int    `json:"max_bytes"`
				Save        bool   `json:"save"`
			} `json:"hook"`
			FetchDelaySeconds float64  `json:"fetch_delay_seconds"`
			Timezone          string   `json:"timezone"`
			HistoryTypes      []string `json:"history_types"`
			ExcludeLabels     []string `json:"exclude_labels"`
		} `json:"request"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &got); decodeErr != nil {
		t.Fatalf("decode dry-run: %v\noutput=%q", decodeErr, stdout.String())
	}
	if !got.DryRun || got.Op != "gmail.watch.serve" {
		t.Fatalf("unexpected dry-run envelope: %#v", got)
	}
	if got.Request.Account != "a@b.com" ||
		got.Request.Listen != "0.0.0.0:9999" ||
		got.Request.Path != "/hook" ||
		!got.Request.Auth.VerifyOIDC ||
		!got.Request.Auth.OIDCEmailSet ||
		!got.Request.Auth.OIDCAudienceSet ||
		!got.Request.Auth.SharedTokenSet ||
		got.Request.Hook.Source != "flags" ||
		!got.Request.Hook.URLSet ||
		!got.Request.Hook.TokenSet ||
		!got.Request.Hook.IncludeBody ||
		got.Request.Hook.MaxBytes != 42 ||
		!got.Request.Hook.Save ||
		got.Request.FetchDelaySeconds != 0.75 ||
		got.Request.Timezone != "UTC" ||
		len(got.Request.HistoryTypes) != 2 ||
		len(got.Request.ExcludeLabels) != 2 {
		t.Fatalf("unexpected dry-run request: %#v", got.Request)
	}
	if bytes.Contains(stdout.Bytes(), []byte("shared-secret")) ||
		bytes.Contains(stdout.Bytes(), []byte("hook-secret")) ||
		bytes.Contains(stdout.Bytes(), []byte("secret=value")) {
		t.Fatalf("dry-run exposed secret input: %s", stdout.String())
	}
}

func TestGmailWatchServeCmd_DryRunReadsStoredHookWithoutLocking(t *testing.T) {
	setWatchTestConfigHome(t)
	layout := gmailWatchTestLayout(t)
	statePath := filepath.Join(layout.GmailWatchDir(), sanitizeAccountForPath("a@b.com")+".json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatalf("mkdir watch state: %v", err)
	}
	stateBytes, marshalErr := json.Marshal(gmailWatchState{
		Account:   "a@b.com",
		HistoryID: "100",
		Hook: &gmailWatchHook{
			URL:         "https://example.com/hook",
			Token:       "stored-secret",
			IncludeBody: false,
			MaxBytes:    123,
		},
	})
	if marshalErr != nil {
		t.Fatalf("marshal state: %v", marshalErr)
	}
	if writeErr := os.WriteFile(statePath, stateBytes, 0o600); writeErr != nil {
		t.Fatalf("write state: %v", writeErr)
	}

	ctx := newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard)
	invalidErr := runKong(t, &GmailWatchServeCmd{}, []string{
		"--max-bytes", "0",
	}, ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
	if ExitCode(invalidErr) != 2 {
		t.Fatalf("invalid override exit code = %d, want 2: %v", ExitCode(invalidErr), invalidErr)
	}

	var stdout bytes.Buffer
	ctx = newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
	if runErr := runKong(t, &GmailWatchServeCmd{}, nil, ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true}); ExitCode(runErr) != 0 {
		t.Fatalf("stored-hook dry-run: %v", runErr)
	}
	var got struct {
		Request struct {
			Hook struct {
				Source   string `json:"source"`
				TokenSet bool   `json:"token_set"`
				MaxBytes int    `json:"max_bytes"`
			} `json:"hook"`
		} `json:"request"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &got); decodeErr != nil {
		t.Fatalf("decode dry-run: %v\noutput=%q", decodeErr, stdout.String())
	}
	if got.Request.Hook.Source != "stored" || !got.Request.Hook.TokenSet || got.Request.Hook.MaxBytes != 123 {
		t.Fatalf("unexpected stored hook plan: %#v", got.Request.Hook)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(statePath), ".lock")); !os.IsNotExist(statErr) {
		t.Fatalf("dry-run created watch lock: %v", statErr)
	}
	after, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read state after dry-run: %v", readErr)
	}
	if !bytes.Equal(after, stateBytes) {
		t.Fatalf("dry-run changed watch state:\nwant=%s\ngot=%s", stateBytes, after)
	}
}

func TestGmailWatchServeCmd_UsesStoredHook(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
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

	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &gmail.Service{})
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook"}, ctx, flags); execErr != nil {
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

	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
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

	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &gmail.Service{})
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook", "--max-bytes", "0"}, ctx, flags); execErr != nil {
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
	if got.cfg.FetchDelay != defaultHistoryFetchDelay {
		t.Fatalf("expected default fetch delay %v, got %v", defaultHistoryFetchDelay, got.cfg.FetchDelay)
	}
	if len(got.cfg.ExcludeLabels) != 2 || got.cfg.ExcludeLabels[0] != "SPAM" || got.cfg.ExcludeLabels[1] != "TRASH" {
		t.Fatalf("unexpected exclude labels: %#v", got.cfg.ExcludeLabels)
	}
}

func TestGmailWatchServeCmd_FetchDelaySeconds(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
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

	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &gmail.Service{})
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook", "--fetch-delay", "5"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if got == nil {
		t.Fatalf("expected server")
	}
	if got.cfg.FetchDelay != 5*time.Second {
		t.Fatalf("expected fetch delay 5s, got %v", got.cfg.FetchDelay)
	}
}

func TestGmailWatchServeCmd_FetchDelayDuration(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
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

	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &gmail.Service{})
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook", "--fetch-delay", "750ms"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if got == nil {
		t.Fatalf("expected server")
	}
	if got.cfg.FetchDelay != 750*time.Millisecond {
		t.Fatalf("expected fetch delay 750ms, got %v", got.cfg.FetchDelay)
	}
}

func TestGmailWatchServeCmd_ExcludeLabels_Disable(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
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

	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &gmail.Service{})
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook", "--exclude-labels", ""}, ctx, flags); execErr != nil {
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

	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
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

	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &gmail.Service{})
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{
		"--port", "9999",
		"--path", "/hook",
		"--verify-oidc",
		"--hook-url", "http://example.com/hook",
		"--hook-token", "tok",
		"--include-body",
		"--max-bytes", "10",
		"--save-hook",
	}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if got == nil || got.validator == nil || !got.cfg.VerifyOIDC {
		t.Fatalf("expected oidc validator")
	}

	loaded := loadGmailWatchTestStore(t, "a@b.com")
	if loaded.Get().Hook == nil || loaded.Get().Hook.URL != "http://example.com/hook" {
		t.Fatalf("expected hook saved, got %#v", loaded.Get().Hook)
	}
}

func TestGmailWatchServeCmd_PreservesClientOverrideForRequestContexts(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
	updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		return nil
	})
	if updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	flags := &RootFlags{Account: "a@b.com", Client: "personal"}
	var got *gmailWatchServer
	listenAndServe = func(srv *http.Server) error {
		if gs, ok := srv.Handler.(*gmailWatchServer); ok {
			got = gs
		}
		return nil
	}

	ctx := withGmailTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(ctx context.Context, _ string) (*gmail.Service, error) {
		if client := authclient.ClientOverrideFromContext(ctx); client != "personal" {
			t.Fatalf("expected client override personal, got %q", client)
		}
		return &gmail.Service{}, nil
	})
	if execErr := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if got == nil {
		t.Fatalf("expected server")
	}
	if _, callErr := got.newService(context.Background(), "a@b.com"); callErr != nil {
		t.Fatalf("newService: %v", callErr)
	}
}
