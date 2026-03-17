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

func newLabelsStyleService(t *testing.T, handler http.HandlerFunc) {
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

func newLabelsStyleContext(t *testing.T, jsonMode bool) context.Context {
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

func TestGmailLabelsStyleCmd_JSON_ExactID(t *testing.T) {
	patchCalled := false

	newLabelsStyleService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			if pathTail(r.URL.Path) != "Label_1" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_1", "name": "MyLabel", "type": "user"})
			return
		case r.Method == http.MethodPatch && isLabelsItemPath(r.URL.Path):
			patchCalled = true
			if pathTail(r.URL.Path) != "Label_1" {
				http.Error(w, "wrong patch id", http.StatusBadRequest)
				return
			}
			var body struct {
				Color struct {
					BackgroundColor string `json:"backgroundColor"`
					TextColor       string `json:"textColor"`
				} `json:"color"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Color.BackgroundColor != "#16a765" || body.Color.TextColor != "#ffffff" {
				http.Error(w, "unexpected color", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_1",
				"name": "MyLabel",
				"type": "user",
				"color": map[string]any{
					"backgroundColor": "#16a765",
					"textColor":       "#ffffff",
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsStyleContext(t, true)

	out := captureStdout(t, func() {
		if err := runKong(t, &GmailLabelsStyleCmd{}, []string{"Label_1", "--background-color", "#16a765", "--text-color", "#ffffff"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !patchCalled {
		t.Fatal("expected patch call")
	}

	var parsed struct {
		Label struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Color struct {
				BackgroundColor string `json:"backgroundColor"`
				TextColor       string `json:"textColor"`
			} `json:"color"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.Color.BackgroundColor != "#16a765" || parsed.Label.Color.TextColor != "#ffffff" {
		t.Fatalf("unexpected color output: %#v", parsed.Label.Color)
	}
}

func TestGmailLabelsStyleCmd_NameFallback(t *testing.T) {
	patchCalled := false
	listCalled := false

	newLabelsStyleService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && isLabelsItemPath(r.URL.Path):
			id := pathTail(r.URL.Path)
			if id == "MyLabel" || id == "mylabel" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 404, "message": "Requested entity was not found."}})
				return
			}
			if id == "Label_5" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "Label_5", "name": "MyLabel", "type": "user"})
				return
			}
			http.NotFound(w, r)
			return
		case r.Method == http.MethodGet && isLabelsListPath(r.URL.Path):
			listCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]any{
				{"id": "Label_5", "name": "MyLabel", "type": "user"},
			}})
			return
		case r.Method == http.MethodPatch && isLabelsItemPath(r.URL.Path):
			patchCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_5",
				"name": "MyLabel",
				"type": "user",
				"color": map[string]any{
					"backgroundColor": "#16a765",
					"textColor":       "#ffffff",
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsStyleContext(t, false)

	var buf strings.Builder
	u, err := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx = ui.WithUI(ctx, u)

	if err := runKong(t, &GmailLabelsStyleCmd{}, []string{"MyLabel", "--background-color", "#16a765", "--text-color", "#ffffff"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !listCalled {
		t.Fatal("expected list call for name fallback")
	}
	if !patchCalled {
		t.Fatal("expected patch call")
	}
	out := buf.String()
	if !strings.Contains(out, "Styled label") {
		t.Fatalf("missing 'Styled label' in output: %q", out)
	}
}

func TestGmailLabelsStyleCmd_SystemLabelBlocked(t *testing.T) {
	patchCalled := false

	newLabelsStyleService(t, func(w http.ResponseWriter, r *http.Request) {
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
	ctx := newLabelsStyleContext(t, false)
	err := runKong(t, &GmailLabelsStyleCmd{}, []string{"INBOX", "--background-color", "#16a765", "--text-color", "#ffffff"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `cannot style system label "INBOX"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled {
		t.Fatal("patch should not run for system labels")
	}
}

func TestGmailLabelsStyleCmd_NotFound(t *testing.T) {
	newLabelsStyleService(t, func(w http.ResponseWriter, r *http.Request) {
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
	ctx := newLabelsStyleContext(t, false)
	err := runKong(t, &GmailLabelsStyleCmd{}, []string{"NoSuchLabel", "--background-color", "#16a765", "--text-color", "#ffffff"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "label not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailLabelsStyleCmd_MissingBothColors(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsStyleContext(t, false)
	err := runKong(t, &GmailLabelsStyleCmd{}, []string{"Label_1"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing colors")
	}
	if !strings.Contains(err.Error(), "both --background-color and --text-color are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailLabelsStyleCmd_MissingOneColor(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newLabelsStyleContext(t, false)

	err := runKong(t, &GmailLabelsStyleCmd{}, []string{"Label_1", "--background-color", "#16a765"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing text-color")
	}
	if !strings.Contains(err.Error(), "both --background-color and --text-color are required") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = runKong(t, &GmailLabelsStyleCmd{}, []string{"Label_1", "--text-color", "#ffffff"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing background-color")
	}
	if !strings.Contains(err.Error(), "both --background-color and --text-color are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
