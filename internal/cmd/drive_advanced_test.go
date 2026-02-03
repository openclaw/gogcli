package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/option"
)

func TestDriveOrphansListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{"id": "f1", "name": "Orphan", "mimeType": "text/plain"},
			},
		})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveOrphansListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Orphan") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDriveOrphansCollectCmd(t *testing.T) {
	var updated bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{{"id": "f1", "parents": []string{}}},
			})
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/files/f1"):
			updated = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "f1"})
		default:
			http.NotFound(w, r)
		}
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &DriveOrphansCollectCmd{Folder: "folder1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !updated {
		t.Fatalf("expected update request")
	}
	if !strings.Contains(out, "Moved 1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDriveCleanupEmptyFoldersCmd(t *testing.T) {
	var deleted bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files") && strings.Contains(q, "mimeType"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{{"id": "folder1", "name": "Empty"}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files") && strings.Contains(q, "'folder1' in parents"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{}})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/files/folder1"):
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &DriveCleanupEmptyFoldersCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleted {
		t.Fatalf("expected delete")
	}
	if !strings.Contains(out, "Deleted 1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDriveRevisionsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/file1/revisions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"revisions": []map[string]any{{"id": "1", "modifiedTime": "2026-01-01T00:00:00Z", "keepForever": false}},
		})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveRevisionsListCmd{FileID: "file1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "2026-01-01") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDriveShortcutsCreateCmd(t *testing.T) {
	var gotMime string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			MimeType string `json:"mimeType"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotMime = payload.MimeType
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "shortcut1", "name": "Shortcut"})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveShortcutsCreateCmd{Target: "file1", Name: "Shortcut"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotMime != driveMimeShortcut {
		t.Fatalf("unexpected mime: %s", gotMime)
	}
	if !strings.Contains(out, "Created shortcut") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDriveActivityCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/activity:query") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activities": []map[string]any{{"timestamp": "2026-01-02T00:00:00Z", "primaryActionDetail": map[string]any{"edit": map[string]any{}}, "actors": []map[string]any{{"administrator": map[string]any{}}}}},
		})
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "edit") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDriveTransferCmd(t *testing.T) {
	var permCreated bool
	var permDeleted bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file1/permissions"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{{"id": "perm-old", "emailAddress": "old@example.com"}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/files/file1/permissions"):
			permCreated = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm-new"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{{"id": "file1"}},
			})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/files/file1/permissions/perm-old"):
			permDeleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &DriveTransferCmd{From: "old@example.com", To: "new@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !permCreated || !permDeleted {
		t.Fatalf("expected permission changes, created=%v deleted=%v", permCreated, permDeleted)
	}
	if !strings.Contains(out, "Transferred 1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubDriveActivity(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newDriveActivityService
	svc, err := driveactivity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new driveactivity service: %v", err)
	}
	newDriveActivityService = func(context.Context, string) (*driveactivity.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newDriveActivityService = orig
		srv.Close()
	})
	return srv
}
