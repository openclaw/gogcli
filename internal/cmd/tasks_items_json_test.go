package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestTasksItems_JSONPaths(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tasks/v1/users/@me/lists" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "l1", "title": "One"},
				},
			})
			return
		case strings.HasSuffix(r.URL.Path, "/tasks/v1/lists/l1/tasks") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items":         []map[string]any{{"id": "t1", "title": "Task"}},
				"nextPageToken": "next",
			})
			return
		case strings.HasSuffix(r.URL.Path, "/tasks/v1/lists/l1/tasks") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1", "title": "Task"})
			return
		case strings.HasSuffix(r.URL.Path, "/tasks/v1/lists/l1/tasks/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1", "title": "Task"})
			return
		case strings.Contains(r.URL.Path, "/tasks/v1/lists/l1/tasks/t1") && r.Method == http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1", "title": "Task", "status": "completed"})
			return
		case strings.Contains(r.URL.Path, "/tasks/v1/lists/l1/tasks/t1") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1", "title": "Task"})
			return
		case strings.Contains(r.URL.Path, "/tasks/v1/lists/l1/tasks/t1") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		case r.URL.Path == "/tasks/v1/lists/l1/clear" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	// list
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksListCmd{}, []string{
			"l1",
			"--due-min", "2025-01-01T00:00:00Z",
			"--due-max", "2025-01-02T00:00:00Z",
			"--completed-min", "2025-01-01T00:00:00Z",
			"--completed-max", "2025-01-02T00:00:00Z",
			"--updated-min", "2025-01-01T00:00:00Z",
		}, ctx, flags); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	// add
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksAddCmd{}, []string{
			"l1",
			"--title", "Task",
		}, ctx, flags); err != nil {
			t.Fatalf("add: %v", err)
		}
	})

	// get
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksGetCmd{}, []string{
			"l1", "t1",
		}, ctx, flags); err != nil {
			t.Fatalf("get: %v", err)
		}
	})

	// update
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksUpdateCmd{}, []string{
			"l1", "t1",
			"--status", "completed",
		}, ctx, flags); err != nil {
			t.Fatalf("update: %v", err)
		}
	})

	// done
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksDoneCmd{}, []string{"l1", "t1"}, ctx, flags); err != nil {
			t.Fatalf("done: %v", err)
		}
	})

	// undo
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksUndoCmd{}, []string{"l1", "t1"}, ctx, flags); err != nil {
			t.Fatalf("undo: %v", err)
		}
	})

	// delete
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksDeleteCmd{}, []string{"l1", "t1"}, ctx, flags); err != nil {
			t.Fatalf("delete: %v", err)
		}
	})

	// clear
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksClearCmd{}, []string{"l1"}, ctx, flags); err != nil {
			t.Fatalf("clear: %v", err)
		}
	})
}

func TestTasksAddCmd_MissingTitle(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	if err := runKong(t, &TasksAddCmd{}, []string{"l1"}, context.Background(), flags); err == nil {
		t.Fatalf("expected error")
	}
}
