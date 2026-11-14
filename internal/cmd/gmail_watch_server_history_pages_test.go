package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/gmailwatch"
)

// A push whose history spans two pages must deliver the messages from both
// pages to the hook and only then store the mailbox's historyId.
func TestGmailWatchServer_DeliversHistoryBeyondFirstPage(t *testing.T) {
	setWatchTestConfigHome(t)
	store := newGmailWatchTestStore(t, "a@b.com")
	_ = store.Update(func(s *gmailWatchState) error {
		s.Account = "a@b.com"
		s.HistoryID = "100"
		return nil
	})

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/gmail/v1/users/me/history" && r.URL.Query().Get("pageToken") == "":
			_, _ = io.WriteString(w, `{"history":[{"id":"150","messagesAdded":[{"message":{"id":"m1"}}]}],"nextPageToken":"p2","historyId":"300"}`)
		case r.URL.Path == "/gmail/v1/users/me/history" && r.URL.Query().Get("pageToken") == "p2":
			_, _ = io.WriteString(w, `{"history":[{"id":"250","messagesAdded":[{"message":{"id":"m2"}}]}],"historyId":"300"}`)
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"):
			id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
			_, _ = io.WriteString(w, `{"id":"`+id+`","threadId":"t-`+id+`","payload":{"headers":[{"name":"Subject","value":"s"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	gsvc := newGmailServiceFromServer(t, api)

	var delivered gmailwatch.Payload
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&delivered)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(hook.Close)

	server := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:     "a@b.com",
			Path:        "/hook",
			SharedToken: "tok",
			HookURL:     hook.URL,
			HistoryMax:  1,
		},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return gsvc, nil },
		hookClient: hook.Client(),
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@b.com","historyId":"300"}`))
	body, _ := json.Marshal(push)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook?token=tok", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rr.Code, rr.Body.String())
	}

	ids := make([]string, 0, len(delivered.Messages))
	for _, m := range delivered.Messages {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "m1,m2" {
		t.Fatalf("hook got messages %v, want m1,m2", ids)
	}
	if got := store.Get().HistoryID; got != "300" {
		t.Fatalf("stored historyId = %q, want 300", got)
	}
}
