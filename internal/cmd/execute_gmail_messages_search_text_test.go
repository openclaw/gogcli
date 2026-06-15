package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecute_GmailMessagesSearch_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/messages") && !strings.Contains(path, "/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"id": "m1", "threadId": "t1"},
					{"id": "m2", "threadId": "t1"},
				},
			})
			return
		case strings.Contains(path, "/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"mimeType": "text/plain",
					"headers": []map[string]any{
						{"name": "From", "value": "Example <no-reply@example.com>"},
						{"name": "Subject", "value": "Receipt"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/messages/m2"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m2",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "Example <no-reply@example.com>"},
						{"name": "Subject", "value": "Receipt"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:05:05 -0700"},
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--max", "2"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "m1") || !strings.Contains(result.stdout, "m2") {
		t.Fatalf("expected both message IDs, got: %q", result.stdout)
	}
}

func TestExecute_GmailMessagesSearch_JSON_IncludeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/messages") && !strings.Contains(path, "/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"id": "m1", "threadId": "t1"},
				},
			})
			return
		case strings.Contains(path, "/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"mimeType": "multipart/alternative",
					"headers": []map[string]any{
						{"name": "From", "value": "Example <no-reply@example.com>"},
						{"name": "Subject", "value": "Receipt"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"headers": []map[string]any{
								{"name": "Content-Transfer-Encoding", "value": "quoted-printable"},
								{"name": "Content-Type", "value": "text/plain; charset=utf-8"},
							},
							"body": map[string]any{
								"data": encodeBase64URL("Total =E2=82=AC99.99"),
							},
						},
						{
							"mimeType": "text/html",
							"body": map[string]any{
								"data": encodeBase64URL("<strong>Total €99.99</strong>"),
							},
						},
						{
							"filename": "invite.ics",
							"mimeType": "text/calendar",
							"body": map[string]any{
								"attachmentId": "att-ics",
								"size":         2048,
							},
						},
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
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
	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--include-body"},
		svc,
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	out := result.stdout
	if !strings.Contains(out, "Total €99.99") {
		t.Fatalf("expected decoded body, got: %q", out)
	}
	if strings.Contains(out, "<strong>") {
		t.Fatalf("expected text body by default, got: %q", out)
	}
	var parsed struct {
		Messages []struct {
			Attachments []struct {
				Filename     string `json:"filename"`
				MimeType     string `json:"mimeType"`
				Size         int64  `json:"size"`
				AttachmentID string `json:"attachmentId"`
			} `json:"attachments"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode search json: %v", err)
	}
	if len(parsed.Messages) != 1 || len(parsed.Messages[0].Attachments) != 1 {
		t.Fatalf("expected one attachment, got: %#v", parsed.Messages)
	}
	att := parsed.Messages[0].Attachments[0]
	if att.Filename != "invite.ics" || att.MimeType != "text/calendar" || att.Size != 2048 || att.AttachmentID != "att-ics" {
		t.Fatalf("unexpected attachment: %#v", att)
	}

	htmlResult := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--include-body", "--body-format", "html"},
		svc,
	)
	if htmlResult.err != nil {
		t.Fatalf("Execute HTML: %v\nstderr=%q", htmlResult.err, htmlResult.stderr)
	}
	if !strings.Contains(htmlResult.stdout, "<strong>Total €99.99</strong>") {
		t.Fatalf("expected html body, got: %q", htmlResult.stdout)
	}
}

func TestExecute_GmailMessagesSearch_Text_IncludeBodyTruncationDiscoverable(t *testing.T) {
	longBody := strings.Repeat("b", 240)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/messages") && !strings.Contains(path, "/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"id": "m1", "threadId": "t1"},
				},
			})
			return
		case strings.Contains(path, "/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"mimeType": "text/plain",
					"headers": []map[string]any{
						{"name": "From", "value": "Example <no-reply@example.com>"},
						{"name": "Subject", "value": "Receipt"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
					},
					"body": map[string]any{
						"data": encodeBase64URL(longBody),
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
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

	defaultResult := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--include-body"},
		svc,
	)
	if defaultResult.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", defaultResult.err, defaultResult.stderr)
	}
	if !strings.Contains(defaultResult.stdout, strings.Repeat("b", 200)+gmailTextTruncationMarker) {
		t.Fatalf("expected actionable truncation marker, got: %q", defaultResult.stdout)
	}
	if strings.Contains(defaultResult.stdout, longBody) {
		t.Fatalf("expected body to be truncated, got: %q", defaultResult.stdout)
	}

	fullResult := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--full"},
		svc,
	)
	if fullResult.err != nil {
		t.Fatalf("Execute full: %v\nstderr=%q", fullResult.err, fullResult.stderr)
	}
	if strings.Contains(fullResult.stdout, "[truncated") {
		t.Fatalf("expected full output without truncation marker, got: %q", fullResult.stdout)
	}
	if !strings.Contains(fullResult.stdout, longBody) {
		t.Fatalf("expected full body, got: %q", fullResult.stdout)
	}
}

func TestExecute_GmailMessagesSearch_AppliesSystemLabelFilters(t *testing.T) {
	var gotQuery string
	var gotLabels []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/messages") && !strings.Contains(path, "/users/me/messages/"):
			gotQuery = r.URL.Query().Get("q")
			gotLabels = r.URL.Query()["labelIds"]
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "messages", "search", "in:spam is:unread", "--max", "1000"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if gotQuery != "in:spam is:unread" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
	assertSameStrings(t, gotLabels, []string{"SPAM", "UNREAD"})
}
