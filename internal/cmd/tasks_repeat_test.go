package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestTasksAddCmd_RepeatCreatesMultiple(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	var (
		counter   int32
		gotTitles []string
		gotDue    []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tasks/v1/users/@me/lists" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "l1", "title": "One"},
				},
			})
			return
		}
		if !(r.URL.Path == "/tasks/v1/lists/l1/tasks" && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if title, ok := body["title"].(string); ok {
			gotTitles = append(gotTitles, title)
		}
		if due, ok := body["due"].(string); ok {
			gotDue = append(gotDue, due)
		}
		id := atomic.AddInt32(&counter, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    fmt.Sprintf("t%d", id),
			"title": body["title"],
			"due":   body["due"],
		})
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &TasksAddCmd{}, []string{
			"l1",
			"--title", "Task",
			"--due", "2025-01-01",
			"--repeat", "daily",
			"--repeat-count", "3",
		}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if len(gotTitles) != 3 || len(gotDue) != 3 {
		t.Fatalf("expected 3 tasks, got titles=%d due=%d", len(gotTitles), len(gotDue))
	}
	if gotTitles[0] != "Task (#1/3)" || gotTitles[2] != "Task (#3/3)" {
		t.Fatalf("unexpected titles: %#v", gotTitles)
	}
	if gotDue[0] != "2025-01-01T00:00:00Z" || gotDue[1] != "2025-01-02T00:00:00Z" || gotDue[2] != "2025-01-03T00:00:00Z" {
		t.Fatalf("unexpected due schedule: %#v", gotDue)
	}

	var parsed struct {
		Count int `json:"count"`
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.Count != 3 || len(parsed.Tasks) != 3 {
		t.Fatalf("unexpected repeat output: %#v", parsed)
	}
}

func TestTasksAddCmd_RepeatUntilDateOnlyWithTimeDue(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	var gotDue []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tasks/v1/users/@me/lists" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "l1", "title": "One"},
				},
			})
			return
		}
		if !(r.URL.Path == "/tasks/v1/lists/l1/tasks" && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if due, ok := body["due"].(string); ok {
			gotDue = append(gotDue, due)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":  "t1",
			"due": body["due"],
		})
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	_ = captureStdout(t, func() {
		if err := runKong(t, &TasksAddCmd{}, []string{
			"l1",
			"--title", "Task",
			"--due", "2025-01-01T10:00:00Z",
			"--repeat", "daily",
			"--repeat-until", "2025-01-03",
		}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if len(gotDue) != 3 {
		t.Fatalf("expected 3 tasks, got due=%d", len(gotDue))
	}
	if gotDue[0] != "2025-01-01T10:00:00Z" || gotDue[1] != "2025-01-02T10:00:00Z" || gotDue[2] != "2025-01-03T10:00:00Z" {
		t.Fatalf("unexpected due schedule: %#v", gotDue)
	}
}

func TestParseTaskDate_FlexibleFormats(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		value       string
		wantErr     bool
		wantHasTime bool
	}{
		{name: "date", value: "2026-02-13", wantHasTime: false},
		{name: "rfc3339", value: "2026-02-13T10:20:30Z", wantHasTime: true},
		{name: "local minutes T", value: "2026-02-13T10:20", wantHasTime: true},
		{name: "local seconds space", value: "2026-02-13 10:20:30", wantHasTime: true},
		{name: "iso offset", value: "2026-02-13T10:20:30-0800", wantHasTime: true},
		{name: "invalid", value: "nope", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, gotHasTime, err := parseTaskDate(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTaskDate: %v", err)
			}
			if gotHasTime != tc.wantHasTime {
				t.Fatalf("hasTime=%v want %v", gotHasTime, tc.wantHasTime)
			}
		})
	}
}
