package cmd

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/ui"
)

func printCalendarEvent(u *ui.UI, event *calendar.Event) {
	printCalendarEventWithTimezone(u, event, "", nil)
}

func printCalendarEventWithTimezone(u *ui.UI, event *calendar.Event, calendarTimezone string, loc *time.Location) {
	if u == nil || event == nil {
		return
	}
	calendarTimezone = strings.TrimSpace(calendarTimezone)
	eventTimezone := eventTimezone(event)

	if loc == nil && calendarTimezone != "" {
		if loaded, err := time.LoadLocation(calendarTimezone); err == nil {
			loc = loaded
		} else {
			calendarTimezone = ""
		}
	}
	if calendarTimezone == "" {
		calendarTimezone = eventTimezone
		if loc == nil && calendarTimezone != "" {
			if loaded, err := time.LoadLocation(calendarTimezone); err == nil {
				loc = loaded
			} else {
				calendarTimezone = ""
			}
		}
	}

	u.Out().Printf("id\t%s", event.Id)
	u.Out().Printf("summary\t%s", orEmpty(event.Summary, "(no title)"))
	if event.EventType != "" && event.EventType != eventTypeDefault {
		u.Out().Printf("type\t%s", event.EventType)
	}
	if calendarTimezone != "" {
		u.Out().Printf("timezone\t%s", calendarTimezone)
	}
	if eventTimezone != "" && eventTimezone != calendarTimezone {
		u.Out().Printf("event-timezone\t%s", eventTimezone)
	}

	u.Out().Printf("start\t%s", eventStart(event))
	startDay, endDay := eventDaysOfWeek(event)
	if startDay != "" {
		u.Out().Printf("start-day-of-week\t%s", startDay)
	}
	if startLocal := formatEventLocal(event.Start, loc); startLocal != "" {
		u.Out().Printf("start-local\t%s", startLocal)
	}
	u.Out().Printf("end\t%s", eventEnd(event))
	if endDay != "" {
		u.Out().Printf("end-day-of-week\t%s", endDay)
	}
	if endLocal := formatEventLocal(event.End, loc); endLocal != "" {
		u.Out().Printf("end-local\t%s", endLocal)
	}
	if event.Description != "" {
		u.Out().Printf("description\t%s", event.Description)
	}
	if event.Location != "" {
		u.Out().Printf("location\t%s", event.Location)
	}
	if event.ColorId != "" {
		u.Out().Printf("color\t%s", event.ColorId)
	}
	if event.Visibility != "" && event.Visibility != "default" {
		u.Out().Printf("visibility\t%s", event.Visibility)
	}
	if event.Transparency == "transparent" {
		u.Out().Printf("show-as\tfree")
	}
	if len(event.Attendees) > 0 {
		for _, a := range event.Attendees {
			if a == nil || strings.TrimSpace(a.Email) == "" {
				continue
			}
			status := a.ResponseStatus
			if a.Optional {
				status += " (optional)"
			}
			u.Out().Printf("attendee\t%s\t%s", strings.TrimSpace(a.Email), status)
		}
	}
	if event.GuestsCanInviteOthers != nil && !*event.GuestsCanInviteOthers {
		u.Out().Printf("guests-can-invite\tfalse")
	}
	if event.GuestsCanModify {
		u.Out().Printf("guests-can-modify\ttrue")
	}
	if event.GuestsCanSeeOtherGuests != nil && !*event.GuestsCanSeeOtherGuests {
		u.Out().Printf("guests-can-see-others\tfalse")
	}
	if event.HangoutLink != "" {
		u.Out().Printf("meet\t%s", event.HangoutLink)
	}
	if event.ConferenceData != nil && len(event.ConferenceData.EntryPoints) > 0 {
		for _, ep := range event.ConferenceData.EntryPoints {
			if ep.EntryPointType == "video" {
				u.Out().Printf("video-link\t%s", ep.Uri)
			}
		}
	}
	if len(event.Recurrence) > 0 {
		u.Out().Printf("recurrence\t%s", strings.Join(event.Recurrence, "; "))
	}
	if event.Reminders != nil {
		if event.Reminders.UseDefault {
			u.Out().Printf("reminders\t(calendar default)")
		} else if len(event.Reminders.Overrides) > 0 {
			reminders := make([]string, 0, len(event.Reminders.Overrides))
			for _, r := range event.Reminders.Overrides {
				if r != nil {
					reminders = append(reminders, fmt.Sprintf("%s:%dm", r.Method, r.Minutes))
				}
			}
			u.Out().Printf("reminders\t%s", strings.Join(reminders, ", "))
		}
	}
	if len(event.Attachments) > 0 {
		for _, a := range event.Attachments {
			if a != nil {
				u.Out().Printf("attachment\t%s", a.FileUrl)
			}
		}
	}
	if event.FocusTimeProperties != nil {
		u.Out().Printf("auto-decline\t%s", event.FocusTimeProperties.AutoDeclineMode)
		if event.FocusTimeProperties.ChatStatus != "" {
			u.Out().Printf("chat-status\t%s", event.FocusTimeProperties.ChatStatus)
		}
	}
	if event.OutOfOfficeProperties != nil {
		u.Out().Printf("auto-decline\t%s", event.OutOfOfficeProperties.AutoDeclineMode)
		if event.OutOfOfficeProperties.DeclineMessage != "" {
			u.Out().Printf("decline-message\t%s", event.OutOfOfficeProperties.DeclineMessage)
		}
	}
	if event.WorkingLocationProperties != nil {
		u.Out().Printf("location-type\t%s", event.WorkingLocationProperties.Type)
	}
	if event.Source != nil && event.Source.Url != "" {
		if event.Source.Title != "" {
			u.Out().Printf("source\t%s (%s)", event.Source.Url, event.Source.Title)
		} else {
			u.Out().Printf("source\t%s", event.Source.Url)
		}
	}
	if event.HtmlLink != "" {
		u.Out().Printf("link\t%s", event.HtmlLink)
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
