package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestGmailThreadGetAndAttachments_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachmentData := base64.RawURLEncoding.EncodeToString([]byte("payload"))
	threadResp := map[string]any{
		"id": "t1",
		"messages": []map[string]any{
			{
				"id": "m1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "a@example.com"},
						{"name": "To", "value": "b@example.com"},
						{"name": "Subject", "value": "Hi"},
						{"name": "Date", "value": "Mon, 1 Jan 2025 00:00:00 +0000"},
					},
					"mimeType": "multipart/mixed",
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte("hello")),
							},
						},
						{
							"filename": "note.txt",
							"mimeType": "text/plain",
							"body": map[string]any{
								"attachmentId": "att1",
								"size":         7,
							},
						},
					},
				},
			},
		},
	}
	emptyThreadResp := map[string]any{
		"id":       "empty",
		"messages": []map[string]any{},
	}
	noAttsThreadResp := map[string]any{
		"id": "noatts",
		"messages": []map[string]any{
			{
				"id": "m2",
				"payload": map[string]any{
					"mimeType": "text/plain",
					"body": map[string]any{
						"data": base64.RawURLEncoding.EncodeToString([]byte("hello")),
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/threads/t1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(threadResp)
			return
		case r.Method == http.MethodGet && path == "/users/me/threads/empty":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(emptyThreadResp)
			return
		case r.Method == http.MethodGet && path == "/users/me/threads/noatts":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(noAttsThreadResp)
			return
		case r.Method == http.MethodGet && path == "/users/me/messages/m1/attachments/att1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": attachmentData,
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

	outDir := t.TempDir()
	getOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "thread", "get", "t1", "--download", "--out-dir", outDir}); err != nil {
				t.Fatalf("Execute thread get: %v", err)
			}
		})
	})

	var payload struct {
		Thread     map[string]any   `json:"thread"`
		Downloaded []map[string]any `json:"downloaded"`
	}
	if err := json.Unmarshal([]byte(getOut), &payload); err != nil {
		t.Fatalf("decode thread json: %v", err)
	}
	if payload.Thread == nil || len(payload.Downloaded) != 1 {
		t.Fatalf("unexpected thread payload: %#v", payload)
	}
	path, ok := payload.Downloaded[0]["path"].(string)
	if !ok || path == "" {
		t.Fatalf("expected download path, got: %#v", payload.Downloaded)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected downloaded file: %v", statErr)
	}

	attachmentsOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "thread", "attachments", "t1"}); err != nil {
				t.Fatalf("Execute attachments: %v", err)
			}
		})
	})
	var attachments struct {
		ThreadID    string           `json:"threadId"`
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(attachmentsOut), &attachments); err != nil {
		t.Fatalf("decode attachments json: %v", err)
	}
	if attachments.ThreadID != "t1" || len(attachments.Attachments) != 1 {
		t.Fatalf("unexpected attachments payload: %#v", attachments)
	}
	if attachments.Attachments[0]["filename"] != "note.txt" {
		t.Fatalf("unexpected attachment filename: %#v", attachments.Attachments[0])
	}

	attachmentsDownloadOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "thread", "attachments", "t1", "--download", "--out-dir", outDir}); err != nil {
				t.Fatalf("Execute attachments download: %v", err)
			}
		})
	})
	var attachmentsDownloaded struct {
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(attachmentsDownloadOut), &attachmentsDownloaded); err != nil {
		t.Fatalf("decode attachments download: %v", err)
	}
	if len(attachmentsDownloaded.Attachments) != 1 {
		t.Fatalf("unexpected download attachments: %#v", attachmentsDownloaded.Attachments)
	}
	if _, ok := attachmentsDownloaded.Attachments[0]["path"]; !ok {
		t.Fatalf("expected download path in attachments: %#v", attachmentsDownloaded.Attachments[0])
	}

	plainOutDir := t.TempDir()
	plainDownloadOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "thread", "attachments", "t1", "--download", "--out-dir", plainOutDir}); err != nil {
				t.Fatalf("Execute attachments download plain: %v", err)
			}
		})
	})
	if !strings.Contains(plainDownloadOut, "Saved") {
		t.Fatalf("unexpected download output: %q", plainDownloadOut)
	}

	cachedOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "thread", "attachments", "t1", "--download", "--out-dir", plainOutDir}); err != nil {
				t.Fatalf("Execute attachments cached: %v", err)
			}
		})
	})
	if !strings.Contains(cachedOut, "Cached") {
		t.Fatalf("unexpected cached output: %q", cachedOut)
	}

	// Ensure path is within the requested output dir when downloading attachments.
	if !strings.HasPrefix(path, filepath.Clean(outDir)+string(os.PathSeparator)) {
		t.Fatalf("unexpected download path: %s", path)
	}

	plainOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "thread", "get", "t1"}); err != nil {
				t.Fatalf("Execute thread get plain: %v", err)
			}
		})
	})
	if !strings.Contains(plainOut, "Thread contains") {
		t.Fatalf("unexpected plain output: %q", plainOut)
	}

	emptyErr := captureStderr(t, func() {
		if err := Execute([]string{"--account", "a@b.com", "gmail", "thread", "get", "empty"}); err != nil {
			t.Fatalf("Execute empty thread: %v", err)
		}
	})
	if !strings.Contains(emptyErr, "Empty thread") {
		t.Fatalf("unexpected empty thread stderr: %q", emptyErr)
	}

	noAttsOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "thread", "attachments", "noatts"}); err != nil {
				t.Fatalf("Execute no attachments: %v", err)
			}
		})
	})
	if !strings.Contains(noAttsOut, "No attachments found") {
		t.Fatalf("unexpected no attachments output: %q", noAttsOut)
	}

	emptyAttachOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "thread", "attachments", "empty"}); err != nil {
				t.Fatalf("Execute empty attachments json: %v", err)
			}
		})
	})
	if !strings.Contains(emptyAttachOut, "\"attachments\"") {
		t.Fatalf("unexpected empty attachments output: %q", emptyAttachOut)
	}
}

