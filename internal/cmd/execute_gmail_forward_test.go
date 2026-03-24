package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// TestExecute_GmailForward_DefaultSubjectAndAttachments verifies that forwarding
// a message with a text/plain body and one attachment produces the correct
// default "Fwd: " subject, includes the forwarded-message header block, and
// carries the original attachment through to the sent MIME.
func TestExecute_GmailForward_DefaultSubjectAndAttachments(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachmentData := []byte("binary-payload")
	attachmentEncoded := base64.RawURLEncoding.EncodeToString(attachmentData)
	plainBody := base64.RawURLEncoding.EncodeToString([]byte("Original body text."))

	var attachmentFetched int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/a1"):
			atomic.AddInt32(&attachmentFetched, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"mimeType": "multipart/mixed",
					"headers": []map[string]any{
						{"name": "Subject", "value": "Hello"},
						{"name": "From", "value": "sender@example.com"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": plainBody},
						},
						{
							"filename": "report.pdf",
							"mimeType": "application/pdf",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         len(attachmentData),
							},
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
				t.Fatalf("missing or wrong Subject in raw:\n%s", s)
			}
			if !strings.Contains(s, "---------- Forwarded message ----------") {
				t.Fatalf("missing forwarded message header in raw:\n%s", s)
			}
			if !strings.Contains(s, "Forwarding note.") {
				t.Fatalf("missing preface body in raw:\n%s", s)
			}
			if !strings.Contains(s, "Original body text.") {
				t.Fatalf("missing original body text in raw:\n%s", s)
			}
			// Verify attachment is present
			if !strings.Contains(s, "report.pdf") {
				t.Fatalf("missing attachment filename in raw:\n%s", s)
			}
			attachB64 := base64.StdEncoding.EncodeToString(attachmentData)
			if !strings.Contains(s, attachB64) {
				t.Fatalf("missing attachment data in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s1", "threadId": "t2"})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"gmail", "forward", "m1",
				"--to", "to@example.com",
				"--body", "Forwarding note.",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if atomic.LoadInt32(&attachmentFetched) != 1 {
		t.Fatalf("expected 1 attachment fetch, got %d", atomic.LoadInt32(&attachmentFetched))
	}

	var parsed map[string]any
	if unmarshalErr := json.Unmarshal([]byte(out), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, out)
	}
	if parsed["messageId"] != "s1" {
		t.Fatalf("unexpected messageId: %v", parsed["messageId"])
	}
}

