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

func TestDriveComments_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DriveCommentsListCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected list missing fileId error")
	}
	if err := (&DriveCommentsListCmd{FileID: "f1", Max: 0}).Run(ctx, flags); err == nil {
		t.Fatalf("expected list max error")
	}
	if err := (&DriveCommentsListCmd{FileID: "f1", Max: 1, Since: "2026-06-04T10:00:00"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected list since error")
	}
	if err := (&DriveCommentsGetCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected get missing fileId error")
	}
	if err := (&DriveCommentsGetCmd{FileID: "f1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected get missing commentId error")
	}
	if err := (&DriveCommentsCreateCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected create missing fileId error")
	}
	if err := (&DriveCommentsCreateCmd{FileID: "f1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected create missing content error")
	}
	if err := (&DriveCommentsUpdateCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected update missing fileId error")
	}
	if err := (&DriveCommentsUpdateCmd{FileID: "f1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected update missing commentId error")
	}
	if err := (&DriveCommentsUpdateCmd{FileID: "f1", CommentID: "c1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected update missing content error")
	}
	if err := (&DriveCommentsDeleteCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected delete missing fileId error")
	}
	if err := (&DriveCommentsDeleteCmd{FileID: "f1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected delete missing commentId error")
	}
	if err := (&DriveCommentReplyCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected reply missing fileId error")
	}
	if err := (&DriveCommentReplyCmd{FileID: "f1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected reply missing commentId error")
	}
	if err := (&DriveCommentReplyCmd{FileID: "f1", CommentID: "c1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected reply missing content error")
	}
}

func TestDriveCommentsList_NoQuoted(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/f1/comments") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"comments": []map[string]any{
					{
						"id":          "c1",
						"author":      map[string]any{"displayName": "A"},
						"content":     "Hello",
						"createdTime": "2025-01-01T00:00:00Z",
						"resolved":    false,
						"replies":     []map[string]any{},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		if err := runKong(t, &DriveCommentsListCmd{}, []string{"f1"}, ctx, flags); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
	if !strings.Contains(out, "Hello") {
		t.Fatalf("unexpected output: %q", out)
	}
}
