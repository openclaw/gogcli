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
			t.Errorf("format=%q", got)
			http.Error(w, "bad format", http.StatusBadRequest)
			return
		}
		gotHeaders := r.URL.Query()["metadataHeaders"]
		if len(gotHeaders) != 3 || !containsAll(gotHeaders, []string{"Subject", "Date", "List-Unsubscribe"}) {
			t.Errorf("metadataHeaders=%#v", gotHeaders)
			http.Error(w, "bad metadataHeaders", http.StatusBadRequest)
			return
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

func TestExecute_GmailGet_Metadata_DefaultHeadersIncludeThreading(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("format"); got != "metadata" {
			t.Errorf("format=%q", got)
			http.Error(w, "bad format", http.StatusBadRequest)
			return
		}
		want := []string{
			"From", "To", "Cc", "Bcc", "Subject", "Date",
			"Message-ID", "In-Reply-To", "References", "List-Unsubscribe",
		}
		if gotHeaders := r.URL.Query()["metadataHeaders"]; !containsAll(gotHeaders, want) {
			t.Errorf("metadataHeaders=%#v missing one of %v", gotHeaders, want)
			http.Error(w, "bad metadataHeaders", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"payload": map[string]any{
				"headers": []map[string]any{
					{"name": "Message-ID", "value": "<orig@id>"},
					{"name": "In-Reply-To", "value": "<parent@id>"},
					{"name": "References", "value": "<parent@id> <orig@id>"},
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
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "<orig@id>") || !strings.Contains(out, "<parent@id>") {
		t.Fatalf("expected threading headers in metadata JSON, got: %q", out)
	}
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
			t.Errorf("format=%q", got)
			http.Error(w, "bad format", http.StatusBadRequest)
			return
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

func TestExecute_GmailGet_Full_JSON_Body(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	plain := base64.RawURLEncoding.EncodeToString([]byte("plain body"))
	html := base64.RawURLEncoding.EncodeToString([]byte("<p>html body</p>"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("format"); got != "full" {
			t.Errorf("format=%q", got)
			http.Error(w, "bad format", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "m1",
			"payload": map[string]any{
				"mimeType": "multipart/alternative",
				"parts": []map[string]any{
					{"mimeType": "text/html", "body": map[string]any{"data": html}},
					{"mimeType": "text/plain; charset=utf-8", "body": map[string]any{"data": plain}},
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
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Body != "plain body" {
		t.Fatalf("unexpected body: %q", parsed.Body)
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
			t.Errorf("format=%q", got)
			http.Error(w, "bad format", http.StatusBadRequest)
			return
		}
		gotHeaders := r.URL.Query()["metadataHeaders"]
		if len(gotHeaders) != 4 || !containsAll(gotHeaders, []string{"From", "Subject", "Cc", "List-Unsubscribe"}) {
			t.Errorf("metadataHeaders=%#v", gotHeaders)
			http.Error(w, "bad metadataHeaders", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"headers": []map[string]any{
					{"name": "From", "value": "Me <me@example.com>"},
					{"name": "CC", "value": "cc@example.com"},
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
				"--headers", "From,Subject,Cc",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "id\tm1") || !strings.Contains(out, "cc\tcc@example.com") || !strings.Contains(out, "subject\tHello") {
		t.Fatalf("unexpected out=%q", out)
	}
}
