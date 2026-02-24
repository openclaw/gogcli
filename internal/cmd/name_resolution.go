package cmd

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/tasks/v1"
)

const (
	defaultTaskListID = "@default"
	primaryCalendarID = "primary"
)

type calendarSelectionKind int

const (
	calendarSelectionName calendarSelectionKind = iota
	calendarSelectionIndex
)

type calendarSelectionInput struct {
	kind  calendarSelectionKind
	raw   string
	lower string
	index int
}

type calendarResolveOptions struct {
	strict        bool
	allowIndex    bool
	allowIDLookup bool
}

type calendarSelectionData struct {
	calendars []*calendar.CalendarListEntry
	byID      map[string]string
	bySummary map[string][]string
}

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

	ids, err := resolveCalendarInputs(ctx, svc, []string{in}, calendarResolveOptions{
		strict: false,
	})
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return in, nil
	}
	return ids[0], nil
}

func resolveCalendarInputs(ctx context.Context, svc *calendar.Service, inputs []string, opts calendarResolveOptions) ([]string, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	data, err := buildCalendarSelectionData(ctx, svc)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	var unrecognized []string

	for _, raw := range inputs {
		input, err := parseCalendarSelectionInput(raw)
		if err != nil {
			return nil, err
		}
		if input.raw == "" {
			continue
		}

		if input.kind == calendarSelectionIndex && opts.allowIndex {
			idx := input.index
			if idx < 1 || idx > len(data.calendars) {
				return nil, usagef("calendar index %d out of range (have %d calendars)", idx, len(data.calendars))
			}
			cal := data.calendars[idx-1]
			if cal == nil || strings.TrimSpace(cal.Id) == "" {
				return nil, usagef("calendar index %d has no id", idx)
			}
			appendUniqueCalendarID(&out, seen, cal.Id)
			continue
		}

		if ids, ok := data.bySummary[input.lower]; ok {
			if len(ids) > 1 {
				return nil, ambiguousCalendarError(input.raw, ids)
			}
			appendUniqueCalendarID(&out, seen, ids[0])
			continue
		}

		if opts.allowIDLookup {
			if id, ok := data.byID[input.lower]; ok {
				appendUniqueCalendarID(&out, seen, id)
				continue
			}
		}

		if !opts.strict {
			appendUniqueCalendarID(&out, seen, input.raw)
			continue
		}
		unrecognized = append(unrecognized, input.raw)
	}

	if len(unrecognized) > 0 {
		return nil, usagef("unrecognized calendar name(s): %s", strings.Join(unrecognized, ", "))
	}

	return out, nil
}

func resolveCalendarIDList(calendars []*calendar.CalendarListEntry) *calendarSelectionData {
	byID := make(map[string]string, len(calendars))
	bySummary := make(map[string][]string, len(calendars))
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
	return &calendarSelectionData{
		calendars: calendars,
		byID:      byID,
		bySummary: bySummary,
	}
}

func buildCalendarSelectionData(ctx context.Context, svc *calendar.Service) (*calendarSelectionData, error) {
	calendars, err := listCalendarList(ctx, svc)
	if err != nil {
		return nil, err
	}
	return resolveCalendarIDList(calendars), nil
}

func parseCalendarSelectionInput(raw string) (calendarSelectionInput, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return calendarSelectionInput{}, nil
	}

	idx, isIndex, err := parseCalendarSelectionIndex(value)
	if err != nil {
		return calendarSelectionInput{}, err
	}
	input := calendarSelectionInput{
		raw:   value,
		lower: strings.ToLower(value),
	}
	if isIndex {
		input.kind = calendarSelectionIndex
		input.index = idx
		return input, nil
	}
	return input, nil
}

func parseCalendarSelectionIndex(value string) (int, bool, error) {
	if !isDigits(value) {
		return 0, false, nil
	}
	index, err := strconv.Atoi(value)
	if err != nil {
		return 0, true, usagef("invalid calendar index: %s", value)
	}
	return index, true, nil
}

func ambiguousCalendarError(input string, ids []string) error {
	if len(ids) == 0 {
		return usagef("ambiguous calendar %q", input)
	}
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)
	return usagef("ambiguous calendar %q; matches: %s", input, strings.Join(sorted, ", "))
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
