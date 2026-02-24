package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_DriveGet_Text(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "id1",
			"name":         "Doc",
			"mimeType":     "application/pdf",
			"size":         "1024",
			"createdTime":  "2025-12-11T00:00:00Z",
			"modifiedTime": "2025-12-12T14:37:47Z",
			"starred":      true,
			"webViewLink":  "https://example.com/id1",
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "drive", "get", "id1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "id\tid1") || !strings.Contains(out, "name\tDoc") || !strings.Contains(out, "starred\ttrue") || !strings.Contains(out, "link\thttps://example.com/id1") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_DrivePermissions_Text_NoPermissions(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/permissions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"permissions": []any{}})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "drive", "permissions", "id1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(errOut, "No permissions") {
		t.Fatalf("unexpected stderr=%q", errOut)
	}
}

func TestExecute_DrivePermissions_Text_WithPermissions(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/permissions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"permissions": []map[string]any{
				{"id": "p1", "type": "anyone", "role": "reader"},
				{"id": "p2", "type": "user", "role": "writer", "emailAddress": "a@b.com"},
			},
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "drive", "permissions", "id1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "ID") || !strings.Contains(out, "EMAIL") || !strings.Contains(out, "p1") || !strings.Contains(out, "p2") || !strings.Contains(out, "a@b.com") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_DriveSearch_Text(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") || strings.Contains(r.URL.Path, "/files/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{"id": "id1", "name": "Doc", "mimeType": "application/pdf", "size": "1", "modifiedTime": "2025-12-12T14:37:47Z"},
			},
			"nextPageToken": "npt",
		})
	}))
	defer closeSrv()

	newDriveService = stubDriveService(svc)

	out := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "drive", "search", "Doc", "--max", "1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(errOut, "# Next page: --page npt") {
			t.Fatalf("unexpected stderr=%q", errOut)
		}
	})
	if !strings.Contains(out, "ID") || !strings.Contains(out, "Doc") || !strings.Contains(out, "file") || !strings.Contains(out, "2025-12-12") {
		t.Fatalf("unexpected out=%q", out)
	}
}
