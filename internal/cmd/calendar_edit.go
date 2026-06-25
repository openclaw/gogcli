package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/ui"
	"github.com/steipete/gogcli/internal/zoom"
)

var errZoomConferenceAlreadyHandled = errors.New("zoom conference already handled")

type CalendarCreateCmd struct {
	CalendarID            string   `arg:"" name:"calendarId" help:"Calendar ID"`
	Summary               string   `name:"summary" help:"Event summary/title"`
	From                  string   `name:"from" help:"Start time (RFC3339)"`
	To                    string   `name:"to" help:"End time (RFC3339)"`
	StartTimezone         string   `name:"start-timezone" aliases:"from-timezone" help:"IANA timezone metadata for --from (e.g., Europe/Rome)"`
	EndTimezone           string   `name:"end-timezone" aliases:"to-timezone" help:"IANA timezone metadata for --to (e.g., America/New_York)"`
	Timezone              string   `name:"timezone" aliases:"tz" help:"IANA timezone metadata applied to both --from and --to (e.g., America/Los_Angeles); mutually exclusive with --start-timezone/--end-timezone"`
	Description           string   `name:"description" help:"Description"`
	Location              string   `name:"location" help:"Location"`
	LocationSearch        string   `name:"location-search" help:"Resolve a Google Places text search and use the best match as event location"`
	PlaceID               string   `name:"place-id" help:"Resolve a Google Places ID and use it as event location"`
	PlaceLanguage         string   `name:"place-language" help:"Places API language code for location lookup"`
	PlaceRegion           string   `name:"place-region" help:"Places API region code for location lookup"`
	Attendees             string   `name:"attendees" help:"Comma-separated attendee emails; modifiers: ;optional, ;resource, ;comment=TEXT"`
	AllDay                bool     `name:"all-day" help:"All-day event (use date-only in --from/--to)"`
	Recurrence            []string `name:"rrule" help:"Recurrence rules (e.g., 'RRULE:FREQ=MONTHLY;BYMONTHDAY=11'). Can be repeated." sep:"none"`
	Reminders             []string `name:"reminder" help:"Custom reminders as method:duration (e.g., popup:30m, email:1d). Can be repeated (max 5)."`
	ColorId               string   `name:"event-color" help:"Event color ID (1-11). Use 'gog calendar colors' to see available colors."`
	Visibility            string   `name:"visibility" help:"Event visibility: default, public, private, confidential"`
	Transparency          string   `name:"transparency" help:"Show as busy (opaque) or free (transparent). Aliases: busy, free"`
	SendUpdates           string   `name:"send-updates" help:"Notification mode: all, externalOnly, none (default: none)"`
	GuestsCanInviteOthers *bool    `name:"guests-can-invite" help:"Allow guests to invite others"`
	GuestsCanModify       *bool    `name:"guests-can-modify" help:"Allow guests to modify event"`
	GuestsCanSeeOthers    *bool    `name:"guests-can-see-others" help:"Allow guests to see other guests"`
	WithMeet              bool     `name:"with-meet" help:"Create a Google Meet video conference for this event"`
	WithZoom              bool     `name:"with-zoom" help:"Create a Zoom video conference for this event"`
	IncludePasswords      bool     `name:"include-passwords" help:"Do not redact Zoom meeting passwords in output" env:"GOG_ZOOM_INCLUDE_PASSWORDS"`
	SourceUrl             string   `name:"source-url" help:"URL where event was created/imported from"`
	SourceTitle           string   `name:"source-title" help:"Title of the source"`
	Attachments           []string `name:"attachment" help:"File attachment URL (can be repeated)"`
	PrivateProps          []string `name:"private-prop" help:"Private extended property (key=value, can be repeated)"`
	SharedProps           []string `name:"shared-prop" help:"Shared extended property (key=value, can be repeated)"`
	EventType             string   `name:"event-type" help:"Event type: default, focus-time, out-of-office, working-location"`
	FocusAutoDecline      string   `name:"focus-auto-decline" help:"Focus Time auto-decline mode: none, all, new"`
	FocusDeclineMessage   string   `name:"focus-decline-message" help:"Focus Time decline message"`
	FocusChatStatus       string   `name:"focus-chat-status" help:"Focus Time chat status: available, doNotDisturb"`
	OOOAutoDecline        string   `name:"ooo-auto-decline" help:"Out of Office auto-decline mode: none, all, new"`
	OOODeclineMessage     string   `name:"ooo-decline-message" help:"Out of Office decline message"`
	WorkingLocationType   string   `name:"working-location-type" help:"Working location type: home, office, custom"`
	WorkingOfficeLabel    string   `name:"working-office-label" help:"Working location office name/label"`
	WorkingBuildingId     string   `name:"working-building-id" help:"Working location building ID"`
	WorkingFloorId        string   `name:"working-floor-id" help:"Working location floor ID"`
	WorkingDeskId         string   `name:"working-desk-id" help:"Working location desk ID"`
	WorkingCustomLabel    string   `name:"working-custom-label" help:"Working location custom label"`
	resolvedPlace         *calendarPlace
}

