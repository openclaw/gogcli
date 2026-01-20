package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/ui"
)

func TestEventStartEnd_Extra(t *testing.T) {
	event := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z"},
		End:   &calendar.EventDateTime{Date: "2025-01-02"},
	}
	if eventStart(event) != "2025-01-01T10:00:00Z" {
		t.Fatalf("unexpected start")
	}
	if eventEnd(event) != "2025-01-02" {
		t.Fatalf("unexpected end")
	}
	if eventStart(nil) != "" || eventEnd(nil) != "" {
		t.Fatalf("expected empty for nil")
	}
}

func TestOrEmpty_Extra(t *testing.T) {
	if orEmpty("", "fallback") != "fallback" {
		t.Fatalf("expected fallback")
	}
	if orEmpty("value", "fallback") != "value" {
		t.Fatalf("expected value")
	}
}

func TestPrintCalendarEvent_AllFields(t *testing.T) {
	var out bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &out, Stderr: &out, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}

	guestsCanInvite := false
	guestsCanSee := false
	event := &calendar.Event{
		Id:           "ev1",
		Summary:      "",
		EventType:    "focusTime",
		Description:  "desc",
		Location:     "office",
		ColorId:      "1",
		Visibility:   "private",
		Transparency: "transparent",
		Start:        &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z"},
		End:          &calendar.EventDateTime{DateTime: "2025-01-01T11:00:00Z"},
		Attendees: []*calendar.EventAttendee{
			{Email: "a@example.com", ResponseStatus: "accepted"},
			{Email: "b@example.com", ResponseStatus: "declined", Optional: true},
			{Email: ""},
		},
		GuestsCanInviteOthers:   &guestsCanInvite,
		GuestsCanModify:         true,
		GuestsCanSeeOtherGuests: &guestsCanSee,
		HangoutLink:             "https://meet.example.com/abc",
		ConferenceData: &calendar.ConferenceData{EntryPoints: []*calendar.EntryPoint{
			{EntryPointType: "video", Uri: "https://video.example.com/room"},
		}},
		Recurrence: []string{"RRULE:FREQ=DAILY"},
		Reminders: &calendar.EventReminders{
			UseDefault: false,
			Overrides: []*calendar.EventReminder{
				{Method: "email", Minutes: 30},
			},
		},
		Attachments: []*calendar.EventAttachment{
			{FileUrl: "https://files.example.com/1"},
		},
		FocusTimeProperties: &calendar.EventFocusTimeProperties{
			AutoDeclineMode: "declineAll",
			ChatStatus:      "doNotDisturb",
		},
		OutOfOfficeProperties: &calendar.EventOutOfOfficeProperties{
			AutoDeclineMode: "declineNone",
			DeclineMessage:  "OOO",
		},
		WorkingLocationProperties: &calendar.EventWorkingLocationProperties{
			Type: "officeLocation",
		},
		Source: &calendar.EventSource{
			Url:   "https://source.example.com",
			Title: "Source",
		},
		HtmlLink: "https://calendar.example.com/ev1",
	}

	printCalendarEventWithTimezone(u, event, "UTC", time.UTC)
	got := out.String()

	for _, want := range []string{
		"id\tev1",
		"summary\t(no title)",
		"type\tfocusTime",
		"timezone\tUTC",
		"start\t2025-01-01T10:00:00Z",
		"start-day-of-week\tWednesday",
		"start-local\t2025-01-01T10:00:00Z",
		"end\t2025-01-01T11:00:00Z",
		"end-day-of-week\tWednesday",
		"end-local\t2025-01-01T11:00:00Z",
		"description\tdesc",
		"location\toffice",
		"color\t1",
		"visibility\tprivate",
		"show-as\tfree",
		"attendee\ta@example.com\taccepted",
		"attendee\tb@example.com\tdeclined (optional)",
		"guests-can-invite\tfalse",
		"guests-can-modify\ttrue",
		"guests-can-see-others\tfalse",
		"meet\thttps://meet.example.com/abc",
		"video-link\thttps://video.example.com/room",
		"recurrence\tRRULE:FREQ=DAILY",
		"reminders\temail:30m",
		"attachment\thttps://files.example.com/1",
		"auto-decline\tdeclineAll",
		"chat-status\tdoNotDisturb",
		"auto-decline\tdeclineNone",
		"decline-message\tOOO",
		"location-type\tofficeLocation",
		"source\thttps://source.example.com (Source)",
		"link\thttps://calendar.example.com/ev1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in output: %q", want, got)
		}
	}
}
