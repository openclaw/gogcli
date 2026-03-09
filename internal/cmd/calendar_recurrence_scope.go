package cmd

import (
	"context"

	"google.golang.org/api/calendar/v3"
)

type recurringScopeResolution struct {
	TargetEventID    string
	ParentEventID    string
	ParentRecurrence []string
}

func resolveRecurringScopeResolution(ctx context.Context, svc *calendar.Service, calendarID, eventID, scope, originalStartTime string) (recurringScopeResolution, error) {
	resolution := recurringScopeResolution{
		TargetEventID: eventID,
		ParentEventID: eventID,
	}
	recurringEventID := eventID

	if scope == scopeFuture {
		parentID, recurrence, err := resolveRecurringParentEvent(ctx, svc, calendarID, eventID)
		if err != nil {
			return recurringScopeResolution{}, err
		}
		resolution.ParentEventID = parentID
		resolution.ParentRecurrence = recurrence
		recurringEventID = parentID
	}

	if scope == scopeSingle || scope == scopeFuture {
		if scope == scopeSingle {
			var err error
			recurringEventID, err = resolveRecurringSeriesID(ctx, svc, calendarID, eventID)
			if err != nil {
				return recurringScopeResolution{}, err
			}
		}
		instanceID, err := resolveRecurringInstanceID(ctx, svc, calendarID, recurringEventID, originalStartTime)
		if err != nil {
			return recurringScopeResolution{}, err
		}
		resolution.TargetEventID = instanceID
	}

	return resolution, nil
}
