package cmd

import (
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

type eventWithDays struct {
	*calendar.Event
	StartDayOfWeek string `json:"startDayOfWeek,omitempty"`
	EndDayOfWeek   string `json:"endDayOfWeek,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	EventTimezone  string `json:"eventTimezone,omitempty"`
	StartLocal     string `json:"startLocal,omitempty"`
	EndLocal       string `json:"endLocal,omitempty"`
}

func (e *eventWithDays) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	return marshalCalendarEventWithFields(e.Event, map[string]string{
		"startDayOfWeek": e.StartDayOfWeek,
		"endDayOfWeek":   e.EndDayOfWeek,
		"timezone":       e.Timezone,
		"eventTimezone":  e.EventTimezone,
		"startLocal":     e.StartLocal,
		"endLocal":       e.EndLocal,
	})
}

func wrapEventsWithDays(events []*calendar.Event) []*eventWithDays {
	if len(events) == 0 {
		return []*eventWithDays{}
	}
	out := make([]*eventWithDays, 0, len(events))
	for _, ev := range events {
		out = append(out, wrapEventWithDaysWithTimezone(ev, "", nil))
	}
	return out
}

func wrapEventWithDaysWithTimezone(event *calendar.Event, calendarTimezone string, loc *time.Location) *eventWithDays {
	if event == nil {
		return nil
	}
	evTimezone := eventTimezone(event)
	calendarTimezone, loc = resolveEventTimezone(event, calendarTimezone, loc)
	startDay, endDay := eventDaysOfWeekInLocation(event, loc)

	startLocal := formatEventLocal(event.Start, loc)
	endLocal := formatEventLocal(event.End, loc)

	wrapped := &eventWithDays{
		Event:          event,
		StartDayOfWeek: startDay,
		EndDayOfWeek:   endDay,
		Timezone:       calendarTimezone,
		StartLocal:     startLocal,
		EndLocal:       endLocal,
	}
	if evTimezone != "" && evTimezone != calendarTimezone {
		wrapped.EventTimezone = evTimezone
	}
	return wrapped
}

func eventDaysOfWeek(event *calendar.Event) (string, string) {
	return eventDaysOfWeekInLocation(event, nil)
}

func eventDaysOfWeekInLocation(event *calendar.Event, loc *time.Location) (string, string) {
	if event == nil {
		return "", ""
	}
	startDay := dayOfWeekFromEventDateTime(event.Start, loc)
	endDay := dayOfWeekFromEventDateTime(event.End, loc)
	return startDay, endDay
}

func dayOfWeekFromEventDateTime(dt *calendar.EventDateTime, loc *time.Location) string {
	if dt == nil {
		return ""
	}
	if dt.DateTime != "" {
		if t, ok := parseEventTime(dt.DateTime, dt.TimeZone); ok {
			if loc != nil {
				t = t.In(loc)
			}
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
	return tryLoadTimezoneLocation(tz)
}

// resolveEventTimezone resolves the timezone and location for an event.
// It tries the calendar timezone first, then falls back to the event's timezone.
// Returns the resolved timezone name and location. If loc is already provided,
// it will be used as-is (only calendarTimezone may be updated for display).
func resolveEventTimezone(event *calendar.Event, calendarTimezone string, loc *time.Location) (string, *time.Location) {
	calendarTimezone = strings.TrimSpace(calendarTimezone)
	evTimezone := eventTimezone(event)

	if loc == nil && calendarTimezone != "" {
		if loaded, ok := tryLoadTimezoneLocation(calendarTimezone); ok {
			loc = loaded
		} else {
			calendarTimezone = ""
		}
	}
	if calendarTimezone == "" {
		calendarTimezone = evTimezone
		if loc == nil && calendarTimezone != "" {
			if loaded, ok := tryLoadTimezoneLocation(calendarTimezone); ok {
				loc = loaded
			} else {
				calendarTimezone = ""
			}
		}
	}
	return calendarTimezone, loc
}

func marshalCalendarEventWithFields(event *calendar.Event, fields map[string]string) ([]byte, error) {
	raw := map[string]any{}
	if event != nil {
		data, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		if string(data) != "null" {
			if err := json.Unmarshal(data, &raw); err != nil {
				return nil, err
			}
		}
	}
	for key, value := range fields {
		if value != "" {
			raw[key] = value
		}
	}
	return json.Marshal(raw)
}
