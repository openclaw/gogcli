package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecute_GmailMessagesSearch_IncludeAttachments(t *testing.T) {
	var sawFormat, sawFields string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/messages") && !strings.Contains(path, "/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{{"id": "m1", "threadId": "t1"}},
			})
		case strings.Contains(path, "/users/me/messages/m1"):
			sawFormat = r.URL.Query().Get("format")
			sawFields = r.URL.Query().Get("fields")
			w.Header().Set("Content-Type", "application/json")
			// invoice.pdf sits three MIME levels down, to exercise deep nesting.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1", "threadId": "t1", "labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"mimeType": "multipart/mixed",
					"headers": []map[string]any{
						{"name": "From", "value": "Example <no-reply@example.com>"},
						{"name": "Subject", "value": "Receipt"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": encodeBase64URL("secret body text")},
						},
						{
							"mimeType": "multipart/related",
							"parts": []map[string]any{{
								"mimeType": "multipart/alternative",
								"parts": []map[string]any{{
									"filename": "invoice.pdf",
									"mimeType": "application/pdf",
									"body":     map[string]any{"attachmentId": "att-pdf", "size": 4096},
								}},
							}},
						},
					},
				},
			})
		case strings.Contains(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{{"id": "INBOX", "name": "INBOX", "type": "system"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)

	type searchOut struct {
		Messages []struct {
			Body        string `json:"body"`
			Attachments []struct {
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
				MimeType string `json:"mimeType"`
			} `json:"attachments"`
		} `json:"messages"`
	}

	// --include-attachments lists the attachments but does not render the body.
	res := executeWithGmailTestService(t,
		[]string{"--json", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--include-attachments"},
		svc)
	if res.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", res.err, res.stderr)
	}
	var parsed searchOut
	if err := json.Unmarshal([]byte(res.stdout), &parsed); err != nil {
		t.Fatalf("decode: %v\nout=%q", err, res.stdout)
	}
	if len(parsed.Messages) != 1 || len(parsed.Messages[0].Attachments) != 1 {
		t.Fatalf("expected one attachment, got: %#v", parsed.Messages)
	}
	att := parsed.Messages[0].Attachments[0]
	if att.Filename != "invoice.pdf" || att.MimeType != "application/pdf" || att.Size != 4096 {
		t.Fatalf("unexpected attachment: %#v", att)
	}
	if parsed.Messages[0].Body != "" || strings.Contains(res.stdout, "secret body text") {
		t.Fatalf("body must not be included with --include-attachments: %q", res.stdout)
	}
	// Fetched as a complete format=full with no capping parts mask, which is what
	// lets the attachment nested three levels down be listed at all.
	if sawFormat != "full" || strings.Contains(sawFields, "parts(") {
		t.Fatalf("include-attachments must fetch format=full without a capping parts mask; format=%q fields=%q", sawFormat, sawFields)
	}

	// Default search lists neither body nor attachments.
	plain := executeWithGmailTestService(t,
		[]string{"--json", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com"},
		svc)
	if plain.err != nil {
		t.Fatalf("Execute plain: %v\nstderr=%q", plain.err, plain.stderr)
	}
	if strings.Contains(plain.stdout, "invoice.pdf") || strings.Contains(plain.stdout, "attachments") {
		t.Fatalf("default search must not list attachments: %q", plain.stdout)
	}
}
