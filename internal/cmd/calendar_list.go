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

func listCalendarEvents(ctx context.Context, svc *calendar.Service, calendarID, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	u := ui.FromContext(ctx)

	call := svc.Events.List(calendarID).
		TimeMin(from).
		TimeMax(to).
		MaxResults(maxResults).
		PageToken(page).
		SingleEvents(true).
		OrderBy("startTime")
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
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"events":        wrapEventsWithDays(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No events")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	if showWeekday {
		fmt.Fprintln(w, "ID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY")
		for _, e := range resp.Items {
			startDay, endDay := eventDaysOfWeek(e)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Id, eventStart(e), startDay, eventEnd(e), endDay, e.Summary)
		}
		printNextPageHint(u, resp.NextPageToken)
		return nil
	}

	fmt.Fprintln(w, "ID\tSTART\tEND\tSUMMARY")
	for _, e := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Id, eventStart(e), eventEnd(e), e.Summary)
	}
	printNextPageHint(u, resp.NextPageToken)
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

func listAllCalendarsEvents(ctx context.Context, svc *calendar.Service, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	u := ui.FromContext(ctx)

	calResp, err := svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return err
	}

	if len(calResp.Items) == 0 {
		u.Err().Println("No calendars")
		return nil
	}

	all := []*eventWithCalendar{}
	for _, cal := range calResp.Items {
		call := svc.Events.List(cal.Id).
			TimeMin(from).
			TimeMax(to).
			MaxResults(maxResults).
			PageToken(page).
			SingleEvents(true).
			OrderBy("startTime")
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
		events, err := call.Context(ctx).Do()
		if err != nil {
			u.Err().Printf("calendar %s: %v", cal.Id, err)
			continue
		}
		for _, e := range events.Items {
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"events": all})
	}
	if len(all) == 0 {
		u.Err().Println("No events")
		return nil
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
