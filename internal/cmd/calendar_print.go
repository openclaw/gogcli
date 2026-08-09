package cmd

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/openclaw/gogcli/internal/ui"
)

const calendarEntryPointTypeVideo = "video"

func printCalendarEventWithTimezone(u *ui.UI, event *calendar.Event, calendarTimezone string, loc *time.Location) {
	printCalendarEventWithTimezoneOverride(u, event, calendarTimezone, loc, false)
}

func printCalendarEventWithTimezoneOverride(u *ui.UI, event *calendar.Event, calendarTimezone string, loc *time.Location, forceDisplayTimezone bool) {
	if u == nil || event == nil {
		return
	}
	eventTimezone := eventTimezone(event)
	startLoc, endLoc := loc, loc
	if forceDisplayTimezone {
		calendarTimezone = strings.TrimSpace(calendarTimezone)
	} else {
		fallbackTimezone, fallbackLoc := calendarTimezone, loc
		calendarTimezone = resolveEventTimezone(event, calendarTimezone, loc)
		startLoc = resolveEventDateTimeTimezone(event.Start, fallbackTimezone, fallbackLoc)
		endLoc = resolveEventDateTimeTimezone(event.End, fallbackTimezone, fallbackLoc)
	}

	u.Out().Linef("id\t%s", event.Id)
	if event.RecurringEventId != "" {
		u.Out().Linef("recurringEventId\t%s", event.RecurringEventId)
	}
	u.Out().Linef("summary\t%s", orEmpty(event.Summary, "(no title)"))
	if event.EventType != "" && event.EventType != eventTypeDefault {
		u.Out().Linef("type\t%s", event.EventType)
	}
	if calendarTimezone != "" {
		u.Out().Linef("timezone\t%s", calendarTimezone)
	}
	if eventTimezone != "" && eventTimezone != calendarTimezone {
		u.Out().Linef("event-timezone\t%s", eventTimezone)
	}

	u.Out().Linef("start\t%s", eventStart(event))
	startDay := dayOfWeekFromEventDateTime(event.Start, startLoc)
	endDay := dayOfWeekFromEventDateTime(event.End, endLoc)
	if startDay != "" {
		u.Out().Linef("start-day-of-week\t%s", startDay)
	}
	if startLocal := formatEventLocal(event.Start, startLoc); startLocal != "" {
		u.Out().Linef("start-local\t%s", startLocal)
	}
	u.Out().Linef("end\t%s", eventEnd(event))
	if endDay != "" {
		u.Out().Linef("end-day-of-week\t%s", endDay)
	}
	if endLocal := formatEventLocal(event.End, endLoc); endLocal != "" {
		u.Out().Linef("end-local\t%s", endLocal)
	}
	if event.Description != "" {
		u.Out().Linef("description\t%s", event.Description)
	}
	if event.Location != "" {
		u.Out().Linef("location\t%s", event.Location)
	}
	if event.ColorId != "" {
		u.Out().Linef("color\t%s", event.ColorId)
	}
	if event.Visibility != "" && event.Visibility != eventTypeDefault {
		u.Out().Linef("visibility\t%s", event.Visibility)
	}
	if event.Transparency == "transparent" {
		u.Out().Linef("show-as\tfree")
	}
	printEventAttendees(u, event.Attendees)
	if event.GuestsCanInviteOthers != nil && !*event.GuestsCanInviteOthers {
		u.Out().Linef("guests-can-invite\tfalse")
	}
	if event.GuestsCanModify {
		u.Out().Linef("guests-can-modify\ttrue")
	}
	if event.GuestsCanSeeOtherGuests != nil && !*event.GuestsCanSeeOtherGuests {
		u.Out().Linef("guests-can-see-others\tfalse")
	}
	if event.HangoutLink != "" {
		u.Out().Linef("meet\t%s", event.HangoutLink)
	}
	if event.ConferenceData != nil && len(event.ConferenceData.EntryPoints) > 0 {
		for _, ep := range event.ConferenceData.EntryPoints {
			if ep.EntryPointType == calendarEntryPointTypeVideo {
				u.Out().Linef("video-link\t%s", ep.Uri)
			}
		}
	}
	if len(event.Recurrence) > 0 {
		u.Out().Linef("recurrence\t%s", strings.Join(event.Recurrence, "; "))
	}
	printEventReminders(u, event.Reminders)
	if len(event.Attachments) > 0 {
		for _, a := range event.Attachments {
			if a != nil {
				u.Out().Linef("attachment\t%s", a.FileUrl)
			}
		}
	}
	if event.FocusTimeProperties != nil {
		u.Out().Linef("auto-decline\t%s", event.FocusTimeProperties.AutoDeclineMode)
		if event.FocusTimeProperties.ChatStatus != "" {
			u.Out().Linef("chat-status\t%s", event.FocusTimeProperties.ChatStatus)
		}
	}
	if event.OutOfOfficeProperties != nil {
		u.Out().Linef("auto-decline\t%s", event.OutOfOfficeProperties.AutoDeclineMode)
		if event.OutOfOfficeProperties.DeclineMessage != "" {
			u.Out().Linef("decline-message\t%s", event.OutOfOfficeProperties.DeclineMessage)
		}
	}
	if event.WorkingLocationProperties != nil {
		u.Out().Linef("location-type\t%s", event.WorkingLocationProperties.Type)
	}
	if event.Source != nil && event.Source.Url != "" {
		if event.Source.Title != "" {
			u.Out().Linef("source\t%s (%s)", event.Source.Url, event.Source.Title)
		} else {
			u.Out().Linef("source\t%s", event.Source.Url)
		}
	}
	if event.HtmlLink != "" {
		u.Out().Linef("link\t%s", event.HtmlLink)
	}
}

