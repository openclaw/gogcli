package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newLabelsDeleteService(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }
}

func newLabelsDeleteContext(t *testing.T, jsonMode bool) context.Context {
	t.Helper()

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	if jsonMode {
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	}
	return ctx
}

func isLabelsListPath(path string) bool {
	return strings.HasSuffix(path, "/users/me/labels") || strings.HasSuffix(path, "/gmail/v1/users/me/labels")
}

func isLabelsItemPath(path string) bool {
	return (strings.Contains(path, "/users/me/labels/") || strings.Contains(path, "/gmail/v1/users/me/labels/")) && !isLabelsListPath(path)
}

func pathTail(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return path
	}
	return path[idx+1:]
}

func TestGmailLabelsDeleteCmd_JSON_ExactID(t *testing.T) {
	deleteCalled := false
	listCalled := false

	newLabelsDeleteService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			if pathTail(r.URL.Path) != "Label_123" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_123", "name": "My Label", "type": "user"})
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			listCalled = true
			http.Error(w, "list should not be called", http.StatusInternalServerError)
			return
		case r.Method == http.MethodDelete && isLabelsItemPath(r.URL.Path):
			deleteCalled = true
			if pathTail(r.URL.Path) != "Label_123" {
				http.Error(w, "wrong delete id", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := newLabelsDeleteContext(t, true)

	out := captureStdout(t, func() {
		if err := runKong(t, &GmailLabelsDeleteCmd{}, []string{"Label_123"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if listCalled {
		t.Fatal("unexpected list call")
	}
	if !deleteCalled {
		t.Fatal("expected delete call")
	}

	var parsed struct {
		Deleted bool   `json:"deleted"`
		ID      string `json:"id"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Deleted || parsed.ID != "Label_123" || parsed.Name != "My Label" {
		t.Fatalf("unexpected output: %#v", parsed)
	}
}

func TestGmailLabelsDeleteCmd_NameFallback(t *testing.T) {
	deleteCalled := false
	listCalled := false
	getByIDCalled := false

	newLabelsDeleteService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			id := pathTail(r.URL.Path)
			if id == "custom" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 404, "message": "Requested entity was not found."}})
				return
			}
			if id != "Label_9" {
				http.NotFound(w, r)
				return
			}
			getByIDCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_9", "name": "Custom", "type": "user"})
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			listCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{{"id": "Label_9", "name": "Custom", "type": "user"}}})
			return
		case r.Method == http.MethodDelete && isLabelsItemPath(r.URL.Path):
			deleteCalled = true
			if pathTail(r.URL.Path) != "Label_9" {
				http.Error(w, "wrong delete id", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := newLabelsDeleteContext(t, false)
	if err := runKong(t, &GmailLabelsDeleteCmd{}, []string{"custom"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !listCalled {
		t.Fatal("expected list call")
	}
	if !getByIDCalled {
		t.Fatal("expected follow-up get by resolved ID")
	}
	if !deleteCalled {
		t.Fatal("expected delete call")
	}
}

func TestGmailLabelsDeleteCmd_WrongCaseIDDoesNotResolveAsIDAlias(t *testing.T) {
	deleteCalled := false
	getByIDCalled := false

	newLabelsDeleteService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			id := pathTail(r.URL.Path)
			if id == "label_777" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 404, "message": "Requested entity was not found."}})
				return
			}
			if id == "Label_777" {
				getByIDCalled = true
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_777", "name": "Some Label", "type": "user"})
				return
			}
			http.NotFound(w, r)
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{{"id": "Label_777", "name": "Some Label", "type": "user"}}})
			return
		case r.Method == http.MethodDelete && isLabelsItemPath(r.URL.Path):
			deleteCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := newLabelsDeleteContext(t, false)

	err := runKong(t, &GmailLabelsDeleteCmd{}, []string{"label_777"}, ctx, flags)
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !strings.Contains(err.Error(), "label not found: label_777") {
		t.Fatalf("unexpected error: %v", err)
	}
	if getByIDCalled {
		t.Fatal("wrong-case ID should not resolve to exact ID")
	}
	if deleteCalled {
		t.Fatal("delete should not run")
	}
}

func TestGmailLabelsDeleteCmd_NoFallbackOnNonNotFound(t *testing.T) {
	listCalled := false

	newLabelsDeleteService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 403, "message": "forbidden"}})
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			listCalled = true
			http.Error(w, "list should not be called", http.StatusInternalServerError)
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := newLabelsDeleteContext(t, false)
	if err := runKong(t, &GmailLabelsDeleteCmd{}, []string{"Label_403"}, ctx, flags); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(strings.ToLower(err.Error()), "forbidden") {
		t.Fatalf("unexpected error: %v", err)
	}
	if listCalled {
		t.Fatal("unexpected list fallback")
	}
}

func TestGmailLabelsDeleteCmd_SystemLabelBlocked(t *testing.T) {
	deleteCalled := false

	newLabelsDeleteService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "INBOX", "name": "INBOX", "type": "system"})
			return
		case r.Method == http.MethodDelete && isLabelsItemPath(r.URL.Path):
			deleteCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := newLabelsDeleteContext(t, false)
	err := runKong(t, &GmailLabelsDeleteCmd{}, []string{"INBOX"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `cannot delete system label "INBOX"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleteCalled {
		t.Fatal("delete should not run for system labels")
	}
}
