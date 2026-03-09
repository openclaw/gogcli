package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailFiltersCreate_Validation(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", Force: true}

	cmd := &GmailFiltersCreateCmd{}
	if err := runKong(t, cmd, []string{}, context.Background(), flags); err == nil {
		t.Fatalf("expected missing criteria error")
	}

	cmd = &GmailFiltersCreateCmd{}
	if err := runKong(t, cmd, []string{"--from", "a@example.com"}, context.Background(), flags); err == nil {
		t.Fatalf("expected missing action error")
	}
}

func TestGmailFiltersCreate_Forward_NoInputRequiresForce(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", NoInput: true}
	cmd := &GmailFiltersCreateCmd{}
	err := runKong(t, cmd, []string{"--from", "a@example.com", "--forward", "f@example.com"}, context.Background(), flags)
	if err == nil || !strings.Contains(err.Error(), "refusing to create gmail filter forwarding") {
		t.Fatalf("expected refusing error, got %v", err)
	}
}

func TestGmailFilters_TextPaths(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var createReq gmail.Filter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX"},
					{"id": "Label_1", "name": "Custom"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(r.URL.Path, "/filters/") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "f1",
					"criteria": map[string]any{
						"from":           "a@example.com",
						"to":             "b@example.com",
						"subject":        "hi",
						"query":          "q",
						"hasAttachment":  true,
						"negatedQuery":   "-spam",
						"size":           10,
						"sizeComparison": "larger",
						"excludeChats":   true,
					},
					"action": map[string]any{
						"addLabelIds":    []string{"Label_1"},
						"removeLabelIds": []string{"INBOX"},
						"forward":        "f@example.com",
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"filter": []map[string]any{
					{"id": "f1", "criteria": map[string]any{"from": "a@example.com"}},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&createReq)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "f2",
				"criteria": map[string]any{
					"from":    "a@example.com",
					"to":      "b@example.com",
					"subject": "hi",
					"query":   "q",
				},
				"action": map[string]any{
					"addLabelIds": []string{"Label_1"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
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

	flags := &RootFlags{Account: "a@b.com", Force: true}

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		if err := runKong(t, &GmailFiltersListCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("list: %v", err)
		}

		if err := runKong(t, &GmailFiltersGetCmd{}, []string{"f1"}, ctx, flags); err != nil {
			t.Fatalf("get: %v", err)
		}

		if err := runKong(t, &GmailFiltersCreateCmd{}, []string{
			"--from", "a@example.com",
			"--to", "b@example.com",
			"--subject", "hi",
			"--query", "q",
			"--has-attachment",
			"--add-label", "Custom",
			"--remove-label", "INBOX",
			"--archive",
			"--mark-read",
			"--star",
			"--forward", "f@example.com",
			"--trash",
			"--never-spam",
			"--important",
		}, ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}

		if err := runKong(t, &GmailFiltersDeleteCmd{}, []string{"f2"}, ctx, flags); err != nil {
			t.Fatalf("delete: %v", err)
		}
	})

	if createReq.Action == nil || len(createReq.Action.AddLabelIds) == 0 {
		t.Fatalf("expected add labels in create request")
	}
}

func TestGmailFiltersList_NoFilters(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"filter": []map[string]any{}})
			return
		}
		http.NotFound(w, r)
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
	_ = captureStderr(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		if err := runKong(t, &GmailFiltersListCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
}

func TestGmailFiltersExport(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"filter": []map[string]any{
					{"id": "f1", "criteria": map[string]any{"from": "a@example.com"}},
				},
			})
			return
		}
		http.NotFound(w, r)
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

	t.Run("stdout json", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := runKong(t, &GmailFiltersExportCmd{}, []string{}, ctx, flags); err != nil {
				t.Fatalf("export stdout: %v", err)
			}
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("json parse: %v", err)
		}
		filters, ok := payload["filters"].([]any)
		if !ok || len(filters) != 1 {
			t.Fatalf("unexpected payload: %#v", payload)
		}
	})

	t.Run("file export", func(t *testing.T) {
		path := t.TempDir() + "/filters.json"
		if err := runKong(t, &GmailFiltersExportCmd{}, []string{"--out", path}, ctx, flags); err != nil {
			t.Fatalf("export file: %v", err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read export: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(b, &payload); err != nil {
			t.Fatalf("json parse: %v", err)
		}
		filters, ok := payload["filters"].([]any)
		if !ok || len(filters) != 1 {
			t.Fatalf("unexpected payload: %#v", payload)
		}
	})
}
