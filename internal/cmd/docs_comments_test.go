package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

// newCommentsTestServer returns a test server that handles the Drive comments API
// endpoints needed by docs comments commands.
func newCommentsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		// List comments
		case r.Method == http.MethodGet && path == "/files/doc1/comments":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"comments": []map[string]any{
					{
						"id":          "c1",
						"author":      map[string]any{"displayName": "Alice"},
						"content":     "Needs revision",
						"createdTime": "2025-06-01T10:00:00Z",
						"resolved":    false,
						"quotedFileContent": map[string]any{
							"value": "The quick brown fox",
						},
						"replies": []map[string]any{
							{
								"id":          "r1",
								"author":      map[string]any{"displayName": "Bob"},
								"content":     "Working on it",
								"createdTime": "2025-06-01T11:00:00Z",
							},
						},
					},
					{
						"id":          "c2",
						"author":      map[string]any{"displayName": "Charlie"},
						"content":     "LGTM",
						"createdTime": "2025-06-01T09:00:00Z",
						"resolved":    true,
					},
				},
			})
			return

		// List comments: first page has only resolved, second page has open.
		case r.Method == http.MethodGet && path == "/files/scan/comments":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("pageToken") == "p2" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"comments": []map[string]any{
						{
							"id":          "c-open",
							"author":      map[string]any{"displayName": "Dana"},
							"content":     "Open comment",
							"createdTime": "2025-06-03T10:00:00Z",
							"resolved":    false,
						},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"comments": []map[string]any{
					{
						"id":          "c-res",
						"author":      map[string]any{"displayName": "Eli"},
						"content":     "Resolved only",
						"createdTime": "2025-06-03T09:00:00Z",
						"resolved":    true,
					},
				},
				"nextPageToken": "p2",
			})
			return

		// List comments on empty doc
		case r.Method == http.MethodGet && path == "/files/empty/comments":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"comments": []any{}})
			return

		// Get single comment
		case r.Method == http.MethodGet && path == "/files/doc1/comments/c1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "c1",
				"author":       map[string]any{"displayName": "Alice"},
				"content":      "Needs revision",
				"createdTime":  "2025-06-01T10:00:00Z",
				"modifiedTime": "2025-06-01T10:30:00Z",
				"resolved":     false,
				"quotedFileContent": map[string]any{
					"value": "The quick brown fox",
				},
				"replies": []map[string]any{
					{
						"id":          "r1",
						"author":      map[string]any{"displayName": "Bob"},
						"content":     "Working on it",
						"createdTime": "2025-06-01T11:00:00Z",
					},
				},
			})
			return

		// Create comment
		case r.Method == http.MethodPost && path == "/files/doc1/comments":
			var body struct {
				Content           string `json:"content"`
				Anchor            string `json:"anchor"`
				QuotedFileContent struct {
					Value string `json:"value"`
				} `json:"quotedFileContent"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "c3",
				"content":     body.Content,
				"createdTime": "2025-06-02T08:00:00Z",
				"anchor":      body.Anchor,
				"quotedFileContent": map[string]any{
					"value": body.QuotedFileContent.Value,
				},
			})
			return

		// Delete comment
		case r.Method == http.MethodDelete && path == "/files/doc1/comments/c1":
			w.WriteHeader(http.StatusNoContent)
			return

		// Create reply
		case r.Method == http.MethodPost && path == "/files/doc1/comments/c1/replies":
			var body struct {
				Content string `json:"content"`
				Action  string `json:"action"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"id":          "r2",
				"content":     body.Content,
				"createdTime": "2025-06-02T09:00:00Z",
			}
			if body.Action != "" {
				resp["action"] = body.Action
			}
			_ = json.NewEncoder(w).Encode(resp)
			return

		default:
			http.NotFound(w, r)
			return
		}
	}))
}

func setupDriveServiceFromServer(t *testing.T, srv *httptest.Server) {
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

func TestDocsCommentsList_FiltersResolved(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	// Default: open only
	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "list", "doc1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DocID    string           `json:"docId"`
		Comments []*drive.Comment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.DocID != "doc1" {
		t.Fatalf("expected docId=doc1, got %q", parsed.DocID)
	}
	// Should only have the open comment (c1), not the resolved one (c2)
	if len(parsed.Comments) != 1 {
		t.Fatalf("expected 1 open comment, got %d", len(parsed.Comments))
	}
	if parsed.Comments[0].Id != "c1" {
		t.Fatalf("expected comment c1, got %q", parsed.Comments[0].Id)
	}
}

func TestDocsCommentsList_IncludeResolved(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "list", "--include-resolved", "doc1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Comments []*drive.Comment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if len(parsed.Comments) != 2 {
		t.Fatalf("expected 2 comments with --include-resolved, got %d", len(parsed.Comments))
	}
}

