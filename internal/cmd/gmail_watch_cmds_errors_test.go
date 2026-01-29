package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailWatchStartCmd_MissingTopic(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchStartCmd{}, []string{"--ttl", "10"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStartCmd_MissingAccount(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchStartCmd{}).Run(ctx, nil, &RootFlags{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStartCmd_InvalidTTL(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchStartCmd{}, []string{"--topic", "projects/p/topics/t", "--ttl", "nope"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStartCmd_HookTokenRequiresURL(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchStartCmd{}, []string{"--topic", "projects/p/topics/t", "--hook-token", "tok"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStartCmd_NewServiceError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		return nil, errors.New("boom")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchStartCmd{}, []string{"--topic", "projects/p/topics/t"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStartCmd_LabelResolveError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchStartCmd{}, []string{"--topic", "projects/p/topics/t", "--label", "INBOX"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStartCmd_RequestError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/watch") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchStartCmd{}, []string{"--topic", "projects/p/topics/t"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStartCmd_BuildStateError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/watch") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId": 0,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchStartCmd{}, []string{"--topic", "projects/p/topics/t"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStatusCmd_MissingAccount(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchStatusCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStatusCmd_LoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchStatusCmd{}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchRenewCmd_MissingTopic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error { s.Account = "a@b.com"; return nil })

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchRenewCmd{}, []string{}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchRenewCmd_MissingAccount(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchRenewCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchRenewCmd_LoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchRenewCmd{}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchRenewCmd_InvalidTTL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.Topic = "projects/p/topics/t"
		return nil
	})

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchRenewCmd{}, []string{"--ttl", "nope"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchRenewCmd_NewServiceError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		return nil, errors.New("down")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.Topic = "projects/p/topics/t"
		return nil
	})

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchRenewCmd{}, []string{}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStopCmd_ConfirmError(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchStopCmd{}).Run(ctx, &RootFlags{Account: "a@b.com", NoInput: true}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStopCmd_MissingAccount(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchStopCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchStopCmd_ServiceError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		return nil, errors.New("nope")
	}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchStopCmd{}).Run(ctx, &RootFlags{Account: "a@b.com", Force: true}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchServeCmd_LoadStoreError(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	// Guard against hangs if a watch state file exists in the runner's home.
	listenAndServe = func(*http.Server) error { return errors.New("unexpected listen") }

	t.Setenv("HOME", t.TempDir())

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchServeCmd_MissingAccount(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GmailWatchServeCmd{}).Run(ctx, nil, &RootFlags{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchServeCmd_HookFlagsError(t *testing.T) {
	origListen := listenAndServe
	t.Cleanup(func() { listenAndServe = origListen })

	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error { s.Account = "a@b.com"; return nil })

	listenAndServe = func(*http.Server) error { return nil }

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook", "--hook-token", "tok"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchServeCmd_OIDCValidatorError(t *testing.T) {
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
	_ = store.Update(func(s *gmailWatchState) error { s.Account = "a@b.com"; return nil })

	listenAndServe = func(*http.Server) error { return nil }
	newOIDCValidator = func(context.Context, ...idtoken.ClientOption) (*idtoken.Validator, error) {
		return nil, errors.New("oidc down")
	}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "9999", "--path", "/hook", "--verify-oidc"}, ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteWatchState_LastPushMessageID(t *testing.T) {
	var out strings.Builder
	u, uiErr := ui.New(ui.Options{Stdout: &out, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	state := gmailWatchState{
		Account:                "a@b.com",
		Topic:                  "projects/p/topics/t",
		HistoryID:              "100",
		LastDeliveryStatus:     "ok",
		LastDeliveryAtMs:       1,
		LastDeliveryStatusNote: "note",
		LastPushMessageID:      "msg1",
	}
	if err := writeWatchState(ctx, state, false); err != nil {
		t.Fatalf("writeWatchState: %v", err)
	}
	if !strings.Contains(out.String(), "last_push_message_id") {
		t.Fatalf("expected last_push_message_id in output")
	}
}

func TestBuildWatchState_MissingResponse(t *testing.T) {
	if _, err := buildWatchState("a@b.com", "t", nil, nil, 0, nil); err == nil {
		t.Fatalf("expected error")
	}
}
