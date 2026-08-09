package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func gmailSearchTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, "/users/me/threads") && !strings.Contains(path, "/users/me/threads/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{{"id": "t1"}}, "nextPageToken": "npt",
			})
		case strings.Contains(path, "/users/me/threads/t1"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1", "messages": []map[string]any{{
					"id": "m1", "labelIds": []string{"INBOX"},
					"payload": map[string]any{"headers": []map[string]any{
						{"name": "From", "value": "Me <me@example.com>"},
						{"name": "Subject", "value": "Hello"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
					}},
				}},
			})
		case strings.Contains(path, "/users/me/labels"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{{"id": "INBOX", "name": "INBOX", "type": "system"}},
			})
		default:
			http.NotFound(w, r)
		}
	})
}

func newGmailEmptyListTestService(t *testing.T, path, key string) *gmail.Service {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, path) || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{key: []map[string]any{}})
	})
	svc, closeServer := newGoogleTestService(t, handler, gmail.NewService)
	t.Cleanup(closeServer)
	return svc
}

type gmailTestHeader struct {
	Name  string
	Value string
}

type gmailTestMessage struct {
	ThreadID string
	Headers  []gmailTestHeader
}

func newGmailMessagesTestService(t *testing.T, messages map[string]gmailTestMessage) *gmail.Service {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
		message, ok := messages[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		headers := make([]map[string]any, 0, len(message.Headers))
		for _, header := range message.Headers {
			headers = append(headers, map[string]any{"name": header.Name, "value": header.Value})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "threadId": message.ThreadID, "payload": map[string]any{"headers": headers},
		})
	})
	svc, closeServer := newGoogleTestService(t, handler, gmail.NewService)
	t.Cleanup(closeServer)
	return svc
}

func newGmailServiceForTest(t *testing.T, h http.HandlerFunc) (*gmail.Service, func()) {
	t.Helper()

	return newGoogleTestService(t, h, gmail.NewService)
}

func newGmailServiceFromServer(t *testing.T, srv *httptest.Server) *gmail.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", gmail.NewService)
}

func withGmailTestService(ctx context.Context, svc *gmail.Service) context.Context {
	return withGmailTestServiceFactory(ctx, func(context.Context, string) (*gmail.Service, error) {
		return svc, nil
	})
}

func withGmailTestServiceFactory(ctx context.Context, factory app.GmailServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Gmail = factory
	return app.WithRuntime(ctx, runtime)
}

func executeWithGmailTestService(t *testing.T, args []string, svc *gmail.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Gmail: func(context.Context, string) (*gmail.Service, error) { return svc, nil },
	}})
}
