package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
	gapi "google.golang.org/api/googleapi"

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

func listCalendarEvents(ctx context.Context, svc *calendar.Service, calendarID, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
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
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"events":        wrapEventsWithTimezone(items, calendarTimezone, loc),
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(failEmpty)
		}
		return nil
	}
	events := make([]*eventWithCalendar, 0, len(items))
	for _, item := range items {
		events = append(events, wrapEventWithCalendar(item, "", calendarTimezone, loc))
	}
	return renderCalendarEventsTable(ctx, events, nextPageToken, false, showWeekday, failEmpty, true)
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

func listAllCalendarsEvents(ctx context.Context, svc *calendar.Service, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
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
	return listCalendarIDsEvents(ctx, svc, ids, from, to, maxResults, page, allPages, failEmpty, query, privatePropFilter, sharedPropFilter, fields, showWeekday, calendarTimezoneHints(calendars))
}

func listSelectedCalendarsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	return listCalendarIDsEvents(ctx, svc, calendarIDs, from, to, maxResults, page, allPages, failEmpty, query, privatePropFilter, sharedPropFilter, fields, showWeekday, nil)
}

func listCalendarIDsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool, timezoneHints map[string]calendarTimezoneHint) error {
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
			u.Err().Printf("calendar %s: %v", calID, err)
			continue
		}

		for _, e := range events {
			all = append(all, wrapEventWithCalendar(e, calID, calendarTimezone, loc))
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"events": all}); err != nil {
			return err
		}
		if len(all) == 0 {
			return failEmptyExit(failEmpty)
		}
		return nil
	}
	return renderCalendarEventsTable(ctx, all, "", true, showWeekday, failEmpty, false)
}

func renderCalendarEventsTable(ctx context.Context, events []*eventWithCalendar, nextPageToken string, includeCalendar, showWeekday, failEmpty bool, printPageHint bool) error {
	u := ui.FromContext(ctx)
	if len(events) == 0 {
		u.Err().Println("No events")
		return failEmptyExit(failEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	if showWeekday {
		if includeCalendar {
			fmt.Fprintln(w, "CALENDAR\tID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY")
			for _, e := range events {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventDisplayStart(e), e.StartDayOfWeek, eventDisplayEnd(e), e.EndDayOfWeek, e.Summary)
			}
		} else {
			fmt.Fprintln(w, "ID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY")
			for _, e := range events {
				startDay, endDay := e.StartDayOfWeek, e.EndDayOfWeek
				if startDay == "" && endDay == "" {
					startDay, endDay = eventDaysOfWeek(e.Event)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Id, eventDisplayStart(e), startDay, eventDisplayEnd(e), endDay, e.Summary)
			}
		}
	} else {
		if includeCalendar {
			fmt.Fprintln(w, "CALENDAR\tID\tSTART\tEND\tSUMMARY")
			for _, e := range events {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventDisplayStart(e), eventDisplayEnd(e), e.Summary)
			}
		} else {
			fmt.Fprintln(w, "ID\tSTART\tEND\tSUMMARY")
			for _, e := range events {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Id, eventDisplayStart(e), eventDisplayEnd(e), e.Summary)
			}
		}
	}
	if printPageHint {
		printNextPageHint(u, nextPageToken)
	}
	return nil
}

func wrapEventsWithTimezone(events []*calendar.Event, calendarTimezone string, loc *time.Location) []*eventWithDays {
	if len(events) == 0 {
		return []*eventWithDays{}
	}
	out := make([]*eventWithDays, 0, len(events))
	for _, ev := range events {
		out = append(out, wrapEventWithDaysWithTimezone(ev, calendarTimezone, loc))
	}
	return out
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

func resolveCalendarIDs(ctx context.Context, svc *calendar.Service, inputs []string) ([]string, error) {
	prepared, err := prepareCalendarIDs(inputs)
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
