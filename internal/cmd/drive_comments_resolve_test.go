package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestDriveCommentsResolveCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPatch && path == "/files/file1/comments/c1":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if payload["resolved"] != true {
				t.Fatalf("expected resolved=true, got: %#v", payload["resolved"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "c1",
				"resolved":     true,
				"modifiedTime": "2026-02-04T12:00:00Z",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	// Test JSON output
	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "comments", "resolve", "file1", "c1"}); err != nil {
				t.Fatalf("Execute resolve: %v", err)
			}
		})
	})
	var parsed struct {
		FileID    string `json:"fileId"`
		CommentID string `json:"commentId"`
		Resolved  bool   `json:"resolved"`
		Modified  string `json:"modified"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, jsonOut)
	}
	if parsed.FileID != "file1" || parsed.CommentID != "c1" || !parsed.Resolved {
		t.Fatalf("unexpected json: %#v", parsed)
	}

	// Test plain text output
	plainOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "drive", "comments", "resolve", "file1", "c1"}); err != nil {
				t.Fatalf("Execute resolve plain: %v", err)
			}
		})
	})
	if !strings.Contains(plainOut, "resolved") || !strings.Contains(plainOut, "true") {
		t.Fatalf("unexpected plain output: %q", plainOut)
	}
}

func TestDocsCommentsResolveCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPatch && path == "/files/doc1/comments/c1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "c1",
				"resolved":     true,
				"modifiedTime": "2026-02-04T12:00:00Z",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	// Test docs comments resolve (should use same drive endpoint)
	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "resolve", "doc1", "c1"}); err != nil {
				t.Fatalf("Execute docs comments resolve: %v", err)
			}
		})
	})
	var parsed struct {
		Resolved bool `json:"resolved"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, jsonOut)
	}
	if !parsed.Resolved {
		t.Fatalf("expected resolved=true, got: %#v", parsed)
	}
}

func TestDocsCommentsReadCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/files/doc1/comments/c1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "c1",
				"content":     "test comment",
				"createdTime": "2026-02-04T12:00:00Z",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	// Test docs comments read (alias to get)
	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "read", "doc1", "c1"}); err != nil {
				t.Fatalf("Execute docs comments read: %v", err)
			}
		})
	})
	if !strings.Contains(jsonOut, "test comment") {
		t.Fatalf("unexpected read output: %q", jsonOut)
	}
}
