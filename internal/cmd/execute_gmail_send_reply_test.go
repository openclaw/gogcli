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

func TestExecute_GmailSend_ReplyToHeader(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
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
			if !strings.Contains(string(raw), "Reply-To: reply@example.com\r\n") {
				t.Fatalf("missing Reply-To header in raw:\n%s", string(raw))
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
				"gmail", "send",
				"--to", "x@y.com",
				"--subject", "S",
				"--body", "B",
				"--reply-to", "reply@example.com",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_GmailSend_ReplyToMessageID(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m0"):
			if got := r.URL.Query().Get("format"); got != "metadata" {
				t.Fatalf("format=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m0",
				"threadId": "t0",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<orig@id>"},
						{"name": "References", "value": "<ref0>"},
						{"name": "Subject", "value": "Original Subject"},
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
			if msg.ThreadId != "t0" {
				t.Fatalf("expected threadId=t0, got: %q", msg.ThreadId)
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "In-Reply-To: <orig@id>\r\n") {
				t.Fatalf("missing In-Reply-To in raw:\n%s", s)
			}
			if !strings.Contains(s, "References: <ref0> <orig@id>\r\n") {
				t.Fatalf("missing References in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s1", "threadId": "t0"})
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
				"gmail", "send",
				"--to", "x@y.com",
				"--subject", "S",
				"--body", "B",
				"--reply-to-message-id", "m0",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_GmailDraftsCreate_ReplyToMessageID(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m0"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m0",
				"threadId": "t0",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<orig@id>"},
						{"name": "References", "value": "<ref0>"},
						{"name": "Subject", "value": "Original Subject"},
					},
				},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in draft")
			}
			if draft.Message.ThreadId != "t0" {
				t.Fatalf("expected threadId=t0, got: %q", draft.Message.ThreadId)
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "In-Reply-To: <orig@id>\r\n") {
				t.Fatalf("missing In-Reply-To in raw:\n%s", s)
			}
			if !strings.Contains(s, "References: <ref0> <orig@id>\r\n") {
				t.Fatalf("missing References in raw:\n%s", s)
			}
			if !strings.Contains(s, "Subject: Re: Original Subject\r\n") {
				t.Fatalf("missing auto reply subject in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m1", "threadId": "t0"},
			})
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
				"gmail", "drafts", "create",
				"--to", "x@y.com",
				"--body", "B",
				"--reply-to-message-id", "m0",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}
