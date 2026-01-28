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
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailGetCmd_JSON_Full(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := ui.WithUI(context.Background(), u)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

			cmd := &GmailGetCmd{}
			if err := runKong(t, cmd, []string{"m1", "--format", "full"}, ctx, flags); err != nil {
				t.Fatalf("execute: %v", err)
			}
		})
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
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
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := ui.WithUI(context.Background(), u)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

			cmd := &GmailGetCmd{}
			if err := runKong(t, cmd, []string{"m1", "--format", "full"}, ctx, flags); err != nil {
				t.Fatalf("execute: %v", err)
			}
		})
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
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
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := ui.WithUI(context.Background(), u)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

			cmd := &GmailGetCmd{}
			if err := runKong(t, cmd, []string{"m1", "--format", "metadata"}, ctx, flags); err != nil {
				t.Fatalf("execute: %v", err)
			}
		})
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
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

func TestGmailGetCmd_Text_Full_WithAttachments(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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
				"mimeType": "multipart/mixed",
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Cc", "value": "c@example.com"},
					{"name": "Bcc", "value": "d@example.com"},
					{"name": "Subject", "value": "Test"},
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := ui.WithUI(context.Background(), u)

			cmd := &GmailGetCmd{}
			if err := runKong(t, cmd, []string{"m1", "--format", "full"}, ctx, flags); err != nil {
				t.Fatalf("execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "attachment\treport.pdf\t"+formatBytes(54321)+"\tapplication/pdf\tANGjdJ-xyz789") {
		t.Fatalf("expected attachment line in output, got: %q", out)
	}
	if !strings.Contains(out, "cc\tc@example.com") {
		t.Fatalf("expected cc header in output, got: %q", out)
	}
	if !strings.Contains(out, "bcc\td@example.com") {
		t.Fatalf("expected bcc header in output, got: %q", out)
	}
}

func TestGmailGetCmd_Text_Metadata_WithAttachments(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	bodyData := base64.RawURLEncoding.EncodeToString([]byte("metadata body"))
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
					{"name": "Subject", "value": "Metadata"},
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := ui.WithUI(context.Background(), u)

			cmd := &GmailGetCmd{}
			if err := runKong(t, cmd, []string{"m1", "--format", "metadata"}, ctx, flags); err != nil {
				t.Fatalf("execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "attachment\treport.pdf\t"+formatBytes(54321)+"\tapplication/pdf\tANGjdJ-xyz789") {
		t.Fatalf("expected attachment line in output, got: %q", out)
	}
	if strings.Contains(out, "metadata body") {
		t.Fatalf("unexpected body output for metadata: %q", out)
	}
	if !strings.Contains(out, "cc\tc@example.com") {
		t.Fatalf("expected cc header in output, got: %q", out)
	}
	if !strings.Contains(out, "bcc\td@example.com") {
		t.Fatalf("expected bcc header in output, got: %q", out)
	}
}

func TestGmailGetCmd_RawEmpty(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	errOut := captureStderr(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &GmailGetCmd{}
		if err := runKong(t, cmd, []string{"m1", "--format", "raw"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(errOut, "Empty raw message") {
		t.Fatalf("unexpected stderr: %q", errOut)
	}
}
