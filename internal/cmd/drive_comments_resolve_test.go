package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

// replyCapture records the most recent request body the test server saw for
// POST /files/{fileId}/comments/{commentId}/replies. It is goroutine-safe so
// tests can read it after Execute returns.
type replyCapture struct {
	mu      sync.Mutex
	content string
	action  string
	fields  string
}

func (rc *replyCapture) snapshot() (string, string, string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.content, rc.action, rc.fields
}

// newResolveTestServer returns an httptest.Server that accepts replies on
// /files/file1/comments/c1/replies and records the outgoing body in rc.
func newResolveTestServer(t *testing.T) (*httptest.Server, *replyCapture) {
	t.Helper()
	rc := &replyCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		if r.Method == http.MethodPost && path == "/files/file1/comments/c1/replies" {
			var body struct {
				Content string `json:"content"`
				Action  string `json:"action"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			rc.mu.Lock()
			rc.content = body.Content
			rc.action = body.Action
			rc.fields = r.URL.Query().Get("fields")
			rc.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"id":          "r1",
				"content":     body.Content,
				"createdTime": "2025-01-01T02:00:00Z",
			}
			if body.Action != "" {
				resp["action"] = body.Action
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	return srv, rc
}

func setupDriveServiceFromResolveServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }
}

// TestDriveCommentsReply_WithActionResolve verifies that
// `drive comments reply --action=resolve` sends action="resolve" in the
// outgoing replies.create request body alongside the reply content, and that
// the JSON envelope reports the comment as resolved.
func TestDriveCommentsReply_WithActionResolve(t *testing.T) {
	srv, rc := newResolveTestServer(t)
	defer srv.Close()
	setupDriveServiceFromResolveServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "comments", "reply", "file1", "c1", "Resolving with context", "--action", "resolve"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	gotContent, gotAction, gotFields := rc.snapshot()
	if gotContent != "Resolving with context" {
		t.Fatalf("expected outgoing content %q, got %q", "Resolving with context", gotContent)
	}
	if gotAction != "resolve" {
		t.Fatalf("expected outgoing action=resolve, got %q", gotAction)
	}
	if !strings.Contains(gotFields, "action") {
		t.Fatalf("expected response field mask to include 'action', got %q", gotFields)
	}

	var parsed struct {
		Resolved  bool   `json:"resolved"`
		FileID    string `json:"fileId"`
		CommentID string `json:"commentId"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Resolved || parsed.FileID != "file1" || parsed.CommentID != "c1" {
		t.Fatalf("unexpected envelope: %#v", parsed)
	}
}

// TestDriveCommentsReply_WithActionReopen verifies the reopen path on the
// `reply --action` flag end-to-end.
func TestDriveCommentsReply_WithActionReopen(t *testing.T) {
	srv, rc := newResolveTestServer(t)
	defer srv.Close()
	setupDriveServiceFromResolveServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "comments", "reply", "file1", "c1", "Reopening - needs more discussion", "--action", "reopen"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	_, gotAction, _ := rc.snapshot()
	if gotAction != "reopen" {
		t.Fatalf("expected outgoing action=reopen, got %q", gotAction)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed["reopened"] != true {
		t.Fatalf("expected reopened=true in envelope, got: %#v", parsed)
	}
}

// TestDriveCommentsReply_NoActionUnchanged confirms a plain reply with no
// --action flag still posts neither action nor a "resolved" envelope, i.e.
// the existing behaviour is preserved.
func TestDriveCommentsReply_NoActionUnchanged(t *testing.T) {
	srv, rc := newResolveTestServer(t)
	defer srv.Close()
	setupDriveServiceFromResolveServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "comments", "reply", "file1", "c1", "Just a reply"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	_, gotAction, _ := rc.snapshot()
	if gotAction != "" {
		t.Fatalf("expected no outgoing action, got %q", gotAction)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if _, hasResolved := parsed["resolved"]; hasResolved {
		t.Fatalf("expected no resolved key in plain reply envelope, got: %#v", parsed)
	}
	if _, hasReply := parsed["reply"]; !hasReply {
		t.Fatalf("expected reply key in plain reply envelope, got: %#v", parsed)
	}
}

// TestDriveCommentsResolveCmd_PostsResolveAction exercises the sibling verb.
func TestDriveCommentsResolveCmd_PostsResolveAction(t *testing.T) {
	srv, rc := newResolveTestServer(t)
	defer srv.Close()
	setupDriveServiceFromResolveServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "comments", "resolve", "file1", "c1", "--message", "LGTM"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	gotContent, gotAction, _ := rc.snapshot()
	if gotAction != "resolve" {
		t.Fatalf("expected outgoing action=resolve, got %q", gotAction)
	}
	if gotContent != "LGTM" {
		t.Fatalf("expected outgoing content=LGTM, got %q", gotContent)
	}

	var parsed struct {
		Resolved  bool   `json:"resolved"`
		FileID    string `json:"fileId"`
		CommentID string `json:"commentId"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Resolved || parsed.FileID != "file1" || parsed.CommentID != "c1" {
		t.Fatalf("unexpected envelope: %#v", parsed)
	}
}

// TestDriveCommentsResolveCmd_NoMessage covers the action-only path (message
// optional; the API accepts a reply with action and no content).
func TestDriveCommentsResolveCmd_NoMessage(t *testing.T) {
	srv, rc := newResolveTestServer(t)
	defer srv.Close()
	setupDriveServiceFromResolveServer(t, srv)

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "drive", "comments", "resolve", "file1", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	gotContent, gotAction, _ := rc.snapshot()
	if gotAction != "resolve" {
		t.Fatalf("expected outgoing action=resolve, got %q", gotAction)
	}
	if gotContent != "" {
		t.Fatalf("expected empty outgoing content, got %q", gotContent)
	}
}

// TestDriveCommentsReopenCmd_PostsReopenAction exercises the reopen sibling.
func TestDriveCommentsReopenCmd_PostsReopenAction(t *testing.T) {
	srv, rc := newResolveTestServer(t)
	defer srv.Close()
	setupDriveServiceFromResolveServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "drive", "comments", "reopen", "file1", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	_, gotAction, _ := rc.snapshot()
	if gotAction != "reopen" {
		t.Fatalf("expected outgoing action=reopen, got %q", gotAction)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed["reopened"] != true {
		t.Fatalf("expected reopened=true, got: %#v", parsed)
	}
}

// TestDriveCommentsReply_InvalidAction confirms the kong enum constraint
// rejects values outside the supported set without making any network call.
func TestDriveCommentsReply_InvalidAction(t *testing.T) {
	// No HTTP server needed; we expect parsing to fail before any API call.
	err := Execute([]string{"--account", "a@b.com", "drive", "comments", "reply", "file1", "c1", "msg", "--action", "ignore"})
	if err == nil {
		t.Fatal("expected error for invalid --action value, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "action") {
		t.Fatalf("expected error mentioning --action, got: %v", err)
	}
}

// TestValidateDriveReplyAction unit-tests the helper directly.
func TestValidateDriveReplyAction(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"resolve", "resolve", false},
		{"REOPEN", "reopen", false},
		{"  resolve  ", "resolve", false},
		{"reject", "", true},
	}
	for _, tc := range cases {
		got, err := validateDriveReplyAction(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateDriveReplyAction(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("validateDriveReplyAction(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestDocsCommentsReopenCmd_PostsReopenAction adds parity coverage for the
// docs surface: the new docs sibling reopen verb posts action="reopen".
func TestDocsCommentsReopenCmd_PostsReopenAction(t *testing.T) {
	rc := &replyCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		if r.Method == http.MethodPost && path == "/files/doc1/comments/c1/replies" {
			var body struct {
				Content string `json:"content"`
				Action  string `json:"action"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			rc.mu.Lock()
			rc.content = body.Content
			rc.action = body.Action
			rc.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "r1",
				"content":     body.Content,
				"createdTime": "2025-01-01T02:00:00Z",
				"action":      body.Action,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	setupDriveServiceFromResolveServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "reopen", "doc1", "c1", "--message", "still open"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	gotContent, gotAction, _ := rc.snapshot()
	if gotAction != "reopen" {
		t.Fatalf("expected docs reopen action=reopen, got %q", gotAction)
	}
	if gotContent != "still open" {
		t.Fatalf("expected outgoing content=still open, got %q", gotContent)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed["reopened"] != true {
		t.Fatalf("expected reopened=true in docs reopen envelope, got: %#v", parsed)
	}
	if parsed["docId"] != "doc1" {
		t.Fatalf("expected docId=doc1, got: %#v", parsed["docId"])
	}
}

// TestDriveCommentsReplyAction_ValidationErrors exercises the unit-level
// validation paths via direct Run invocation (mirrors existing
// TestDocsComments_ValidationErrors style).
func TestDriveCommentsReplyAction_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DriveCommentsResolveCmd{}).Run(ctx, flags); err == nil {
		t.Fatal("expected resolve missing fileId error")
	}
	if err := (&DriveCommentsResolveCmd{FileID: "f1"}).Run(ctx, flags); err == nil {
		t.Fatal("expected resolve missing commentId error")
	}
	if err := (&DriveCommentsReopenCmd{}).Run(ctx, flags); err == nil {
		t.Fatal("expected reopen missing fileId error")
	}
	if err := (&DriveCommentsReopenCmd{FileID: "f1"}).Run(ctx, flags); err == nil {
		t.Fatal("expected reopen missing commentId error")
	}
	// Reply with invalid action should fail validation at Run time too
	// (kong enum catches CLI parsing, but constructing the struct directly
	// goes through validateDriveReplyAction).
	if err := (&DriveCommentReplyCmd{FileID: "f1", CommentID: "c1", Content: "x", Action: "ignore"}).Run(ctx, flags); err == nil {
		t.Fatal("expected reply with invalid --action to error")
	}
}