func TestGmailThreadGet_Safe(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<html><body><script>fetch('https://track.com/pixel')</script><p>Hello visit https://phish.com/login now</p></body></html>`,
	))
	threadResp := map[string]any{
		"id": "t1",
		"messages": []map[string]any{
			{
				"id": "m1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "a@example.com"},
						{"name": "To", "value": "b@example.com"},
						{"name": "Subject", "value": "Check https://evil.com now"},
						{"name": "Date", "value": "Mon, 1 Jan 2025 00:00:00 +0000"},
					},
					"mimeType": "text/html",
					"body": map[string]any{
						"data": htmlBody,
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		if r.Method == http.MethodGet && path == "/users/me/threads/t1" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(threadResp)
			return
		}
		http.NotFound(w, r)
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

	// Test text output with --safe
	textOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "thread", "get", "t1", "--safe"}); err != nil {
				t.Fatalf("Execute --safe text: %v", err)
			}
		})
	})
	if strings.Contains(textOut, "https://") {
		t.Fatalf("--safe text output should not contain URLs, got: %q", textOut)
	}
	if !strings.Contains(textOut, "[url removed]") {
		t.Fatalf("--safe text output should contain [url removed], got: %q", textOut)
	}
	if !strings.Contains(textOut, "Hello") {
		t.Fatalf("--safe text output should contain plain text 'Hello', got: %q", textOut)
	}
	if strings.Contains(textOut, "<script>") || strings.Contains(textOut, "fetch(") {
		t.Fatalf("--safe text output should not contain script content, got: %q", textOut)
	}

	// Test JSON output with --safe
	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "thread", "get", "t1", "--safe"}); err != nil {
				t.Fatalf("Execute --safe json: %v", err)
			}
		})
	})
	var payload struct {
		Thread map[string]any    `json:"thread"`
		Bodies map[string]string `json:"bodies"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload.Bodies == nil {
		t.Fatalf("--safe JSON should include 'bodies' map")
	}
	safeBody, ok := payload.Bodies["m1"]
	if !ok {
		t.Fatalf("--safe JSON 'bodies' should have entry for m1")
	}
	if strings.Contains(safeBody, "https://") {
		t.Fatalf("--safe JSON body should not contain URLs, got: %q", safeBody)
	}
	if !strings.Contains(safeBody, "Hello") {
		t.Fatalf("--safe JSON body should contain 'Hello', got: %q", safeBody)
	}
}
