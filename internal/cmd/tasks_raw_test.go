package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"
)

func newTasksRawTestServer(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// resolveTasklistID may list tasklists; return an empty list so the resolver falls through to the literal ID.
		if strings.HasSuffix(r.URL.Path, "/users/@me/lists") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
			return
		}
		if !strings.Contains(r.URL.Path, "/lists/") || !strings.Contains(r.URL.Path, "/tasks/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": status, "message": "mock error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func installMockTasksService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := newTasksService
	t.Cleanup(func() { newTasksService = orig })

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }
}

func fullTaskResponse(id string) map[string]any {
	return map[string]any{
		"id":     id,
		"title":  "Buy milk",
		"status": "needsAction",
		"notes":  "2% if possible",
	}
}

func TestTasksRaw_HappyPath(t *testing.T) {
	srv := newTasksRawTestServer(t, 0, fullTaskResponse("t1"))
	defer srv.Close()
	installMockTasksService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &TasksRawCmd{}, []string{"list1", "t1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if got["id"] != "t1" {
		t.Fatalf("expected id=t1, got: %v", got["id"])
	}
	if got["notes"] != "2% if possible" {
		t.Fatalf("expected notes passthrough, got: %v", got["notes"])
	}
}

func TestTasksRaw_APIError(t *testing.T) {
	srv := newTasksRawTestServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()
	installMockTasksService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksRawCmd{}, []string{"list1", "t1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 500")
		}
	})
}

func TestTasksRaw_NotFound(t *testing.T) {
	srv := newTasksRawTestServer(t, http.StatusNotFound, nil)
	defer srv.Close()
	installMockTasksService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksRawCmd{}, []string{"list1", "t1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 404")
		}
	})
}

func TestTasksRaw_EmptyIDs(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&TasksRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty tasklistId")
	}
	if err := (&TasksRawCmd{TasklistID: "list1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty taskId")
	}
}
