package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecute_GmailGet_Text_DraftMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var labels []string
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m-draft"):
			labels = []string{"DRAFT"}
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m-inbound"):
			labels = []string{"INBOX"}
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"),
			"threadId": "t1",
			"labelIds": labels,
			"payload":  map[string]any{"headers": []map[string]any{}},
		})
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)

	t.Run("human output marks a draft", func(t *testing.T) {
		result := executeWithGmailTestService(t, []string{
			"--account", "a@b.com", "gmail", "get", "m-draft",
		}, svc)
		if result.err != nil {
			t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
		}
		if !strings.Contains(result.stdout, "id\tm-draft [DRAFT — NOT SENT]\n") {
			t.Fatalf("expected marked draft heading, got=%q", result.stdout)
		}
	})

	t.Run("human output leaves a non-draft unchanged", func(t *testing.T) {
		result := executeWithGmailTestService(t, []string{
			"--account", "a@b.com", "gmail", "get", "m-inbound",
		}, svc)
		if result.err != nil {
			t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
		}
		if !strings.Contains(result.stdout, "id\tm-inbound\n") || strings.Contains(result.stdout, gmailDraftNotSentMarker) {
			t.Fatalf("expected unchanged non-draft heading, got=%q", result.stdout)
		}
	})

	t.Run("plain output is unchanged", func(t *testing.T) {
		result := executeWithGmailTestService(t, []string{
			"--plain", "--account", "a@b.com", "gmail", "get", "m-draft",
		}, svc)
		if result.err != nil {
			t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
		}
		if !strings.Contains(result.stdout, "id\tm-draft\n") || strings.Contains(result.stdout, gmailDraftNotSentMarker) {
			t.Fatalf("expected unchanged plain heading, got=%q", result.stdout)
		}
	})

	t.Run("JSON output is unchanged", func(t *testing.T) {
		result := executeWithGmailTestService(t, []string{
			"--json", "--account", "a@b.com", "gmail", "get", "m-draft",
		}, svc)
		if result.err != nil {
			t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
		}
		if strings.Contains(result.stdout, gmailDraftNotSentMarker) || !strings.Contains(result.stdout, `"DRAFT"`) {
			t.Fatalf("expected unchanged JSON labels without display marker, got=%q", result.stdout)
		}
	})
}

func TestExecute_GmailGet_Metadata_JSON(t *testing.T) {
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

	result := executeWithGmailTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"gmail", "get", "m1",
		"--format", "metadata",
		"--headers", "Subject,Date",
	}, newGmailServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Message struct {
			ID      string   `json:"id"`
			Thread  string   `json:"threadId"`
			LabelID []string `json:"labelIds"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
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

	result := executeWithGmailTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"gmail", "get", "m1",
		"--format", "metadata",
	}, newGmailServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "<orig@id>") || !strings.Contains(result.stdout, "<parent@id>") {
		t.Fatalf("expected threading headers in metadata JSON, got: %q", result.stdout)
	}
}

func TestExecute_GmailGet_Raw_JSON(t *testing.T) {
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

	result := executeWithGmailTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"gmail", "get", "m1",
		"--format", "raw",
	}, newGmailServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Message struct {
			ID  string `json:"id"`
			Raw string `json:"raw"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.Message.ID != "m1" || parsed.Message.Raw != rawEncoded {
		t.Fatalf("unexpected: %#v", parsed)
	}
}

func TestExecute_GmailGet_Full_JSON_Body(t *testing.T) {
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

	result := executeWithGmailTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"gmail", "get", "m1",
	}, newGmailServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.Body != "plain body" {
		t.Fatalf("unexpected body: %q", parsed.Body)
	}
}

func TestExecute_GmailGet_InvalidFormat(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--account", "a@b.com",
		"gmail", "get", "m1",
		"--format", "nope",
	}, nil)
	if result.err == nil {
		t.Fatalf("expected error")
	}
	if got := ExitCode(result.err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, result.err)
	}
	if !strings.Contains(result.err.Error(), "invalid --format") {
		t.Fatalf("unexpected error: %v", result.err)
	}
}

func TestExecute_GmailGet_Metadata_Text(t *testing.T) {
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

	result := executeWithGmailTestService(t, []string{
		"--plain",
		"--account", "a@b.com",
		"gmail", "get", "m1",
		"--format", "metadata",
		"--headers", "From,Subject,Cc",
	}, newGmailServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "id\tm1") || !strings.Contains(result.stdout, "cc\tcc@example.com") || !strings.Contains(result.stdout, "subject\tHello") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}
