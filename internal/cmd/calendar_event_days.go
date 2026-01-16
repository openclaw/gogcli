package cmd

import (
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

type eventWithDays struct {
	*calendar.Event
	StartDayOfWeek string `json:"startDayOfWeek,omitempty"`
	EndDayOfWeek   string `json:"endDayOfWeek,omitempty"`
}

func wrapEventWithDays(event *calendar.Event) *eventWithDays {
	if event == nil {
		return nil
	}
	startDay, endDay := eventDaysOfWeek(event)
	return &eventWithDays{
		Event:          event,
		StartDayOfWeek: startDay,
		EndDayOfWeek:   endDay,
	}
}

func wrapEventsWithDays(events []*calendar.Event) []*eventWithDays {
	if len(events) == 0 {
		return []*eventWithDays{}
	}
	out := make([]*eventWithDays, 0, len(events))
	for _, ev := range events {
		out = append(out, wrapEventWithDays(ev))
	}
	return out
}

func eventDaysOfWeek(event *calendar.Event) (string, string) {
	if event == nil {
		return "", ""
	}
	startDay := dayOfWeekFromEventDateTime(event.Start)
	endDay := dayOfWeekFromEventDateTime(event.End)
	return startDay, endDay
}

func dayOfWeekFromEventDateTime(dt *calendar.EventDateTime) string {
	if dt == nil {
		return ""
	}
	if dt.DateTime != "" {
		if t, ok := parseEventTime(dt.DateTime, dt.TimeZone); ok {
			return t.Weekday().String()
		}
	}
	if dt.Date != "" {
		if t, ok := parseEventDate(dt.Date, dt.TimeZone); ok {
			return t.Weekday().String()
		}
	}
	return ""
}

func parseEventTime(value string, tz string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		if loc, ok := loadEventLocation(tz); ok {
			return t.In(loc), true
		}
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		if loc, ok := loadEventLocation(tz); ok {
			return t.In(loc), true
		}
		return t, true
	}
	if loc, ok := loadEventLocation(tz); ok {
		if t, err := time.ParseInLocation("2006-01-02T15:04:05", value, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseEventDate(value string, tz string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if loc, ok := loadEventLocation(tz); ok {
		if t, err := time.ParseInLocation("2006-01-02", value, loc); err == nil {
			return t, true
		}
	} else if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func loadEventLocation(tz string) (*time.Location, bool) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return nil, false
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, false
	}
	return loc, true
}
