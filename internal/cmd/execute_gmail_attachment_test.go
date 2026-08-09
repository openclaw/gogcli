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

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
)

func executeGmailAttachmentJSON(t *testing.T, svc *gmail.Service, args ...string) map[string]any {
	t.Helper()
	result := executeWithGmailTestService(t, args, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	return parsed
}

func newGmailAttachmentCacheTestService(t *testing.T, data []byte, filename string, calls *int32) *gmail.Service {
	t.Helper()
	encoded := base64.URLEncoding.EncodeToString(data)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/a1"):
			atomic.AddInt32(calls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": encoded})
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && !strings.Contains(r.URL.Path, "/attachments/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1",
				"payload": map[string]any{"parts": []map[string]any{{
					"filename": filename,
					"body":     map[string]any{"attachmentId": "a1", "size": len(data)},
				}}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	svc, closeServer := newGoogleTestService(t, handler, gmail.NewService)
	t.Cleanup(closeServer)
	return svc
}

func TestExecute_GmailAttachment_OutPath_JSON(t *testing.T) {
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

	svc := newGmailServiceFromServer(t, srv)
	outPath := filepath.Join(t.TempDir(), "a.bin")

	run := func() map[string]any {
		return executeGmailAttachmentJSON(t, svc,
			"--json",
			"--account", "a@b.com",
			"gmail", "attachment", "m1", "a1",
			"--out", outPath,
		)
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
	ambientHome := t.TempDir()
	t.Setenv("HOME", ambientHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(ambientHome, "ambient-config"))
	runtimeRoot := t.TempDir()

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
	svc := newGmailServiceFromServer(t, srv)

	result := executeWithTestRuntime(t, []string{
		"--json",
		"--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--name", "override.bin",
	}, &app.Runtime{
		Layout: config.Layout{
			ConfigDir:      filepath.Join(runtimeRoot, "config"),
			ExplicitConfig: true,
		},
		Services: app.Services{
			Gmail: func(context.Context, string) (*gmail.Service, error) {
				return svc, nil
			},
		},
	})
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	path, _ := parsed["path"].(string)
	if !strings.Contains(path, "override.bin") || !strings.Contains(path, "m1_a1_") {
		t.Fatalf("unexpected path=%q", path)
	}
	wantDir := filepath.Join(runtimeRoot, "config", "gmail-attachments")
	if filepath.Dir(path) != wantDir {
		t.Fatalf("path dir=%q want=%q", filepath.Dir(path), wantDir)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != string(attachmentData) {
		t.Fatalf("content=%q", string(b))
	}
	if _, err := os.Stat(filepath.Join(ambientHome, "ambient-config")); !os.IsNotExist(err) {
		t.Fatalf("ambient config touched: %v", err)
	}
}

func TestExecute_GmailAttachment_NotFound(t *testing.T) {
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

	outPath := filepath.Join(t.TempDir(), "a.bin")
	result := executeWithGmailTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", outPath,
	}, newGmailServiceFromServer(t, srv))
	if result.err == nil {
		t.Fatalf("expected error")
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file written, stat=%v", statErr)
	}
}

func TestExecute_GmailAttachment_OutDirWithName_JSON(t *testing.T) {
	var attachmentCalls int32
	attachmentData := []byte("hello")
	svc := newGmailAttachmentCacheTestService(t, attachmentData, "ignored.bin", &attachmentCalls)
	outDir := t.TempDir()
	wantPath := filepath.Join(outDir, "invoice.pdf")

	run := func() map[string]any {
		return executeGmailAttachmentJSON(t, svc,
			"--json",
			"--account", "a@b.com",
			"gmail", "attachment", "m1", "a1",
			"--out", outDir,
			"--name", "invoice.pdf",
		)
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
	var attachmentCalls int32
	attachmentData := []byte("fresh-bytes")
	svc := newGmailAttachmentCacheTestService(t, attachmentData, "a.bin", &attachmentCalls)

	outPath := filepath.Join(t.TempDir(), "invoice.pdf")
	if writeErr := os.WriteFile(outPath, []byte("stale"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	parsed := executeGmailAttachmentJSON(t, svc,
		"--json",
		"--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", outPath,
	)
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
