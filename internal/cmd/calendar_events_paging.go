package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/calendar/v3"
)

// listCalendarEventsUpTo reads events.list pages until it has max events or
// the listing ends. events.list may return fewer events than maxResults and a
// nextPageToken even below its 2500-event page cap, so one call is not
// enough. more reports that the listing had events beyond max.
func listCalendarEventsUpTo(ctx context.Context, newCall func() *calendar.EventsListCall, max int64) (events []*calendar.Event, more bool, err error) {
	var guard pageTokenGuard
	pageToken := ""
	for {
		if err := guard.check(pageToken); err != nil {
			return nil, false, err
		}
		call := newCall().MaxResults(max - int64(len(events))).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, false, err
		}
		events = append(events, resp.Items...)
		next := strings.TrimSpace(resp.NextPageToken)
		if int64(len(events)) >= max {
			return events[:max], next != "" || int64(len(events)) > max, nil
		}
		if next == "" {
			return events, false, nil
		}
		pageToken = next
	}
}