func TestDocsCommentsList_PlainText(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "docs", "comments", "list", "doc1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Alice") {
		t.Fatalf("expected author Alice in output, got: %q", out)
	}
	if !strings.Contains(out, "Needs revision") {
		t.Fatalf("expected comment content in output, got: %q", out)
	}
	if !strings.Contains(out, "TYPE") {
		t.Fatalf("expected TYPE header in output, got: %q", out)
	}
	if !strings.Contains(out, "Working on it") {
		t.Fatalf("expected reply content in output, got: %q", out)
	}
	// Resolved comment should be filtered out in default mode
	if strings.Contains(out, "LGTM") {
		t.Fatalf("resolved comment should be filtered, got: %q", out)
	}
}

func TestDocsCommentsList_ScansPagesForOpenComments(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "list", "scan"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Comments []*drive.Comment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if len(parsed.Comments) != 1 || parsed.Comments[0].Id != "c-open" {
		t.Fatalf("expected scan to return open comment, got %#v", parsed.Comments)
	}
}

func TestDocsCommentsList_Empty(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	errOut := captureStderr(t, func() {
		if err := Execute([]string{"--account", "a@b.com", "docs", "comments", "list", "empty"}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	if !strings.Contains(errOut, "No comments") {
		t.Fatalf("expected 'No comments' in stderr, got: %q", errOut)
	}
}

func TestDocsCommentsGet_JSON(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "get", "doc1", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Comment *drive.Comment `json:"comment"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.Comment == nil || parsed.Comment.Id != "c1" {
		t.Fatalf("unexpected comment: %#v", parsed.Comment)
	}
	if parsed.Comment.QuotedFileContent == nil || parsed.Comment.QuotedFileContent.Value != "The quick brown fox" {
		t.Fatalf("missing quoted content: %#v", parsed.Comment)
	}
}

func TestDocsCommentsGet_Plain(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "docs", "comments", "get", "doc1", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Alice") {
		t.Fatalf("expected author in output: %q", out)
	}
	if !strings.Contains(out, "Needs revision") {
		t.Fatalf("expected content in output: %q", out)
	}
	if !strings.Contains(out, "The quick brown fox") {
		t.Fatalf("expected quoted text in output: %q", out)
	}
	if !strings.Contains(out, "reply") {
		t.Fatalf("expected reply info in output: %q", out)
	}
}

func TestDocsCommentsAdd_JSON(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "add", "doc1", "Nice work", "--quoted", "some text", "--anchor", "{\"a\":1}"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Comment *drive.Comment `json:"comment"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.Comment == nil || parsed.Comment.Id != "c3" {
		t.Fatalf("unexpected comment: %#v", parsed.Comment)
	}
	if parsed.Comment.Content != "Nice work" {
		t.Fatalf("expected content 'Nice work', got %q", parsed.Comment.Content)
	}
	if parsed.Comment.Anchor != "{\"a\":1}" {
		t.Fatalf("expected anchor, got %q", parsed.Comment.Anchor)
	}
}

func TestDocsCommentsAdd_Plain(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "docs", "comments", "add", "doc1", "A comment"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "c3") {
		t.Fatalf("expected comment ID in output: %q", out)
	}
}

func TestDocsCommentsReply_JSON(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "reply", "doc1", "c1", "Thanks!"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Reply *drive.Reply `json:"reply"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.Reply == nil || parsed.Reply.Id != "r2" {
		t.Fatalf("unexpected reply: %#v", parsed.Reply)
	}
}

func TestDocsCommentsResolve_JSON(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "docs", "comments", "resolve", "doc1", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Resolved  bool   `json:"resolved"`
		DocID     string `json:"docId"`
		CommentID string `json:"commentId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if !parsed.Resolved || parsed.DocID != "doc1" || parsed.CommentID != "c1" {
		t.Fatalf("unexpected resolve output: %#v", parsed)
	}
}

func TestDocsCommentsDelete_JSON(t *testing.T) {
	srv := newCommentsTestServer(t)
	defer srv.Close()
	setupDriveServiceFromServer(t, srv)

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", "a@b.com", "docs", "comments", "delete", "doc1", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Deleted   bool   `json:"deleted"`
		DocID     string `json:"docId"`
		CommentID string `json:"commentId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if !parsed.Deleted || parsed.DocID != "doc1" || parsed.CommentID != "c1" {
		t.Fatalf("unexpected delete output: %#v", parsed)
	}
}

func TestDocsComments_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DocsCommentsListCmd{}).Run(ctx, flags); err == nil {
		t.Fatal("expected list missing docId error")
	}
	if err := (&DocsCommentsGetCmd{}).Run(ctx, flags); err == nil {
		t.Fatal("expected get missing docId error")
	}
	if err := (&DocsCommentsGetCmd{DocID: "d1"}).Run(ctx, flags); err == nil {
		t.Fatal("expected get missing commentId error")
	}
	if err := (&DocsCommentsAddCmd{}).Run(ctx, flags); err == nil {
		t.Fatal("expected add missing docId error")
	}
	if err := (&DocsCommentsAddCmd{DocID: "d1"}).Run(ctx, flags); err == nil {
		t.Fatal("expected add missing content error")
	}
	if err := (&DocsCommentsReplyCmd{}).Run(ctx, flags); err == nil {
		t.Fatal("expected reply missing docId error")
	}
	if err := (&DocsCommentsReplyCmd{DocID: "d1"}).Run(ctx, flags); err == nil {
		t.Fatal("expected reply missing commentId error")
	}
	if err := (&DocsCommentsReplyCmd{DocID: "d1", CommentID: "c1"}).Run(ctx, flags); err == nil {
		t.Fatal("expected reply missing content error")
	}
	if err := (&DocsCommentsResolveCmd{}).Run(ctx, flags); err == nil {
		t.Fatal("expected resolve missing docId error")
	}
	if err := (&DocsCommentsResolveCmd{DocID: "d1"}).Run(ctx, flags); err == nil {
		t.Fatal("expected resolve missing commentId error")
	}
	if err := (&DocsCommentsDeleteCmd{}).Run(ctx, flags); err == nil {
		t.Fatal("expected delete missing docId error")
	}
	if err := (&DocsCommentsDeleteCmd{DocID: "d1"}).Run(ctx, flags); err == nil {
		t.Fatal("expected delete missing commentId error")
	}
}

func TestFilterOpenComments(t *testing.T) {
	comments := []*drive.Comment{
		{Id: "c1", Resolved: false},
		{Id: "c2", Resolved: true},
		{Id: "c3", Resolved: false},
		{Id: "c4", Resolved: true},
	}
	open := filterOpenComments(comments)
	if len(open) != 2 {
		t.Fatalf("expected 2 open comments, got %d", len(open))
	}
	if open[0].Id != "c1" || open[1].Id != "c3" {
		t.Fatalf("unexpected open comments: %v, %v", open[0].Id, open[1].Id)
	}
}

func TestFilterOpenComments_AllOpen(t *testing.T) {
	comments := []*drive.Comment{
		{Id: "c1", Resolved: false},
	}
	open := filterOpenComments(comments)
	if len(open) != 1 {
		t.Fatalf("expected 1, got %d", len(open))
	}
}

func TestFilterOpenComments_AllResolved(t *testing.T) {
	comments := []*drive.Comment{
		{Id: "c1", Resolved: true},
	}
	open := filterOpenComments(comments)
	if len(open) != 0 {
		t.Fatalf("expected 0, got %d", len(open))
	}
}

func TestFilterOpenComments_Nil(t *testing.T) {
	open := filterOpenComments(nil)
	if open != nil {
		t.Fatalf("expected nil, got %v", open)
	}
}

func TestFilterOpenComments_NilElements(t *testing.T) {
	comments := []*drive.Comment{
		nil,
		{Id: "c1", Resolved: true},
		nil,
		{Id: "c2", Resolved: false},
	}
	open := filterOpenComments(comments)
	if len(open) != 1 || open[0].Id != "c2" {
		t.Fatalf("unexpected open comments: %#v", open)
	}
}
