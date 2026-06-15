package cmd

import (
	"context"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func calendarEventsListCall(ctx context.Context, svc *calendar.Service, calendarID, from, to string, maxResults int64, query, privatePropFilter, sharedPropFilter, fields, pageToken string) *calendar.EventsListCall {
	call := svc.Events.List(calendarID).
		TimeMin(from).
		TimeMax(to).
		MaxResults(maxResults).
		SingleEvents(true).
		OrderBy("startTime").
		ShowDeleted(false).
		Context(ctx)
	if strings.TrimSpace(pageToken) != "" {
		call = call.PageToken(pageToken)
	}
	if strings.TrimSpace(query) != "" {
		call = call.Q(query)
	}
	if strings.TrimSpace(privatePropFilter) != "" {
		call = call.PrivateExtendedProperty(privatePropFilter)
	}
	if strings.TrimSpace(sharedPropFilter) != "" {
		call = call.SharedExtendedProperty(sharedPropFilter)
	}
	if strings.TrimSpace(fields) != "" {
		call = call.Fields(gapi.Field(fields))
	}
	return call
}

func listCalendarEvents(ctx context.Context, svc *calendar.Service, calendarID, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool, showLocation bool, sortKey, sortOrder string) error {
	calendarTimezone, loc := calendarDisplayTimezone(ctx, svc, calendarID, nil)
	fetch := func(pageToken string) ([]*calendar.Event, string, error) {
		resp, err := calendarEventsListCall(ctx, svc, calendarID, from, to, maxResults, query, privatePropFilter, sharedPropFilter, fields, pageToken).Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Items, resp.NextPageToken, nil
	}

	items, nextPageToken, err := loadPagedItems(page, allPages, fetch)
	if err != nil {
		return err
	}
	events := make([]*eventWithCalendar, 0, len(items))
	for _, item := range items {
		redactCalendarEventForOutput(ctx, item)
		events = append(events, wrapEventWithCalendar(item, "", calendarTimezone, loc))
	}
	sortEventsBy(events, sortKey, sortOrder)
	if outfmt.IsJSON(ctx) {
		jsonItems := make([]*eventWithDays, 0, len(events))
		for _, e := range events {
			jsonItems = append(jsonItems, wrapEventWithDaysWithTimezone(e.Event, calendarTimezone, loc))
		}
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"events":        jsonItems,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(events) == 0 {
			return failEmptyExit(failEmpty)
		}
		return nil
	}
	return renderCalendarEventsTable(ctx, events, nextPageToken, false, showWeekday, showLocation, failEmpty, true)
}

type eventWithCalendar struct {
	*calendar.Event
	CalendarID     string
	StartDayOfWeek string `json:"startDayOfWeek,omitempty"`
	EndDayOfWeek   string `json:"endDayOfWeek,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	EventTimezone  string `json:"eventTimezone,omitempty"`
	StartLocal     string `json:"startLocal,omitempty"`
	EndLocal       string `json:"endLocal,omitempty"`
}

func (e *eventWithCalendar) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	return marshalCalendarEventWithFields(e.Event, map[string]string{
		"CalendarID":     e.CalendarID,
		"startDayOfWeek": e.StartDayOfWeek,
		"endDayOfWeek":   e.EndDayOfWeek,
		"timezone":       e.Timezone,
		"eventTimezone":  e.EventTimezone,
		"startLocal":     e.StartLocal,
		"endLocal":       e.EndLocal,
	})
}

type calendarTimezoneHint struct {
	timezone string
	loc      *time.Location
}

func listAllCalendarsEvents(ctx context.Context, svc *calendar.Service, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool, showLocation bool, sortKey, sortOrder string) error {
	u := ui.FromContext(ctx)

	calendars, err := listCalendarList(ctx, svc)
	if err != nil {
		return err
	}

	if len(calendars) == 0 {
		u.Err().Println("No calendars")
		return failEmptyExit(failEmpty)
	}

	ids := make([]string, 0, len(calendars))
	for _, cal := range calendars {
		if cal == nil || strings.TrimSpace(cal.Id) == "" {
			continue
		}
		ids = append(ids, cal.Id)
	}
	if len(ids) == 0 {
		u.Err().Println("No calendars")
		return nil
	}
	return listCalendarIDsEvents(ctx, svc, ids, from, to, maxResults, page, allPages, failEmpty, query, privatePropFilter, sharedPropFilter, fields, showWeekday, showLocation, calendarTimezoneHints(calendars), sortKey, sortOrder)
}

func listSelectedCalendarsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool, showLocation bool, sortKey, sortOrder string) error {
	return listCalendarIDsEvents(ctx, svc, calendarIDs, from, to, maxResults, page, allPages, failEmpty, query, privatePropFilter, sharedPropFilter, fields, showWeekday, showLocation, nil, sortKey, sortOrder)
}

func listCalendarIDsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool, showLocation bool, timezoneHints map[string]calendarTimezoneHint, sortKey, sortOrder string) error {
	u := ui.FromContext(ctx)
	all := []*eventWithCalendar{}
	for _, calID := range calendarIDs {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			continue
		}
		calendarTimezone, loc := calendarDisplayTimezone(ctx, svc, calID, timezoneHints)
		fetch := func(pageToken string) ([]*calendar.Event, string, error) {
			resp, err := calendarEventsListCall(ctx, svc, calID, from, to, maxResults, query, privatePropFilter, sharedPropFilter, fields, pageToken).Do()
			if err != nil {
				return nil, "", err
			}
			return resp.Items, resp.NextPageToken, nil
		}

		events, _, err := loadPagedItems(page, allPages, fetch)
		if err != nil {
			u.Err().Linef("calendar %s: %v", calID, err)
			continue
		}

		for _, e := range events {
			redactCalendarEventForOutput(ctx, e)
			all = append(all, wrapEventWithCalendar(e, calID, calendarTimezone, loc))
		}
	}

	sortEventsBy(all, sortKey, sortOrder)

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"events": all}); err != nil {
			return err
		}
		if len(all) == 0 {
			return failEmptyExit(failEmpty)
		}
		return nil
	}
	return renderCalendarEventsTable(ctx, all, "", true, showWeekday, showLocation, failEmpty, false)
}

func renderCalendarEventsTable(ctx context.Context, events []*eventWithCalendar, nextPageToken string, includeCalendar, showWeekday, showLocation, failEmpty bool, printPageHint bool) error {
	u := ui.FromContext(ctx)
	if len(events) == 0 {
		u.Err().Println("No events")
		return failEmptyExit(failEmpty)
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactCalendarRows(events),
		calendarEventColumns(includeCalendar, showWeekday, showLocation),
	); err != nil {
		return err
	}
	if printPageHint {
		printNextPageHintWithAll(u, nextPageToken, "--all-pages")
	}
	return nil
}

func wrapEventWithCalendar(event *calendar.Event, calendarID string, calendarTimezone string, loc *time.Location) *eventWithCalendar {
	wrapped := wrapEventWithDaysWithTimezone(event, calendarTimezone, loc)
	if wrapped == nil {
		return &eventWithCalendar{Event: event, CalendarID: calendarID}
	}
	return &eventWithCalendar{
		Event:          event,
		CalendarID:     calendarID,
		StartDayOfWeek: wrapped.StartDayOfWeek,
		EndDayOfWeek:   wrapped.EndDayOfWeek,
		Timezone:       wrapped.Timezone,
		EventTimezone:  wrapped.EventTimezone,
		StartLocal:     wrapped.StartLocal,
		EndLocal:       wrapped.EndLocal,
	}
}

func eventDisplayStart(e *eventWithCalendar) string {
	if e != nil && e.StartLocal != "" {
		return e.StartLocal
	}
	if e == nil {
		return ""
	}
	return eventStart(e.Event)
}

func eventDisplayEnd(e *eventWithCalendar) string {
	if e != nil && e.EndLocal != "" {
		return e.EndLocal
	}
	if e == nil {
		return ""
	}
	return eventEnd(e.Event)
}

// eventDisplayLocation returns the event location formatted for a single
// table cell. Newlines are collapsed and the value is trimmed so a multi-line
// address from the Calendar API does not break the row layout.
func eventDisplayLocation(e *eventWithCalendar) string {
	if e == nil || e.Event == nil {
		return ""
	}
	loc := strings.TrimSpace(e.Location)
	if loc == "" {
		return ""
	}
	// Calendar locations occasionally arrive with embedded newlines (pasted
	// multi-line addresses); collapse them so the row stays on one line.
	loc = strings.ReplaceAll(loc, "\r\n", " ")
	loc = strings.ReplaceAll(loc, "\n", " ")
	loc = strings.ReplaceAll(loc, "\t", " ")
	return loc
}

func calendarDisplayTimezone(ctx context.Context, svc *calendar.Service, calendarID string, hints map[string]calendarTimezoneHint) (string, *time.Location) {
	if hint, ok := hints[calendarID]; ok {
		return hint.timezone, hint.loc
	}
	tz, loc, err := getCalendarLocation(ctx, svc, calendarID)
	if err != nil {
		return "", nil
	}
	return tz, loc
}

func calendarTimezoneHints(calendars []*calendar.CalendarListEntry) map[string]calendarTimezoneHint {
	hints := make(map[string]calendarTimezoneHint, len(calendars))
	for _, cal := range calendars {
		if cal == nil || strings.TrimSpace(cal.Id) == "" || strings.TrimSpace(cal.TimeZone) == "" {
			continue
		}
		loc, ok := tryLoadTimezoneLocation(cal.TimeZone)
		if !ok {
			continue
		}
		hints[cal.Id] = calendarTimezoneHint{timezone: cal.TimeZone, loc: loc}
	}
	return hints
}

func resolveCalendarIDs(ctx context.Context, store *config.ConfigStore, svc *calendar.Service, inputs []string) ([]string, error) {
	prepared, err := prepareCalendarIDs(store, inputs)
	if err != nil {
		return nil, err
	}
	return resolveCalendarInputs(ctx, svc, prepared, calendarResolveOptions{
		strict:        true,
		allowIndex:    true,
		allowIDLookup: true,
	})
}

func listCalendarList(ctx context.Context, svc *calendar.Service) ([]*calendar.CalendarListEntry, error) {
	var (
		items     []*calendar.CalendarListEntry
		pageToken string
	)
	for {
		call := svc.CalendarList.List().MaxResults(250).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		if len(resp.Items) > 0 {
			items = append(items, resp.Items...)
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return items, nil
}

// sortEventsBy sorts events in place by the given key (start|end|summary|calendar).
// An empty key leaves the slice untouched. The Google Calendar API already
// returns per-calendar events ordered by startTime; this helper is mainly useful
// when aggregating events across multiple calendars (e.g. --all) or when callers
// want a non-default ordering. Sort is stable to preserve API tie-breaks.
//
// Time keys (start, end) compare as instants (parsed time.Time), so events
// crossing timezones interleave correctly. String keys (summary, calendar)
// compare case-insensitive for summary, exact for calendar id.
func sortEventsBy(events []*eventWithCalendar, key, order string) {
	const calendarSortEnd = "end"

	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || len(events) < 2 {
		return
	}
	desc := strings.ToLower(strings.TrimSpace(order)) == "desc"

	switch key {
	case "start", calendarSortEnd:
		instantFn := eventStartInstant
		if key == calendarSortEnd {
			instantFn = eventEndInstant
		}
		sort.SliceStable(events, func(i, j int) bool {
			a, b := instantFn(events[i]), instantFn(events[j])
			if a.Equal(b) {
				return false
			}
			if desc {
				return a.After(b)
			}
			return a.Before(b)
		})
	case "summary":
		sort.SliceStable(events, func(i, j int) bool {
			a, b := strings.ToLower(eventSummary(events[i])), strings.ToLower(eventSummary(events[j]))
			if desc {
				return a > b
			}
			return a < b
		})
	case "calendar":
		sort.SliceStable(events, func(i, j int) bool {
			a, b := eventCalendarID(events[i]), eventCalendarID(events[j])
			if desc {
				return a > b
			}
			return a < b
		})
	}
}

func eventSummary(e *eventWithCalendar) string {
	if e == nil || e.Event == nil {
		return ""
	}
	return e.Summary
}

func eventCalendarID(e *eventWithCalendar) string {
	if e == nil {
		return ""
	}
	return e.CalendarID
}

// eventStartInstant returns the start time as an absolute instant.
// All-day events fall back to midnight UTC, which is consistent enough for
// ordering within a single result set.
func eventStartInstant(e *eventWithCalendar) time.Time {
	if e == nil || e.Event == nil || e.Start == nil {
		return time.Time{}
	}
	return eventDatePointInstant(e.Start)
}

func eventEndInstant(e *eventWithCalendar) time.Time {
	if e == nil || e.Event == nil || e.End == nil {
		return time.Time{}
	}
	return eventDatePointInstant(e.End)
}

func eventDatePointInstant(dt *calendar.EventDateTime) time.Time {
	if dt == nil {
		return time.Time{}
	}
	if t, ok := parseEventTime(dt.DateTime, dt.TimeZone); ok {
		return t
	}
	if t, ok := parseEventDate(dt.Date, dt.TimeZone); ok {
		return t
	}
	return time.Time{}
}
