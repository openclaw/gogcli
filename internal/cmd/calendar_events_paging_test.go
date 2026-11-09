package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// twoPageEventsHandler serves events.list like the real API, which may return
// fewer events than maxResults and a nextPageToken: event1 on the first page,
// event2 on page "p2".
func twoPageEventsHandler(calls *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events") {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		ev := func(id, start string) map[string]any {
			return map[string]any{
				"id": id, "iCalUID": id + "@example.com", "summary": "standup " + id, "status": "confirmed",
				"start": map[string]any{"dateTime": start},
				"end":   map[string]any{"dateTime": strings.Replace(start, ":00:00Z", ":15:00Z", 1)},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{ev("event1", "2026-11-10T10:00:00Z")}, "nextPageToken": "p2"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{ev("event2", "2026-11-11T10:00:00Z")}})
	}
}

func TestCalendarSearch_ReadsLaterPagesUpToMax(t *testing.T) {
	var calls atomic.Int32
	svc, closeServer := newGoogleTestService(t, withPrimaryCalendar(twoPageEventsHandler(&calls)), calendar.NewService)
	t.Cleanup(closeServer)
	for _, mode := range []string{"--json", "--plain"} {
		calls.Store(0)
		res := executeWithCalendarTestService(t, []string{mode, "--account", "a@b.com", "calendar", "search", "standup", "--max", "50"}, svc)
		if res.err != nil {
			t.Fatalf("%s: %v", mode, res.err)
		}
		if !strings.Contains(res.stdout, "event1") || !strings.Contains(res.stdout, "event2") {
			t.Fatalf("%s: want event1 and event2, stdout=%q", mode, res.stdout)
		}
		if strings.Contains(res.stderr, "--max") {
			t.Fatalf("%s: no more results remain, stderr=%q", mode, res.stderr)
		}
	}
}

func TestCalendarSearch_StopsAtMaxAndSaysMoreRemain(t *testing.T) {
	var calls atomic.Int32
	svc, closeServer := newGoogleTestService(t, withPrimaryCalendar(twoPageEventsHandler(&calls)), calendar.NewService)
	t.Cleanup(closeServer)
	res := executeWithCalendarTestService(t, []string{"--json", "--account", "a@b.com", "calendar", "search", "standup", "--max", "1"}, svc)
	if res.err != nil {
		t.Fatalf("search: %v", res.err)
	}
	if !strings.Contains(res.stdout, "event1") || strings.Contains(res.stdout, "event2") {
		t.Fatalf("want only event1, stdout=%q", res.stdout)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (stop once --max is reached)", calls.Load())
	}
	if !strings.Contains(res.stderr, "--max") {
		t.Fatalf("want a hint that more results remain, stderr=%q", res.stderr)
	}
}

func TestCalendarTeamEvents_ReadsLaterPagesPerCalendar(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(twoPageEventsHandler(&calls))
	t.Cleanup(srv.Close)
	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	tr := &TimeRange{
		From:     time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		To:       time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC),
		Location: time.UTC,
	}
	cmd := &CalendarTeamCmd{GroupEmail: "group@example.com", Max: 100}
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	out := captureStdout(t, func() {
		if err := cmd.runEvents(ctx, svc, u, []string{"a@example.com"}, tr); err != nil {
			t.Fatalf("runEvents: %v", err)
		}
	})
	if !strings.Contains(out, "standup event1") || !strings.Contains(out, "standup event2") {
		t.Fatalf("want both pages' events, got %q", out)
	}
}
