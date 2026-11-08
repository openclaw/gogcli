package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordedPatch struct {
	eventID    string
	recurrence []string
}

// runCalendarUpdateFuture serves a daily series "ser1" whose 2025-01-02 instance
// is "ser1_20250102T100000Z", runs calendar update --scope future with eventID,
// and records every PATCH.
func runCalendarUpdateFuture(t *testing.T, eventID string) []recordedPatch {
	t.Helper()
	const instanceID = "ser1_20250102T100000Z"
	var patches []recordedPatch
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal/events/"+instanceID:
			_, _ = io.WriteString(w, `{"id":"`+instanceID+`","recurringEventId":"ser1"}`)
		case r.Method == http.MethodGet && path == "/calendars/cal/events/ser1":
			_, _ = io.WriteString(w, `{"id":"ser1","recurrence":["RRULE:FREQ=DAILY"],"start":{"dateTime":"2025-01-01T10:00:00Z","timeZone":"UTC"},"end":{"dateTime":"2025-01-01T10:30:00Z","timeZone":"UTC"}}`)
		case r.Method == http.MethodGet && path == "/calendars/cal/events/ser1/instances":
			_, _ = io.WriteString(w, `{"items":[{"id":"`+instanceID+`","originalStartTime":{"dateTime":"2025-01-02T10:00:00Z"}}]}`)
		case r.Method == http.MethodPatch && strings.HasPrefix(path, "/calendars/cal/events/"):
			var body struct {
				Recurrence []string `json:"recurrence"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			id := strings.TrimPrefix(path, "/calendars/cal/events/")
			patches = append(patches, recordedPatch{eventID: id, recurrence: body.Recurrence})
			_, _ = io.WriteString(w, `{"id":"`+id+`","summary":"Renamed"}`)
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			_, _ = io.WriteString(w, `{"items":[{"id":"cal","summary":"cal","timeZone":"UTC"}]}`)
		case r.Method == http.MethodGet && path == "/calendars/cal":
			_, _ = io.WriteString(w, `{"id":"cal","timeZone":"UTC"}`)
		default:
			t.Logf("unexpected %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), newCalendarServiceFromServer(t, srv))
	if err := runKong(t, &CalendarUpdateCmd{}, []string{
		"cal", eventID,
		"--scope", "future",
		"--original-start", "2025-01-02T10:00:00Z",
		"--summary", "Renamed",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("update %s: %v", eventID, err)
	}
	return patches
}

func assertFutureScopePatches(t *testing.T, patches []recordedPatch) {
	t.Helper()
	if len(patches) != 2 {
		t.Fatalf("PATCHes = %#v, want instance then series", patches)
	}
	if patches[0].eventID != "ser1_20250102T100000Z" {
		t.Fatalf("first PATCH went to %q, want the instance", patches[0].eventID)
	}
	if patches[1].eventID != "ser1" {
		t.Fatalf("series truncation PATCH went to %q, want ser1", patches[1].eventID)
	}
	if len(patches[1].recurrence) != 1 || !strings.Contains(patches[1].recurrence[0], "UNTIL=20250102T095959Z") {
		t.Fatalf("series recurrence = %q, want UNTIL just before the instance", patches[1].recurrence)
	}
}

func TestCalendarUpdateCmd_ScopeFutureFromSeriesID(t *testing.T) {
	assertFutureScopePatches(t, runCalendarUpdateFuture(t, "ser1"))
}

// calendar events and calendar search list expanded instances, so the ID a
// user has is usually the instance ID. --scope future must still end the old
// series, as calendar delete --scope future already does.
func TestCalendarUpdateCmd_ScopeFutureFromInstanceID(t *testing.T) {
	assertFutureScopePatches(t, runCalendarUpdateFuture(t, "ser1_20250102T100000Z"))
}
