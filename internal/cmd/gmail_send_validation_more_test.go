package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestGmailSendCmd_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	cases := []GmailSendCmd{
		{ReplyToMessageID: "m1", ThreadID: "t1", To: "a@b.com", Subject: "S", Body: "B"},
		{ReplyAll: true, Subject: "S", Body: "B"},
		{Subject: "S", Body: "B"},
		{To: "a@b.com", Body: "B"},
		{To: "a@b.com", Subject: "S"},
		{To: "a@b.com", Subject: "S", Body: "B", TrackSplit: true},
	}

	for _, cmd := range cases {
		if err := cmd.Run(ctx, flags); err == nil {
			t.Fatalf("expected validation error")
		}
	}
}

func TestGmailSendCmd_MissingAccount(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	cmd := &GmailSendCmd{To: "a@b.com", Subject: "S", Body: "B"}
	if err := cmd.Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected missing account error")
	}
}

func TestGmailSendCmd_ServiceError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })
	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		return nil, errors.New("svc")
	}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	if err := (&GmailSendCmd{To: "a@b.com", Subject: "S", Body: "B"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected service error")
	}
}

func TestGmailSendCmd_FromUnverified(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/sendAs/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sendAsEmail":"alias@example.com","verificationStatus":"pending"}`))
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
	cmd := &GmailSendCmd{To: "a@b.com", Subject: "S", Body: "B", From: "alias@example.com"}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected unverified from error")
	}
}

func TestGmailSendCmd_ReplyInfoError(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	cmd := &GmailSendCmd{
		To:               "a@b.com",
		Subject:          "S",
		Body:             "B",
		ReplyToMessageID: "m1",
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected reply info error")
	}
}
