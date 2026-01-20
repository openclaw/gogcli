package cmd

import (
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestEventDaysOfWeek_DateTime(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z"},
		End:   &calendar.EventDateTime{DateTime: "2025-01-01T11:00:00Z"},
	}
	start, end := eventDaysOfWeek(ev)
	if start != "Wednesday" || end != "Wednesday" {
		t.Fatalf("expected Wednesday/Wednesday, got %q/%q", start, end)
	}
}

func TestEventDaysOfWeek_DateOnly(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{Date: "2025-01-02"},
		End:   &calendar.EventDateTime{Date: "2025-01-03"},
	}
	start, end := eventDaysOfWeek(ev)
	if start != "Thursday" || end != "Friday" {
		t.Fatalf("expected Thursday/Friday, got %q/%q", start, end)
	}
}

func TestWrapEventsWithDays(t *testing.T) {
	events := []*calendar.Event{
		{Start: &calendar.EventDateTime{Date: "2025-01-02"}, End: &calendar.EventDateTime{Date: "2025-01-02"}},
	}
	wrapped := wrapEventsWithDays(events)
	if len(wrapped) != 1 {
		t.Fatalf("expected 1 wrapped event, got %d", len(wrapped))
	}
	if wrapped[0].StartDayOfWeek != "Thursday" {
		t.Fatalf("unexpected start day: %q", wrapped[0].StartDayOfWeek)
	}
	if wrapped[0].StartLocal != "2025-01-02" {
		t.Fatalf("unexpected start local: %q", wrapped[0].StartLocal)
	}
}
