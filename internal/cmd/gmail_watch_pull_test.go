package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type fakeGmailPubSubReceiver struct {
	received bool
	closed   bool
	err      error
}

func (r *fakeGmailPubSubReceiver) Receive(context.Context, func(context.Context, *gmailPubSubMessage)) error {
	r.received = true
	return r.err
}

func (r *fakeGmailPubSubReceiver) Close() error {
	r.closed = true
	return nil
}

func TestGmailWatchPullCmd_UsesStoredHookAndReceiver(t *testing.T) {
	origReceiver := newGmailPubSubReceiver
	t.Cleanup(func() { newGmailPubSubReceiver = origReceiver })

	setWatchTestConfigHome(t)
	store := newGmailWatchTestStore(t, "a@b.com")
	if updateErr := store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{
			Account:   "a@b.com",
			Topic:     "projects/p/topics/t",
			HistoryID: "100",
			Hook: &gmailWatchHook{
				URL:         "http://example.com/hook",
				Token:       "tok",
				IncludeBody: true,
				MaxBytes:    123,
			},
		}
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	fakeReceiver := &fakeGmailPubSubReceiver{}
	var gotSubscription string
	var gotSettings gmailPubSubReceiveSettings
	newGmailPubSubReceiver = func(_ context.Context, subscription string, settings gmailPubSubReceiveSettings) (gmailPubSubReceiver, error) {
		gotSubscription = subscription
		gotSettings = settings
		return fakeReceiver, nil
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	args := []string{
		"--subscription", "projects/p/subscriptions/s",
		"--fetch-delay", "0",
	}
	ctx := withGmailTestService(ui.WithUI(context.Background(), u), &gmail.Service{})
	if execErr := runKong(t, &GmailWatchPullCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if gotSubscription != "projects/p/subscriptions/s" {
		t.Fatalf("subscription = %q", gotSubscription)
	}
	if gotSettings.MaxOutstandingMessages != 1 {
		t.Fatalf("max outstanding = %d", gotSettings.MaxOutstandingMessages)
	}
	if !fakeReceiver.received || !fakeReceiver.closed {
		t.Fatalf("expected receiver used and closed: %#v", fakeReceiver)
	}
}

func TestGmailWatchPullCmd_RequiresFullSubscriptionAndHook(t *testing.T) {
	origReceiver := newGmailPubSubReceiver
	t.Cleanup(func() { newGmailPubSubReceiver = origReceiver })
	newGmailPubSubReceiver = func(context.Context, string, gmailPubSubReceiveSettings) (gmailPubSubReceiver, error) {
		t.Fatal("receiver should not be created")
		return &fakeGmailPubSubReceiver{}, nil
	}

	setWatchTestConfigHome(t)
	store := newGmailWatchTestStore(t, "a@b.com")
	if updateErr := store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{Account: "a@b.com", HistoryID: "100"}
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := runKong(t, &GmailWatchPullCmd{}, []string{"--subscription", "plain-sub"}, ctx, flags); err == nil {
		t.Fatalf("expected subscription validation error")
	}
	if err := runKong(t, &GmailWatchPullCmd{}, []string{"--subscription", "projects/p/subscriptions/s"}, ctx, flags); err == nil {
		t.Fatalf("expected missing hook error")
	}
}

func TestGmailWatchPullCmd_DryRunDoesNotCreateReceiverOrState(t *testing.T) {
	origReceiver := newGmailPubSubReceiver
	t.Cleanup(func() { newGmailPubSubReceiver = origReceiver })
	newGmailPubSubReceiver = func(context.Context, string, gmailPubSubReceiveSettings) (gmailPubSubReceiver, error) {
		t.Fatal("receiver should not be created during dry-run")
		return &fakeGmailPubSubReceiver{}, nil
	}

	setWatchTestConfigHome(t)
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	args := []string{
		"--subscription", "projects/p/subscriptions/s",
		"--fetch-delay", "0",
		"--history-types", "messageAdded",
		"--hook-url", "http://127.0.0.1:18789/hooks/gmail",
		"--hook-token", "secret",
		"--save-hook",
	}

	out := captureStdout(t, func() {
		err = runKong(t, &GmailWatchPullCmd{}, args, ctx, &RootFlags{
			Account: "a@b.com",
			DryRun:  true,
		})
	})
	if ExitCode(err) != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}

	var got struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			Account      string `json:"account"`
			Subscription string `json:"subscription"`
			HookURLSet   bool   `json:"hook_url_set"`
			HookTokenSet bool   `json:"hook_token_set"`
			SaveHook     bool   `json:"save_hook"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse dry-run JSON: %v\n%s", err, out)
	}
	if !got.DryRun ||
		got.Request.Account != "a@b.com" ||
		got.Request.Subscription != "projects/p/subscriptions/s" ||
		!got.Request.HookURLSet ||
		!got.Request.HookTokenSet ||
		!got.Request.SaveHook {
		t.Fatalf("unexpected dry-run payload: %#v", got)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("dry-run output leaked hook token: %s", out)
	}

	watchDir := os.Getenv("XDG_CONFIG_HOME")
	if _, err := os.Stat(watchDir); err == nil {
		t.Fatalf("dry-run created config directory %s", watchDir)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat config directory: %v", err)
	}
}

func TestGmailWatchPullCmd_ReadOnlyBlocksPubSubAck(t *testing.T) {
	ctx := googleapi.WithReadOnly(context.Background(), true)
	err := runKong(t, &GmailWatchPullCmd{}, nil, ctx, &RootFlags{ReadOnly: true})
	if !errors.Is(err, googleapi.ErrReadOnly) {
		t.Fatalf("error = %v, want ErrReadOnly", err)
	}
}

func TestGmailWatchPullMessage_AcksInvalidAndWrongAccount(t *testing.T) {
	server := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Account: "a@b.com"},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	invalid, invalidState := trackedPullMessage("m1", []byte("{"))
	server.handlePullMessage(context.Background(), invalid)
	if !invalidState.acked || invalidState.nacked {
		t.Fatalf("invalid payload ack=%v nack=%v", invalidState.acked, invalidState.nacked)
	}

	wrong, wrongState := trackedPullMessage("m2", []byte(`{"emailAddress":"other@example.com","historyId":"200"}`))
	server.handlePullMessage(context.Background(), wrong)
	if !wrongState.acked || wrongState.nacked {
		t.Fatalf("wrong account ack=%v nack=%v", wrongState.acked, wrongState.nacked)
	}
}

func TestGmailWatchPullMessage_NacksHookFailureAndPreservesProgress(t *testing.T) {
	server, hook, cleanup := newPullProcessorTestServer(t, http.StatusOK)
	defer cleanup()
	hook.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	msg, state := trackedPullMessage("m1", []byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	server.handlePullMessage(context.Background(), msg)
	if state.acked || !state.nacked {
		t.Fatalf("hook failure ack=%v nack=%v", state.acked, state.nacked)
	}
	if status := server.store.Get().LastDeliveryStatus; status != gmailWatchStatusHTTPError {
		t.Fatalf("delivery status = %q", status)
	}
	if historyID := server.store.Get().HistoryID; historyID != "100" {
		t.Fatalf("history id = %q", historyID)
	}
}

func TestGmailWatchPullMessage_RetriesHookFailureThenAcksSuccess(t *testing.T) {
	server, hook, cleanup := newPullProcessorTestServer(t, http.StatusOK)
	defer cleanup()

	hookStatus := http.StatusInternalServerError
	hookRequests := 0
	hook.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hookRequests++
		w.WriteHeader(hookStatus)
	})

	first, firstState := trackedPullMessage("m1", []byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	server.handlePullMessage(context.Background(), first)
	if firstState.acked || !firstState.nacked {
		t.Fatalf("first delivery ack=%v nack=%v", firstState.acked, firstState.nacked)
	}
	if historyID := server.store.Get().HistoryID; historyID != "100" {
		t.Fatalf("history id after failure = %q", historyID)
	}

	hookStatus = http.StatusNoContent
	second, secondState := trackedPullMessage("m1", []byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	server.handlePullMessage(context.Background(), second)
	if !secondState.acked || secondState.nacked {
		t.Fatalf("second delivery ack=%v nack=%v", secondState.acked, secondState.nacked)
	}
	if status := server.store.Get().LastDeliveryStatus; status != "ok" {
		t.Fatalf("delivery status after retry = %q", status)
	}
	if historyID := server.store.Get().HistoryID; historyID != "200" {
		t.Fatalf("history id after retry = %q", historyID)
	}
	if hookRequests != 2 {
		t.Fatalf("hook requests = %d", hookRequests)
	}
}

func TestGmailWatchPullMessage_NacksGmailFailure(t *testing.T) {
	server, _, cleanup := newPullProcessorTestServer(t, http.StatusInternalServerError)
	defer cleanup()

	msg, state := trackedPullMessage("m1", []byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	server.handlePullMessage(context.Background(), msg)
	if state.acked || !state.nacked {
		t.Fatalf("gmail failure ack=%v nack=%v", state.acked, state.nacked)
	}
}

type trackedPullMessageState struct {
	acked  bool
	nacked bool
}

func trackedPullMessage(id string, data []byte) (*gmailPubSubMessage, *trackedPullMessageState) {
	state := &trackedPullMessageState{}
	msg := &gmailPubSubMessage{
		ID:   id,
		Data: data,
		ack: func() {
			state.acked = true
		},
		nack: func() {
			state.nacked = true
		},
	}
	return msg, state
}

func newPullProcessorTestServer(t *testing.T, historyStatus int) (*gmailWatchServer, *httptest.Server, func()) {
	t.Helper()
	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
	if updateErr := store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{
			Account:   "a@b.com",
			HistoryID: "100",
		}
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	gmailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
			if historyStatus != http.StatusOK {
				w.WriteHeader(historyStatus)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId": "200",
				"history": []map[string]any{
					{"messagesAdded": []map[string]any{{"message": map[string]any{"id": "m1"}}}},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"snippet":  "hi",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"headers":  []map[string]any{{"name": "Subject", "value": "S"}},
					"mimeType": "text/plain",
					"body": map[string]any{
						"data": base64.RawURLEncoding.EncodeToString([]byte("body")),
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(gmailServer.Client()),
		option.WithEndpoint(gmailServer.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	hookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	processor := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:      "a@b.com",
			HookURL:      hookServer.URL,
			HookTimeout:  defaultHookRequestTimeoutSec * time.Second,
			HistoryMax:   100,
			ResyncMax:    10,
			FetchDelay:   0,
			MaxBodyBytes: defaultHookMaxBytes,
		},
		store:           store,
		newService:      func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient:      hookServer.Client(),
		excludeLabelIDs: map[string]struct{}{},
		logf:            func(string, ...any) {},
		warnf:           func(string, ...any) {},
	}
	cleanup := func() {
		gmailServer.Close()
		hookServer.Close()
		_ = os.Remove(store.Path())
	}
	return processor, hookServer, cleanup
}
