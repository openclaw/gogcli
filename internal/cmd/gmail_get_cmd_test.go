package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGmailGetCmd_JSON_Full(t *testing.T) {
	bodyData := base64.RawURLEncoding.EncodeToString([]byte("hello"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"mimeType": "text/plain",
				"body":     map[string]any{"data": bodyData},
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Cc", "value": "c@example.com"},
					{"name": "Bcc", "value": "d@example.com"},
					{"name": "Subject", "value": "S"},
					{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
					{"name": "List-Unsubscribe", "value": "<mailto:unsubscribe@example.com>"},
				},
			},
		})
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "get", "m1", "--format", "full"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed["body"] != "hello" {
		t.Fatalf("unexpected body: %v", parsed["body"])
	}
	if parsed["unsubscribe"] != "mailto:unsubscribe@example.com" {
		t.Fatalf("unexpected unsubscribe: %v", parsed["unsubscribe"])
	}
	headers, ok := parsed["headers"].(map[string]any)
	if !ok {
		t.Fatalf("expected headers map, got: %T", parsed["headers"])
	}
	if headers["cc"] != "c@example.com" {
		t.Fatalf("unexpected cc header: %v", headers["cc"])
	}
	if headers["bcc"] != "d@example.com" {
		t.Fatalf("unexpected bcc header: %v", headers["bcc"])
	}
}

func TestGmailGetCmd_JSON_Full_WithAttachments(t *testing.T) {
	bodyData := base64.RawURLEncoding.EncodeToString([]byte("hello with attachment"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"mimeType": "multipart/mixed",
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Subject", "value": "Email with attachment"},
					{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
				},
				"parts": []map[string]any{
					{
						"mimeType": "text/plain",
						"body":     map[string]any{"data": bodyData},
					},
					{
						"mimeType": "application/pdf",
						"filename": "document.pdf",
						"body": map[string]any{
							"attachmentId": "ANGjdJ-abc123",
							"size":         12345,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "get", "m1", "--format", "full"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed["body"] != "hello with attachment" {
		t.Fatalf("unexpected body: %v", parsed["body"])
	}
	attachments, ok := parsed["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got: %v", parsed["attachments"])
	}
	att := attachments[0].(map[string]any)
	if att["filename"] != "document.pdf" {
		t.Fatalf("unexpected attachment filename: %v", att["filename"])
	}
	if att["size"] != float64(12345) {
		t.Fatalf("unexpected attachment size: %v", att["size"])
	}
	if att["sizeHuman"] != formatBytes(12345) {
		t.Fatalf("unexpected attachment sizeHuman: %v", att["sizeHuman"])
	}
	if att["mimeType"] != "application/pdf" {
		t.Fatalf("unexpected attachment mime type: %v", att["mimeType"])
	}
	if att["attachmentId"] != "ANGjdJ-abc123" {
		t.Fatalf("unexpected attachment id: %v", att["attachmentId"])
	}
}

func TestGmailGetCmd_JSON_Metadata_WithAttachments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"mimeType": "multipart/mixed",
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Cc", "value": "c@example.com"},
					{"name": "Bcc", "value": "d@example.com"},
					{"name": "Subject", "value": "Metadata attachments"},
					{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
				},
				"parts": []map[string]any{
					{
						"mimeType": "application/pdf",
						"filename": "metadata.pdf",
						"body": map[string]any{
							"attachmentId": "ANGjdJ-meta123",
							"size":         4096,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "get", "m1", "--format", "metadata"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if _, ok := parsed["body"]; ok {
		t.Fatalf("expected no body for metadata output")
	}
	headers, ok := parsed["headers"].(map[string]any)
	if !ok {
		t.Fatalf("expected headers map, got: %T", parsed["headers"])
	}
	if headers["cc"] != "c@example.com" {
		t.Fatalf("unexpected cc header: %v", headers["cc"])
	}
	if headers["bcc"] != "d@example.com" {
		t.Fatalf("unexpected bcc header: %v", headers["bcc"])
	}
	attachments, ok := parsed["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got: %v", parsed["attachments"])
	}
	att := attachments[0].(map[string]any)
	if att["filename"] != "metadata.pdf" {
		t.Fatalf("unexpected attachment filename: %v", att["filename"])
	}
	if att["size"] != float64(4096) {
		t.Fatalf("unexpected attachment size: %v", att["size"])
	}
	if att["sizeHuman"] != formatBytes(4096) {
		t.Fatalf("unexpected attachment sizeHuman: %v", att["sizeHuman"])
	}
	if att["mimeType"] != "application/pdf" {
		t.Fatalf("unexpected attachment mime type: %v", att["mimeType"])
	}
	if att["attachmentId"] != "ANGjdJ-meta123" {
		t.Fatalf("unexpected attachment id: %v", att["attachmentId"])
	}
}

func gmailGetAttachmentHandler(subject, body string) http.Handler {
	bodyData := base64.RawURLEncoding.EncodeToString([]byte(body))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"mimeType": "multipart/mixed",
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Cc", "value": "c@example.com"},
					{"name": "Bcc", "value": "d@example.com"},
					{"name": "Subject", "value": subject},
					{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
				},
				"parts": []map[string]any{
					{
						"mimeType": "text/plain",
						"body":     map[string]any{"data": bodyData},
					},
					{
						"mimeType": "application/pdf",
						"filename": "report.pdf",
						"body": map[string]any{
							"attachmentId": "ANGjdJ-xyz789",
							"size":         54321,
						},
					},
				},
			},
		})
	})
}

