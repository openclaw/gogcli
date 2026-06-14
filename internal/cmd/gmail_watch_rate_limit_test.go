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
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/gmailwatch"
)

func TestGmailWatchServer_History429OpensAccountCircuit(t *testing.T) {
	setWatchTestConfigHome(t)

	store := newRateLimitWatchStore(t)
	var historyCalls int
	gsvc := newRateLimitGmailService(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/history") {
			historyCalls++
			writeGmail429(t, w, "120")
			return
		}
		http.NotFound(w, r)
	})

	var serviceCalls int
	server := &gmailWatchServer{
		cfg:        gmailWatchServeConfig{Account: "a@b.com", HistoryMax: 10},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { serviceCalls++; return gsvc, nil },
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	_, err := server.handlePush(context.Background(), gmailPushPayload{EmailAddress: "a@b.com", HistoryID: "200"})
	var rateErr *gmailWatchRateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected rate limit error, got %v", err)
	}
	if historyCalls != 1 || serviceCalls != 1 {
		t.Fatalf("expected one Gmail call, got history=%d service=%d", historyCalls, serviceCalls)
	}
	state := store.Get()
	if state.RateLimitedUntilMs <= time.Now().UnixMilli() {
		t.Fatalf("expected future rate limit timestamp, got %d", state.RateLimitedUntilMs)
	}
	if state.HistoryID != "100" {
		t.Fatalf("history advanced under rate limit: %q", state.HistoryID)
	}

	_, err = server.handlePush(context.Background(), gmailPushPayload{EmailAddress: "a@b.com", HistoryID: "201"})
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected open circuit error, got %v", err)
	}
	if historyCalls != 1 || serviceCalls != 1 {
		t.Fatalf("open circuit made Gmail calls: history=%d service=%d", historyCalls, serviceCalls)
	}
}

func TestGmailWatchServer_Message429OpensAccountCircuit(t *testing.T) {
	setWatchTestConfigHome(t)

	store := newRateLimitWatchStore(t)
	var historyCalls, messageCalls int
	gsvc := newRateLimitGmailService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
			historyCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId": "200",
				"history": []map[string]any{
					{"messagesAdded": []map[string]any{{"message": map[string]any{"id": "m1"}}}},
				},
			})
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1"):
			messageCalls++
			writeGmail429(t, w, "60")
		default:
			http.NotFound(w, r)
		}
	})

	server := &gmailWatchServer{
		cfg:        gmailWatchServeConfig{Account: "a@b.com", HistoryMax: 10},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	_, err := server.handlePush(context.Background(), gmailPushPayload{EmailAddress: "a@b.com", HistoryID: "200"})
	var rateErr *gmailWatchRateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected rate limit error, got %v", err)
	}
	state := store.Get()
	if state.RateLimitedUntilMs <= time.Now().UnixMilli() {
		t.Fatalf("expected future rate limit timestamp, got %d", state.RateLimitedUntilMs)
	}
	if state.HistoryID != "100" {
		t.Fatalf("history advanced under rate limit: %q", state.HistoryID)
	}

	_, err = server.handlePush(context.Background(), gmailPushPayload{EmailAddress: "a@b.com", HistoryID: "201"})
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected open circuit error, got %v", err)
	}
	if historyCalls != 1 || messageCalls != 1 {
		t.Fatalf("open circuit made Gmail calls: history=%d message=%d", historyCalls, messageCalls)
	}
}

func TestGmailWatchServer_OpenCircuitReturnsRetryAfter(t *testing.T) {
	setWatchTestConfigHome(t)

	store := newRateLimitWatchStore(t)
	until := time.Now().Add(90 * time.Second).UnixMilli()
	if err := store.Update(func(s *gmailWatchState) error {
		s.RateLimitedUntilMs = until
		return nil
	}); err != nil {
		t.Fatalf("store update: %v", err)
	}

	serviceCalls := 0
	server := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Account: "a@b.com", Path: "/hook"},
		store: store,
		newService: func(context.Context, string) (*gmail.Service, error) {
			serviceCalls++
			return nil, errors.New("unexpected Gmail service call")
		},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	body, _ := json.Marshal(push)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook", bytes.NewReader(body))
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}
	if serviceCalls != 0 {
		t.Fatalf("open circuit created Gmail service")
	}
}

func TestUpdateStateAfterHistoryKeepsConcurrentRateLimitCircuit(t *testing.T) {
	until := time.Now().Add(time.Minute).UnixMilli()
	state := gmailWatchState{HistoryID: "100", RateLimitedUntilMs: until}
	if err := gmailwatch.AdvanceHistory(&state, "200", "push1", time.Now()); err != nil {
		t.Fatalf("update state: %v", err)
	}
	if state.RateLimitedUntilMs != until {
		t.Fatalf("rate limit circuit changed: got %d want %d", state.RateLimitedUntilMs, until)
	}
	if state.HistoryID != "200" {
		t.Fatalf("history not updated: %q", state.HistoryID)
	}
}

func newRateLimitWatchStore(t *testing.T) *gmailWatchStore {
	t.Helper()
	store := newGmailWatchTestStore(t, "a@b.com")
	if err := store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	}); err != nil {
		t.Fatalf("store update: %v", err)
	}
	return store
}

func newRateLimitGmailService(t *testing.T, handler http.HandlerFunc) *gmail.Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return gsvc
}

func writeGmail429(t *testing.T, w http.ResponseWriter, retryAfter string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", retryAfter)
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    http.StatusTooManyRequests,
			"message": "User-rate limit exceeded",
			"errors": []map[string]any{
				{"reason": "rateLimitExceeded", "message": "User-rate limit exceeded"},
			},
		},
	})
}
