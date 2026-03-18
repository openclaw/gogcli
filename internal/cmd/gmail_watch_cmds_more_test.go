package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailWatchRenewAndStop_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	setWatchTestConfigHome(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/watch"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId":  "123",
				"expiration": "1730000000000",
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/stop"):
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
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

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{
			Account:      "a@b.com",
			Topic:        "projects/p/topics/t",
			Labels:       []string{"INBOX"},
			HistoryID:    "100",
			RenewAfterMs: time.Now().Add(10 * time.Minute).UnixMilli(),
			ExpirationMs: time.Now().Add(20 * time.Minute).UnixMilli(),
		}
		return nil
	})

	flags := &RootFlags{Account: "a@b.com", Force: true}

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailWatchRenewCmd{}, []string{"--ttl", "3600"}, ctx, flags); err != nil {
			t.Fatalf("renew: %v", err)
		}

		if err := runKong(t, &GmailWatchStopCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("stop: %v", err)
		}
	})

	if _, statErr := os.Stat(store.path); !os.IsNotExist(statErr) {
		t.Fatalf("expected watch state removed, err=%v", statErr)
	}
}

func TestGmailWatchStatusAndStop_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	setWatchTestConfigHome(t)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{
			Account:   "a@b.com",
			Topic:     "projects/p/topics/t",
			HistoryID: "100",
			Hook:      &gmailWatchHook{URL: "http://example.com/hook"},
		}
		return nil
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/stop") {
			w.WriteHeader(http.StatusNoContent)
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

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		if err := runKong(t, &GmailWatchStatusCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("status: %v", err)
		}

		if err := runKong(t, &GmailWatchStopCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("stop: %v", err)
		}
	})
	if !strings.Contains(out, "account") || !strings.Contains(out, "stopped") {
		t.Fatalf("unexpected output: %q", out)
	}
}
