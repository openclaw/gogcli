package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	if _, err := calendarEventsListCall(context.Background(), svc, "primary", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 10, "", "", "", "", "").Do(); err != nil {
		t.Fatalf("Do: %v", err)
	}
}
