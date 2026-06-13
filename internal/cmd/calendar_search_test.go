package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestCalendarSearchCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") && r.Method == http.MethodGet {
			// Verify query parameter is set
			q := r.URL.Query().Get("q")
			if q != "team meeting" {
				t.Errorf("unexpected query: %q", q)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "event1",
						"summary": "Team meeting",
						"start":   map[string]any{"dateTime": "2024-01-15T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2024-01-15T11:00:00Z"},
					},
					{
						"id":      "event2",
						"summary": "Team standup meeting",
						"start":   map[string]any{"dateTime": "2024-01-16T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2024-01-16T09:30:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithCalendarTestService(t, []string{"--json", "--account", "a@b.com", "calendar", "search", "team meeting"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	var parsed struct {
		Events []struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
		} `json:"events"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Query != "team meeting" {
		t.Errorf("unexpected query in output: %q", parsed.Query)
	}
	if len(parsed.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(parsed.Events))
	}
	if parsed.Events[0].ID != "event1" {
		t.Errorf("unexpected first event ID: %q", parsed.Events[0].ID)
	}
	if parsed.Events[1].ID != "event2" {
		t.Errorf("unexpected second event ID: %q", parsed.Events[1].ID)
	}
}

func TestCalendarSearchCmd_EmptyQueryIsUsage(t *testing.T) {
	err := (&CalendarSearchCmd{Query: " "}).Run(context.Background(), &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
}

func TestCalendarSearchCmd_UsesResolvedAliasID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	if err := defaultConfigStoreForTest(t).SetCalendarAlias("family", "family-cal@group.calendar.google.com"); err != nil {
		t.Fatalf("SetCalendarAlias: %v", err)
	}

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") && r.Method == http.MethodGet {
			escapedPath := r.URL.EscapedPath()
			if !strings.Contains(escapedPath, "/family-cal%40group.calendar.google.com/events") {
				t.Fatalf("search used unresolved calendar path: %s", escapedPath)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithCalendarTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"calendar", "search", "meeting",
		"--calendar", "family",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
}

func TestCalendarSearchCmd_NoResults(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithCalendarTestService(t, []string{"--json", "--account", "a@b.com", "calendar", "search", "nonexistent"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	// In JSON mode, should return empty events array
	var parsed struct {
		Events []map[string]any `json:"events"`
		Query  string           `json:"query"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(parsed.Events))
	}
	if parsed.Query != "nonexistent" {
		t.Errorf("unexpected query: %q", parsed.Query)
	}
}

func TestCalendarSearchCmd_WithTimeRange(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") && r.Method == http.MethodGet {
			// Verify time range parameters
			timeMin := r.URL.Query().Get("timeMin")
			timeMax := r.URL.Query().Get("timeMax")
			if timeMin != "2024-01-01T00:00:00Z" {
				t.Errorf("unexpected timeMin: %q", timeMin)
			}
			if timeMax != "2024-01-31T23:59:59Z" {
				t.Errorf("unexpected timeMax: %q", timeMax)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "event1",
						"summary": "Meeting",
						"start":   map[string]any{"dateTime": "2024-01-15T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2024-01-15T11:00:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithCalendarTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"calendar", "search", "meeting",
		"--from", "2024-01-01T00:00:00Z",
		"--to", "2024-01-31T23:59:59Z",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	var parsed struct {
		Events []struct {
			ID string `json:"id"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(parsed.Events))
	}
}

func TestCalendarSearchCmd_FromOnly_DefaultsTo90Days(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") && r.Method == http.MethodGet {
			timeMin := r.URL.Query().Get("timeMin")
			timeMax := r.URL.Query().Get("timeMax")
			minTime, err := time.Parse(time.RFC3339, timeMin)
			if err != nil {
				t.Errorf("invalid timeMin: %v", err)
			}
			maxTime, err := time.Parse(time.RFC3339, timeMax)
			if err != nil {
				t.Errorf("invalid timeMax: %v", err)
			}
			if !maxTime.After(minTime) {
				t.Errorf("expected timeMax after timeMin, got %s <= %s", timeMax, timeMin)
			}
			diff := maxTime.Sub(minTime)
			if diff < 85*24*time.Hour || diff > 100*24*time.Hour {
				t.Errorf("unexpected range: %s (min %s max %s)", diff, timeMin, timeMax)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithCalendarTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"calendar", "search", "meeting",
		"--from", "today",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
}

func TestCalendarSearchCmd_TableOutput(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "event1",
						"summary": "Team meeting",
						"start":   map[string]any{"dateTime": "2024-01-15T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2024-01-15T11:00:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithCalendarTestService(t, []string{"--account", "a@b.com", "calendar", "search", "team"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	// Verify table output contains expected fields
	if !strings.Contains(out, "event1") {
		t.Errorf("output missing event id: %q", out)
	}
	if !strings.Contains(out, "Team meeting") {
		t.Errorf("output missing summary: %q", out)
	}
	if !strings.Contains(out, "2024-01-15T10:00:00Z") {
		t.Errorf("output missing start time: %q", out)
	}
	if !strings.Contains(out, "ID") && !strings.Contains(out, "START") {
		t.Errorf("output missing table headers: %q", out)
	}
}

func TestCalendarSearchCmd_MaxResults(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") && r.Method == http.MethodGet {
			// Verify maxResults parameter
			maxResults := r.URL.Query().Get("maxResults")
			if maxResults != "5" {
				t.Errorf("unexpected maxResults: %q", maxResults)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "event1",
						"summary": "Meeting 1",
						"start":   map[string]any{"dateTime": "2024-01-15T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2024-01-15T11:00:00Z"},
					},
					{
						"id":      "event2",
						"summary": "Meeting 2",
						"start":   map[string]any{"dateTime": "2024-01-16T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2024-01-16T11:00:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithCalendarTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"calendar", "search", "meeting",
		"--max", "5",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	var parsed struct {
		Events []struct {
			ID string `json:"id"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(parsed.Events))
	}
}
