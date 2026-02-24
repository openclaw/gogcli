package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestGmailWatchServer_ServeHTTP_ExcludeLabels_SkipsHook(t *testing.T) {
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

	var hookCalls int
	hookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hookCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer hookSrv.Close()

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
				"snippet":  "spam",
				"labelIds": []string{"SPAM"},
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "Subject", "value": "S"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
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

	s := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:     "a@b.com",
			Path:        "/gmail-pubsub",
			SharedToken: "tok",
			HookURL:     hookSrv.URL,
			HistoryMax:  100,
			ResyncMax:   10,
		},
		store:           store,
		newService:      func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient:      hookSrv.Client(),
		excludeLabelIDs: map[string]struct{}{"SPAM": {}},
		logf:            func(string, ...any) {},
		warnf:           func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@b.com","historyId":"200"}`))
	body, _ := json.Marshal(push)

	req := httptest.NewRequest(http.MethodPost, "/gmail-pubsub?token=tok", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: %d body=%q", rr.Code, rr.Body.String())
	}
	if hookCalls != 0 {
		t.Fatalf("expected no hook calls, got %d", hookCalls)
	}

	st := store.Get()
	if st.HistoryID != "200" {
		t.Fatalf("expected history updated, got %q", st.HistoryID)
	}
}

func TestGmailWatchServer_isExcludedLabel_CaseSensitive(t *testing.T) {
	s := &gmailWatchServer{excludeLabelIDs: map[string]struct{}{"Label_ABC": {}}}
	if !s.isExcludedLabel([]string{"Label_ABC"}) {
		t.Fatalf("expected exact case label to match")
	}
	if s.isExcludedLabel([]string{"label_abc"}) {
		t.Fatalf("expected case-mismatched label not to match")
	}
}
