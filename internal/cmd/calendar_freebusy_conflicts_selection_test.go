package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestCalendarFreeBusyCmd_ResolvesCalendarName(t *testing.T) {
	var gotIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "primary", "summary": "Primary"},
					{"id": "work@example.com", "summary": "Work"},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(path, "/freeBusy"):
			var payload struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			for _, item := range payload.Items {
				gotIDs = append(gotIDs, item.ID)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"work@example.com": map[string]any{"busy": []map[string]any{}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
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
		"calendar", "freebusy",
		"--cal", "Work",
		"--from", "2026-01-10T00:00:00Z",
		"--to", "2026-01-11T00:00:00Z",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if len(gotIDs) != 1 || gotIDs[0] != "work@example.com" {
		t.Fatalf("expected resolved calendar id work@example.com, got %#v", gotIDs)
	}
}

func TestCalendarFreeBusyCmd_RFC3339RangeDoesNotFetchTimezone(t *testing.T) {
	var got struct {
		TimeMin string `json:"timeMin"`
		TimeMax string `json:"timeMax"`
	}
	var unexpected []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && strings.Contains(path, "/freeBusy") {
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"work@example.com": map[string]any{"busy": []map[string]any{}},
				},
			})
			return
		}

		unexpected = append(unexpected, r.Method+" "+path)
		http.NotFound(w, r)
	}))
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
		"calendar", "freebusy",
		"work@example.com",
		"--from", "2026-01-10T00:00:00-08:00",
		"--to", "2026-01-11T00:00:00-08:00",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if len(unexpected) != 0 {
		t.Fatalf("unexpected pre-freebusy requests: %#v", unexpected)
	}
	if got.TimeMin != "2026-01-10T00:00:00-08:00" || got.TimeMax != "2026-01-11T00:00:00-08:00" {
		t.Fatalf("expected original RFC3339 bounds, got %q -> %q", got.TimeMin, got.TimeMax)
	}
}

func TestCalendarFreeBusyCmd_RelativeRangePayload(t *testing.T) {
	var got struct {
		TimeMin string `json:"timeMin"`
		TimeMax string `json:"timeMax"`
		Items   []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, "/freeBusy"):
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"work@example.com": map[string]any{"busy": []map[string]any{}},
				},
			})
		default:
			http.NotFound(w, r)
		}
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
		"calendar", "freebusy",
		"work@example.com",
		"--from", calendarExprToday,
		"--to", calendarExprTomorrow,
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	from, err := time.Parse(time.RFC3339, got.TimeMin)
	if err != nil {
		t.Fatalf("timeMin is not RFC3339: %q: %v", got.TimeMin, err)
	}
	to, err := time.Parse(time.RFC3339, got.TimeMax)
	if err != nil {
		t.Fatalf("timeMax is not RFC3339: %q: %v", got.TimeMax, err)
	}
	if got.TimeMin == calendarExprToday || got.TimeMax == calendarExprTomorrow {
		t.Fatalf("expected parsed RFC3339 values, got timeMin=%q timeMax=%q", got.TimeMin, got.TimeMax)
	}
	if from.Location() != time.UTC || to.Location() != time.UTC {
		t.Fatalf("expected UTC payload values, got %v -> %v", from.Location(), to.Location())
	}
	if from.Hour() != 0 || from.Minute() != 0 || from.Second() != 0 || to.Hour() != 0 || to.Minute() != 0 || to.Second() != 0 {
		t.Fatalf("expected whole-day bounds, got %s -> %s", got.TimeMin, got.TimeMax)
	}
	if to.Sub(from) != 48*time.Hour {
		t.Fatalf("expected today through tomorrow range, got %s -> %s", got.TimeMin, got.TimeMax)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "work@example.com" {
		t.Fatalf("expected work calendar item, got %#v", got.Items)
	}
}

func TestCalendarConflictsCmd_AllCalendarsSelection(t *testing.T) {
	var gotIDs []string
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "primary", "summary": "Primary"},
					{"id": "work@example.com", "summary": "Work"},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(path, "/freeBusy"):
			var payload struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			for _, item := range payload.Items {
				gotIDs = append(gotIDs, item.ID)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"primary":          map[string]any{"busy": []map[string]any{}},
					"work@example.com": map[string]any{"busy": []map[string]any{}},
				},
			})
		default:
			http.NotFound(w, r)
		}
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
		"calendar", "conflicts",
		"--all",
		"--from", "2026-01-10T00:00:00Z",
		"--to", "2026-01-11T00:00:00Z",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	sort.Strings(gotIDs)
	if len(gotIDs) != 2 || gotIDs[0] != "primary" || gotIDs[1] != "work@example.com" {
		t.Fatalf("expected all calendar ids [primary work@example.com], got %#v", gotIDs)
	}
}

func TestCalendarConflictsCmd_DefaultsToAllCalendars(t *testing.T) {
	var gotIDs []string
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "primary", "summary": "Primary"},
					{"id": "work@example.com", "summary": "Work"},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(path, "/freeBusy"):
			var payload struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			for _, item := range payload.Items {
				gotIDs = append(gotIDs, item.ID)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"primary":          map[string]any{"busy": []map[string]any{}},
					"work@example.com": map[string]any{"busy": []map[string]any{}},
				},
			})
		default:
			http.NotFound(w, r)
		}
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
		"calendar", "conflicts",
		"--from", "2026-01-10T00:00:00Z",
		"--to", "2026-01-11T00:00:00Z",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	sort.Strings(gotIDs)
	if len(gotIDs) != 2 || gotIDs[0] != "primary" || gotIDs[1] != "work@example.com" {
		t.Fatalf("expected all calendar ids [primary work@example.com], got %#v", gotIDs)
	}
}

func TestCalendarConflictsCmd_RequiresAtLeastTwoSelectedCalendars(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		"calendar", "conflicts",
		"--cal", "primary",
		"--from", "2026-01-10T00:00:00Z",
		"--to", "2026-01-11T00:00:00Z",
	}, svc)
	if result.err == nil {
		t.Fatal("expected one-calendar conflicts selection to fail")
	}
	if !strings.Contains(result.err.Error(), "requires at least two calendars") {
		t.Fatalf("expected calendar count error, got %v", result.err)
	}
}
