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

func listCalendarEvents(ctx context.Context, svc *calendar.Service, calendarID, from, to string, maxResults int64, page string, allPages bool, failEmpty bool, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	u := ui.FromContext(ctx)

	fetch := func(pageToken string) ([]*calendar.Event, string, error) {
		call := svc.Events.List(calendarID).
			TimeMin(from).
			TimeMax(to).
			MaxResults(maxResults).
			SingleEvents(true).
			OrderBy("startTime")
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
		resp, err := call.Context(ctx).Do()
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

	if len(items) == 0 {
		u.Err().Println("No events")
		return failEmptyExit(failEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	if showWeekday {
		fmt.Fprintln(w, "ID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY")
		for _, e := range items {
			startDay, endDay := eventDaysOfWeek(e)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Id, eventStart(e), startDay, eventEnd(e), endDay, e.Summary)
		}
		printNextPageHint(u, nextPageToken)
		return nil
	}

	fmt.Fprintln(w, "ID\tSTART\tEND\tSUMMARY")
	for _, e := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Id, eventStart(e), eventEnd(e), e.Summary)
	}
	printNextPageHint(u, nextPageToken)
	return nil
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

	calResp, err := svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return err
	}

	if len(calResp.Items) == 0 {
		u.Err().Println("No calendars")
		return failEmptyExit(failEmpty)
	}

	all := []*eventWithCalendar{}
	for _, cal := range calResp.Items {
		fetch := func(pageToken string) ([]*calendar.Event, string, error) {
			call := svc.Events.List(cal.Id).
				TimeMin(from).
				TimeMax(to).
				MaxResults(maxResults).
				SingleEvents(true).
				OrderBy("startTime")
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
			events, callErr := call.Context(ctx).Do()
			if callErr != nil {
				return nil, "", callErr
			}
			return events.Items, events.NextPageToken, nil
		}

		var events []*calendar.Event
		if allPages {
			allEvents, collectErr := collectAllPages(page, fetch)
			if collectErr != nil {
				u.Err().Printf("calendar %s: %v", cal.Id, collectErr)
				continue
			}
			events = allEvents
		} else {
			events, _, err = fetch(page)
			if err != nil {
				u.Err().Printf("calendar %s: %v", cal.Id, err)
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
				CalendarID:     cal.Id,
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
	if len(all) == 0 {
		u.Err().Println("No events")
		return failEmptyExit(failEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	if showWeekday {
		fmt.Fprintln(w, "CALENDAR\tID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY")
		for _, e := range all {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventStart(e.Event), e.StartDayOfWeek, eventEnd(e.Event), e.EndDayOfWeek, e.Summary)
		}
		return nil
	}

	fmt.Fprintln(w, "CALENDAR\tID\tSTART\tEND\tSUMMARY")
	for _, e := range all {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventStart(e.Event), eventEnd(e.Event), e.Summary)
	}
	return nil
}