func eventStart(e *calendar.Event) string {
	if e == nil || e.Start == nil {
		return ""
	}
	if e.Start.DateTime != "" {
		return e.Start.DateTime
	}
	return e.Start.Date
}

func eventEnd(e *calendar.Event) string {
	if e == nil || e.End == nil {
		return ""
	}
	if e.End.DateTime != "" {
		return e.End.DateTime
	}
	return e.End.Date
}

func eventTimezone(e *calendar.Event) string {
	if e == nil {
		return ""
	}
	if e.Start != nil && strings.TrimSpace(e.Start.TimeZone) != "" {
		return strings.TrimSpace(e.Start.TimeZone)
	}
	if e.End != nil && strings.TrimSpace(e.End.TimeZone) != "" {
		return strings.TrimSpace(e.End.TimeZone)
	}
	return ""
}

func formatEventLocal(dt *calendar.EventDateTime, loc *time.Location) string {
	if dt == nil {
		return ""
	}
	if dt.DateTime != "" {
		if loc == nil && strings.TrimSpace(dt.TimeZone) != "" {
			if loaded, err := time.LoadLocation(strings.TrimSpace(dt.TimeZone)); err == nil {
				loc = loaded
			}
		}
		if t, ok := parseEventTime(dt.DateTime, dt.TimeZone); ok {
			if loc != nil {
				return t.In(loc).Format(time.RFC3339)
			}
			return t.Format(time.RFC3339)
		}
	}
	if dt.Date != "" {
		return dt.Date
	}
	return ""
}

func orEmpty(s string, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func printEventAttendees(u *ui.UI, attendees []*calendar.EventAttendee) {
	for _, a := range attendees {
		if a == nil || strings.TrimSpace(a.Email) == "" {
			continue
		}
		status := a.ResponseStatus
		if a.Optional {
			status += " (optional)"
		}
		u.Out().Linef("attendee\t%s\t%s", strings.TrimSpace(a.Email), status)
	}
}

func printEventReminders(u *ui.UI, reminders *calendar.EventReminders) {
	if reminders == nil {
		return
	}
	if reminders.UseDefault {
		u.Out().Linef("reminders\t(calendar default)")
		return
	}
	if len(reminders.Overrides) > 0 {
		parts := make([]string, 0, len(reminders.Overrides))
		for _, r := range reminders.Overrides {
			if r != nil {
				parts = append(parts, fmt.Sprintf("%s:%dm", r.Method, r.Minutes))
			}
		}
		u.Out().Linef("reminders\t%s", strings.Join(parts, ", "))
	}
}
