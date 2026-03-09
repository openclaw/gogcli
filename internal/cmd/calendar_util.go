package cmd

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/config"
)

func isAllDayEvent(e *calendar.Event) bool {
	return e != nil && e.Start != nil && e.Start.Date != ""
}

// prepareCalendarID resolves aliases before any API-backed calendar lookup.
// When defaultPrimary is true, empty input becomes the primary calendar.
func prepareCalendarID(calendarID string, defaultPrimary bool) (string, error) {
	calendarID = strings.TrimSpace(calendarID)
	if calendarID == "" {
		if defaultPrimary {
			return primaryCalendarID, nil
		}
		return "", usage("empty calendarId")
	}

	resolved, err := config.ResolveCalendarID(calendarID)
	if err != nil {
		return "", err
	}

	return resolved, nil
}

func resolveCalendarSelector(ctx context.Context, svc *calendar.Service, calendarID string, defaultPrimary bool) (string, error) {
	prepared, err := prepareCalendarID(calendarID, defaultPrimary)
	if err != nil {
		return "", err
	}
	return resolveCalendarID(ctx, svc, prepared)
}

func prepareCalendarIDs(inputs []string) ([]string, error) {
	prepared := make([]string, 0, len(inputs))
	for _, input := range inputs {
		resolved, err := prepareCalendarID(input, false)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, resolved)
	}
	return prepared, nil
}

func collectCalendarInputs(cal []string, calendars string) []string {
	inputs := append([]string{}, cal...)
	if strings.TrimSpace(calendars) != "" {
		inputs = append(inputs, splitCSV(calendars)...)
	}
	return inputs
}

func resolveAllCalendarIDs(ctx context.Context, svc *calendar.Service) ([]string, error) {
	calendars, err := listCalendarList(ctx, svc)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(calendars))
	seen := make(map[string]struct{}, len(calendars))
	for _, cal := range calendars {
		if cal == nil {
			continue
		}
		id := strings.TrimSpace(cal.Id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func resolveSelectedCalendarIDs(ctx context.Context, svc *calendar.Service, cal []string, calendars string, all, defaultPrimary bool) ([]string, error) {
	inputs := collectCalendarInputs(cal, calendars)
	if all {
		if len(inputs) > 0 {
			return nil, usage("--cal/--calendars not allowed with --all")
		}
		return resolveAllCalendarIDs(ctx, svc)
	}

	if len(inputs) == 0 && defaultPrimary {
		inputs = []string{primaryCalendarID}
	}
	if len(inputs) == 0 {
		return nil, usage("no calendar IDs provided")
	}

	prepared, err := prepareCalendarIDs(inputs)
	if err != nil {
		return nil, err
	}

	needsLookup := false
	hasIndex := false
	for _, input := range prepared {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if strings.EqualFold(input, primaryCalendarID) || strings.Contains(input, "@") {
			continue
		}
		if isDigits(input) {
			hasIndex = true
		}
		needsLookup = true
		break
	}
	if !needsLookup {
		return dedupePreparedCalendarIDs(prepared), nil
	}

	ids, err := resolveCalendarInputs(ctx, svc, prepared, calendarResolveOptions{
		strict:        true,
		allowIndex:    true,
		allowIDLookup: true,
	})
	if err == nil {
		return ids, nil
	}
	var exitErr *ExitError
	if !hasIndex && !errors.As(err, &exitErr) && isGoogleNotFound(err) {
		return dedupePreparedCalendarIDs(prepared), nil
	}
	return nil, err
}

func dedupePreparedCalendarIDs(inputs []string) []string {
	out := make([]string, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		if strings.EqualFold(input, primaryCalendarID) {
			input = primaryCalendarID
		}
		if input == "" {
			continue
		}
		if _, ok := seen[input]; ok {
			continue
		}
		seen[input] = struct{}{}
		out = append(out, input)
	}
	return out
}
