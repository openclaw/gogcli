package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailWatchServer_ServeHTTP_AllowNoHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// Seed state so StartHistoryID returns non-zero.
	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
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
					"headers": []map[string]any{
						{"name": "From", "value": "a@example.com"},
						{"name": "To", "value": "b@example.com"},
						{"name": "Subject", "value": "S"},
						{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
					},
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
	defer srv.Close()

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	s := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:      "a@b.com",
			Path:         "/gmail-pubsub",
			SharedToken:  "tok",
			AllowNoHook:  true,
			IncludeBody:  true,
			MaxBodyBytes: 10,
			HistoryMax:   100,
			ResyncMax:    10,
		},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient: srv.Client(),
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	body, _ := json.Marshal(push)

	req := httptest.NewRequest(http.MethodPost, "/gmail-pubsub?token=tok", bytes.NewReader(body))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%q", rr.Code, rr.Body.String())
	}

	var got gmailHookPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if got.Account != "a@b.com" || got.Source != "gmail" || got.HistoryID == "" || len(got.Messages) != 1 {
		t.Fatalf("unexpected payload: %#v", got)
	}
	if got.Messages[0].Body == "" {
		t.Fatalf("expected body")
	}

	// State updated.
	st := store.Get()
	if st.HistoryID != "200" {
		t.Fatalf("expected history updated, got %q", st.HistoryID)
	}
}

func TestGmailWatchServer_ServeHTTP_HistoryTypes_NoMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// Seed state so StartHistoryID returns non-zero.
	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId": "200",
				"history":   []map[string]any{},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	s := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:      "a@b.com",
			Path:         "/gmail-pubsub",
			SharedToken:  "tok",
			AllowNoHook:  true,
			HistoryMax:   100,
			ResyncMax:    10,
			HistoryTypes: []string{"messageAdded"},
		},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient: srv.Client(),
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	body, _ := json.Marshal(push)

	req := httptest.NewRequest(http.MethodPost, "/gmail-pubsub?token=tok", bytes.NewReader(body))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: %d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", rr.Body.String())
	}

	st := store.Get()
	if st.HistoryID != "200" {
		t.Fatalf("expected history updated, got %q", st.HistoryID)
	}
}

func TestGmailWatchServer_ServeHTTP_HistoryTypes_DeletedOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	var messageGetCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId": "200",
				"history": []map[string]any{
					{
						"messagesDeleted": []map[string]any{
							{"message": map[string]any{"id": "m1"}},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1"):
			messageGetCalled = true
			w.WriteHeader(http.StatusInternalServerError)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	s := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:      "a@b.com",
			Path:         "/gmail-pubsub",
			SharedToken:  "tok",
			AllowNoHook:  true,
			HistoryMax:   100,
			ResyncMax:    10,
			HistoryTypes: []string{"messageDeleted"},
		},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient: srv.Client(),
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	body, _ := json.Marshal(push)

	req := httptest.NewRequest(http.MethodPost, "/gmail-pubsub?token=tok", bytes.NewReader(body))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%q", rr.Code, rr.Body.String())
	}

	var got gmailHookPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if got.HistoryID != "200" {
		t.Fatalf("expected historyId 200, got %q", got.HistoryID)
	}
	if len(got.Messages) != 0 {
		t.Fatalf("expected no fetched messages, got: %#v", got.Messages)
	}
	if len(got.DeletedMessageIDs) != 1 || got.DeletedMessageIDs[0] != "m1" {
		t.Fatalf("unexpected deleted ids: %#v", got.DeletedMessageIDs)
	}
	if messageGetCalled {
		t.Fatalf("deleted-only history should not fetch deleted message bodies")
	}

	st := store.Get()
	if st.HistoryID != "200" {
		t.Fatalf("expected history updated, got %q", st.HistoryID)
	}
}

func TestGmailWatchHelpers(t *testing.T) {
	if got := bearerToken(&http.Request{Header: http.Header{"Authorization": []string{"Bearer tok"}}}); got != "tok" {
		t.Fatalf("bearer: %q", got)
	}
	r := httptest.NewRequest(http.MethodPost, "/x?token=q", nil)
	r.Header.Set("x-gog-token", "h")
	if !sharedTokenMatches(r, "h") {
		t.Fatalf("expected shared token match")
	}
	if !pathMatches("/x/", "/x/y") || !pathMatches("/x", "/x/y") {
		t.Fatalf("pathMatches")
	}

	got, truncated := truncateUTF8Bytes("héllö", 3)
	if got == "" || !truncated {
		t.Fatalf("truncate: %q %v", got, truncated)
	}

	if d, err := parseDurationSeconds("5"); err != nil || d != 5*time.Second {
		t.Fatalf("duration: %v %v", d, err)
	}
}

func TestGmailWatchServer_OIDCAudience(t *testing.T) {
	s := &gmailWatchServer{
		cfg: gmailWatchServeConfig{OIDCAudience: ""},
	}
	r := httptest.NewRequest(http.MethodPost, "https://example.com/x", nil)
	r.Host = "example.com"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "proxy.example.com")
	if got := s.oidcAudience(r); got != "https://proxy.example.com/x" {
		t.Fatalf("unexpected audience: %q", got)
	}
}

