package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/app"
)

func executeCalendarTeamTest(t *testing.T, args []string, calSvc *calendar.Service, cloudSvc *cloudidentity.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Calendar: func(context.Context, string) (*calendar.Service, error) {
			return calSvc, nil
		},
		CloudIdentity: func(context.Context, string) (*cloudidentity.Service, error) {
			return cloudSvc, nil
		},
	}})
}

func TestExecute_CalendarTeam_JSON(t *testing.T) {
	// Mock Cloud Identity server
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "groups:lookup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "groups/abc123",
			})
		case strings.Contains(r.URL.Path, "groups/abc123/memberships"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{
						"preferredMemberKey": map[string]any{"id": "alice@example.com"},
						"type":               "USER",
					},
					{
						"preferredMemberKey": map[string]any{"id": "bob@example.com"},
						"type":               "USER",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cloudSrv.Close()

	cloudSvc, err := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(cloudSrv.Client()),
		option.WithEndpoint(cloudSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService (cloud): %v", err)
	}

	// Mock Calendar server
	calSrv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendars/alice@example.com/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1",
						"summary": "Daily Standup",
						"start":   map[string]any{"dateTime": "2026-01-05T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T09:30:00Z"},
					},
				},
			})
		case strings.Contains(r.URL.Path, "/calendars/bob@example.com/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1", // Same event (shared meeting)
						"summary": "Daily Standup",
						"start":   map[string]any{"dateTime": "2026-01-05T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T09:30:00Z"},
					},
					{
						"id":      "ev2",
						"summary": "Bob's 1:1",
						"start":   map[string]any{"dateTime": "2026-01-05T14:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T15:00:00Z"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})))
	defer calSrv.Close()

	calSvc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(calSrv.Client()),
		option.WithEndpoint(calSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService (cal): %v", err)
	}

	result := executeCalendarTeamTest(t, []string{
		"--json",
		"--account", "a@b.com",
		"calendar", "team", "engineering@example.com",
		"--from", "2026-01-05T00:00:00Z",
		"--to", "2026-01-06T00:00:00Z",
	}, calSvc, cloudSvc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	var parsed struct {
		Group    string `json:"group"`
		TimeMin  string `json:"timeMin"`
		TimeMax  string `json:"timeMax"`
		Timezone string `json:"timezone"`
		Events   []struct {
			Who     string `json:"who"`
			ID      string `json:"id"`
			Summary string `json:"summary"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	// Should have 2 events (ev1 deduplicated, ev2 unique)
	if len(parsed.Events) != 2 {
		t.Fatalf("expected 2 events (deduplicated), got %d: %+v", len(parsed.Events), parsed.Events)
	}

	// ev1 should show both attendees
	foundStandup := false
	for _, ev := range parsed.Events {
		if ev.Summary == "Daily Standup" {
			foundStandup = true
			if !strings.Contains(ev.Who, "alice") || !strings.Contains(ev.Who, "bob") {
				t.Fatalf("expected both attendees in deduplicated event, got: %s", ev.Who)
			}
		}
	}
	if !foundStandup {
		t.Fatal("Daily Standup event not found")
	}
}

func TestExecute_CalendarTeam_WrapsGroupPermissionError(t *testing.T) {
	cloudSvc := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "groups:lookup") {
			writeGroupsPermissionError(t, w)
			return
		}
		http.NotFound(w, r)
	}))
	calSvc, closeCal := newCalendarServiceForTest(t, withPrimaryCalendar(http.NotFoundHandler()))
	t.Cleanup(closeCal)

	result := executeCalendarTeamTest(t, []string{
		"--account", "admin@example.com", "calendar", "team", "engineering@example.com",
	}, calSvc, cloudSvc)
	if result.err == nil || ExitCode(result.err) != exitCodePermissionDenied {
		t.Fatalf("unexpected error: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "gog auth service-account set admin@example.com") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestExecute_CalendarTeam_FreeBusy(t *testing.T) {
	// Mock Cloud Identity server
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "groups:lookup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/abc"})
		case strings.Contains(r.URL.Path, "groups/abc/memberships"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{"preferredMemberKey": map[string]any{"id": "alice@example.com"}, "type": "USER"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cloudSrv.Close()

	cloudSvc, _ := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(cloudSrv.Client()),
		option.WithEndpoint(cloudSrv.URL+"/"),
	)
	// Mock Calendar server
	calSrv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "freeBusy") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"alice@example.com": map[string]any{
						"busy": []map[string]any{
							{"start": "2026-01-05T09:00:00Z", "end": "2026-01-05T10:00:00Z"},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer calSrv.Close()

	calSvc, _ := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(calSrv.Client()),
		option.WithEndpoint(calSrv.URL+"/"),
	)
	result := executeCalendarTeamTest(t, []string{
		"--json",
		"--account", "a@b.com",
		"calendar", "team", "eng@example.com",
		"--freebusy",
		"--from", "2026-01-05T00:00:00Z",
		"--to", "2026-01-06T00:00:00Z",
	}, calSvc, cloudSvc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	var parsed struct {
		FreeBusy []struct {
			Email string   `json:"email"`
			Busy  []string `json:"busy"`
		} `json:"freebusy"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if len(parsed.FreeBusy) != 1 {
		t.Fatalf("expected 1 freebusy entry, got %d", len(parsed.FreeBusy))
	}
	if parsed.FreeBusy[0].Email != "alice@example.com" {
		t.Fatalf("unexpected email: %s", parsed.FreeBusy[0].Email)
	}
	if len(parsed.FreeBusy[0].Busy) != 1 {
		t.Fatalf("expected 1 busy block, got %d", len(parsed.FreeBusy[0].Busy))
	}
}

func TestExecute_CalendarTeam_Text(t *testing.T) {
	// Mock Cloud Identity server
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "groups:lookup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/abc"})
		case strings.Contains(r.URL.Path, "groups/abc/memberships"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{"preferredMemberKey": map[string]any{"id": "alice@example.com"}, "type": "USER"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cloudSrv.Close()

	cloudSvc, _ := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(cloudSrv.Client()),
		option.WithEndpoint(cloudSrv.URL+"/"),
	)
	// Mock Calendar server
	calSrv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/alice@example.com/events") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1",
						"summary": "Team Meeting",
						"start":   map[string]any{"dateTime": "2026-01-05T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T11:00:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer calSrv.Close()

	calSvc, _ := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(calSrv.Client()),
		option.WithEndpoint(calSrv.URL+"/"),
	)
	result := executeCalendarTeamTest(t, []string{
		"--account", "a@b.com",
		"calendar", "team", "eng@example.com",
		"--from", "2026-01-05T00:00:00Z",
		"--to", "2026-01-06T00:00:00Z",
	}, calSvc, cloudSvc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	// Check text output format
	if !strings.Contains(out, "WHO") || !strings.Contains(out, "START") || !strings.Contains(out, "SUMMARY") {
		t.Fatalf("missing table headers in output: %q", out)
	}
	if !strings.Contains(out, "alice@example.com") {
		t.Fatalf("missing alice in output: %q", out)
	}
	if !strings.Contains(out, "Team Meeting") {
		t.Fatalf("missing event summary in output: %q", out)
	}
}