// TestExecute_GmailForward_HTMLOnlyMessage verifies that when the original
// message has only an HTML body (no text/plain), the forward generates a
// plain-text fallback by stripping HTML tags and preserves the HTML in
// the HTML part.
func TestExecute_GmailForward_HTMLOnlyMessage(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	htmlContent := "<p>Hello <b>world</b></p>"
	htmlEncoded := base64.RawURLEncoding.EncodeToString([]byte(htmlContent))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m2"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m2",
				"threadId": "t2",
				"payload": map[string]any{
					"mimeType": "text/html",
					"headers": []map[string]any{
						{"name": "Subject", "value": "HTML Only"},
						{"name": "From", "value": "html@example.com"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"body": map[string]any{"data": htmlEncoded},
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
			// Plain-text fallback should have HTML tags stripped
			if !strings.Contains(s, "Hello world") {
				t.Fatalf("missing plain-text fallback (HTML stripped) in raw:\n%s", s)
			}
			// HTML part should preserve the original
			if !strings.Contains(s, htmlContent) {
				t.Fatalf("missing original HTML in raw:\n%s", s)
			}
			if !strings.Contains(s, "---------- Forwarded message ----------") {
				t.Fatalf("missing forwarded message header in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s2", "threadId": "t3"})
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
				"gmail", "forward", "m2",
				"--to", "to@example.com",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

// TestExecute_GmailForward_CustomSubject verifies that --subject overrides
// the default "Fwd: <original>" subject.
func TestExecute_GmailForward_CustomSubject(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	plainBody := base64.RawURLEncoding.EncodeToString([]byte("Some text."))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m3"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m3",
				"threadId": "t3",
				"payload": map[string]any{
					"mimeType": "text/plain",
					"headers": []map[string]any{
						{"name": "Subject", "value": "Original Subject"},
						{"name": "From", "value": "orig@example.com"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"body": map[string]any{"data": plainBody},
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
			if !strings.Contains(s, "Subject: Custom Subject\r\n") {
				t.Fatalf("expected custom subject in raw:\n%s", s)
			}
			if strings.Contains(s, "Fwd: Original Subject") {
				t.Fatalf("should not have default Fwd: subject in raw:\n%s", s)
			}
			if !strings.Contains(s, "FYI") {
				t.Fatalf("missing body preface in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s3", "threadId": "t4"})
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
				"gmail", "forward", "m3",
				"--to", "to@example.com",
				"--subject", "Custom Subject",
				"--body", "FYI",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

// TestExecute_GmailForward_CcAndBcc verifies that --cc and --bcc recipients
// appear in the sent MIME headers.
func TestExecute_GmailForward_CcAndBcc(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	plainBody := base64.RawURLEncoding.EncodeToString([]byte("Body content."))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m4"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m4",
				"threadId": "t4",
				"payload": map[string]any{
					"mimeType": "text/plain",
					"headers": []map[string]any{
						{"name": "Subject", "value": "CC Test"},
						{"name": "From", "value": "orig@example.com"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"body": map[string]any{"data": plainBody},
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
			if !strings.Contains(s, "Cc: cc@example.com\r\n") {
				t.Fatalf("missing Cc header in raw:\n%s", s)
			}
			if !strings.Contains(s, "Bcc: bcc@example.com\r\n") {
				t.Fatalf("missing Bcc header in raw:\n%s", s)
			}
			if !strings.Contains(s, "To: to@example.com\r\n") {
				t.Fatalf("missing To header in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s4", "threadId": "t5"})
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
				"gmail", "forward", "m4",
				"--to", "to@example.com",
				"--cc", "cc@example.com",
				"--bcc", "bcc@example.com",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

// TestExecute_GmailForward_NoAttachments verifies that --no-attachments
// strips the original attachment and does NOT call the attachment fetch API.
func TestExecute_GmailForward_NoAttachments(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	plainBody := base64.RawURLEncoding.EncodeToString([]byte("Text body."))
	attachmentData := []byte("should-not-appear")
	attachmentEncoded := base64.RawURLEncoding.EncodeToString(attachmentData)

	var attachmentFetched int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m5/attachments/a1"):
			atomic.AddInt32(&attachmentFetched, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m5"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m5",
				"threadId": "t5",
				"payload": map[string]any{
					"mimeType": "multipart/mixed",
					"headers": []map[string]any{
						{"name": "Subject", "value": "With Attachment"},
						{"name": "From", "value": "orig@example.com"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": plainBody},
						},
						{
							"filename": "secret.pdf",
							"mimeType": "application/pdf",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         len(attachmentData),
							},
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
			// Should NOT contain the attachment
			if strings.Contains(s, "secret.pdf") {
				t.Fatalf("attachment should have been stripped but found filename in raw:\n%s", s)
			}
			attachB64 := base64.StdEncoding.EncodeToString(attachmentData)
			if strings.Contains(s, attachB64) {
				t.Fatalf("attachment data should have been stripped but found in raw:\n%s", s)
			}
			// Should still have the forwarded body
			if !strings.Contains(s, "---------- Forwarded message ----------") {
				t.Fatalf("missing forwarded message header in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s5", "threadId": "t6"})
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
				"gmail", "forward", "m5",
				"--to", "to@example.com",
				"--no-attachments",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if atomic.LoadInt32(&attachmentFetched) != 0 {
		t.Fatalf("expected 0 attachment fetch calls with --no-attachments, got %d", atomic.LoadInt32(&attachmentFetched))
	}
}

// TestExecute_GmailForward_ExtraLocalAttachments verifies that --attach adds
// a local file alongside the original attachment in the forwarded MIME.
func TestExecute_GmailForward_ExtraLocalAttachments(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	origAttachData := []byte("original-attachment")
	origAttachEncoded := base64.RawURLEncoding.EncodeToString(origAttachData)
	plainBody := base64.RawURLEncoding.EncodeToString([]byte("Message body."))

	// Create a local temp file to attach.
	tmpFile, tmpErr := os.CreateTemp(t.TempDir(), "local-*.txt")
	if tmpErr != nil {
		t.Fatalf("CreateTemp: %v", tmpErr)
	}
	localContent := []byte("local-file-content")
	if _, writeErr := tmpFile.Write(localContent); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	_ = tmpFile.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m6/attachments/a1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": origAttachEncoded})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m6"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m6",
				"threadId": "t6",
				"payload": map[string]any{
					"mimeType": "multipart/mixed",
					"headers": []map[string]any{
						{"name": "Subject", "value": "With Attach"},
						{"name": "From", "value": "orig@example.com"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": plainBody},
						},
						{
							"filename": "orig.bin",
							"mimeType": "application/octet-stream",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         len(origAttachData),
							},
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
			// Both the original attachment and the local file should be present
			origB64 := base64.StdEncoding.EncodeToString(origAttachData)
			if !strings.Contains(s, origB64) {
				t.Fatalf("missing original attachment data in raw:\n%s", s)
			}
			if !strings.Contains(s, "orig.bin") {
				t.Fatalf("missing original attachment filename in raw:\n%s", s)
			}
			localB64 := base64.StdEncoding.EncodeToString(localContent)
			if !strings.Contains(s, localB64) {
				t.Fatalf("missing local attachment data in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s6", "threadId": "t7"})
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
				"gmail", "forward", "m6",
				"--to", "to@example.com",
				"--attach", tmpFile.Name(),
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

// TestExecute_GmailForward_NoAttachmentsWithLocalAttach verifies that
// --no-attachments strips the original attachment while --attach still
// includes the local file.
func TestExecute_GmailForward_NoAttachmentsWithLocalAttach(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	origAttachData := []byte("original-should-not-appear")
	origAttachEncoded := base64.RawURLEncoding.EncodeToString(origAttachData)
	plainBody := base64.RawURLEncoding.EncodeToString([]byte("Some text."))

	// Create a local temp file to attach.
	tmpFile, tmpErr := os.CreateTemp(t.TempDir(), "local-*.txt")
	if tmpErr != nil {
		t.Fatalf("CreateTemp: %v", tmpErr)
	}
	localContent := []byte("local-only-content")
	if _, writeErr := tmpFile.Write(localContent); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	_ = tmpFile.Close()

	var attachmentFetched int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m7/attachments/a1"):
			atomic.AddInt32(&attachmentFetched, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": origAttachEncoded})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m7"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m7",
				"threadId": "t7",
				"payload": map[string]any{
					"mimeType": "multipart/mixed",
					"headers": []map[string]any{
						{"name": "Subject", "value": "Mixed"},
						{"name": "From", "value": "orig@example.com"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": plainBody},
						},
						{
							"filename": "orig-secret.bin",
							"mimeType": "application/octet-stream",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         len(origAttachData),
							},
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
			// Original attachment should NOT be present
			origB64 := base64.StdEncoding.EncodeToString(origAttachData)
			if strings.Contains(s, origB64) {
				t.Fatalf("original attachment data should be stripped but found in raw:\n%s", s)
			}
			if strings.Contains(s, "orig-secret.bin") {
				t.Fatalf("original attachment filename should be stripped but found in raw:\n%s", s)
			}
			// Local file SHOULD be present
			localB64 := base64.StdEncoding.EncodeToString(localContent)
			if !strings.Contains(s, localB64) {
				t.Fatalf("missing local attachment data in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s7", "threadId": "t8"})
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
				"gmail", "forward", "m7",
				"--to", "to@example.com",
				"--no-attachments",
				"--attach", tmpFile.Name(),
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if atomic.LoadInt32(&attachmentFetched) != 0 {
		t.Fatalf("expected 0 attachment fetch calls with --no-attachments, got %d", atomic.LoadInt32(&attachmentFetched))
	}
}
