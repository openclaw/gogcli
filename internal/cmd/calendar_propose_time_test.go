package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func TestProposeTimeURLGeneration(t *testing.T) {
	tests := []struct {
		name       string
		eventID    string
		calendarID string
		wantURL    string
	}{
		{
			name:       "basic event",
			eventID:    "rp2rg301pirvlufurh62sfkh74",
			calendarID: "vladimir.novosselov@gmail.com",
			wantURL:    "https://calendar.google.com/calendar/u/0/r/proposetime/cnAycmczMDFwaXJ2bHVmdXJoNjJzZmtoNzQgdmxhZGltaXIubm92b3NzZWxvdkBnbWFpbC5jb20=",
		},
		{
			name:       "simple ids",
			eventID:    "evt123",
			calendarID: "test@example.com",
			wantURL:    "https://calendar.google.com/calendar/u/0/r/proposetime/" + base64.StdEncoding.EncodeToString([]byte("evt123 test@example.com")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := tt.eventID + " " + tt.calendarID
			encoded := base64.StdEncoding.EncodeToString([]byte(payload))
			got := "https://calendar.google.com/calendar/u/0/r/proposetime/" + encoded

			if got != tt.wantURL {
				t.Errorf("URL mismatch:\ngot:  %s\nwant: %s", got, tt.wantURL)
			}
		})
	}
}

func TestCalendarProposeTimeCmd_Text(t *testing.T) {
	origNew := newCalendarService
	origOpen := openProposeTimeBrowser
	t.Cleanup(func() {
		newCalendarService = origNew
		openProposeTimeBrowser = origOpen
	})

	// Mock browser open to track if called
	var browserOpened string
	openProposeTimeBrowser = func(url string) error {
		browserOpened = url
		return nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if strings.Contains(path, "/calendars/cal1/events/evt1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt1",
				"summary": "Team Meeting",
				"start":   map[string]string{"dateTime": "2026-01-16T19:30:00-08:00"},
				"end":     map[string]string{"dateTime": "2026-01-16T20:30:00-08:00"},
				"attendees": []map[string]any{
					{"email": "a@b.com", "self": true},
					{"email": "organizer@b.com", "organizer": true},
				},
			})
			return
		}
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &CalendarProposeTimeCmd{}
		if err := runKong(t, cmd, []string{"cal1", "evt1", "--open"}, ctx, flags); err != nil {
			t.Fatalf("propose-time: %v", err)
		}
	})

	// Verify output contains expected fields
	if !strings.Contains(out, "propose_url") {
		t.Errorf("output missing propose_url: %q", out)
	}
	if !strings.Contains(out, "Team Meeting") {
		t.Errorf("output missing event summary: %q", out)
	}
	if !strings.Contains(out, "proposetime/") {
		t.Errorf("output missing proposetime URL path: %q", out)
	}
	if !strings.Contains(out, proposeTimeIssueTrackerURL) {
		t.Errorf("output missing issue tracker URL: %q", out)
	}
	if !strings.Contains(out, "Error: "+proposeTimeErrorMessage) {
		t.Errorf("output missing error message: %q", out)
	}

	// Verify browser was opened
	if browserOpened == "" {
		t.Error("browser was not opened despite --open flag")
	}
	if !strings.Contains(browserOpened, "proposetime/") {
		t.Errorf("browser URL incorrect: %q", browserOpened)
	}
}

func TestCalendarProposeTimeCmd_WithDecline(t *testing.T) {
	origNew := newCalendarService
	origOpen := openProposeTimeBrowser
	t.Cleanup(func() {
		newCalendarService = origNew
		openProposeTimeBrowser = origOpen
	})
	openProposeTimeBrowser = func(url string) error { return nil }

	var patchCalled bool
	var patchedComment string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case strings.Contains(path, "/calendars/cal1/events/evt1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt1",
				"summary": "Team Meeting",
				"start":   map[string]string{"dateTime": "2026-01-16T19:30:00-08:00"},
				"end":     map[string]string{"dateTime": "2026-01-16T20:30:00-08:00"},
				"attendees": []map[string]any{
					{"email": "a@b.com", "self": true},
					{"email": "organizer@b.com", "organizer": true},
				},
			})
		case strings.Contains(path, "/calendars/cal1/events/evt1") && r.Method == http.MethodPatch:
			patchCalled = true
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if attendees, ok := body["attendees"].([]any); ok && len(attendees) > 0 {
				if att, ok := attendees[0].(map[string]any); ok {
					if c, ok := att["comment"].(string); ok {
						patchedComment = c
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "evt1", "summary": "Team Meeting"})
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &CalendarProposeTimeCmd{}
		if err := runKong(t, cmd, []string{"cal1", "evt1", "--comment", "Can we do 5pm instead?"}, ctx, flags); err != nil {
			t.Fatalf("propose-time with decline: %v", err)
		}
	})

	if !patchCalled {
		t.Error("PATCH was not called despite --comment flag")
	}
	if patchedComment != "Can we do 5pm instead?" {
		t.Errorf("comment not passed correctly, got: %q", patchedComment)
	}
	if !strings.Contains(out, "declined\tyes") {
		t.Errorf("output should show declined status: %q", out)
	}
}
