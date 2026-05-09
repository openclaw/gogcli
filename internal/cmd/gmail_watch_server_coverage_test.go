package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGmailWatchServer_SendHook_TransportError(t *testing.T) {
	setWatchTestConfigHome(t)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error { s.Account = "a@b.com"; return nil })

	server := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account: "a@b.com",
			HookURL: "https://example.com/hook",
		},
		store: store,
		hookClient: &http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			}),
		},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	err = server.sendHook(context.Background(), &gmailHookPayload{Source: "gmail", Account: "a@b.com", HistoryID: "1"})
	if err == nil || !strings.Contains(err.Error(), "dial failed") {
		t.Fatalf("expected transport error, got: %v", err)
	}
	state := store.Get()
	if state.LastDeliveryStatus != "error" {
		t.Fatalf("unexpected status: %q", state.LastDeliveryStatus)
	}
	if !strings.Contains(state.LastDeliveryStatusNote, "dial failed") {
		t.Fatalf("unexpected note: %q", state.LastDeliveryStatusNote)
	}
}

func TestGmailWatchServer_ServeHTTP_HandlePushError(t *testing.T) {
	server := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Path: "/hook", SharedToken: "tok"},
		store: &gmailWatchStore{state: gmailWatchState{HistoryID: "bad"}},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"historyId":"200"}`))
	body, _ := json.Marshal(push)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook?token=tok", bytes.NewReader(body))
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestGmailWatchServer_ServeHTTP_NoHook_Accepted(t *testing.T) {
	setWatchTestConfigHome(t)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	})

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
				"payload":  map[string]any{"headers": []map[string]any{}},
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
			Account:     "a@b.com",
			Path:        "/hook",
			SharedToken: "tok",
			HistoryMax:  10,
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

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook?token=tok", bytes.NewReader(body))
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestGmailWatchServer_ServeHTTP_HookSuccess(t *testing.T) {
	setWatchTestConfigHome(t)

	store, err := newGmailWatchStore("a@b.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	})

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
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hookSrv.Close()

	server := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:     "a@b.com",
			Path:        "/hook",
			SharedToken: "tok",
			HookURL:     hookSrv.URL,
			HistoryMax:  10,
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
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook?token=tok", bytes.NewReader(body))
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestGmailWatchServer_HandlePush_NewServiceError(t *testing.T) {
	store := &gmailWatchStore{state: gmailWatchState{HistoryID: "100"}}
	server := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Account: "a@b.com"},
		store: store,
		newService: func(context.Context, string) (*gmail.Service, error) {
			return nil, errors.New("service down")
		},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	if _, err := server.handlePush(context.Background(), gmailPushPayload{HistoryID: "200"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchServer_HandlePush_HistoryError(t *testing.T) {
	store := &gmailWatchStore{state: gmailWatchState{HistoryID: "100"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/history") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": http.StatusInternalServerError, "message": "oops"},
			})
			return
		}
		http.NotFound(w, r)
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
		cfg:        gmailWatchServeConfig{Account: "a@b.com", HistoryMax: 10},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	if _, err := server.handlePush(context.Background(), gmailPushPayload{HistoryID: "200"}); err == nil {
		t.Fatalf("expected history error")
	}
}

func TestGmailWatchServer_HandlePush_StaleHistory(t *testing.T) {
	server := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Account: "a@b.com"},
		store: &gmailWatchStore{state: gmailWatchState{HistoryID: "100"}},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	if _, err := server.handlePush(context.Background(), gmailPushPayload{HistoryID: "99"}); !errors.Is(err, errNoNewMessages) {
		t.Fatalf("expected no new messages, got %v", err)
	}
}

func TestGmailWatchServer_HandlePush_FetchMessagesError(t *testing.T) {
	store := &gmailWatchStore{state: gmailWatchState{HistoryID: "100"}}

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

	server := &gmailWatchServer{
		cfg:        gmailWatchServeConfig{Account: "a@b.com", HistoryMax: 10},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	if _, err := server.handlePush(context.Background(), gmailPushPayload{HistoryID: "200"}); err == nil {
		t.Fatalf("expected fetch error")
	}
}

func TestGmailWatchServer_HandlePush_UpdateError(t *testing.T) {
	store := &gmailWatchStore{state: gmailWatchState{HistoryID: "100"}}

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
				"payload":  map[string]any{"headers": []map[string]any{}},
			})
			return
		}
		http.NotFound(w, r)
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
		cfg:        gmailWatchServeConfig{Account: "a@b.com", HistoryMax: 10},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	got, err := server.handlePush(context.Background(), gmailPushPayload{HistoryID: "200"})
	if err != nil {
		t.Fatalf("handlePush: %v", err)
	}
	if got == nil || got.HistoryID == "" {
		t.Fatalf("expected payload")
	}
}

func TestGmailWatchServer_HandlePush_UpdateError_InvalidHistoryID(t *testing.T) {
	store := &gmailWatchStore{state: gmailWatchState{HistoryID: "100"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/history") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId": "0",
				"history":   []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
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
		cfg:        gmailWatchServeConfig{Account: "a@b.com", HistoryMax: 10},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	got, err := server.handlePush(context.Background(), gmailPushPayload{HistoryID: "bad"})
	if err != nil {
		t.Fatalf("handlePush: %v", err)
	}
	if got == nil {
		t.Fatalf("expected payload")
	}
}

func TestGmailWatchServer_ResyncHistory_ListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": http.StatusInternalServerError, "message": "boom"},
			})
			return
		}
		http.NotFound(w, r)
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
		cfg:   gmailWatchServeConfig{ResyncMax: 10},
		store: &gmailWatchStore{},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	if _, err := server.resyncHistory(context.Background(), gsvc, "200", ""); err == nil {
		t.Fatalf("expected resync error")
	}
}

func TestGmailWatchServer_ResyncHistory_FetchMessagesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusInternalServerError)
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{{"id": "m1"}},
			})
			return
		}
		http.NotFound(w, r)
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
		cfg:   gmailWatchServeConfig{ResyncMax: 10},
		store: &gmailWatchStore{},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	if _, err := server.resyncHistory(context.Background(), gsvc, "200", ""); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchServer_ResyncHistory_UpdateError_InvalidHistoryID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{{"id": "m1"}},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload":  map[string]any{"headers": []map[string]any{}},
			})
			return
		}
		http.NotFound(w, r)
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
		cfg:   gmailWatchServeConfig{Account: "a@b.com", ResyncMax: 10},
		store: &gmailWatchStore{state: gmailWatchState{HistoryID: "100"}},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	if _, err := server.resyncHistory(context.Background(), gsvc, "bad", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("read error") }
func (errReadCloser) Close() error             { return nil }

func TestParsePubSubPush_ReadError(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", errReadCloser{})
	if _, err := parsePubSubPush(req); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGmailWatchServer_OIDCAudience_Explicit(t *testing.T) {
	s := &gmailWatchServer{cfg: gmailWatchServeConfig{OIDCAudience: "https://example.com/hook"}}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://ignored/hook", nil)
	if got := s.oidcAudience(r); got != "https://example.com/hook" {
		t.Fatalf("unexpected audience: %q", got)
	}
}
