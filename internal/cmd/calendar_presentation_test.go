package cmd

import (
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/people/v1"
)

func TestCalendarPresentationSchemas(t *testing.T) {
	t.Parallel()

	t.Run("calendars", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*calendar.CalendarListEntry{{
			Id:         "cal1",
			Summary:    "Primary",
			AccessRole: "owner",
		}}, calendarListColumns())
		assertTableOutput(t, got, "ID\tNAME\tROLE\ncal1\tPrimary\towner\n")
	})

	t.Run("acl", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*calendar.AclRule{
			{Role: "default"},
			{
				Role:  "reader",
				Scope: &calendar.AclRuleScope{Type: "user", Value: "reader@example.com"},
			},
		}, calendarACLColumns())
		assertTableOutput(
			t,
			got,
			"SCOPE_TYPE\tSCOPE_VALUE\tROLE\n"+
				"\t\tdefault\n"+
				"user\treader@example.com\treader\n",
		)
	})

	t.Run("aliases", func(t *testing.T) {
		t.Parallel()
		rows := calendarAliasRows(map[string]string{
			"work":     "work@example.com",
			"personal": "personal@example.com",
		})
		got := renderPlainTable(t, rows, calendarAliasColumns())
		assertTableOutput(
			t,
			got,
			"ALIAS\tCALENDAR_ID\n"+
				"personal\tpersonal@example.com\n"+
				"work\twork@example.com\n",
		)
	})

	t.Run("freebusy", func(t *testing.T) {
		t.Parallel()
		rows := calendarFreeBusyRows(map[string]calendar.FreeBusyCalendar{
			"cal1": {
				Busy: []*calendar.TimePeriod{
					nil,
					{Start: "2026-06-12T10:00:00Z", End: "2026-06-12T11:00:00Z"},
				},
			},
		}, nil)
		got := renderPlainTable(t, rows, calendarFreeBusyColumns())
		assertTableOutput(
			t,
			got,
			"CALENDAR\tSTART\tEND\ncal1\t2026-06-12T10:00:00Z\t2026-06-12T11:00:00Z\n",
		)
	})

	t.Run("users", func(t *testing.T) {
		t.Parallel()
		rows := calendarUserRows([]*people.Person{
			nil,
			{Names: []*people.Name{{DisplayName: "No Email"}}},
			{
				EmailAddresses: []*people.EmailAddress{{Value: "user@example.com"}},
				Names:          []*people.Name{{DisplayName: "User\tOne"}},
			},
		})
		got := renderPlainTable(t, rows, calendarUserColumns())
		assertTableOutput(t, got, "EMAIL\tNAME\nuser@example.com\tUser One\n")
	})

	t.Run("team busy", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []calendarTeamBusyResult{
			{Email: "busy@example.com", Busy: []string{"10:00-11:00", "13:00-14:00"}},
			{Email: "free@example.com"},
			{Email: "error@example.com", Errors: []string{"not\tfound"}},
		}, calendarTeamBusyColumns())
		assertTableOutput(
			t,
			got,
			"WHO\tBUSY BLOCKS\n"+
				"busy@example.com\t10:00-11:00, 13:00-14:00\n"+
				"free@example.com\t(free)\n"+
				"error@example.com\terror: not found\n",
		)
	})

	t.Run("team events", func(t *testing.T) {
		t.Parallel()
		summary := strings.Repeat("x", 41)
		got := renderPlainTable(t, []teamEvent{{
			Who:     "user\texample.com",
			Start:   "start\tlocal",
			End:     "end\tlocal",
			Summary: summary,
		}}, calendarTeamEventColumns())
		assertTableOutput(
			t,
			got,
			"WHO\tSTART\tEND\tSUMMARY\n"+
				"user example.com\tstart local\tend local\t"+strings.Repeat("x", 37)+"...\n",
		)
	})
}

func TestCalendarEventPresentationSchemas(t *testing.T) {
	t.Parallel()

	event := &eventWithCalendar{
		Event: &calendar.Event{
			Id:       "event1",
			Summary:  "Planning",
			Location: "Main\nOffice\tRoom",
			Start:    &calendar.EventDateTime{DateTime: "2026-06-12T10:00:00Z"},
			End:      &calendar.EventDateTime{DateTime: "2026-06-12T11:00:00Z"},
		},
		CalendarID: "cal1",
		StartLocal: "2026-06-12T12:00:00+02:00",
		EndLocal:   "2026-06-12T13:00:00+02:00",
	}

	t.Run("basic", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*eventWithCalendar{event}, calendarEventColumns(false, false, false))
		assertTableOutput(
			t,
			got,
			"ID\tSTART\tEND\tSUMMARY\n"+
				"event1\t2026-06-12T12:00:00+02:00\t2026-06-12T13:00:00+02:00\tPlanning\n",
		)
	})

	t.Run("all calendars weekdays location", func(t *testing.T) {
		t.Parallel()
		withDays := *event
		withDays.StartDayOfWeek = "Friday"
		withDays.EndDayOfWeek = "Friday"
		got := renderPlainTable(
			t,
			[]*eventWithCalendar{&withDays},
			calendarEventColumns(true, true, true),
		)
		assertTableOutput(
			t,
			got,
			"CALENDAR\tID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY\tLOCATION\n"+
				"cal1\tevent1\t2026-06-12T12:00:00+02:00\tFriday\t"+
				"2026-06-12T13:00:00+02:00\tFriday\tPlanning\tMain Office Room\n",
		)
	})

	t.Run("single calendar weekday fallback", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*eventWithCalendar{event}, calendarEventColumns(false, true, false))
		assertTableOutput(
			t,
			got,
			"ID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY\n"+
				"event1\t2026-06-12T12:00:00+02:00\tFriday\t"+
				"2026-06-12T13:00:00+02:00\tFriday\tPlanning\n",
		)
	})
}

func TestCompactCalendarRows(t *testing.T) {
	t.Parallel()

	event := &eventWithCalendar{Event: &calendar.Event{Id: "event1"}}
	rows := compactCalendarRows([]*eventWithCalendar{nil, event, nil})
	if len(rows) != 1 || rows[0] != event {
		t.Fatalf("rows = %#v, want only event1", rows)
	}
}
