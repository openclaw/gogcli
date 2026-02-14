package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/tasks/v1"
)

const (
	defaultTaskListID = "@default"
	primaryCalendarID = "primary"
)

// resolveTasklistID resolves a task list title to an ID (case-insensitive exact match).
// If input matches an existing ID, it is returned unchanged.
//
// This is intentionally conservative: we only resolve exact title matches and error
// on ambiguity.
func resolveTasklistID(ctx context.Context, svc *tasks.Service, input string) (string, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return "", nil
	}
	// Common agent desire path.
	if strings.EqualFold(in, "default") {
		in = defaultTaskListID
	}
	// Special task list ID used by the API.
	if in == defaultTaskListID {
		return in, nil
	}
	// Heuristic: task list IDs are typically long opaque strings. Avoid extra API
	// calls when the input already looks like an ID.
	if !strings.ContainsAny(in, " \t\r\n") && len(in) >= 16 {
		return in, nil
	}

	type match struct {
		ID    string
		Title string
	}

	var titleMatches []match
	seenTokens := map[string]bool{}
	pageToken := ""
	for {
		if seenTokens[pageToken] {
			return "", fmt.Errorf("pagination loop while listing tasklists (repeated page token %q)", pageToken)
		}
		seenTokens[pageToken] = true

		call := svc.Tasklists.List().MaxResults(1000).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return "", err
		}
		for _, tl := range resp.Items {
			if tl == nil {
				continue
			}
			id := strings.TrimSpace(tl.Id)
			if id != "" && id == in {
				return in, nil
			}
			if id != "" && strings.EqualFold(strings.TrimSpace(tl.Title), in) {
				titleMatches = append(titleMatches, match{ID: id, Title: strings.TrimSpace(tl.Title)})
			}
		}
		next := strings.TrimSpace(resp.NextPageToken)
		if next == "" {
			break
		}
		pageToken = next
	}

	if len(titleMatches) == 1 {
		return titleMatches[0].ID, nil
	}
	if len(titleMatches) > 1 {
		sort.Slice(titleMatches, func(i, j int) bool { return titleMatches[i].ID < titleMatches[j].ID })
		parts := make([]string, 0, len(titleMatches))
		for _, m := range titleMatches {
			label := m.Title
			if label == "" {
				label = "(untitled)"
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", label, m.ID))
		}
		return "", usagef("ambiguous tasklist %q; matches: %s", in, strings.Join(parts, ", "))
	}

	return in, nil
}

// resolveCalendarID resolves a calendar summary/name to an ID (case-insensitive exact match).
// If input is an email-like ID or "primary", it is returned unchanged.
func resolveCalendarID(ctx context.Context, svc *calendar.Service, input string) (string, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return "", nil
	}
	if strings.EqualFold(in, primaryCalendarID) {
		return primaryCalendarID, nil
	}
	// Calendar IDs are almost always email-like; avoid extra API calls when the
	// user already provided an ID.
	if strings.Contains(in, "@") {
		return in, nil
	}

	type match struct {
		ID      string
		Summary string
	}

	var matches []match
	seenTokens := map[string]bool{}
	pageToken := ""
	for {
		if seenTokens[pageToken] {
			return "", fmt.Errorf("pagination loop while listing calendars (repeated page token %q)", pageToken)
		}
		seenTokens[pageToken] = true

		call := svc.CalendarList.List().MaxResults(250).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return "", err
		}
		for _, cal := range resp.Items {
			if cal == nil {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(cal.Summary), in) {
				id := strings.TrimSpace(cal.Id)
				if id != "" {
					matches = append(matches, match{ID: id, Summary: strings.TrimSpace(cal.Summary)})
				}
			}
		}
		next := strings.TrimSpace(resp.NextPageToken)
		if next == "" {
			break
		}
		pageToken = next
	}

	if len(matches) == 1 {
		return matches[0].ID, nil
	}
	if len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
		parts := make([]string, 0, len(matches))
		for _, m := range matches {
			label := m.Summary
			if label == "" {
				label = "(unnamed)"
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", label, m.ID))
		}
		return "", usagef("ambiguous calendar %q; matches: %s", in, strings.Join(parts, ", "))
	}

	return in, nil
}
