package cmd

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
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
	origNow := nowGmailFiltersExport
	t.Cleanup(func() {
		newGmailService = origNew
		nowGmailFiltersExport = origNow
	})
	nowGmailFiltersExport = func() time.Time { return time.Date(2026, 5, 5, 1, 2, 3, 0, time.UTC) }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "Notifications & Alerts"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"filter": []map[string]any{
					{
						"id": "f1",
						"criteria": map[string]any{
							"from":           "a@example.com",
							"to":             "b@example.com",
							"subject":        "A&B",
							"query":          `from:alerts has:attachment`,
							"negatedQuery":   "category:promotions",
							"hasAttachment":  true,
							"excludeChats":   true,
							"size":           1024,
							"sizeComparison": "larger",
						},
						"action": map[string]any{
							"addLabelIds":    []string{"Label_1", "STARRED", "IMPORTANT", "CATEGORY_SOCIAL"},
							"removeLabelIds": []string{"INBOX", "UNREAD", "SPAM"},
							"forward":        "f@example.com",
						},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
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

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	t.Run("stdout xml", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := runKong(t, &GmailFiltersExportCmd{}, []string{}, ctx, flags); err != nil {
				t.Fatalf("export stdout: %v", err)
			}
		})
		if !strings.HasPrefix(out, xml.Header) {
			t.Fatalf("missing XML header: %q", out)
		}
		if !strings.Contains(out, `xmlns:apps="http://schemas.google.com/apps/2006"`) {
			t.Fatalf("missing apps namespace: %q", out)
		}
		if !strings.Contains(out, `name="label" value="Notifications &amp; Alerts"`) {
			t.Fatalf("missing escaped label name: %q", out)
		}
		for _, want := range []string{
			`name="from" value="a@example.com"`,
			`name="subject" value="A&amp;B"`,
			`name="hasTheWord" value="from:alerts has:attachment"`,
			`name="doesNotHaveTheWord" value="category:promotions"`,
			`name="hasAttachment" value="true"`,
			`name="excludeChats" value="true"`,
			`name="size" value="1024"`,
			`name="sizeUnit" value="s_sb"`,
			`name="sizeOperator" value="s_sl"`,
			`name="shouldStar" value="true"`,
			`name="shouldAlwaysMarkAsImportant" value="true"`,
			`name="smartLabelToApply" value="^smartlabel_social"`,
			`name="shouldArchive" value="true"`,
			`name="shouldMarkAsRead" value="true"`,
			`name="shouldNeverSpam" value="true"`,
			`name="forwardTo" value="f@example.com"`,
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("missing %s in XML:\n%s", want, out)
			}
		}
		var parsed gmailFiltersXMLFeed
		if err := xml.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("xml parse: %v", err)
		}
		if parsed.Author.Email != "a@b.com" || len(parsed.Entries) != 1 {
			t.Fatalf("unexpected parsed feed: %#v", parsed)
		}
	})

	t.Run("stdout json compatibility", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := runKong(t, &GmailFiltersExportCmd{}, []string{"--format", "json"}, ctx, flags); err != nil {
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

	t.Run("global json keeps old stdout json", func(t *testing.T) {
		jsonCtx := outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		jsonFlags := *flags
		jsonFlags.JSON = true
		out := captureStdout(t, func() {
			if err := runKong(t, &GmailFiltersExportCmd{}, []string{}, jsonCtx, &jsonFlags); err != nil {
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

	t.Run("file xml export", func(t *testing.T) {
		path := t.TempDir() + "/mailFilters.xml"
		if err := runKong(t, &GmailFiltersExportCmd{}, []string{"--out", path}, ctx, flags); err != nil {
			t.Fatalf("export file: %v", err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read export: %v", err)
		}
		if !strings.Contains(string(b), "<feed") || !strings.Contains(string(b), "Mail Filters") {
			t.Fatalf("unexpected XML export: %s", b)
		}
	})

	t.Run("file json export", func(t *testing.T) {
		path := t.TempDir() + "/filters.json"
		if err := runKong(t, &GmailFiltersExportCmd{}, []string{"--format", "json", "--out", path}, ctx, flags); err != nil {
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
	})
}

func TestGmailFiltersCreate_RetriesFailedPrecondition(t *testing.T) {
	origNew := newGmailService
	origSleep := sleepBeforeGmailFilterRetry
	t.Cleanup(func() {
		newGmailService = origNew
		sleepBeforeGmailFilterRetry = origSleep
	})

	sleepBeforeGmailFilterRetry = func(context.Context, time.Duration) error { return nil }

	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodPost:
			n := posts.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if n < 3 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    400,
						"message": "Precondition check failed.",
						"errors": []map[string]any{{
							"message": "Precondition check failed.",
							"reason":  "failedPrecondition",
						}},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "f-retried",
				"criteria": map[string]any{
					"query": "subject:\"retry-me\"",
				},
				"action": map[string]any{
					"removeLabelIds": []string{"INBOX"},
				},
			})
			return
		default:
			http.NotFound(w, r)
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
	captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		if err := runKong(t, &GmailFiltersCreateCmd{}, []string{
			"--query", "subject:\"retry-me\"",
			"--archive",
		}, ctx, flags); err != nil {
			t.Fatalf("create with retry: %v", err)
		}
	})

	if posts.Load() != 3 {
		t.Fatalf("expected 3 create attempts, got %d", posts.Load())
	}
}

func TestGmailFiltersCreate_DuplicateReturnsExistingFilter(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var (
		posts int
		lists int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodPost:
			posts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    400,
					"message": "Filter already exists",
					"errors": []map[string]any{{
						"message": "Filter already exists",
						"reason":  "failedPrecondition",
					}},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodGet:
			lists++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"filter": []map[string]any{
					{
						"id": "f-existing",
						"criteria": map[string]any{
							"query": "subject:\"duplicate-me\"",
						},
						"action": map[string]any{
							"removeLabelIds": []string{"INBOX"},
						},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
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

	flags := &RootFlags{Account: "a@b.com", Force: true, JSON: true}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true}), u)

		if err := runKong(t, &GmailFiltersCreateCmd{}, []string{
			"--query", "subject:\"duplicate-me\"",
			"--archive",
		}, ctx, flags); err != nil {
			t.Fatalf("create duplicate: %v", err)
		}
	})

	if posts != 1 {
		t.Fatalf("expected 1 create attempt, got %d", posts)
	}
	if lists != 1 {
		t.Fatalf("expected 1 filters list lookup, got %d", lists)
	}
	if !strings.Contains(out, "\"f-existing\"") {
		t.Fatalf("expected existing filter output, got %q", out)
	}
}
