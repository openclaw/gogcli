package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	fetch := func(pageToken string) ([]*calendar.Event, string, error) {
		resp, err := calendarEventsListCall(ctx, svc, calendarID, from, to, maxResults, query, privatePropFilter, sharedPropFilter, fields, pageToken).Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Items, resp.NextPageToken, nil
	}

	var items []*calendar.Event
	nextPageToken := ""
	if allPages {
		all, err := collectAllPages(page, fetch)
		if err != nil {
			return err
		}
		items = all
	} else {
		var err error
		items, nextPageToken, err = fetch(page)
		if err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"events":        wrapEventsWithDays(items),
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
		events = append(events, &eventWithCalendar{Event: item})
	}
	return renderCalendarEventsTable(ctx, events, nextPageToken, false, showWeekday, failEmpty, true)
}

type eventWithCalendar struct {
	*calendar.Event
	CalendarID     string
	StartDayOfWeek string `json:"startDayOfWeek,omitempty"`
	EndDayOfWeek   string `json:"endDayOfWeek,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	StartLocal     string `json:"startLocal,omitempty"`
	EndLocal       string `json:"endLocal,omitempty"`
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
	return listCalendarIDsEvents(ctx, svc, ids, from, to, maxResults, page, allPages, failEmpty, query, privatePropFilter, sharedPropFilter, fields, showWeekday)
}

func listSelectedCalendarsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	return listCalendarIDsEvents(ctx, svc, calendarIDs, from, to, maxResults, page, allPages, failEmpty, query, privatePropFilter, sharedPropFilter, fields, showWeekday)
}

func listCalendarIDsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	u := ui.FromContext(ctx)
	all := []*eventWithCalendar{}
	for _, calID := range calendarIDs {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			continue
		}
		fetch := func(pageToken string) ([]*calendar.Event, string, error) {
			resp, err := calendarEventsListCall(ctx, svc, calID, from, to, maxResults, query, privatePropFilter, sharedPropFilter, fields, pageToken).Do()
			if err != nil {
				return nil, "", err
			}
			return resp.Items, resp.NextPageToken, nil
		}

		var events []*calendar.Event
		var err error
		if allPages {
			allEvents, collectErr := collectAllPages(page, fetch)
			if collectErr != nil {
				u.Err().Printf("calendar %s: %v", calID, collectErr)
				continue
			}
			events = allEvents
		} else {
			events, _, err = fetch(page)
			if err != nil {
				u.Err().Printf("calendar %s: %v", calID, err)
				continue
			}
		}

		for _, e := range events {
			startDay, endDay := eventDaysOfWeek(e)
			evTimezone := eventTimezone(e)
			startLocal := formatEventLocal(e.Start, nil)
			endLocal := formatEventLocal(e.End, nil)
			all = append(all, &eventWithCalendar{
				Event:          e,
				CalendarID:     calID,
				StartDayOfWeek: startDay,
				EndDayOfWeek:   endDay,
				Timezone:       evTimezone,
				StartLocal:     startLocal,
				EndLocal:       endLocal,
			})
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
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventStart(e.Event), e.StartDayOfWeek, eventEnd(e.Event), e.EndDayOfWeek, e.Summary)
			}
		} else {
			fmt.Fprintln(w, "ID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY")
			for _, e := range events {
				startDay, endDay := eventDaysOfWeek(e.Event)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Id, eventStart(e.Event), startDay, eventEnd(e.Event), endDay, e.Summary)
			}
		}
	} else {
		if includeCalendar {
			fmt.Fprintln(w, "CALENDAR\tID\tSTART\tEND\tSUMMARY")
			for _, e := range events {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventStart(e.Event), eventEnd(e.Event), e.Summary)
			}
		} else {
			fmt.Fprintln(w, "ID\tSTART\tEND\tSUMMARY")
			for _, e := range events {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Id, eventStart(e.Event), eventEnd(e.Event), e.Summary)
			}
		}
	}
	if printPageHint {
		printNextPageHint(u, nextPageToken)
	}
	return nil
}

func resolveCalendarIDs(ctx context.Context, svc *calendar.Service, inputs []string) ([]string, error) {
	return resolveCalendarInputs(ctx, svc, inputs, calendarResolveOptions{
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
