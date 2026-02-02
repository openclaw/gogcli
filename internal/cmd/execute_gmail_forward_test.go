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
			if unmarshalErr := json.Unmarshal(body, &msg); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
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

func TestExecute_GmailForward_HTMLOnlyMessage(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	htmlBody := "<p>HTML <strong>body</strong></p>"
	htmlEncoded := base64.RawURLEncoding.EncodeToString([]byte(htmlBody))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m2"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m2",
				"threadId": "t2",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "sender@example.com"},
						{"name": "To", "value": "you@example.com"},
						{"name": "Subject", "value": "HTML Email"},
						{"name": "Date", "value": "Wed, 17 Dec 2025 15:00:00 -0800"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/html",
							"body":     map[string]any{"data": htmlEncoded},
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
			if unmarshalErr := json.Unmarshal(body, &msg); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "Subject: Fwd: HTML Email\r\n") {
				t.Fatalf("missing forward subject in raw:\n%s", s)
			}
			if !strings.Contains(s, "-------- Forwarded message --------") {
				t.Fatalf("missing forwarded header in raw:\n%s", s)
			}
			if !strings.Contains(s, "HTML body") {
				t.Fatalf("missing stripped HTML content in plain part:\n%s", s)
			}
			if !strings.Contains(s, htmlBody) {
				t.Fatalf("missing original HTML in HTML part:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s2", "threadId": "t2"})
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
				"gmail", "forward", "m2", "to@example.com",
				"--body", "Check this out.",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_GmailForward_CustomSubject(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	bodyText := "Original body"
	bodyEncoded := base64.RawURLEncoding.EncodeToString([]byte(bodyText))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m3"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m3",
				"threadId": "t3",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "sender@example.com"},
						{"name": "To", "value": "you@example.com"},
						{"name": "Subject", "value": "Original Subject"},
						{"name": "Date", "value": "Wed, 17 Dec 2025 16:00:00 -0800"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": bodyEncoded},
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
			if unmarshalErr := json.Unmarshal(body, &msg); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "Subject: Custom Forward Subject\r\n") {
				t.Fatalf("missing custom subject in raw:\n%s", s)
			}
			if strings.Contains(s, "Subject: Fwd: Original Subject\r\n") {
				t.Fatalf("should not contain default Fwd: subject:\n%s", s)
			}
			if !strings.Contains(s, "-------- Forwarded message --------") {
				t.Fatalf("missing forwarded header in raw:\n%s", s)
			}
			if !strings.Contains(s, bodyText) {
				t.Fatalf("missing original body in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s3", "threadId": "t3"})
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
				"gmail", "forward", "m3", "to@example.com",
				"--subject", "Custom Forward Subject",
				"--body", "FYI.",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_GmailForward_CcAndBcc(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	bodyText := "Original body"
	bodyEncoded := base64.RawURLEncoding.EncodeToString([]byte(bodyText))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m4"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m4",
				"threadId": "t4",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "sender@example.com"},
						{"name": "To", "value": "you@example.com"},
						{"name": "Subject", "value": "Test Message"},
						{"name": "Date", "value": "Wed, 17 Dec 2025 17:00:00 -0800"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": bodyEncoded},
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
			if unmarshalErr := json.Unmarshal(body, &msg); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "To: to@example.com\r\n") {
				t.Fatalf("missing To header in raw:\n%s", s)
			}
			if !strings.Contains(s, "Cc: cc1@example.com, cc2@example.com\r\n") {
				t.Fatalf("missing Cc header in raw:\n%s", s)
			}
			if !strings.Contains(s, "Bcc: bcc@example.com\r\n") {
				t.Fatalf("missing Bcc header in raw:\n%s", s)
			}
			if !strings.Contains(s, "-------- Forwarded message --------") {
				t.Fatalf("missing forwarded header in raw:\n%s", s)
			}
			if !strings.Contains(s, bodyText) {
				t.Fatalf("missing original body in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s4", "threadId": "t4"})
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
				"gmail", "forward", "m4", "to@example.com",
				"--cc", "cc1@example.com,cc2@example.com",
				"--bcc", "bcc@example.com",
				"--body", "Important message.",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}
