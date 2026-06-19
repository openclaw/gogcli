package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestCalendarEventsListCall_HidesCancelledEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("showDeleted"); got != "false" {
			t.Fatalf("expected showDeleted=false, got %q", got)
		}
		if got := r.URL.Query().Get("singleEvents"); got != "true" {
			t.Fatalf("expected singleEvents=true, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := calendarEventsListCall(context.Background(), svc, "primary", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 10, "", "", "", "", nil, "").Do(); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestCalendarEventsListCall_EventTypesFilter(t *testing.T) {
	cases := []struct {
		name       string
		eventTypes []string
		want       []string
	}{
		// No filter: the eventTypes query param must be absent, preserving the
		// API default of returning all event types (non-breaking).
		{"unset omits the filter", nil, nil},
		// Filter: the requested types are sent as repeated eventTypes params.
		{"sends requested types", []string{eventTypeBirthday, eventTypeDefault}, []string{eventTypeBirthday, eventTypeDefault}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotTypes []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotTypes = r.URL.Query()["eventTypes"]
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
			}))
			defer srv.Close()

			svc, err := calendar.NewService(context.Background(),
				option.WithHTTPClient(srv.Client()),
				option.WithEndpoint(srv.URL+"/"),
				option.WithoutAuthentication(),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			if _, err := calendarEventsListCall(context.Background(), svc, "primary", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 10, "", "", "", "", tc.eventTypes, "").Do(); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if !slices.Equal(gotTypes, tc.want) {
				t.Fatalf("eventTypes query = %v, want %v", gotTypes, tc.want)
			}
		})
	}
}
