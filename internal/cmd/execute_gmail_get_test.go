package cmd

import (
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

func TestExecute_GmailGet_Metadata_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("format"); got != "metadata" {
			t.Fatalf("format=%q", got)
		}
		gotHeaders := r.URL.Query()["metadataHeaders"]
		if len(gotHeaders) != 3 || !containsAll(gotHeaders, []string{"Subject", "Date", "List-Unsubscribe"}) {
			t.Fatalf("metadataHeaders=%#v", gotHeaders)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"headers": []map[string]any{
					{"name": "Subject", "value": "Hello"},
					{"name": "Date", "value": "Wed, 17 Dec 2025 14:00:00 -0800"},
				},
			},
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"gmail", "get", "m1",
				"--format", "metadata",
				"--headers", "Subject,Date",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Message struct {
			ID      string   `json:"id"`
			Thread  string   `json:"threadId"`
			LabelID []string `json:"labelIds"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Message.ID != "m1" || parsed.Message.Thread != "t1" || len(parsed.Message.LabelID) != 1 || parsed.Message.LabelID[0] != "INBOX" {
		t.Fatalf("unexpected: %#v", parsed)
	}
}

func containsAll(got []string, want []string) bool {
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func TestExecute_GmailGet_Raw_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	raw := "Subject: hi\r\n\r\nbody"
	rawEncoded := base64.RawURLEncoding.EncodeToString([]byte(raw))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("format"); got != "raw" {
			t.Fatalf("format=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":  "m1",
			"raw": rawEncoded,
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"gmail", "get", "m1",
				"--format", "raw",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Message struct {
			ID  string `json:"id"`
			Raw string `json:"raw"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Message.ID != "m1" || parsed.Message.Raw != rawEncoded {
		t.Fatalf("unexpected: %#v", parsed)
	}
}

func TestExecute_GmailGet_InvalidFormat(t *testing.T) {
	_ = captureStderr(t, func() {
		err := Execute([]string{
			"--account", "a@b.com",
			"gmail", "get", "m1",
			"--format", "nope",
		})
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "invalid --format") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestExecute_GmailGet_Metadata_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("format"); got != "metadata" {
			t.Fatalf("format=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"headers": []map[string]any{
					{"name": "From", "value": "Me <me@example.com>"},
					{"name": "Subject", "value": "Hello"},
				},
			},
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"gmail", "get", "m1",
				"--format", "metadata",
				"--headers", "From,Subject",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "id\tm1") || !strings.Contains(out, "subject\tHello") {
		t.Fatalf("unexpected out=%q", out)
	}
}
