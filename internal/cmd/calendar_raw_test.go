package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func newCalendarRawTestServer(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Calendar API uses /calendars/{calId}/events/{evId} for Events.Get
		// and /users/me/calendarList for list operations used by the resolver.
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if strings.HasPrefix(path, "/users/me/calendarList") {
			// Respond with an empty list so the resolver treats the input as a literal ID.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
			return
		}
		if !strings.Contains(path, "/events/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": status, "message": "mock error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func installMockCalendarService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", calendar.NewService)
	stubGoogleTestService(t, &newCalendarService, svc)
}

func fullCalendarEventResponse(id string) map[string]any {
	return map[string]any{
		"id":      id,
		"summary": "Lunch",
		"start":   map[string]any{"dateTime": "2026-04-08T12:00:00Z"},
		"end":     map[string]any{"dateTime": "2026-04-08T13:00:00Z"},
		"attendees": []map[string]any{
			{"email": "a@b.com", "responseStatus": "accepted"},
		},
	}
}

func TestCalendarRaw_HappyPath(t *testing.T) {
	srv := newCalendarRawTestServer(t, 0, fullCalendarEventResponse("ev1"))
	defer srv.Close()
	installMockCalendarService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &CalendarRawCmd{}, []string{"primary", "ev1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if got["id"] != "ev1" {
		t.Fatalf("expected id=ev1, got: %v", got["id"])
	}
	if _, ok := got["attendees"]; !ok {
		t.Fatalf("expected attendees in raw output")
	}
}

func TestCalendarRaw_APIError(t *testing.T) {
	srv := newCalendarRawTestServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()
	installMockCalendarService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &CalendarRawCmd{}, []string{"primary", "ev1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 500")
		}
	})
}

func TestCalendarRaw_NotFound(t *testing.T) {
	srv := newCalendarRawTestServer(t, http.StatusNotFound, nil)
	defer srv.Close()
	installMockCalendarService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &CalendarRawCmd{}, []string{"primary", "ev1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 404")
		}
	})
}

func TestCalendarRaw_EmptyEventID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&CalendarRawCmd{CalendarID: "primary"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty eventId")
	}
}