func calendarCreateFieldsFromKong(kctx *kong.Context) calendarCreateFields {
	return calendarCreateFields{
		Location:       flagProvided(kctx, "location"),
		LocationSearch: flagProvided(kctx, "location-search"),
		PlaceID:        flagProvided(kctx, "place-id"),
		WithMeet:       flagProvided(kctx, "with-meet"),
		WithZoom:       flagProvided(kctx, "with-zoom"),
	}
}

func calendarCreateInputFromCommand(c *CalendarCreateCmd) calendarCreateInput {
	return calendarCreateInput{
		CalendarID:            c.CalendarID,
		Summary:               c.Summary,
		From:                  c.From,
		To:                    c.To,
		StartTimezone:         c.StartTimezone,
		EndTimezone:           c.EndTimezone,
		Timezone:              c.Timezone,
		Description:           c.Description,
		Location:              c.Location,
		Attendees:             c.Attendees,
		AllDay:                c.AllDay,
		Recurrence:            c.Recurrence,
		Reminders:             c.Reminders,
		ColorID:               c.ColorId,
		Visibility:            c.Visibility,
		Transparency:          c.Transparency,
		SendUpdates:           c.SendUpdates,
		GuestsCanInviteOthers: c.GuestsCanInviteOthers,
		GuestsCanModify:       c.GuestsCanModify,
		GuestsCanSeeOthers:    c.GuestsCanSeeOthers,
		WithMeet:              c.WithMeet,
		WithZoom:              c.WithZoom,
		SourceURL:             c.SourceUrl,
		SourceTitle:           c.SourceTitle,
		Attachments:           c.Attachments,
		PrivateProps:          c.PrivateProps,
		SharedProps:           c.SharedProps,
		EventType:             c.EventType,
		FocusAutoDecline:      c.FocusAutoDecline,
		FocusDeclineMessage:   c.FocusDeclineMessage,
		FocusChatStatus:       c.FocusChatStatus,
		OOOAutoDecline:        c.OOOAutoDecline,
		OOODeclineMessage:     c.OOODeclineMessage,
		WorkingLocationType:   c.WorkingLocationType,
		WorkingOfficeLabel:    c.WorkingOfficeLabel,
		WorkingBuildingID:     c.WorkingBuildingId,
		WorkingFloorID:        c.WorkingFloorId,
		WorkingDeskID:         c.WorkingDeskId,
		WorkingCustomLabel:    c.WorkingCustomLabel,
		LocationSearch:        c.LocationSearch,
		PlaceID:               c.PlaceID,
		PlaceLanguage:         c.PlaceLanguage,
		PlaceRegion:           c.PlaceRegion,
		ResolvedPlace:         c.resolvedPlace,
	}
}

