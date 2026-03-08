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

func newLabelsRenameService(t *testing.T, handler http.HandlerFunc) {
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

func newLabelsRenameContext(t *testing.T, jsonMode bool) context.Context {
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

func TestGmailLabelsRenameCmd_JSON_ExactID(t *testing.T) {
	patchCalled := false

	newLabelsRenameService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			if pathTail(r.URL.Path) != "Label_1" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_1", "name": "Old Name", "type": "user"})
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{
				{"id": "Label_1", "name": "Old Name", "type": "user"},
			}})
			return
		case r.Method == http.MethodPatch && isLabelsItemPath(r.URL.Path):
			patchCalled = true
			if pathTail(r.URL.Path) != "Label_1" {
				http.Error(w, "wrong patch id", http.StatusBadRequest)
				return
			}
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Name != "New Name" {
				http.Error(w, "unexpected name", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_1",
				"name": "New Name",
				"type": "user",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsRenameContext(t, true)

	out := captureStdout(t, func() {
		if err := runKong(t, &GmailLabelsRenameCmd{}, []string{"Label_1", "New Name"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !patchCalled {
		t.Fatal("expected patch call")
	}

	var parsed struct {
		Label struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.ID != "Label_1" || parsed.Label.Name != "New Name" {
		t.Fatalf("unexpected output: %#v", parsed.Label)
	}
}

func TestGmailLabelsRenameCmd_NameFallback(t *testing.T) {
	patchCalled := false
	listCalled := false

	newLabelsRenameService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			id := pathTail(r.URL.Path)
			if id == "old+name" || id == "old%20name" || id == "old name" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 404, "message": "Requested entity was not found."}})
				return
			}
			if id == "Label_5" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_5", "name": "Old Name", "type": "user"})
				return
			}
			http.NotFound(w, r)
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			listCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{
				{"id": "Label_5", "name": "Old Name", "type": "user"},
			}})
			return
		case r.Method == http.MethodPatch && isLabelsItemPath(r.URL.Path):
			patchCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_5",
				"name": "New Name",
				"type": "user",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsRenameContext(t, false)

	var buf strings.Builder
	u, err := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx = ui.WithUI(ctx, u)

	if err := runKong(t, &GmailLabelsRenameCmd{}, []string{"Old Name", "New Name"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !listCalled {
		t.Fatal("expected list call for name fallback")
	}
	if !patchCalled {
		t.Fatal("expected patch call")
	}
	out := buf.String()
	if !strings.Contains(out, "Renamed label:") {
		t.Fatalf("missing 'Renamed label:' in output: %q", out)
	}
}

func TestGmailLabelsRenameCmd_SystemLabelBlocked(t *testing.T) {
	patchCalled := false

	newLabelsRenameService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "INBOX", "name": "INBOX", "type": "system"})
			return
		case r.Method == http.MethodPatch && isLabelsItemPath(r.URL.Path):
			patchCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsRenameContext(t, false)
	err := runKong(t, &GmailLabelsRenameCmd{}, []string{"INBOX", "MyInbox"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `cannot rename system label "INBOX"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled {
		t.Fatal("patch should not run for system labels")
	}
}

func TestGmailLabelsRenameCmd_DuplicateNewName(t *testing.T) {
	patchCalled := false

	newLabelsRenameService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			if pathTail(r.URL.Path) == "Label_1" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_1", "name": "Source", "type": "user"})
				return
			}
			http.NotFound(w, r)
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{
				{"id": "Label_1", "name": "Source", "type": "user"},
				{"id": "Label_2", "name": "Taken", "type": "user"},
			}})
			return
		case r.Method == http.MethodPatch && isLabelsItemPath(r.URL.Path):
			patchCalled = true
			http.Error(w, "should not patch", http.StatusInternalServerError)
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsRenameContext(t, false)
	err := runKong(t, &GmailLabelsRenameCmd{}, []string{"Label_1", "Taken"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "label already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled {
		t.Fatal("patch should not run when new name is taken")
	}
}

func TestGmailLabelsRenameCmd_NotFound(t *testing.T) {
	newLabelsRenameService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 404, "message": "Requested entity was not found."}})
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{}})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsRenameContext(t, false)
	err := runKong(t, &GmailLabelsRenameCmd{}, []string{"NoSuchLabel", "Whatever"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "label not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailLabelsRenameCmd_WrongCaseIDDoesNotResolveAsIDAlias(t *testing.T) {
	listCalled := false
	patchCalled := false
	getByResolvedIDCalled := false

	newLabelsRenameService(t, func(w http.ResponseWriter, r *http.Request) {
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
				getByResolvedIDCalled = true
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_777", "name": "Some Label", "type": "user"})
				return
			}
			http.NotFound(w, r)
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			listCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{{"id": "Label_777", "name": "Some Label", "type": "user"}}})
			return
		case r.Method == http.MethodPatch && isLabelsItemPath(r.URL.Path):
			patchCalled = true
			http.Error(w, "patch should not be called", http.StatusInternalServerError)
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsRenameContext(t, false)

	err := runKong(t, &GmailLabelsRenameCmd{}, []string{"label_777", "Renamed"}, ctx, flags)
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !strings.Contains(err.Error(), "label not found: label_777") {
		t.Fatalf("unexpected error: %v", err)
	}
	if listCalled {
		t.Fatal("wrong-case ID should not trigger name fallback")
	}
	if getByResolvedIDCalled || patchCalled {
		t.Fatal("rename should not continue after wrong-case ID miss")
	}
}

func TestGmailLabelsRenameCmd_NoFallbackOnNonNotFound(t *testing.T) {
	listCalled := false

	newLabelsRenameService(t, func(w http.ResponseWriter, r *http.Request) {
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

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsRenameContext(t, false)

	err := runKong(t, &GmailLabelsRenameCmd{}, []string{"Label_403", "Renamed"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error")
	}
	if listCalled {
		t.Fatal("name fallback should not run on non-404 errors")
	}
}

func TestGmailLabelsRenameCmd_EmptyArgs(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for empty args")
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsRenameCmd{Label: "   ", NewName: "New"}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty label")
	}
	if !strings.Contains(err.Error(), "label is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd = &GmailLabelsRenameCmd{Label: "Old", NewName: "   "}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty new name")
	}
	if !strings.Contains(err.Error(), "new name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
