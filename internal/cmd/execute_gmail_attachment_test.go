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
	"sync/atomic"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestExecute_GmailAttachment_OutPath_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var attachmentCalls int32
	var messageCalls int32
	// 2 bytes => base64 has padding; exercises padded-base64 fallback decode path.
	attachmentData := []byte("ab")
	attachmentEncoded := base64.URLEncoding.EncodeToString(attachmentData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/a1"):
			atomic.AddInt32(&attachmentCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && !strings.Contains(r.URL.Path, "/attachments/"):
			atomic.AddInt32(&messageCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1",
				"payload": map[string]any{
					"parts": []map[string]any{
						{
							"filename": "a.bin",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         len(attachmentData),
							},
						},
					},
				},
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

	outPath := filepath.Join(t.TempDir(), "a.bin")

	run := func() map[string]any {
		out := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if execErr := Execute([]string{
					"--json",
					"--account", "a@b.com",
					"gmail", "attachment", "m1", "a1",
					"--out", outPath,
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
	if atomic.LoadInt32(&messageCalls) != 0 {
		t.Fatalf("messageCalls=%d", messageCalls)
	}
	if atomic.LoadInt32(&attachmentCalls) != 1 {
		t.Fatalf("attachmentCalls=%d", attachmentCalls)
	}
	if parsed1["path"] != outPath {
		t.Fatalf("path=%v", parsed1["path"])
	}
	if parsed1["cached"] != false {
		t.Fatalf("cached=%v", parsed1["cached"])
	}
	if parsed1["bytes"] != float64(len(attachmentData)) {
		t.Fatalf("bytes=%v", parsed1["bytes"])
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != string(attachmentData) {
		t.Fatalf("content=%q", string(b))
	}

	parsed2 := run()
	if atomic.LoadInt32(&messageCalls) != 1 {
		t.Fatalf("messageCalls=%d", messageCalls)
	}
	if atomic.LoadInt32(&attachmentCalls) != 1 {
		t.Fatalf("attachmentCalls=%d", attachmentCalls)
	}
	if parsed2["cached"] != true {
		t.Fatalf("cached=%v", parsed2["cached"])
	}
}

func TestExecute_GmailAttachment_NameOverride_ConfigDir_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	// Keep this unpadded base64url variant working too.
	attachmentData := []byte("ab")
	attachmentEncoded := base64.RawURLEncoding.EncodeToString(attachmentData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/a1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && !strings.Contains(r.URL.Path, "/attachments/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1",
				"payload": map[string]any{
					"parts": []map[string]any{
						{
							"filename": "override.bin",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         len(attachmentData),
							},
						},
					},
				},
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"gmail", "attachment", "m1", "a1",
				"--name", "override.bin",
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	var parsed map[string]any
	if unmarshalErr := json.Unmarshal([]byte(out), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, out)
	}
	path, _ := parsed["path"].(string)
	if !strings.Contains(path, "override.bin") || !strings.Contains(path, "m1_a1_") {
		t.Fatalf("unexpected path=%q", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != string(attachmentData) {
		t.Fatalf("content=%q", string(b))
	}
}

func TestExecute_GmailAttachment_NotFound(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/"):
			http.NotFound(w, r)
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && !strings.Contains(r.URL.Path, "/attachments/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1",
				"payload": map[string]any{
					"parts": []map[string]any{
						{
							"filename": "a.bin",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         2,
							},
						},
					},
				},
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

	outPath := filepath.Join(t.TempDir(), "a.bin")

	err = Execute([]string{
		"--json",
		"--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", outPath,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file written, stat=%v", statErr)
	}
}

func TestExecute_GmailAttachment_OutDirWithName_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var attachmentCalls int32
	attachmentData := []byte("hello")
	attachmentEncoded := base64.URLEncoding.EncodeToString(attachmentData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/a1"):
			atomic.AddInt32(&attachmentCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && !strings.Contains(r.URL.Path, "/attachments/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1",
				"payload": map[string]any{
					"parts": []map[string]any{
						{
							"filename": "ignored.bin",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         len(attachmentData),
							},
						},
					},
				},
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
	wantPath := filepath.Join(outDir, "invoice.pdf")

	run := func() map[string]any {
		out := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if execErr := Execute([]string{
					"--json",
					"--account", "a@b.com",
					"gmail", "attachment", "m1", "a1",
					"--out", outDir,
					"--name", "invoice.pdf",
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
	if parsed1["path"] != wantPath {
		t.Fatalf("path=%v want=%s", parsed1["path"], wantPath)
	}
	if parsed1["cached"] != false {
		t.Fatalf("cached=%v", parsed1["cached"])
	}
	if atomic.LoadInt32(&attachmentCalls) != 1 {
		t.Fatalf("attachmentCalls=%d", attachmentCalls)
	}

	parsed2 := run()
	if parsed2["cached"] != true {
		t.Fatalf("cached=%v", parsed2["cached"])
	}
	if atomic.LoadInt32(&attachmentCalls) != 1 {
		t.Fatalf("attachmentCalls=%d", attachmentCalls)
	}
}

func TestExecute_GmailAttachment_StaleFileIsRedownloaded(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var attachmentCalls int32
	attachmentData := []byte("fresh-bytes")
	attachmentEncoded := base64.URLEncoding.EncodeToString(attachmentData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/a1"):
			atomic.AddInt32(&attachmentCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && !strings.Contains(r.URL.Path, "/attachments/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1",
				"payload": map[string]any{
					"parts": []map[string]any{
						{
							"filename": "a.bin",
							"body": map[string]any{
								"attachmentId": "a1",
								"size":         len(attachmentData),
							},
						},
					},
				},
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

	outPath := filepath.Join(t.TempDir(), "invoice.pdf")
	if writeErr := os.WriteFile(outPath, []byte("stale"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"gmail", "attachment", "m1", "a1",
				"--out", outPath,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	var parsed map[string]any
	if unmarshalErr := json.Unmarshal([]byte(out), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, out)
	}
	if parsed["cached"] != false {
		t.Fatalf("cached=%v", parsed["cached"])
	}
	if atomic.LoadInt32(&attachmentCalls) != 1 {
		t.Fatalf("attachmentCalls=%d", attachmentCalls)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != string(attachmentData) {
		t.Fatalf("content=%q", string(b))
	}
}
