package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestApplyUpdateScopeFuture_UsesParentRecurrence(t *testing.T) {
	originalStart := "2025-01-02T10:00:00Z"
	var patchedRecurrence []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ev",
				"recurrence": []string{"RRULE:FREQ=DAILY"},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/cal/events/ev/instances"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "ev_1",
						"originalStartTime": map[string]any{
							"dateTime": originalStart,
						},
					},
				},
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal/events/ev":
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			patchedRecurrence = append([]string{}, body.Recurrence...)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
			return
		default:
			http.NotFound(w, r)
			return
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

	patch := &calendar.Event{Summary: "updated"}
	targetID, parentRecurrence, err := applyUpdateScope(context.Background(), svc, "cal", "ev", scopeFuture, originalStart, patch)
	if err != nil {
		t.Fatalf("applyUpdateScope: %v", err)
	}
	if targetID != "ev_1" {
		t.Fatalf("unexpected target id: %q", targetID)
	}
	if len(parentRecurrence) != 1 || parentRecurrence[0] != "RRULE:FREQ=DAILY" {
		t.Fatalf("unexpected parent recurrence: %#v", parentRecurrence)
	}
	if len(patch.Recurrence) != 1 || patch.Recurrence[0] != "RRULE:FREQ=DAILY" {
		t.Fatalf("patch did not inherit recurrence: %#v", patch.Recurrence)
	}

	if err := truncateParentRecurrence(context.Background(), svc, "cal", "ev", parentRecurrence, originalStart, ""); err != nil {
		t.Fatalf("truncateParentRecurrence: %v", err)
	}
	if len(patchedRecurrence) != 1 || !strings.Contains(patchedRecurrence[0], "UNTIL=") {
		t.Fatalf("expected truncated recurrence, got: %#v", patchedRecurrence)
	}
}

func TestApplyUpdateScopeFuture_NonRecurring(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodGet && path == "/calendars/cal/events/ev" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
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

	patch := &calendar.Event{Summary: "updated"}
	if _, _, err := applyUpdateScope(context.Background(), svc, "cal", "ev", scopeFuture, "2025-01-02T10:00:00Z", patch); err == nil {
		t.Fatalf("expected error for non-recurring event")
	}
}
