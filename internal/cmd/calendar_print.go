package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/ui"
)

func printCalendarEvent(u *ui.UI, event *calendar.Event) {
	if u == nil || event == nil {
		return
	}
	u.Out().Printf("id\t%s", event.Id)
	u.Out().Printf("summary\t%s", orEmpty(event.Summary, "(no title)"))
	if event.EventType != "" && event.EventType != eventTypeDefault {
		u.Out().Printf("type\t%s", event.EventType)
	}
	u.Out().Printf("start\t%s", eventStart(event))
	u.Out().Printf("end\t%s", eventEnd(event))
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

func orEmpty(s string, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