func TestGmailGetCmd_Text_Full_WithAttachments(t *testing.T) {
	srv := httptest.NewServer(gmailGetAttachmentHandler("Test", "hello"))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "get", "m1", "--format", "full"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if !strings.Contains(result.stdout, "attachment\treport.pdf\t"+formatBytes(54321)+"\tapplication/pdf\tANGjdJ-xyz789") {
		t.Fatalf("expected attachment line in output, got: %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "cc\tc@example.com") {
		t.Fatalf("expected cc header in output, got: %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "bcc\td@example.com") {
		t.Fatalf("expected bcc header in output, got: %q", result.stdout)
	}
}

func TestGmailGetCmd_Text_Metadata_WithAttachments(t *testing.T) {
	srv := httptest.NewServer(gmailGetAttachmentHandler("Metadata", "metadata body"))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "get", "m1", "--format", "metadata"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if !strings.Contains(result.stdout, "attachment\treport.pdf\t"+formatBytes(54321)+"\tapplication/pdf\tANGjdJ-xyz789") {
		t.Fatalf("expected attachment line in output, got: %q", result.stdout)
	}
	if strings.Contains(result.stdout, "metadata body") {
		t.Fatalf("unexpected body output for metadata: %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "cc\tc@example.com") {
		t.Fatalf("expected cc header in output, got: %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "bcc\td@example.com") {
		t.Fatalf("expected bcc header in output, got: %q", result.stdout)
	}
}

func TestGmailGetCmd_RawAcceptsPaddedBase64URL(t *testing.T) {
	rawMessage := "Subject: padded\r\nContent-Type: multipart/mixed\r\n\r\nbody!"
	encoded := base64.URLEncoding.EncodeToString([]byte(rawMessage))
	if !strings.HasSuffix(encoded, "=") {
		t.Fatal("test fixture must include base64 padding")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"raw":      encoded,
		})
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "get", "m1", "--format", "raw"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, rawMessage) {
		t.Fatalf("expected decoded raw message, got: %q", result.stdout)
	}
}

func TestGmailGetCmd_RawEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"raw":      "",
			"payload":  map[string]any{"headers": []map[string]any{}},
		})
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "get", "m1", "--format", "raw"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "Empty raw message") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}
