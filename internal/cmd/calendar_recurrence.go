package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

func resolveRecurringSeriesID(ctx context.Context, svc *calendar.Service, calendarID, eventID string) (string, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return "", fmt.Errorf("event ID required")
	}

	event, err := svc.Events.Get(calendarID, eventID).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	if recurringEventID := strings.TrimSpace(event.RecurringEventId); recurringEventID != "" {
		return recurringEventID, nil
	}
	if len(event.Recurrence) > 0 {
		if resolvedID := strings.TrimSpace(event.Id); resolvedID != "" {
			return resolvedID, nil
		}
		return eventID, nil
	}
	return "", fmt.Errorf("event %s is not a recurring event", eventID)
}

func resolveRecurringParentEvent(ctx context.Context, svc *calendar.Service, calendarID, eventID string) (string, []string, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return "", nil, fmt.Errorf("event ID required")
	}

	parentID, err := resolveRecurringSeriesID(ctx, svc, calendarID, eventID)
	if err != nil {
		return "", nil, err
	}
	parent, err := svc.Events.Get(calendarID, parentID).Context(ctx).Do()
	if err != nil {
		return "", nil, err
	}
	if len(parent.Recurrence) == 0 {
		return "", nil, fmt.Errorf("event %s is not a recurring event", eventID)
	}
	if resolvedID := strings.TrimSpace(parent.Id); resolvedID != "" {
		parentID = resolvedID
	}
	return parentID, parent.Recurrence, nil
}

func resolveRecurringInstanceID(ctx context.Context, svc *calendar.Service, calendarID, recurringEventID, originalStart string) (string, error) {
	originalStart = strings.TrimSpace(originalStart)
	if originalStart == "" {
		return "", fmt.Errorf("original start time required")
	}

	timeMin, timeMax, err := originalStartRange(originalStart)
	if err != nil {
		return "", err
	}

	call := svc.Events.Instances(calendarID, recurringEventID).
		ShowDeleted(false).
		TimeMin(timeMin).
		TimeMax(timeMax)

	for {
		resp, err := call.Context(ctx).Do()
		if err != nil {
			return "", err
		}
		for _, item := range resp.Items {
			if matchesOriginalStart(item, originalStart) {
				return item.Id, nil
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		call = svc.Events.Instances(calendarID, recurringEventID).
			ShowDeleted(false).
			TimeMin(timeMin).
			TimeMax(timeMax).
			PageToken(resp.NextPageToken)
	}

	return "", fmt.Errorf("no instance found for original start %q", originalStart)
}

func matchesOriginalStart(event *calendar.Event, originalStart string) bool {
	if event == nil {
		return false
	}
	originalStart = strings.TrimSpace(originalStart)
	if event.OriginalStartTime != nil {
		if event.OriginalStartTime.DateTime == originalStart || event.OriginalStartTime.Date == originalStart {
			return true
		}
	}
	if event.Start != nil {
		if event.Start.DateTime == originalStart || event.Start.Date == originalStart {
			return true
		}
	}
	return false
}

func originalStartRange(originalStart string) (string, string, error) {
	if strings.Contains(originalStart, "T") {
		parsed, err := time.Parse(time.RFC3339, originalStart)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339Nano, originalStart)
		}
		if err != nil {
			return "", "", fmt.Errorf("invalid original start time %q", originalStart)
		}
		return parsed.Format(time.RFC3339), parsed.Add(time.Minute).Format(time.RFC3339), nil
	}
	parsed, err := time.Parse("2006-01-02", originalStart)
	if err != nil {
		return "", "", fmt.Errorf("invalid original start date %q", originalStart)
	}
	return parsed.Format(time.RFC3339), parsed.Add(24 * time.Hour).Format(time.RFC3339), nil
}

func truncateRecurrence(rules []string, originalStart string) ([]string, error) {
	if len(rules) == 0 {
		return nil, fmt.Errorf("recurrence rules missing")
	}
	untilValue, err := recurrenceUntil(originalStart)
	if err != nil {
		return nil, err
	}

	updated := make([]string, 0, len(rules))
	foundRule := false
	for _, rule := range rules {
		trimmed := strings.TrimSpace(rule)
		upper := strings.ToUpper(trimmed)
		if !strings.HasPrefix(upper, "RRULE") {
			updated = append(updated, trimmed)
			continue
		}
		foundRule = true
		body := strings.TrimPrefix(trimmed, "RRULE:")
		if body == trimmed {
			body = strings.TrimPrefix(trimmed, "RRULE")
			body = strings.TrimPrefix(body, ":")
		}
		parts := strings.Split(body, ";")
		filtered := make([]string, 0, len(parts)+1)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			upperPart := strings.ToUpper(part)
			if strings.HasPrefix(upperPart, "UNTIL=") || strings.HasPrefix(upperPart, "COUNT=") {
				continue
			}
			filtered = append(filtered, part)
		}
		filtered = append(filtered, "UNTIL="+untilValue)
		updated = append(updated, "RRULE:"+strings.Join(filtered, ";"))
	}
	if !foundRule {
		return nil, fmt.Errorf("recurrence has no RRULE")
	}
	return updated, nil
}

func recurrenceUntil(originalStart string) (string, error) {
	originalStart = strings.TrimSpace(originalStart)
	if originalStart == "" {
		return "", fmt.Errorf("original start time required")
	}
	if strings.Contains(originalStart, "T") {
		parsed, err := time.Parse(time.RFC3339, originalStart)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339Nano, originalStart)
		}
		if err != nil {
			return "", fmt.Errorf("invalid original start time %q", originalStart)
		}
		until := parsed.Add(-time.Second).UTC()
		return until.Format("20060102T150405Z"), nil
	}
	parsed, err := time.Parse("2006-01-02", originalStart)
	if err != nil {
		return "", fmt.Errorf("invalid original start date %q", originalStart)
	}
	until := parsed.AddDate(0, 0, -1)
	return until.Format("20060102"), nil
}
