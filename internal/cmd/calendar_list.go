package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
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
			call := svc.Events.List(calID).
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

func resolveCalendarIDs(ctx context.Context, svc *calendar.Service, inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	calendars, err := listCalendarList(ctx, svc)
	if err != nil {
		return nil, err
	}

	bySummary := make(map[string][]string, len(calendars))
	byID := make(map[string]string, len(calendars))
	for _, cal := range calendars {
		if cal == nil {
			continue
		}
		if strings.TrimSpace(cal.Id) != "" {
			byID[strings.ToLower(strings.TrimSpace(cal.Id))] = cal.Id
		}
		if strings.TrimSpace(cal.Summary) != "" {
			summaryKey := strings.ToLower(strings.TrimSpace(cal.Summary))
			bySummary[summaryKey] = append(bySummary[summaryKey], cal.Id)
		}
	}

	out := make([]string, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	var unrecognized []string

	for _, raw := range inputs {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if isDigits(value) {
			idx, err := strconv.Atoi(value)
			if err != nil {
				return nil, usagef("invalid calendar index: %s", value)
			}
			if idx < 1 || idx > len(calendars) {
				return nil, usagef("calendar index %d out of range (have %d calendars)", idx, len(calendars))
			}
			cal := calendars[idx-1]
			if cal == nil || strings.TrimSpace(cal.Id) == "" {
				return nil, usagef("calendar index %d has no id", idx)
			}
			appendUniqueCalendarID(&out, seen, cal.Id)
			continue
		}

		key := strings.ToLower(value)
		if ids, ok := bySummary[key]; ok {
			if len(ids) > 1 {
				return nil, usagef("calendar name %q is ambiguous", value)
			}
			if len(ids) == 1 {
				appendUniqueCalendarID(&out, seen, ids[0])
				continue
			}
			continue
		}
		if id, ok := byID[key]; ok {
			appendUniqueCalendarID(&out, seen, id)
			continue
		}
		unrecognized = append(unrecognized, value)
	}

	if len(unrecognized) > 0 {
		return nil, usagef("unrecognized calendar name(s): %s", strings.Join(unrecognized, ", "))
	}

	return out, nil
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

func appendUniqueCalendarID(out *[]string, seen map[string]struct{}, id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if _, ok := seen[id]; ok {
		return
	}
	seen[id] = struct{}{}
	*out = append(*out, id)
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