func (c *CalendarCreateCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	ctx = withZoomIncludePasswords(ctx, c.IncludePasswords)
	fields := calendarCreateFieldsFromKong(kctx)
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	plan, err := buildCalendarCreatePlan(store, calendarCreateInputFromCommand(c), fields)
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "calendar.create", plan.dryRunRequest()); dryRunErr != nil {
		return dryRunErr
	}
	if plan.PlaceLookup != nil {
		if placeErr := c.resolvePlace(ctx, fields); placeErr != nil {
			return placeErr
		}
		plan, err = buildCalendarCreatePlan(store, calendarCreateInputFromCommand(c), fields)
		if err != nil {
			return err
		}
	}

	mutation, err := newCalendarMutationContext(ctx, flags, plan.CalendarID)
	if err != nil {
		return err
	}

	var zoomMeeting *zoom.Meeting
	if plan.WithZoom {
		zoomMeeting, err = createZoomMeetingForEvent(ctx, plan.Event)
		if err != nil {
			return err
		}
		plan.Event.Description = applyZoomDescriptionBlock(plan.Event.Description, buildZoomDescriptionBlock(zoomMeeting))
	}

	created, err := mutation.insertEvent(ctx, plan.Event, calendarInsertOptions{
		sendUpdates:         plan.SendUpdates,
		conferenceVersion1:  plan.WithMeet,
		supportsAttachments: len(plan.Event.Attachments) > 0,
	})
	if err != nil {
		if zoomMeeting != nil {
			_ = cancelZoomMeeting(ctx, zoomMeetingID(zoomMeeting), "delete")
		}
		return err
	}
	return mutation.writeEvent(ctx, created)
}

