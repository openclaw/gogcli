package cmd

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/app"
)

func TestGmailFiltersCreate_Validation(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", Force: true}

	cmd := &GmailFiltersCreateCmd{}
	if err := runKong(t, cmd, []string{}, context.Background(), flags); err == nil || ExitCode(err) != 2 {
		t.Fatalf("expected missing criteria usage error, got %v", err)
	}

	cmd = &GmailFiltersCreateCmd{}
	if err := runKong(t, cmd, []string{"--from", "a@example.com"}, context.Background(), flags); err == nil || ExitCode(err) != 2 {
		t.Fatalf("expected missing action usage error, got %v", err)
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

func TestGmailFiltersCreate_InvalidForwardFailsBeforeDryRun(t *testing.T) {
	result := executeWithTestRuntime(t,
		[]string{"--account", "a@b.com", "--dry-run", "gmail", "filters", "create", "--from", "a@example.com", "--forward", "nope"},
		&app.Runtime{Services: app.Services{Gmail: func(context.Context, string) (*gmail.Service, error) {
			t.Fatal("expected validation to fail before creating gmail service")
			return nil, errors.New("unexpected gmail service call")
		}}},
	)
	var exitErr *ExitError
	if !errors.As(result.err, &exitErr) || exitErr.Code != 2 || !strings.Contains(result.err.Error(), "invalid --forward") {
		t.Fatalf("unexpected err: %v", result.err)
	}
}

func TestGmailFilters_TextPaths(t *testing.T) {
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

	svc := newGmailServiceFromServer(t, srv)
	for name, args := range map[string][]string{
		"list": {"--plain", "--account", "a@b.com", "gmail", "filters", "list"},
		"get":  {"--plain", "--account", "a@b.com", "gmail", "filters", "get", "f1"},
		"create": {
			"--plain", "--force", "--account", "a@b.com",
			"gmail", "filters", "create",
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
		},
		"delete": {"--plain", "--force", "--account", "a@b.com", "gmail", "filters", "delete", "f2"},
	} {
		result := executeWithGmailTestService(t, args, svc)
		if result.err != nil {
			t.Fatalf("%s: %v\nstderr=%q", name, result.err, result.stderr)
		}
	}

	if createReq.Action == nil || len(createReq.Action.AddLabelIds) == 0 {
		t.Fatalf("expected add labels in create request")
	}
}

func TestGmailFiltersList_NoFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"filter": []map[string]any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "filters", "list"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("list: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "No filters") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestGmailFiltersList_JSONEmptyArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "filters", "list"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("list: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Filters []json.RawMessage `json:"filters"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Filters == nil {
		t.Fatalf("filters must be an empty array, got nil: %s", result.stdout)
	}
	if len(parsed.Filters) != 0 {
		t.Fatalf("filters len = %d, want 0", len(parsed.Filters))
	}
}

func TestGmailFiltersExport_JSONEmptyArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "filters", "export", "--format", "json"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("export: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Filters []json.RawMessage `json:"filters"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Filters == nil {
		t.Fatalf("filters must be an empty array, got nil: %s", result.stdout)
	}
	if len(parsed.Filters) != 0 {
		t.Fatalf("filters len = %d, want 0", len(parsed.Filters))
	}
}

func TestGmailFiltersExport(t *testing.T) {
	origNow := nowGmailFiltersExport
	t.Cleanup(func() {
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

	svc := newGmailServiceFromServer(t, srv)

	t.Run("stdout xml", func(t *testing.T) {
		result := executeWithGmailTestService(t, []string{
			"--plain", "--account", "a@b.com", "gmail", "filters", "export",
		}, svc)
		if result.err != nil {
			t.Fatalf("export stdout: %v\nstderr=%q", result.err, result.stderr)
		}
		if !strings.HasPrefix(result.stdout, xml.Header) {
			t.Fatalf("missing XML header: %q", result.stdout)
		}
		if !strings.Contains(result.stdout, `xmlns:apps="http://schemas.google.com/apps/2006"`) {
			t.Fatalf("missing apps namespace: %q", result.stdout)
		}
		if !strings.Contains(result.stdout, `name="label" value="Notifications &amp; Alerts"`) {
			t.Fatalf("missing escaped label name: %q", result.stdout)
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
			if !strings.Contains(result.stdout, want) {
				t.Fatalf("missing %s in XML:\n%s", want, result.stdout)
			}
		}
		var parsed gmailFiltersXMLFeed
		if err := xml.Unmarshal([]byte(result.stdout), &parsed); err != nil {
			t.Fatalf("xml parse: %v", err)
		}
		if parsed.Author.Email != "a@b.com" || len(parsed.Entries) != 1 {
			t.Fatalf("unexpected parsed feed: %#v", parsed)
		}
	})

	t.Run("stdout json compatibility", func(t *testing.T) {
		result := executeWithGmailTestService(t, []string{
			"--plain", "--account", "a@b.com", "gmail", "filters", "export", "--format", "json",
		}, svc)
		if result.err != nil {
			t.Fatalf("export stdout: %v\nstderr=%q", result.err, result.stderr)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
			t.Fatalf("json parse: %v", err)
		}
		filters, ok := payload["filters"].([]any)
		if !ok || len(filters) != 1 {
			t.Fatalf("unexpected payload: %#v", payload)
		}
	})

	t.Run("global json keeps old stdout json", func(t *testing.T) {
		result := executeWithGmailTestService(t, []string{
			"--json", "--account", "a@b.com", "gmail", "filters", "export",
		}, svc)
		if result.err != nil {
			t.Fatalf("export stdout: %v\nstderr=%q", result.err, result.stderr)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
			t.Fatalf("json parse: %v", err)
		}
		filters, ok := payload["filters"].([]any)
		if !ok || len(filters) != 1 {
			t.Fatalf("unexpected payload: %#v", payload)
		}
	})

	t.Run("file xml export", func(t *testing.T) {
		path := t.TempDir() + "/mailFilters.xml"
		result := executeWithGmailTestService(t, []string{
			"--plain", "--account", "a@b.com", "gmail", "filters", "export", "--out", path,
		}, svc)
		if result.err != nil {
			t.Fatalf("export file: %v\nstderr=%q", result.err, result.stderr)
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
		result := executeWithGmailTestService(t, []string{
			"--plain", "--account", "a@b.com", "gmail", "filters", "export", "--format", "json", "--out", path,
		}, svc)
		if result.err != nil {
			t.Fatalf("export file: %v\nstderr=%q", result.err, result.stderr)
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
	origSleep := sleepBeforeGmailFilterRetry
	t.Cleanup(func() {
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

	result := executeWithGmailTestService(t, []string{
		"--plain", "--force", "--account", "a@b.com",
		"gmail", "filters", "create",
		"--query", "subject:\"retry-me\"",
		"--archive",
	}, newGmailServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatalf("create with retry: %v\nstderr=%q", result.err, result.stderr)
	}

	if posts.Load() != 3 {
		t.Fatalf("expected 3 create attempts, got %d", posts.Load())
	}
}

func TestGmailFiltersCreate_DuplicateReturnsExistingFilter(t *testing.T) {
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

	result := executeWithGmailTestService(t, []string{
		"--json", "--force", "--account", "a@b.com",
		"gmail", "filters", "create",
		"--query", "subject:\"duplicate-me\"",
		"--archive",
	}, newGmailServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatalf("create duplicate: %v\nstderr=%q", result.err, result.stderr)
	}

	if posts != 1 {
		t.Fatalf("expected 1 create attempt, got %d", posts)
	}
	if lists != 1 {
		t.Fatalf("expected 1 filters list lookup, got %d", lists)
	}
	if !strings.Contains(result.stdout, "\"f-existing\"") {
		t.Fatalf("expected existing filter output, got %q", result.stdout)
	}
}
