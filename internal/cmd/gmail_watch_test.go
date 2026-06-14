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
	"time"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/gmailwatch"
)

func TestGmailWatchStartCmd_JSON(t *testing.T) {
	setWatchTestConfigHome(t)

	var watchReq struct {
		TopicName string   `json:"topicName"`
		LabelIds  []string `json:"labelIds"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX"},
					{"id": "Label_1", "name": "Custom"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/watch"):
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &watchReq)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId":  "123",
				"expiration": "1730000000000",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)

	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	if execErr := runKong(t, &GmailWatchStartCmd{}, []string{
		"--topic", "projects/p/topics/t",
		"--label", "INBOX",
		"--label", "Custom",
		"--hook-url", "http://127.0.0.1:1/hooks",
		"--hook-token", "tok",
		"--include-body",
		"--max-bytes", "5",
	}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	if watchReq.TopicName != "projects/p/topics/t" {
		t.Fatalf("unexpected topic: %#v", watchReq)
	}
	if len(watchReq.LabelIds) != 2 || watchReq.LabelIds[0] != "INBOX" || watchReq.LabelIds[1] != "Label_1" {
		t.Fatalf("unexpected labels: %#v", watchReq.LabelIds)
	}

	var parsed struct {
		Watch gmailWatchState `json:"watch"`
	}
	if parseErr := json.Unmarshal(out.Bytes(), &parsed); parseErr != nil {
		t.Fatalf("json parse: %v", parseErr)
	}
	if parsed.Watch.HistoryID != "123" {
		t.Fatalf("unexpected history: %#v", parsed.Watch)
	}
	if parsed.Watch.Hook == nil || parsed.Watch.Hook.URL == "" || !parsed.Watch.Hook.IncludeBody {
		t.Fatalf("missing hook: %#v", parsed.Watch.Hook)
	}
	if parsed.Watch.Hook.MaxBytes != 5 {
		t.Fatalf("unexpected max bytes: %#v", parsed.Watch.Hook)
	}

	store := loadGmailWatchTestStore(t, "a@b.com")
	if store.Get().HistoryID != "123" {
		t.Fatalf("store missing history: %#v", store.Get())
	}
}

func TestGmailWatchServerServeHTTP_TruncateBody(t *testing.T) {
	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "me@example.com")
	if updateErr := store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{Account: "me@example.com", HistoryID: "100"}
		return nil
	}); updateErr != nil {
		t.Fatalf("store update: %v", updateErr)
	}

	bodyEncoded := base64.RawURLEncoding.EncodeToString([]byte("hello world"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/history"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"history": []map[string]any{
					{
						"id":            "1",
						"messagesAdded": []map[string]any{{"message": map[string]any{"id": "m1"}}},
					},
				},
				"historyId": "250",
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"snippet":  "snippet",
				"payload": map[string]any{
					"mimeType": "text/plain",
					"body":     map[string]any{"data": bodyEncoded},
					"headers": []map[string]any{
						{"name": "From", "value": "From <from@example.com>"},
						{"name": "To", "value": "To <to@example.com>"},
						{"name": "Subject", "value": "Hi"},
						{"name": "Date", "value": "Wed, 17 Dec 2025 14:00:00 -0800"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)

	hookServer := &gmailWatchServer{
		cfg: gmailWatchServeConfig{
			Account:      "me@example.com",
			Path:         "/gmail-pubsub",
			SharedToken:  "token",
			IncludeBody:  true,
			MaxBodyBytes: 5,
			HistoryMax:   defaultHistoryMaxResults,
			ResyncMax:    defaultHistoryResyncMax,
			AllowNoHook:  true,
		},
		store:      store,
		newService: func(context.Context, string) (*gmail.Service, error) { return svc, nil },
		hookClient: &http.Client{Timeout: time.Second},
		logf:       func(string, ...any) {},
		warnf:      func(string, ...any) {},
	}

	payload, _ := json.Marshal(gmailPushPayload{EmailAddress: "me@example.com", HistoryID: "200"})
	env := pubsubPushEnvelope{}
	env.Message.Data = base64.StdEncoding.EncodeToString(payload)

	data, _ := json.Marshal(env)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/gmail-pubsub", bytes.NewReader(data))
	req.Header.Set("x-gog-token", "token")
	rec := httptest.NewRecorder()

	hookServer.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK {
		body, _ := io.ReadAll(rec.Result().Body)
		t.Fatalf("unexpected status: %d body=%q", rec.Result().StatusCode, string(body))
	}

	var parsed gmailHookPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("unexpected messages: %#v", parsed.Messages)
	}
	msg := parsed.Messages[0]
	if msg.Body != "hello" || !msg.BodyTruncated {
		t.Fatalf("unexpected body: %#v", msg)
	}
}

func TestDecodeGmailPushPayload_NumberHistoryID(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"emailAddress": "a@b.com",
		"historyId":    1234,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	env := &pubsubPushEnvelope{}
	env.Message.Data = base64.StdEncoding.EncodeToString(payload)
	got, err := gmailwatch.DecodePushPayload(env)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.HistoryID != "1234" {
		t.Fatalf("unexpected history id: %q", got.HistoryID)
	}
}

func TestDecodeGmailPushPayload_StringHistoryID(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"emailAddress": "a@b.com",
		"historyId":    "5678",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	env := &pubsubPushEnvelope{}
	env.Message.Data = base64.StdEncoding.EncodeToString(payload)
	got, err := gmailwatch.DecodePushPayload(env)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.HistoryID != "5678" {
		t.Fatalf("unexpected history id: %q", got.HistoryID)
	}
}

func TestDecodeGmailPushPayload_InvalidHistoryID(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"emailAddress": "a@b.com",
		"historyId":    map[string]any{"bad": "value"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	env := &pubsubPushEnvelope{}
	env.Message.Data = base64.StdEncoding.EncodeToString(payload)
	if _, err := gmailwatch.DecodePushPayload(env); err == nil {
		t.Fatalf("expected error")
	}
}