func TestGmailWatchServer_ResyncHistory_OnStaleError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    http.StatusNotFound,
					"message": "HistoryId not found",
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"id": "m1"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"snippet":  "hi",
				"payload": map[string]any{
					"headers": []map[string]any{{"name": "Subject", "value": "S"}},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	server := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:    "a@b.com",
			HistoryMax: 10,
			ResyncMax:  10,
		},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient: srv.Client(),
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	got, err := server.handlePush(context.Background(), gmailPushPayload{EmailAddress: "a@b.com", HistoryID: "200"})
	if err != nil {
		t.Fatalf("handlePush: %v", err)
	}
	if got == nil || got.HistoryID != "200" || len(got.Messages) != 1 {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestGmailWatchServer_HandlePush_DuplicateMessageID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		s.LastPushMessageID = "dup"
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	server := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Account: "a@b.com"},
		store: store,
		newService: func(context.Context, string) (*gmail.Service, error) {
			t.Fatalf("unexpected service call")
			return nil, errors.New("unexpected service call")
		},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	_, err = server.handlePush(context.Background(), gmailPushPayload{
		EmailAddress: "a@b.com",
		HistoryID:    "200",
		MessageID:    "dup",
	})
	if err == nil || !errors.Is(err, errNoNewMessages) {
		t.Fatalf("expected no new messages, got %v", err)
	}
	if store.Get().LastPushMessageID != "dup" {
		t.Fatalf("expected last push unchanged")
	}
}

func TestGmailWatchServer_HandlePush_SkipsMissingMessages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId": "200",
				"history": []map[string]any{
					{"messagesAdded": []map[string]any{
						{"message": map[string]any{"id": "m1"}},
						{"message": map[string]any{"id": "m2"}},
					}},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"snippet":  "hi",
				"payload":  map[string]any{"headers": []map[string]any{{"name": "Subject", "value": "S"}}},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m2"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    http.StatusNotFound,
					"message": "Requested entity was not found.",
					"errors": []map[string]any{
						{"reason": "notFound", "message": "Requested entity was not found."},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	server := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:    "a@b.com",
			HistoryMax: 10,
			ResyncMax:  10,
		},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient: srv.Client(),
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	got, err := server.handlePush(context.Background(), gmailPushPayload{EmailAddress: "a@b.com", HistoryID: "200"})
	if err != nil {
		t.Fatalf("handlePush: %v", err)
	}
	if got == nil || len(got.Messages) != 1 || got.Messages[0].ID != "m1" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestGmailWatchServer_SendHook_UpdatesState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error { s.Account = "a@b.com"; return nil })

	var calls int
	hookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hookSrv.Close()

	server := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:   "a@b.com",
			HookURL:   hookSrv.URL,
			HookToken: "tok",
		},
		store:      store,
		hookClient: hookSrv.Client(),
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	err = server.sendHook(context.Background(), &gmailHookPayload{Source: "gmail", Account: "a@b.com", HistoryID: "1"})
	if err == nil || !strings.Contains(err.Error(), "hook status") {
		t.Fatalf("expected http error, got: %v", err)
	}
	if store.Get().LastDeliveryStatus != "http_error" {
		t.Fatalf("unexpected state: %#v", store.Get())
	}

	if err := server.sendHook(context.Background(), &gmailHookPayload{Source: "gmail", Account: "a@b.com", HistoryID: "1"}); err != nil {
		t.Fatalf("expected ok, got: %v", err)
	}
	if store.Get().LastDeliveryStatus != "ok" {
		t.Fatalf("unexpected state: %#v", store.Get())
	}
}

func TestGmailWatchServer_ServeHTTP_HookError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	}); updateErr != nil {
		t.Fatalf("seed: %v", updateErr)
	}

	gmailSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
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
				"payload":  map[string]any{"headers": []map[string]any{}},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer gmailSrv.Close()

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(gmailSrv.Client()),
		option.WithEndpoint(gmailSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	hookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer hookSrv.Close()

	server := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:    "a@b.com",
			Path:       "/hook",
			HookURL:    hookSrv.URL,
			HistoryMax: 10,
			ResyncMax:  10,
		},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient: hookSrv.Client(),
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	body, _ := json.Marshal(push)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if store.Get().LastDeliveryStatus != "http_error" {
		t.Fatalf("unexpected state: %#v", store.Get())
	}
}

func TestIsStaleHistoryError(t *testing.T) {
	err := &gapi.Error{Code: http.StatusNotFound, Message: "HistoryId not found"}
	if !isStaleHistoryError(err) {
		t.Fatalf("expected stale history")
	}
}

func TestVerifyOIDCToken_NoValidator(t *testing.T) {
	ok, err := verifyOIDCToken(context.Background(), nil, "tok", "aud", "")
	if ok || err == nil || !strings.Contains(err.Error(), "no OIDC validator") {
		t.Fatalf("unexpected: ok=%v err=%v", ok, err)
	}
}

func TestFormatUnixMillis(t *testing.T) {
	if got := formatUnixMillis(0); got != "" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := formatUnixMillis(1730000000000); strings.TrimSpace(got) == "" {
		t.Fatalf("unexpected: %q", got)
	}
}
