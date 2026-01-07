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

func TestExecute_GmailThread_Text_Download(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	wd := t.TempDir()
	origWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	attData := []byte("hello")
	attEncoded := base64.RawURLEncoding.EncodeToString(attData)
	bodyEncoded := base64.RawURLEncoding.EncodeToString([]byte("body"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t-thread-1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t-thread-1",
				"messages": []map[string]any{
					{
						"id": "m-thread-1",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "From", "value": "Me <me@example.com>"},
								{"name": "To", "value": "You <you@example.com>"},
								{"name": "Subject", "value": "Hello"},
								{"name": "Date", "value": "Wed, 17 Dec 2025 14:00:00 -0800"},
							},
							"parts": []map[string]any{
								{ // body
									"mimeType": "text/plain",
									"body":     map[string]any{"data": bodyEncoded},
								},
								{ // attachment
									"filename": "a.txt",
									"mimeType": "text/plain",
									"body":     map[string]any{"attachmentId": "a-thread-1", "size": len(attData)},
								},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m-thread-1/attachments/a-thread-1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attEncoded})
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
			if execErr := Execute([]string{"--account", "a@b.com", "gmail", "thread", "get", "t-thread-1", "--download"}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})
	if !strings.Contains(out, "=== Message 1/1: m-thread-1 ===") || !strings.Contains(out, "Attachments:") || !(strings.Contains(out, "Saved:") || strings.Contains(out, "Cached:")) {
		t.Fatalf("unexpected out=%q", out)
	}

	expectedPath := filepath.Join(wd, "m-thread-1_a-thread_a.txt")
	b, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != string(attData) {
		t.Fatalf("content=%q", string(b))
	}
}

func TestExecute_GmailDraftsGet_Text_Download(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	attData := []byte("hello")
	attEncoded := base64.RawURLEncoding.EncodeToString(attData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m-draft-1",
					"payload": map[string]any{
						"headers": []map[string]any{
							{"name": "To", "value": "x@y.com"},
							{"name": "Subject", "value": "S"},
						},
						"parts": []map[string]any{
							{
								"filename": "a.txt",
								"mimeType": "text/plain",
								"body":     map[string]any{"attachmentId": "a-draft-1", "size": len(attData)},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m-draft-1/attachments/a-draft-1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attEncoded})
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
			if execErr := Execute([]string{"--account", "a@b.com", "gmail", "drafts", "get", "d1", "--download"}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})
	if !strings.Contains(out, "Draft-ID: d1") || !strings.Contains(out, "Attachments:") || (!strings.Contains(out, "Saved:") && !strings.Contains(out, "Cached:")) {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_GmailThread_OutDir_CreatesParents_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	wd := t.TempDir()
	origWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	attData := []byte("hello")
	attEncoded := base64.RawURLEncoding.EncodeToString(attData)

	attachmentCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t-thread-1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t-thread-1",
				"messages": []map[string]any{
					{
						"id": "m-thread-1",
						"payload": map[string]any{
							"parts": []map[string]any{
								{
									"filename": "a.txt",
									"mimeType": "text/plain",
									"body":     map[string]any{"attachmentId": "a-thread-1", "size": len(attData)},
								},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m-thread-1/attachments/a-thread-1"):
			attachmentCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attEncoded})
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

	outDir := filepath.Join("nested", "attachments")
	run := func() map[string]any {
		out := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if execErr := Execute([]string{
					"--json",
					"--account", "a@b.com",
					"gmail", "thread", "get", "t-thread-1",
					"--download",
					"--out-dir", outDir,
				}); execErr != nil {
					t.Fatalf("Execute: %v", execErr)
				}
			})
		})
		var parsed map[string]any
		if unmarshalErr := json.Unmarshal([]byte(out), &parsed); unmarshalErr != nil {
			t.Fatalf("json parse: %v\nout=%q", unmarshalErr, out)
		}
		return parsed
	}

	parsed1 := run()
	if attachmentCalls != 1 {
		t.Fatalf("attachmentCalls=%d", attachmentCalls)
	}
	downloaded, _ := parsed1["downloaded"].([]any)
	if len(downloaded) != 1 {
		t.Fatalf("downloaded=%v", parsed1["downloaded"])
	}
	item, _ := downloaded[0].(map[string]any)
	path, _ := item["path"].(string)
	if path != filepath.Join(outDir, "m-thread-1_a-thread_a.txt") {
		t.Fatalf("path=%q", path)
	}
	b, err := os.ReadFile(filepath.Join(wd, path))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != string(attData) {
		t.Fatalf("content=%q", string(b))
	}

	parsed2 := run()
	if attachmentCalls != 1 {
		t.Fatalf("attachmentCalls=%d", attachmentCalls)
	}
	downloaded2, _ := parsed2["downloaded"].([]any)
	if len(downloaded2) != 1 {
		t.Fatalf("downloaded=%v", parsed2["downloaded"])
	}
	item2, _ := downloaded2[0].(map[string]any)
	if item2["cached"] != true {
		t.Fatalf("cached=%v", item2["cached"])
	}
}
