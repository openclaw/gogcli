package cmd

import (
	"context"
	"os"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type calendarMutationContext struct {
	u          *ui.UI
	svc        *calendar.Service
	calendarID string
}

type calendarInsertOptions struct {
	sendUpdates         string
	conferenceVersion1  bool
	supportsAttachments bool
}

func newCalendarMutationContext(ctx context.Context, flags *RootFlags, calendarID string) (*calendarMutationContext, error) {
	_, svc, err := requireCalendarService(ctx, flags)
	if err != nil {
		return nil, err
	}
	resolvedCalendarID, err := resolveCalendarID(ctx, svc, calendarID)
	if err != nil {
		return nil, err
	}
	return &calendarMutationContext{
		u:          ui.FromContext(ctx),
		svc:        svc,
		calendarID: resolvedCalendarID,
	}, nil
}

func (m *calendarMutationContext) insertEvent(ctx context.Context, event *calendar.Event, opts calendarInsertOptions) (*calendar.Event, error) {
	call := m.svc.Events.Insert(m.calendarID, event).Context(ctx)
	if opts.sendUpdates != "" {
		call = call.SendUpdates(opts.sendUpdates)
	}
	if opts.conferenceVersion1 {
		call = call.ConferenceDataVersion(1)
	}
	if opts.supportsAttachments {
		call = call.SupportsAttachments(true)
	}
	return call.Do()
}

func (m *calendarMutationContext) patchEvent(ctx context.Context, eventID string, patch *calendar.Event, sendUpdates string) (*calendar.Event, error) {
	call := m.svc.Events.Patch(m.calendarID, eventID, patch).Context(ctx)
	if sendUpdates != "" {
		call = call.SendUpdates(sendUpdates)
	}
	if patch.ConferenceData != nil {
		call = call.ConferenceDataVersion(1)
	}
	return call.Do()
}

func (m *calendarMutationContext) deleteEvent(ctx context.Context, eventID, sendUpdates string) error {
	call := m.svc.Events.Delete(m.calendarID, eventID).Context(ctx)
	if sendUpdates != "" {
		call = call.SendUpdates(sendUpdates)
	}
	return call.Do()
}

func (m *calendarMutationContext) moveEvent(ctx context.Context, eventID, destinationCalendarID, sendUpdates string) (*calendar.Event, error) {
	call := m.svc.Events.Move(m.calendarID, eventID, destinationCalendarID).Context(ctx)
	if sendUpdates != "" {
		call = call.SendUpdates(sendUpdates)
	}
	return call.Do()
}

func (m *calendarMutationContext) writeEvent(ctx context.Context, event *calendar.Event) error {
	tz, loc, _ := getCalendarLocation(ctx, m.svc, m.calendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"event": wrapEventWithDaysWithTimezone(event, tz, loc)})
	}
	printCalendarEventWithTimezone(m.u, event, tz, loc)
	return nil
}
