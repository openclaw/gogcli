package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecute_DriveMoreCommands_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/files") && r.Method == http.MethodGet:
			// files.list or files.get
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(path, "/files/") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "id1",
					"name":        "Doc",
					"parents":     []string{"p0"},
					"webViewLink": "https://example.com/id1",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "id1", "name": "Doc", "mimeType": "application/pdf"},
				},
				"nextPageToken": "npt",
			})
			return
		case strings.Contains(path, "/upload/drive/v3/files") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "up1",
				"name":        "upload.bin",
				"mimeType":    "application/octet-stream",
				"webViewLink": "https://example.com/up1",
			})
			return
		case strings.Contains(path, "/files") && r.Method == http.MethodPost && !strings.Contains(path, "/permissions"):
			// mkdir
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "f1",
				"name":        "Folder",
				"webViewLink": "https://example.com/f1",
			})
			return
		case strings.Contains(path, "/files/id1") && r.Method == http.MethodDelete:
			if got := r.URL.Query().Get("supportsAllDrives"); got != "true" {
				t.Fatalf("expected supportsAllDrives=true, got: %q (raw=%q)", got, r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		case strings.Contains(path, "/files/id1") && (r.Method == http.MethodPatch || r.Method == http.MethodPut):
			w.Header().Set("Content-Type", "application/json")
			requireSupportsAllDrives(t, r)
			if addParents := r.URL.Query().Get("addParents"); addParents != "" {
				if addParents != "np" {
					t.Fatalf("expected addParents=np, got: %q", r.URL.RawQuery)
				}
				if got := r.URL.Query().Get("removeParents"); got != "p0" {
					t.Fatalf("expected removeParents=p0, got: %q", r.URL.RawQuery)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "id1",
					"name":        "New",
					"parents":     []string{"np"},
					"webViewLink": "https://example.com/id1",
				})
				return
			}
			body := readBody(t, r)
			if strings.Contains(body, "\"trashed\":true") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      "id1",
					"trashed": true,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "id1",
				"name":        "New",
				"parents":     []string{"p0"},
				"webViewLink": "https://example.com/id1",
			})
			return
		case strings.Contains(path, "/files/id1/permissions") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "p1", "type": "anyone", "role": "reader"})
			return
		case strings.Contains(path, "/files/id1/permissions") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"id": "p1", "type": "anyone", "role": "reader"},
				},
			})
			return
		case strings.Contains(path, "/files/id1/permissions/p1") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	tmpFile := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(tmpFile, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "search", "hello"}); err != nil {
				t.Fatalf("search: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "upload", tmpFile, "--name", "upload.bin", "--parent", "np"}); err != nil {
				t.Fatalf("upload: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "mkdir", "Folder", "--parent", "np"}); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "rename", "id1", "New"}); err != nil {
				t.Fatalf("rename: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "move", "id1", "--parent", "np"}); err != nil {
				t.Fatalf("move: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "share", "id1", "--anyone", "--role", "reader"}); err != nil {
				t.Fatalf("share: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "permissions", "id1"}); err != nil {
				t.Fatalf("permissions: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", "a@b.com", "drive", "unshare", "id1", "p1"}); err != nil {
				t.Fatalf("unshare: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", "a@b.com", "drive", "delete", "id1"}); err != nil {
				t.Fatalf("delete: %v", err)
			}
		})
	})
}

func TestDriveShare_ValidationErrors(t *testing.T) {
	_ = captureStderr(t, func() {
		if err := Execute([]string{"--account", "a@b.com", "drive", "share", "id1"}); err == nil {
			t.Fatalf("expected error")
		}
		if err := Execute([]string{"--account", "a@b.com", "drive", "share", "id1", "--anyone", "--role", "nope"}); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestExecute_DriveMoreCommands_Text(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/files") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(path, "/files/") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "id1",
					"name":        "Doc",
					"parents":     []string{"p0"},
					"webViewLink": "https://example.com/id1",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "id1", "name": "Doc", "mimeType": "application/pdf"},
				},
			})
			return
		case strings.Contains(path, "/upload/drive/v3/files") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "up1",
				"name":        "upload.bin",
				"mimeType":    "application/octet-stream",
				"webViewLink": "https://example.com/up1",
			})
			return
		case strings.Contains(path, "/files") && r.Method == http.MethodPost && !strings.Contains(path, "/permissions"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "f1",
				"name":        "Folder",
				"webViewLink": "https://example.com/f1",
			})
			return
		case strings.Contains(path, "/files/id1") && r.Method == http.MethodDelete:
			if got := r.URL.Query().Get("supportsAllDrives"); got != "true" {
				t.Fatalf("expected supportsAllDrives=true, got: %q (raw=%q)", got, r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		case strings.Contains(path, "/files/id1") && (r.Method == http.MethodPatch || r.Method == http.MethodPut):
			w.Header().Set("Content-Type", "application/json")
			requireSupportsAllDrives(t, r)
			body := readBody(t, r)
			if strings.Contains(body, "\"trashed\":true") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      "id1",
					"trashed": true,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "id1",
				"name":        "New",
				"parents":     []string{"p0"},
				"webViewLink": "https://example.com/id1",
			})
			return
		case strings.Contains(path, "/files/id1/permissions") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "p1", "type": "anyone", "role": "reader"})
			return
		case strings.Contains(path, "/files/id1/permissions") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"id": "p1", "type": "anyone", "role": "reader"},
				},
			})
			return
		case strings.Contains(path, "/files/id1/permissions/p1") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	tmpFile := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(tmpFile, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "drive", "search", "hello"}); err != nil {
				t.Fatalf("search: %v", err)
			}
			if err := Execute([]string{"--account", "a@b.com", "drive", "upload", tmpFile, "--name", "upload.bin", "--parent", "np"}); err != nil {
				t.Fatalf("upload: %v", err)
			}
			if err := Execute([]string{"--account", "a@b.com", "drive", "mkdir", "Folder", "--parent", "np"}); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := Execute([]string{"--account", "a@b.com", "drive", "rename", "id1", "New"}); err != nil {
				t.Fatalf("rename: %v", err)
			}
			if err := Execute([]string{"--account", "a@b.com", "drive", "move", "id1", "--parent", "np"}); err != nil {
				t.Fatalf("move: %v", err)
			}
			if err := Execute([]string{"--account", "a@b.com", "drive", "share", "id1", "--anyone", "--role", "reader"}); err != nil {
				t.Fatalf("share: %v", err)
			}
			if err := Execute([]string{"--account", "a@b.com", "drive", "permissions", "id1"}); err != nil {
				t.Fatalf("permissions: %v", err)
			}
			if err := Execute([]string{"--force", "--account", "a@b.com", "drive", "unshare", "id1", "p1"}); err != nil {
				t.Fatalf("unshare: %v", err)
			}
			if err := Execute([]string{"--force", "--account", "a@b.com", "drive", "delete", "id1"}); err != nil {
				t.Fatalf("delete: %v", err)
			}
		})
	})
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected text output")
	}
}
