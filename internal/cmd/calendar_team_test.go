package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

func TestCalendarTeamRunFreeBusy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/freeBusy" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"a@example.com": map[string]any{
						"busy": []map[string]any{
							{"start": "2025-01-02T10:00:00Z", "end": "2025-01-02T11:00:00Z"},
						},
					},
					"b@example.com": map[string]any{
						"errors": []map[string]any{
							{"reason": "notFound"},
						},
						"busy": []map[string]any{},
					},
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

	tr := &TimeRange{
		From:     time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC),
		To:       time.Date(2025, 1, 2, 18, 0, 0, 0, time.UTC),
		Location: time.UTC,
	}

	cmd := &CalendarTeamCmd{GroupEmail: "group@example.com"}
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.runFreeBusy(ctx, svc, []string{"a@example.com", "b@example.com"}, tr); err != nil {
			t.Fatalf("runFreeBusy: %v", err)
		}
	})
	if !strings.Contains(out, "\"freebusy\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCalendarTeamRunEvents_Dedupe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/") && strings.HasSuffix(path, "/events") {
			email := strings.TrimPrefix(path, "/calendars/")
			email = strings.TrimSuffix(email, "/events")

			items := []map[string]any{
				{
					"id":      "ev1",
					"iCalUID": "uid1",
					"summary": "Meeting",
					"status":  "confirmed",
					"start":   map[string]any{"dateTime": "2025-01-02T10:00:00Z"},
					"end":     map[string]any{"dateTime": "2025-01-02T11:00:00Z"},
					"attendees": []map[string]any{
						{"self": true, "responseStatus": "accepted"},
					},
				},
			}

			// Add a declined event for the first email to exercise skip.
			if email == "a@example.com" {
				items = append(items, map[string]any{
					"id":      "ev2",
					"summary": "Declined",
					"start":   map[string]any{"dateTime": "2025-01-02T12:00:00Z"},
					"end":     map[string]any{"dateTime": "2025-01-02T13:00:00Z"},
					"attendees": []map[string]any{
						{"self": true, "responseStatus": "declined"},
					},
				})
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": items,
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

	tr := &TimeRange{
		From:     time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC),
		To:       time.Date(2025, 1, 2, 18, 0, 0, 0, time.UTC),
		Location: time.UTC,
	}

	cmd := &CalendarTeamCmd{GroupEmail: "group@example.com"}
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.runEvents(ctx, svc, u, []string{"a@example.com", "b@example.com"}, tr); err != nil {
			t.Fatalf("runEvents: %v", err)
		}
	})
	if !strings.Contains(out, "\"events\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}
