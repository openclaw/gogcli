package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestExecute_GmailForward_DefaultSubjectAndAttachments(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	bodyText := "Original body"
	bodyEncoded := base64.RawURLEncoding.EncodeToString([]byte(bodyText))
	attData := []byte("hello")
	attEncoded := base64.RawURLEncoding.EncodeToString(attData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/a1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attEncoded})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "Sender <sender@example.com>"},
						{"name": "To", "value": "you@example.com"},
						{"name": "Subject", "value": "Hello"},
						{"name": "Date", "value": "Wed, 17 Dec 2025 14:00:00 -0800"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": bodyEncoded},
						},
						{
							"filename": "a.txt",
							"mimeType": "text/plain",
							"body":     map[string]any{"attachmentId": "a1", "size": len(attData)},
						},
					},
				},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/send"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var msg gmail.Message
			if err := json.Unmarshal(body, &msg); err != nil {
				t.Fatalf("unmarshal: %v body=%q", err, string(body))
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "Subject: Fwd: Hello\r\n") {
				t.Fatalf("missing forward subject in raw:\n%s", s)
			}
			if !strings.Contains(s, "-------- Forwarded message --------") {
				t.Fatalf("missing forwarded header in raw:\n%s", s)
			}
			if !strings.Contains(s, "Subject: Hello") {
				t.Fatalf("missing original subject in forwarded header:\n%s", s)
			}
			if !strings.Contains(s, "From: Sender <sender@example.com>") {
				t.Fatalf("missing original From in forwarded header:\n%s", s)
			}
			if !strings.Contains(s, "Forwarding note.") {
				t.Fatalf("missing preface body in raw:\n%s", s)
			}
			prefaceIndex := strings.Index(s, "Forwarding note.")
			forwardIndex := strings.Index(s, "-------- Forwarded message --------")
			if prefaceIndex == -1 || forwardIndex == -1 || prefaceIndex > forwardIndex {
				t.Fatalf("expected preface before forwarded header:\n%s", s)
			}
			if !strings.Contains(s, bodyText) {
				t.Fatalf("missing original body in raw:\n%s", s)
			}
			if !strings.Contains(s, "Content-Disposition: attachment; filename=\"a.txt\"") {
				t.Fatalf("missing attachment filename in raw:\n%s", s)
			}
			if !strings.Contains(s, base64.StdEncoding.EncodeToString(attData)) {
				t.Fatalf("missing attachment data in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s1", "threadId": "t1"})
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

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"gmail", "forward", "m1", "to@example.com",
				"--body", "Forwarding note.",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}