type CalendarUpdateCmd struct {
	CalendarID            string   `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID               string   `arg:"" name:"eventId" help:"Event ID"`
	Summary               string   `name:"summary" help:"New summary/title (set empty to clear)"`
	From                  string   `name:"from" help:"New start time (RFC3339; set empty to clear)"`
	To                    string   `name:"to" help:"New end time (RFC3339; set empty to clear)"`
	StartTimezone         string   `name:"start-timezone" aliases:"from-timezone" help:"IANA timezone metadata for --from (e.g., Europe/Rome)"`
	EndTimezone           string   `name:"end-timezone" aliases:"to-timezone" help:"IANA timezone metadata for --to (e.g., America/New_York)"`
	Description           string   `name:"description" help:"New description (set empty to clear)"`
	Location              string   `name:"location" help:"New location (set empty to clear)"`
	LocationSearch        string   `name:"location-search" help:"Resolve a Google Places text search and use the best match as event location"`
	PlaceID               string   `name:"place-id" help:"Resolve a Google Places ID and use it as event location"`
	PlaceLanguage         string   `name:"place-language" help:"Places API language code for location lookup"`
	PlaceRegion           string   `name:"place-region" help:"Places API region code for location lookup"`
	Attendees             string   `name:"attendees" help:"Comma-separated attendee emails (replaces all; set empty to clear); modifiers: ;optional, ;resource, ;comment=TEXT"`
	AddAttendee           string   `name:"add-attendee" help:"Comma-separated attendee emails to add (preserves existing attendees); modifiers: ;optional, ;resource, ;comment=TEXT"`
	Attachments           []string `name:"attachment" help:"File attachment URL (can be repeated; replaces all; set empty to clear)"`
	AllDay                bool     `name:"all-day" help:"All-day event (use date-only in --from/--to)"`
	Recurrence            []string `name:"rrule" help:"Recurrence rules (e.g., 'RRULE:FREQ=MONTHLY;BYMONTHDAY=11'). Can be repeated. Set empty to clear." sep:"none"`
	Reminders             []string `name:"reminder" help:"Custom reminders as method:duration (e.g., popup:30m, email:1d). Can be repeated (max 5). Set empty to clear."`
	ColorId               string   `name:"event-color" help:"Event color ID (1-11, or empty to clear)"`
	Visibility            string   `name:"visibility" help:"Event visibility: default, public, private, confidential"`
	Transparency          string   `name:"transparency" help:"Show as busy (opaque) or free (transparent). Aliases: busy, free"`
	GuestsCanInviteOthers *bool    `name:"guests-can-invite" help:"Allow guests to invite others"`
	GuestsCanModify       *bool    `name:"guests-can-modify" help:"Allow guests to modify event"`
	GuestsCanSeeOthers    *bool    `name:"guests-can-see-others" help:"Allow guests to see other guests"`
	WithMeet              bool     `name:"with-meet" help:"Create a Google Meet video conference for this event"`
	RegenerateMeet        bool     `name:"regenerate-meet" help:"Replace the event's Google Meet video conference"`
	WithZoom              bool     `name:"with-zoom" help:"Create a Zoom video conference for this event"`
	RegenerateZoom        bool     `name:"regenerate-zoom" help:"Replace the event's Zoom video conference"`
	RemoveZoom            bool     `name:"remove-zoom" help:"Remove the event's Zoom video conference"`
	IncludePasswords      bool     `name:"include-passwords" help:"Do not redact Zoom meeting passwords in output" env:"GOG_ZOOM_INCLUDE_PASSWORDS"`
	Scope                 string   `name:"scope" help:"For recurring events: single, future, all" default:"all"`
	OriginalStartTime     string   `name:"original-start" help:"Original start time of instance (required for scope=single,future)"`
	PrivateProps          []string `name:"private-prop" help:"Private extended property (key=value, can be repeated)"`
	SharedProps           []string `name:"shared-prop" help:"Shared extended property (key=value, can be repeated)"`
	EventType             string   `name:"event-type" help:"Event type: default, focus-time, out-of-office, working-location"`
	FocusAutoDecline      string   `name:"focus-auto-decline" help:"Focus Time auto-decline mode: none, all, new"`
	FocusDeclineMessage   string   `name:"focus-decline-message" help:"Focus Time decline message (set empty to clear)"`
	FocusChatStatus       string   `name:"focus-chat-status" help:"Focus Time chat status: available, doNotDisturb"`
	OOOAutoDecline        string   `name:"ooo-auto-decline" help:"Out of Office auto-decline mode: none, all, new"`
	OOODeclineMessage     string   `name:"ooo-decline-message" help:"Out of Office decline message (set empty to clear)"`
	WorkingLocationType   string   `name:"working-location-type" help:"Working location type: home, office, custom"`
	WorkingOfficeLabel    string   `name:"working-office-label" help:"Working location office name/label"`
	WorkingBuildingId     string   `name:"working-building-id" help:"Working location building ID"`
	WorkingFloorId        string   `name:"working-floor-id" help:"Working location floor ID"`
	WorkingDeskId         string   `name:"working-desk-id" help:"Working location desk ID"`
	WorkingCustomLabel    string   `name:"working-custom-label" help:"Working location custom label"`
	SendUpdates           string   `name:"send-updates" help:"Notification mode: all, externalOnly, none (default: none)"`
	resolvedPlace         *calendarPlace
	createdZoomMeetingID  string
}

func calendarUpdateFieldsFromKong(kctx *kong.Context) calendarUpdateFields {
	return calendarUpdateFields{
		Summary:               flagProvided(kctx, "summary"),
		Description:           flagProvided(kctx, "description"),
		Location:              flagProvided(kctx, "location"),
		LocationSearch:        flagProvided(kctx, "location-search"),
		PlaceID:               flagProvided(kctx, "place-id"),
		From:                  flagProvided(kctx, "from"),
		To:                    flagProvided(kctx, "to"),
		StartTimezone:         flagProvided(kctx, "start-timezone"),
		EndTimezone:           flagProvided(kctx, "end-timezone"),
		AllDay:                flagProvided(kctx, "all-day"),
		Attendees:             flagProvided(kctx, "attendees"),
		AddAttendee:           flagProvided(kctx, "add-attendee"),
		Attachments:           flagProvided(kctx, "attachment"),
		Recurrence:            flagProvided(kctx, "rrule"),
		Reminders:             flagProvided(kctx, "reminder"),
		ColorID:               flagProvided(kctx, "event-color"),
		Visibility:            flagProvided(kctx, "visibility"),
		Transparency:          flagProvided(kctx, "transparency"),
		GuestsCanInviteOthers: flagProvided(kctx, "guests-can-invite"),
		GuestsCanModify:       flagProvided(kctx, "guests-can-modify"),
		GuestsCanSeeOthers:    flagProvided(kctx, "guests-can-see-others"),
		WithMeet:              flagProvided(kctx, "with-meet"),
		RegenerateMeet:        flagProvided(kctx, "regenerate-meet"),
		WithZoom:              flagProvided(kctx, "with-zoom"),
		RegenerateZoom:        flagProvided(kctx, "regenerate-zoom"),
		RemoveZoom:            flagProvided(kctx, "remove-zoom"),
		PrivateProps:          flagProvided(kctx, "private-prop"),
		SharedProps:           flagProvided(kctx, "shared-prop"),
		FocusAutoDecline:      flagProvided(kctx, "focus-auto-decline"),
		FocusDeclineMessage:   flagProvided(kctx, "focus-decline-message"),
		FocusChatStatus:       flagProvided(kctx, "focus-chat-status"),
		OOOAutoDecline:        flagProvided(kctx, "ooo-auto-decline"),
		OOODeclineMessage:     flagProvided(kctx, "ooo-decline-message"),
		WorkingLocationType:   flagProvided(kctx, "working-location-type"),
		WorkingOfficeLabel:    flagProvided(kctx, "working-office-label"),
		WorkingBuildingID:     flagProvided(kctx, "working-building-id"),
		WorkingFloorID:        flagProvided(kctx, "working-floor-id"),
		WorkingDeskID:         flagProvided(kctx, "working-desk-id"),
		WorkingCustomLabel:    flagProvided(kctx, "working-custom-label"),
	}
}

func calendarUpdateInputFromCommand(c *CalendarUpdateCmd) calendarUpdateInput {
	return calendarUpdateInput{
		CalendarID:            c.CalendarID,
		EventID:               c.EventID,
		Summary:               c.Summary,
		From:                  c.From,
		To:                    c.To,
		StartTimezone:         c.StartTimezone,
		EndTimezone:           c.EndTimezone,
		Description:           c.Description,
		Location:              c.Location,
		LocationSearch:        c.LocationSearch,
		PlaceID:               c.PlaceID,
		PlaceLanguage:         c.PlaceLanguage,
		PlaceRegion:           c.PlaceRegion,
		Attendees:             c.Attendees,
		AddAttendee:           c.AddAttendee,
		Attachments:           c.Attachments,
		AllDay:                c.AllDay,
		Recurrence:            c.Recurrence,
		Reminders:             c.Reminders,
		ColorID:               c.ColorId,
		Visibility:            c.Visibility,
		Transparency:          c.Transparency,
		GuestsCanInviteOthers: c.GuestsCanInviteOthers,
		GuestsCanModify:       c.GuestsCanModify,
		GuestsCanSeeOthers:    c.GuestsCanSeeOthers,
		Scope:                 c.Scope,
		OriginalStartTime:     c.OriginalStartTime,
		PrivateProps:          c.PrivateProps,
		SharedProps:           c.SharedProps,
		EventType:             c.EventType,
		FocusAutoDecline:      c.FocusAutoDecline,
		FocusDeclineMessage:   c.FocusDeclineMessage,
		FocusChatStatus:       c.FocusChatStatus,
		OOOAutoDecline:        c.OOOAutoDecline,
		OOODeclineMessage:     c.OOODeclineMessage,
		WorkingLocationType:   c.WorkingLocationType,
		WorkingOfficeLabel:    c.WorkingOfficeLabel,
		WorkingBuildingID:     c.WorkingBuildingId,
		WorkingFloorID:        c.WorkingFloorId,
		WorkingDeskID:         c.WorkingDeskId,
		WorkingCustomLabel:    c.WorkingCustomLabel,
		SendUpdates:           c.SendUpdates,
		ResolvedPlace:         c.resolvedPlace,
	}
}

func (c *CalendarUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	ctx = withZoomIncludePasswords(ctx, c.IncludePasswords)
	fields := calendarUpdateFieldsFromKong(kctx)
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	plan, err := buildCalendarUpdatePlan(store, calendarUpdateInputFromCommand(c), fields)
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "calendar.update", plan.dryRunRequest()); dryRunErr != nil {
		return dryRunErr
	}
	if plan.PlaceLookup != nil {
		if placeErr := c.resolvePlace(ctx, fields); placeErr != nil {
			return placeErr
		}
		plan, err = buildCalendarUpdatePlan(store, calendarUpdateInputFromCommand(c), fields)
		if err != nil {
			return err
		}
	}

	mutation, err := newCalendarMutationContext(ctx, flags, plan.CalendarID)
	if err != nil {
		return err
	}

	patch := plan.Patch
	changed := plan.Changed

	// For --add-attendee, fetch current event to preserve existing attendees with metadata.
	if plan.WantsAddAttendee {
		existing, getErr := mutation.svc.Events.Get(mutation.calendarID, plan.EventID).Context(ctx).Do()
		if getErr != nil {
			return fmt.Errorf("failed to fetch current event: %w", getErr)
		}
		merged, attendeesChanged := mergeAttendeesWithChange(existing.Attendees, plan.AddAttendee)
		if attendeesChanged {
			patch.Attendees = merged
			changed = true
		}
		if !changed {
			return usage("no updates provided")
		}
	}

	if fields.zoomMutation() {
		var zoomErr error
		patch, _, zoomErr = c.prepareZoomConferencePatch(ctx, mutation, plan.EventID, plan.Scope, plan.OriginalStartTime, patch, changed, fields)
		if errors.Is(zoomErr, errZoomConferenceAlreadyHandled) {
			return nil
		}
		if zoomErr != nil {
			return zoomErr
		}
	}

	if patch.ConferenceData != nil && !fields.RegenerateMeet && !fields.zoomMutation() && patchOnlyConferenceData(patch) {
		resolution, resolveErr := resolveRecurringScopeResolution(ctx, mutation.svc, mutation.calendarID, plan.EventID, plan.Scope, plan.OriginalStartTime)
		if resolveErr != nil {
			return resolveErr
		}
		existing, getErr := mutation.svc.Events.Get(mutation.calendarID, resolution.TargetEventID).Context(ctx).Do()
		if getErr != nil {
			return fmt.Errorf("failed to fetch current event for conference data: %w", getErr)
		}
		if eventHasConferenceLink(existing) {
			return mutation.writeEvent(ctx, existing)
		}
	}

	targetEventID, parentRecurrence, err := applyUpdateScope(ctx, mutation.svc, mutation.calendarID, plan.EventID, plan.Scope, plan.OriginalStartTime, patch)
	if err != nil {
		return err
	}
	if patch.ConferenceData != nil && !fields.RegenerateMeet && !fields.zoomMutation() {
		existing, getErr := mutation.svc.Events.Get(mutation.calendarID, targetEventID).Context(ctx).Do()
		if getErr != nil {
			return fmt.Errorf("failed to fetch current event for conference data: %w", getErr)
		}
		if eventHasConferenceLink(existing) {
			onlyConferenceData := patchOnlyConferenceData(patch)
			patch.ConferenceData = nil
			if onlyConferenceData {
				return mutation.writeEvent(ctx, existing)
			}
		}
	}
	if plan.RecurrenceProvided {
		if enrichErr := ensureRecurringPatchDateTimes(ctx, mutation.svc, mutation.calendarID, targetEventID, patch); enrichErr != nil {
			return enrichErr
		}
	}

	updated, err := mutation.patchEvent(ctx, targetEventID, patch, plan.SendUpdates)
	if err != nil {
		if c.createdZoomMeetingID != "" {
			_ = cancelZoomMeeting(ctx, c.createdZoomMeetingID, "delete")
		}
		return err
	}
	if plan.Scope == scopeFuture {
		if err := truncateParentRecurrence(ctx, mutation.svc, mutation.calendarID, plan.EventID, parentRecurrence, plan.OriginalStartTime, plan.SendUpdates); err != nil {
			return err
		}
	}
	return mutation.writeEvent(ctx, updated)
}

func ensureRecurringPatchDateTimes(ctx context.Context, svc *calendar.Service, calendarID, eventID string, patch *calendar.Event) error {
	if len(patch.Recurrence) == 0 {
		return nil
	}

	patch.Start = normalizeRecurringPatchDateTime(patch.Start, nil)
	patch.End = normalizeRecurringPatchDateTime(patch.End, nil)
	if !recurringPatchDateTimeNeedsFetch(patch.Start) && !recurringPatchDateTimeNeedsFetch(patch.End) {
		return nil
	}

	current, err := svc.Events.Get(calendarID, eventID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to fetch current event for recurrence timezone: %w", err)
	}

	patch.Start = normalizeRecurringPatchDateTime(patch.Start, current.Start)
	patch.End = normalizeRecurringPatchDateTime(patch.End, current.End)
	return nil
}

func (c *CalendarUpdateCmd) prepareZoomConferencePatch(
	ctx context.Context,
	mutation *calendarMutationContext,
	eventID, scope, originalStartTime string,
	patch *calendar.Event,
	changed bool,
	fields calendarUpdateFields,
) (*calendar.Event, bool, error) {
	resolution, err := resolveRecurringScopeResolution(ctx, mutation.svc, mutation.calendarID, eventID, scope, originalStartTime)
	if err != nil {
		return patch, changed, err
	}
	existing, err := mutation.svc.Events.Get(mutation.calendarID, resolution.TargetEventID).Context(ctx).Do()
	if err != nil {
		return patch, changed, fmt.Errorf("failed to fetch current event for conference data: %w", err)
	}

	switch {
	case fields.WithZoom:
		provider := eventConferenceProvider(existing)
		switch provider {
		case conferenceProviderZoom:
			if patchOnlyConferenceData(patch) || patchEffectivelyEmpty(patch) {
				if err := mutation.writeEvent(ctx, existing); err != nil {
					return patch, false, err
				}
				return patch, false, errZoomConferenceAlreadyHandled
			}
			return patch, changed, nil
		case conferenceProviderMeet:
			return patch, changed, usage("event already has a Meet conference; use --remove-meet first, then --with-zoom")
		case "other":
			return patch, changed, usage("event already has a conference; remove it before using --with-zoom")
		}
		meeting, createErr := createZoomMeetingForEvent(ctx, mergeEventPatch(existing, patch))
		if createErr != nil {
			return patch, changed, createErr
		}
		c.createdZoomMeetingID = zoomMeetingID(meeting)
		patch.Description = applyZoomDescriptionBlock(descriptionForPatch(existing, patch), buildZoomDescriptionBlock(meeting))
		return patch, true, nil

	case fields.RegenerateZoom:
		if meetingID, ok := extractZoomMeetingID(existing); ok {
			if err := cancelZoomMeeting(ctx, meetingID, "regenerate"); err != nil && !errors.Is(err, zoom.ErrMeetingNotFound) {
				return patch, changed, err
			}
		} else {
			warnUnparseableZoomMeeting(ctx, mutation.u)
		}
		meeting, createErr := createZoomMeetingForEvent(ctx, mergeEventPatch(existing, patch))
		if createErr != nil {
			return patch, changed, createErr
		}
		c.createdZoomMeetingID = zoomMeetingID(meeting)
		patch.Description = applyZoomDescriptionBlock(descriptionForPatch(existing, patch), buildZoomDescriptionBlock(meeting))
		return patch, true, nil

	case fields.RemoveZoom:
		if meetingID, ok := extractZoomMeetingID(existing); ok {
			if err := cancelZoomMeeting(ctx, meetingID, "delete"); err != nil && !errors.Is(err, zoom.ErrMeetingNotFound) {
				if mutation.u != nil {
					mutation.u.Err().Linef("warning\tfailed to delete Zoom meeting %s: %v", meetingID, err)
				}
			}
		} else {
			warnUnparseableZoomMeeting(ctx, mutation.u)
		}
		// Strip the gog-managed Zoom block from the description. Also clear
		// any legacy ConferenceData (events created by the Zoom for Google
		// Workspace add-on, or future re-introduction of the Marketplace
		// add-on path) so --remove-zoom is idempotent across both shapes.
		patch.Description = applyZoomDescriptionBlock(descriptionForPatch(existing, patch), "")
		if strings.TrimSpace(patch.Description) == "" {
			patch.ForceSendFields = appendForceSendField(patch.ForceSendFields, "Description")
		}
		if existing != nil && existing.ConferenceData != nil && isZoomConferenceData(existing.ConferenceData) {
			patch.ConferenceData = nil
			patch.NullFields = append(patch.NullFields, "ConferenceData")
		}
		return patch, true, nil
	}
	return patch, changed, nil
}

func applyUpdateScope(ctx context.Context, svc *calendar.Service, calendarID, eventID, scope, originalStartTime string, patch *calendar.Event) (string, []string, error) {
	resolution, err := resolveRecurringScopeResolution(ctx, svc, calendarID, eventID, scope, originalStartTime)
	if err != nil {
		return "", nil, err
	}

	if scope == scopeFuture {
		parentRecurrence := resolution.ParentRecurrence
		recurrenceOverride := len(patch.Recurrence) > 0
		if !recurrenceOverride {
			for _, field := range patch.ForceSendFields {
				if field == "Recurrence" {
					recurrenceOverride = true
					break
				}
			}
		}
		if !recurrenceOverride {
			patch.Recurrence = parentRecurrence
		}
	}

	return resolution.TargetEventID, resolution.ParentRecurrence, nil
}

func truncateParentRecurrence(ctx context.Context, svc *calendar.Service, calendarID, eventID string, parentRecurrence []string, originalStartTime, sendUpdates string) error {
	truncated, err := truncateRecurrence(parentRecurrence, originalStartTime)
	if err != nil {
		return err
	}
	call := svc.Events.Patch(calendarID, eventID, &calendar.Event{Recurrence: truncated}).Context(ctx)
	if sendUpdates != "" {
		call = call.SendUpdates(sendUpdates)
	}
	_, err = call.Do()
	return err
}

func resolveRecurringScope(scopeValue, originalStartTime string) (string, error) {
	scope := strings.TrimSpace(strings.ToLower(scopeValue))
	if scope == "" {
		scope = scopeAll
	}
	switch scope {
	case scopeSingle, scopeFuture:
		if strings.TrimSpace(originalStartTime) == "" {
			return "", usage(fmt.Sprintf("--original-start required when --scope=%s", scope))
		}
	case scopeAll:
	default:
		return "", usagef("invalid scope: %q (must be single, future, or all)", scope)
	}
	return scope, nil
}

type CalendarDeleteCmd struct {
	CalendarID        string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID           string `arg:"" name:"eventId" help:"Event ID"`
	Scope             string `name:"scope" help:"For recurring events: single, future, all" default:"all"`
	OriginalStartTime string `name:"original-start" help:"Original start time of instance (required for scope=single,future)"`
	SendUpdates       string `name:"send-updates" help:"Notification mode: all, externalOnly, none (default: none)"`
}

func (c *CalendarDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	calendarID, err := prepareCalendarID(store, c.CalendarID, false)
	if err != nil {
		return err
	}
	eventID := normalizeCalendarEventID(c.EventID)
	if eventID == "" {
		return usage("empty eventId")
	}

	scope, err := resolveRecurringScope(c.Scope, c.OriginalStartTime)
	if err != nil {
		return err
	}

	sendUpdates, err := validateSendUpdates(c.SendUpdates)
	if err != nil {
		return err
	}

	confirmMessage := fmt.Sprintf("delete event %s from calendar %s", eventID, calendarID)
	if scope == scopeSingle {
		confirmMessage = fmt.Sprintf("delete event %s (instance start %s) from calendar %s", eventID, c.OriginalStartTime, calendarID)
	}
	if scope == scopeFuture {
		confirmMessage = fmt.Sprintf("delete event %s (instance start %s) and all following from calendar %s", eventID, c.OriginalStartTime, calendarID)
	}
	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "calendar.delete", map[string]any{
		"calendar_id":    calendarID,
		"event_id":       eventID,
		"scope":          scope,
		"original_start": c.OriginalStartTime,
		"send_updates":   sendUpdates,
	}, confirmMessage); confirmErr != nil {
		return confirmErr
	}

	mutation, err := newCalendarMutationContext(ctx, flags, calendarID)
	if err != nil {
		return err
	}

	resolution, err := resolveRecurringScopeResolution(ctx, mutation.svc, mutation.calendarID, eventID, scope, c.OriginalStartTime)
	if err != nil {
		return err
	}

	if err := mutation.deleteEvent(ctx, resolution.TargetEventID, sendUpdates); err != nil {
		return err
	}
	if scope == scopeFuture {
		truncated, truncateErr := truncateRecurrence(resolution.ParentRecurrence, c.OriginalStartTime)
		if truncateErr != nil {
			return truncateErr
		}
		_, patchErr := mutation.patchEvent(ctx, resolution.ParentEventID, &calendar.Event{Recurrence: truncated}, sendUpdates)
		if patchErr != nil {
			return patchErr
		}
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("calendarId", mutation.calendarID),
		kv("eventId", resolution.TargetEventID),
	)
}
