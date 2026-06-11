package cmd

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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
	Description           string   `name:"description" help:"Description"`
	Location              string   `name:"location" help:"Location"`
	LocationSearch        string   `name:"location-search" help:"Resolve a Google Places text search and use the best match as event location"`
	PlaceID               string   `name:"place-id" help:"Resolve a Google Places ID and use it as event location"`
	PlaceLanguage         string   `name:"place-language" help:"Places API language code for location lookup"`
	PlaceRegion           string   `name:"place-region" help:"Places API region code for location lookup"`
	Attendees             string   `name:"attendees" help:"Comma-separated attendee emails"`
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

func (c *CalendarCreateCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	ctx = withZoomIncludePasswords(ctx, c.IncludePasswords)
	if kctx != nil && flagProvided(kctx, "with-meet") && flagProvided(kctx, "with-zoom") {
		return usage("use only one of --with-zoom or --with-meet")
	}
	if flags != nil && flags.DryRun {
		placeLookup, err := validateCalendarPlaceLookup(calendarPlaceLookup{
			LocationSet:       flagProvided(kctx, "location") || strings.TrimSpace(c.Location) != "",
			LocationSearch:    c.LocationSearch,
			LocationSearchSet: flagProvided(kctx, "location-search"),
			PlaceID:           c.PlaceID,
			PlaceIDSet:        flagProvided(kctx, "place-id"),
			LanguageCode:      c.PlaceLanguage,
			RegionCode:        c.PlaceRegion,
		})
		if err != nil {
			return err
		}
		plan, err := buildCalendarCreatePlan(c)
		if err != nil {
			return err
		}
		calendarID, err := prepareCalendarID(plan.CalendarID, false)
		if err != nil {
			return err
		}
		request := map[string]any{
			"calendar_id":          calendarID,
			"send_updates":         plan.SendUpdates,
			"conference_version_1": plan.WithMeet,
			"supports_attachments": len(plan.Event.Attachments) > 0,
			"event":                plan.Event,
		}
		if plan.WithZoom {
			request["zoom"] = zoomDryRunPayload("create")
		}
		if placeLookup != nil {
			request["place_lookup"] = placeLookup.dryRunPayload()
		}
		return dryRunExit(ctx, flags, "calendar.create", request)
	}

	if err := c.resolvePlace(ctx, kctx); err != nil {
		return err
	}

	plan, err := buildCalendarCreatePlan(c)
	if err != nil {
		return err
	}

	calendarID, err := prepareCalendarID(plan.CalendarID, false)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "calendar.create", map[string]any{
		"calendar_id":          calendarID,
		"send_updates":         plan.SendUpdates,
		"conference_version_1": plan.WithMeet || plan.WithZoom,
		"supports_attachments": len(plan.Event.Attachments) > 0,
		"event":                plan.Event,
	}); dryRunErr != nil {
		return dryRunErr
	}

	mutation, err := newCalendarMutationContext(ctx, flags, calendarID)
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

func (c *CalendarCreateCmd) resolveCreateEventType() (string, error) {
	focusFlags := strings.TrimSpace(c.FocusAutoDecline) != "" ||
		strings.TrimSpace(c.FocusDeclineMessage) != "" ||
		strings.TrimSpace(c.FocusChatStatus) != ""
	oooFlags := strings.TrimSpace(c.OOOAutoDecline) != "" ||
		strings.TrimSpace(c.OOODeclineMessage) != ""
	workingFlags := strings.TrimSpace(c.WorkingLocationType) != "" ||
		strings.TrimSpace(c.WorkingOfficeLabel) != "" ||
		strings.TrimSpace(c.WorkingBuildingId) != "" ||
		strings.TrimSpace(c.WorkingFloorId) != "" ||
		strings.TrimSpace(c.WorkingDeskId) != "" ||
		strings.TrimSpace(c.WorkingCustomLabel) != ""

	return resolveEventType(c.EventType, focusFlags, oooFlags, workingFlags)
}

func (c *CalendarCreateCmd) defaultSummaryForEventType(eventType string) string {
	switch eventType {
	case eventTypeFocusTime:
		return defaultFocusSummary
	case eventTypeOutOfOffice:
		return defaultOOOSummary
	case eventTypeWorkingLocation:
		return workingLocationSummary(workingLocationInput{
			Type:        c.WorkingLocationType,
			OfficeLabel: c.WorkingOfficeLabel,
			CustomLabel: c.WorkingCustomLabel,
		})
	default:
		return ""
	}
}

func resolveCreateAllDay(from, to string, allDay bool, eventType string) (bool, error) {
	if eventType == eventTypeOutOfOffice {
		if allDay {
			return false, usage("out-of-office events cannot be all-day; provide RFC3339 datetime --from/--to without --all-day")
		}
		if !strings.Contains(from, "T") || !strings.Contains(to, "T") {
			return false, usage("out-of-office requires RFC3339 datetime --from/--to; date-only out-of-office events are not supported by Google Calendar API")
		}
		return false, nil
	}
	if eventType != eventTypeWorkingLocation {
		return allDay, nil
	}
	if strings.Contains(from, "T") || strings.Contains(to, "T") {
		return false, usage("working-location requires date-only --from/--to (YYYY-MM-DD)")
	}
	return true, nil
}

func applyEventTypeTransparencyDefault(transparency, eventType string) string {
	if transparency == "" && (eventType == eventTypeFocusTime || eventType == eventTypeOutOfOffice) {
		return transparencyOpaque
	}
	if transparency == "" && eventType == eventTypeWorkingLocation {
		return transparencyTransparent
	}
	return transparency
}

func applyEventTypeVisibilityDefault(visibility, eventType string) string {
	if visibility == "" && eventType == eventTypeWorkingLocation {
		return visibilityPublic
	}
	return visibility
}

func (c *CalendarCreateCmd) applyCreateEventType(event *calendar.Event, eventType string) error {
	switch eventType {
	case eventTypeDefault:
		event.EventType = eventTypeDefault
	case eventTypeFocusTime:
		props, err := c.buildFocusTimeProperties()
		if err != nil {
			return err
		}
		event.EventType = eventTypeFocusTime
		event.FocusTimeProperties = props
	case eventTypeOutOfOffice:
		props, err := c.buildOutOfOfficeProperties()
		if err != nil {
			return err
		}
		event.EventType = eventTypeOutOfOffice
		event.OutOfOfficeProperties = props
	case eventTypeWorkingLocation:
		props, err := buildWorkingLocationProperties(workingLocationInput{
			Type:        c.WorkingLocationType,
			OfficeLabel: c.WorkingOfficeLabel,
			BuildingId:  c.WorkingBuildingId,
			FloorId:     c.WorkingFloorId,
			DeskId:      c.WorkingDeskId,
			CustomLabel: c.WorkingCustomLabel,
		})
		if err != nil {
			return err
		}
		event.EventType = eventTypeWorkingLocation
		event.WorkingLocationProperties = props
	}
	return nil
}

func (c *CalendarCreateCmd) buildFocusTimeProperties() (*calendar.EventFocusTimeProperties, error) {
	return buildFocusTimeProperties(focusTimeInput{
		AutoDecline:    c.FocusAutoDecline,
		DeclineMessage: c.FocusDeclineMessage,
		ChatStatus:     c.FocusChatStatus,
	})
}

func (c *CalendarCreateCmd) buildOutOfOfficeProperties() (*calendar.EventOutOfOfficeProperties, error) {
	return buildOutOfOfficeProperties(outOfOfficeInput{
		AutoDecline:            c.OOOAutoDecline,
		DeclineMessage:         c.OOODeclineMessage,
		DeclineMessageProvided: false,
	})
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
	Attendees             string   `name:"attendees" help:"Comma-separated attendee emails (replaces all; set empty to clear)"`
	AddAttendee           string   `name:"add-attendee" help:"Comma-separated attendee emails to add (preserves existing attendees)"`
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

//nolint:gocyclo,cyclop // Calendar update already handles many flag families; Zoom adds one narrow branch.
func (c *CalendarUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	ctx = withZoomIncludePasswords(ctx, c.IncludePasswords)
	calendarID, err := prepareCalendarID(c.CalendarID, false)
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

	// If --all-day changed, require from/to to update both date/time fields.
	if flagProvided(kctx, "all-day") {
		if !flagProvided(kctx, "from") || !flagProvided(kctx, "to") {
			return usage("when changing --all-day, also provide --from and --to")
		}
	}

	// Cannot use both --attendees and --add-attendee at the same time.
	if flagProvided(kctx, "attendees") && flagProvided(kctx, "add-attendee") {
		return usage("cannot use both --attendees and --add-attendee; use --attendees to replace all, or --add-attendee to add")
	}
	if flagProvided(kctx, "with-meet") && flagProvided(kctx, "regenerate-meet") {
		return usage("use only one of --with-meet or --regenerate-meet")
	}
	if mutexErr := validateZoomConferenceFlagMutex(kctx); mutexErr != nil {
		return mutexErr
	}
	placeLookup, err := validateCalendarPlaceLookup(calendarPlaceLookup{
		LocationSet:       flagProvided(kctx, "location"),
		LocationSearch:    c.LocationSearch,
		LocationSearchSet: flagProvided(kctx, "location-search"),
		PlaceID:           c.PlaceID,
		PlaceIDSet:        flagProvided(kctx, "place-id"),
		LanguageCode:      c.PlaceLanguage,
		RegionCode:        c.PlaceRegion,
	})
	if err != nil {
		return err
	}
	if !(flags != nil && flags.DryRun) {
		if placeErr := c.resolvePlace(ctx, kctx); placeErr != nil {
			return placeErr
		}
	}

	sendUpdates, err := validateSendUpdates(c.SendUpdates)
	if err != nil {
		return err
	}
	recurrenceProvided := flagProvided(kctx, "rrule")

	patch, changed, err := c.buildUpdatePatch(kctx)
	if err != nil {
		return err
	}

	wantsAddAttendee := flagProvided(kctx, "add-attendee")
	if wantsAddAttendee && strings.TrimSpace(c.AddAttendee) == "" {
		return usage("empty --add-attendee")
	}

	if !changed && !wantsAddAttendee && placeLookup == nil {
		return usage("no updates provided")
	}

	request := map[string]any{
		"calendar_id":          calendarID,
		"event_id":             eventID,
		"send_updates":         sendUpdates,
		"scope":                scope,
		"original_start_time":  strings.TrimSpace(c.OriginalStartTime),
		"add_attendee":         strings.TrimSpace(c.AddAttendee),
		"patch":                patch,
		"wants_add_attendee":   wantsAddAttendee,
		"conference_version_1": patchHasConferenceDataMutation(patch),
		"supports_attachments": patchHasAttachmentsMutation(patch),
	}
	if placeLookup != nil {
		request["place_lookup"] = placeLookup.dryRunPayload()
	}
	if zoomPayload := zoomUpdateDryRunPayload(kctx); zoomPayload != nil {
		request["zoom"] = zoomPayload
	}
	if dryRunErr := dryRunExit(ctx, flags, "calendar.update", request); dryRunErr != nil {
		return dryRunErr
	}

	mutation, err := newCalendarMutationContext(ctx, flags, calendarID)
	if err != nil {
		return err
	}

	// For --add-attendee, fetch current event to preserve existing attendees with metadata.
	if wantsAddAttendee {
		existing, getErr := mutation.svc.Events.Get(mutation.calendarID, eventID).Context(ctx).Do()
		if getErr != nil {
			return fmt.Errorf("failed to fetch current event: %w", getErr)
		}
		merged, attendeesChanged := mergeAttendeesWithChange(existing.Attendees, c.AddAttendee)
		if attendeesChanged {
			patch.Attendees = merged
			changed = true
		}
		if !changed {
			return usage("no updates provided")
		}
	}

	if flagProvidedAny(kctx, "with-zoom", "regenerate-zoom", "remove-zoom") {
		var zoomErr error
		patch, _, zoomErr = c.prepareZoomConferencePatch(ctx, mutation, eventID, scope, c.OriginalStartTime, patch, changed, kctx)
		if errors.Is(zoomErr, errZoomConferenceAlreadyHandled) {
			return nil
		}
		if zoomErr != nil {
			return zoomErr
		}
	}

	if patch.ConferenceData != nil && !flagProvided(kctx, "regenerate-meet") && !flagProvidedAny(kctx, "with-zoom", "regenerate-zoom", "remove-zoom") && patchOnlyConferenceData(patch) {
		resolution, resolveErr := resolveRecurringScopeResolution(ctx, mutation.svc, mutation.calendarID, eventID, scope, c.OriginalStartTime)
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

	targetEventID, parentRecurrence, err := applyUpdateScope(ctx, mutation.svc, mutation.calendarID, eventID, scope, c.OriginalStartTime, patch)
	if err != nil {
		return err
	}
	if patch.ConferenceData != nil && !flagProvided(kctx, "regenerate-meet") && !flagProvidedAny(kctx, "with-zoom", "regenerate-zoom", "remove-zoom") {
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
	if recurrenceProvided {
		if enrichErr := ensureRecurringPatchDateTimes(ctx, mutation.svc, mutation.calendarID, targetEventID, patch); enrichErr != nil {
			return enrichErr
		}
	}

	updated, err := mutation.patchEvent(ctx, targetEventID, patch, sendUpdates)
	if err != nil {
		if c.createdZoomMeetingID != "" {
			_ = cancelZoomMeeting(ctx, c.createdZoomMeetingID, "delete")
		}
		return err
	}
	if scope == scopeFuture {
		if err := truncateParentRecurrence(ctx, mutation.svc, mutation.calendarID, eventID, parentRecurrence, c.OriginalStartTime, sendUpdates); err != nil {
			return err
		}
	}
	return mutation.writeEvent(ctx, updated)
}

func (c *CalendarUpdateCmd) buildUpdatePatch(kctx *kong.Context) (*calendar.Event, bool, error) {
	patch := &calendar.Event{}
	changed := false

	eventType, eventTypeRequested, focusFlags, oooFlags, workingFlags, err := c.resolveUpdateEventType(kctx)
	if err != nil {
		return nil, false, err
	}

	if c.applyTextFields(kctx, patch) {
		changed = true
	}

	timeChanged, err := c.applyTimeFields(kctx, patch, eventType)
	if err != nil {
		return nil, false, err
	}
	if timeChanged {
		changed = true
	}

	if c.applyAttendees(kctx, patch) {
		changed = true
	}

	if c.applyAttachments(kctx, patch) {
		changed = true
	}

	if c.applyRecurrence(kctx, patch) {
		changed = true
	}

	remindersChanged, err := c.applyReminders(kctx, patch)
	if err != nil {
		return nil, false, err
	}
	if remindersChanged {
		changed = true
	}

	displayChanged, err := c.applyDisplayOptions(kctx, patch)
	if err != nil {
		return nil, false, err
	}
	if displayChanged {
		changed = true
	}

	if c.applyGuestOptions(kctx, patch) {
		changed = true
	}

	if c.applyConferenceData(kctx, patch) {
		changed = true
	}

	if c.applyExtendedProperties(kctx, patch) {
		changed = true
	}
	if c.resolvedPlace != nil {
		applyCalendarPlaceProperties(patch, c.resolvedPlace)
		changed = true
	}

	eventTypeChanged, err := c.applyEventTypeProperties(kctx, patch, eventType, eventTypeRequested, focusFlags, oooFlags, workingFlags)
	if err != nil {
		return nil, false, err
	}
	if eventTypeChanged {
		changed = true
	}

	return patch, changed, nil
}

func (c *CalendarUpdateCmd) resolveUpdateEventType(kctx *kong.Context) (string, bool, bool, bool, bool, error) {
	focusFlags := flagProvidedAny(kctx, "focus-auto-decline", "focus-decline-message", "focus-chat-status")
	oooFlags := flagProvidedAny(kctx, "ooo-auto-decline", "ooo-decline-message")
	workingFlags := flagProvidedAny(kctx, "working-location-type", "working-office-label", "working-building-id", "working-floor-id", "working-desk-id", "working-custom-label")
	eventType, err := resolveEventType(c.EventType, focusFlags, oooFlags, workingFlags)
	if err != nil {
		return "", false, false, false, false, err
	}
	return eventType, eventType != "", focusFlags, oooFlags, workingFlags, nil
}

func (c *CalendarUpdateCmd) applyTextFields(kctx *kong.Context, patch *calendar.Event) bool {
	changed := false
	if flagProvided(kctx, "summary") {
		patch.Summary = strings.TrimSpace(c.Summary)
		changed = true
	}
	if flagProvided(kctx, "description") {
		patch.Description = strings.TrimSpace(c.Description)
		if patch.Description == "" {
			patch.ForceSendFields = appendForceSendField(patch.ForceSendFields, "Description")
		}
		changed = true
	}
	if flagProvided(kctx, "location") {
		patch.Location = strings.TrimSpace(c.Location)
		changed = true
	}
	if c.resolvedPlace != nil {
		patch.Location = formatCalendarPlaceLocation(c.resolvedPlace)
		changed = true
	}
	return changed
}

func resolveUpdateAllDay(value string, allDay bool, eventType string) (bool, error) {
	if eventType == eventTypeOutOfOffice {
		if allDay {
			return false, usage("out-of-office events cannot be all-day; provide RFC3339 datetime --from/--to without --all-day")
		}
		if !strings.Contains(value, "T") {
			return false, usage("out-of-office requires RFC3339 datetime --from/--to; date-only out-of-office events are not supported by Google Calendar API")
		}
		return false, nil
	}
	if eventType != eventTypeWorkingLocation {
		return allDay, nil
	}
	if strings.Contains(value, "T") {
		return false, usage("working-location requires date-only --from/--to (YYYY-MM-DD)")
	}
	return true, nil
}

func (c *CalendarUpdateCmd) applyTimeFields(kctx *kong.Context, patch *calendar.Event, eventType string) (bool, error) {
	changed := false
	if flagProvided(kctx, "start-timezone") && !flagProvided(kctx, "from") {
		return false, usage("--start-timezone requires --from")
	}
	if flagProvided(kctx, "end-timezone") && !flagProvided(kctx, "to") {
		return false, usage("--end-timezone requires --to")
	}
	if flagProvided(kctx, "from") {
		allDay, err := resolveUpdateAllDay(c.From, c.AllDay, eventType)
		if err != nil {
			return false, err
		}
		patch.Start, err = buildEventDateTimeWithTimezone(c.From, allDay, c.StartTimezone, "--start-timezone")
		if err != nil {
			return false, err
		}
		changed = true
	}
	if flagProvided(kctx, "to") {
		allDay, err := resolveUpdateAllDay(c.To, c.AllDay, eventType)
		if err != nil {
			return false, err
		}
		patch.End, err = buildEventDateTimeWithTimezone(c.To, allDay, c.EndTimezone, "--end-timezone")
		if err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func (c *CalendarUpdateCmd) applyAttendees(kctx *kong.Context, patch *calendar.Event) bool {
	if !flagProvided(kctx, "attendees") {
		return false
	}
	patch.Attendees = buildAttendees(c.Attendees)
	return true
}

func (c *CalendarUpdateCmd) applyAttachments(kctx *kong.Context, patch *calendar.Event) bool {
	if !flagProvided(kctx, "attachment") {
		return false
	}
	patch.Attachments = buildAttachments(c.Attachments)
	if len(patch.Attachments) == 0 {
		patch.Attachments = []*calendar.EventAttachment{}
		patch.ForceSendFields = appendForceSendField(patch.ForceSendFields, "Attachments")
	}
	return true
}

func (c *CalendarUpdateCmd) applyRecurrence(kctx *kong.Context, patch *calendar.Event) bool {
	if !flagProvided(kctx, "rrule") {
		return false
	}
	recurrence := buildRecurrence(c.Recurrence)
	if recurrence == nil {
		patch.Recurrence = []string{}
		patch.ForceSendFields = append(patch.ForceSendFields, "Recurrence")
	} else {
		patch.Recurrence = recurrence
	}
	return true
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

func recurringPatchDateTimeNeedsFetch(dt *calendar.EventDateTime) bool {
	if dt == nil {
		return true
	}
	if strings.TrimSpace(dt.Date) != "" {
		return false
	}
	return strings.TrimSpace(dt.DateTime) == "" || strings.TrimSpace(dt.TimeZone) == ""
}

func normalizeRecurringPatchDateTime(primary, fallback *calendar.EventDateTime) *calendar.EventDateTime {
	if primary == nil && fallback == nil {
		return nil
	}

	var out *calendar.EventDateTime
	if primary != nil {
		out = cloneEventDateTime(primary)
	} else {
		out = cloneEventDateTime(fallback)
	}
	if out == nil {
		return nil
	}

	if strings.TrimSpace(out.Date) != "" {
		out.DateTime = ""
		out.TimeZone = ""
		return out
	}
	if strings.TrimSpace(out.DateTime) == "" && fallback != nil {
		if strings.TrimSpace(fallback.Date) != "" {
			return &calendar.EventDateTime{Date: fallback.Date}
		}
		out.DateTime = fallback.DateTime
	}
	if strings.TrimSpace(out.TimeZone) == "" && fallback != nil {
		out.TimeZone = strings.TrimSpace(fallback.TimeZone)
	}
	if strings.TrimSpace(out.TimeZone) == "" && strings.TrimSpace(out.DateTime) != "" {
		out.TimeZone = extractTimezone(out.DateTime)
	}
	return out
}

func cloneEventDateTime(in *calendar.EventDateTime) *calendar.EventDateTime {
	if in == nil {
		return nil
	}
	return &calendar.EventDateTime{
		Date:     in.Date,
		DateTime: in.DateTime,
		TimeZone: in.TimeZone,
	}
}

func (c *CalendarUpdateCmd) applyReminders(kctx *kong.Context, patch *calendar.Event) (bool, error) {
	if !flagProvided(kctx, "reminder") {
		return false, nil
	}
	reminders, err := buildReminders(c.Reminders)
	if err != nil {
		return false, err
	}
	if reminders == nil {
		patch.Reminders = &calendar.EventReminders{UseDefault: true}
		patch.ForceSendFields = append(patch.ForceSendFields, "Reminders")
	} else {
		patch.Reminders = reminders
	}
	return true, nil
}

func (c *CalendarUpdateCmd) applyDisplayOptions(kctx *kong.Context, patch *calendar.Event) (bool, error) {
	changed := false
	if flagProvided(kctx, "event-color") {
		colorId, err := validateColorId(c.ColorId)
		if err != nil {
			return false, err
		}
		patch.ColorId = colorId
		changed = true
	}
	if flagProvided(kctx, "visibility") {
		visibility, err := validateVisibility(c.Visibility)
		if err != nil {
			return false, err
		}
		patch.Visibility = visibility
		changed = true
	}
	if flagProvided(kctx, "transparency") {
		transparency, err := validateTransparency(c.Transparency)
		if err != nil {
			return false, err
		}
		patch.Transparency = transparency
		changed = true
	}
	return changed, nil
}

func (c *CalendarUpdateCmd) applyGuestOptions(kctx *kong.Context, patch *calendar.Event) bool {
	changed := false
	if flagProvided(kctx, "guests-can-invite") {
		if c.GuestsCanInviteOthers != nil {
			patch.GuestsCanInviteOthers = c.GuestsCanInviteOthers
		}
		patch.ForceSendFields = append(patch.ForceSendFields, "GuestsCanInviteOthers")
		changed = true
	}
	if flagProvided(kctx, "guests-can-modify") {
		if c.GuestsCanModify != nil {
			patch.GuestsCanModify = *c.GuestsCanModify
		}
		patch.ForceSendFields = append(patch.ForceSendFields, "GuestsCanModify")
		changed = true
	}
	if flagProvided(kctx, "guests-can-see-others") {
		if c.GuestsCanSeeOthers != nil {
			patch.GuestsCanSeeOtherGuests = c.GuestsCanSeeOthers
		}
		patch.ForceSendFields = append(patch.ForceSendFields, "GuestsCanSeeOtherGuests")
		changed = true
	}
	return changed
}

func (c *CalendarUpdateCmd) applyConferenceData(kctx *kong.Context, patch *calendar.Event) bool {
	if flagProvided(kctx, "remove-zoom") {
		patch.NullFields = append(patch.NullFields, "ConferenceData")
		return true
	}
	if flagProvided(kctx, "with-zoom") || flagProvided(kctx, "regenerate-zoom") {
		return true
	}
	if !flagProvided(kctx, "with-meet") && !flagProvided(kctx, "regenerate-meet") {
		return false
	}
	patch.ConferenceData = buildMeetConferenceData()
	return true
}

func eventHasConferenceLink(event *calendar.Event) bool {
	if event == nil {
		return false
	}
	if strings.TrimSpace(event.HangoutLink) != "" {
		return true
	}
	if event.ConferenceData == nil {
		return false
	}
	for _, ep := range event.ConferenceData.EntryPoints {
		if ep != nil && strings.TrimSpace(ep.Uri) != "" {
			return true
		}
	}
	return false
}

func patchOnlyConferenceData(event *calendar.Event) bool {
	if event == nil || !patchHasConferenceDataMutation(event) {
		return false
	}
	clone := *event
	clone.ConferenceData = nil
	clone.NullFields = removeStringField(clone.NullFields, "ConferenceData")
	return reflect.DeepEqual(clone, calendar.Event{})
}

func validateZoomConferenceFlagMutex(kctx *kong.Context) error {
	pairs := [][2]string{
		{"with-zoom", "regenerate-zoom"},
		{"with-zoom", "remove-zoom"},
		{"regenerate-zoom", "remove-zoom"},
		{"with-zoom", "with-meet"},
		{"with-zoom", "regenerate-meet"},
		{"regenerate-zoom", "with-meet"},
		{"regenerate-zoom", "regenerate-meet"},
	}
	for _, pair := range pairs {
		if flagProvided(kctx, pair[0]) && flagProvided(kctx, pair[1]) {
			return usage(fmt.Sprintf("use only one of --%s or --%s", pair[0], pair[1]))
		}
	}
	return nil
}

func zoomUpdateDryRunPayload(kctx *kong.Context) map[string]any {
	switch {
	case flagProvided(kctx, "with-zoom"):
		return zoomDryRunPayload("create")
	case flagProvided(kctx, "regenerate-zoom"):
		return zoomDryRunPayload("regenerate")
	case flagProvided(kctx, "remove-zoom"):
		return zoomDryRunPayload("remove")
	default:
		return nil
	}
}

func zoomDryRunPayload(action string) map[string]any {
	return map[string]any{
		"action":           action,
		"description_mode": true,
	}
}

func (c *CalendarUpdateCmd) prepareZoomConferencePatch(
	ctx context.Context,
	mutation *calendarMutationContext,
	eventID, scope, originalStartTime string,
	patch *calendar.Event,
	changed bool,
	kctx *kong.Context,
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
	case flagProvided(kctx, "with-zoom"):
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

	case flagProvided(kctx, "regenerate-zoom"):
		if meetingID, ok := extractZoomMeetingID(existing); ok {
			if err := cancelZoomMeeting(ctx, meetingID, "regenerate"); err != nil && !errors.Is(err, zoom.ErrMeetingNotFound) {
				return patch, changed, err
			}
		} else {
			warnUnparseableZoomMeeting(mutation.u)
		}
		meeting, createErr := createZoomMeetingForEvent(ctx, mergeEventPatch(existing, patch))
		if createErr != nil {
			return patch, changed, createErr
		}
		c.createdZoomMeetingID = zoomMeetingID(meeting)
		patch.Description = applyZoomDescriptionBlock(descriptionForPatch(existing, patch), buildZoomDescriptionBlock(meeting))
		return patch, true, nil

	case flagProvided(kctx, "remove-zoom"):
		if meetingID, ok := extractZoomMeetingID(existing); ok {
			if err := cancelZoomMeeting(ctx, meetingID, "delete"); err != nil && !errors.Is(err, zoom.ErrMeetingNotFound) {
				if mutation.u != nil {
					mutation.u.Err().Linef("warning\tfailed to delete Zoom meeting %s: %v", meetingID, err)
				}
			}
		} else {
			warnUnparseableZoomMeeting(mutation.u)
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

func mergeEventPatch(existing, patch *calendar.Event) *calendar.Event {
	if existing == nil {
		return patch
	}
	merged := *existing
	if patch == nil {
		return &merged
	}
	if strings.TrimSpace(patch.Summary) != "" {
		merged.Summary = patch.Summary
	}
	if strings.TrimSpace(patch.Description) != "" || forceSendsField(patch, "Description") {
		merged.Description = patch.Description
	}
	if patch.Start != nil {
		merged.Start = patch.Start
	}
	if patch.End != nil {
		merged.End = patch.End
	}
	return &merged
}

func patchHasConferenceDataMutation(event *calendar.Event) bool {
	if event == nil {
		return false
	}
	if event.ConferenceData != nil {
		return true
	}
	for _, field := range event.NullFields {
		if field == "ConferenceData" {
			return true
		}
	}
	return false
}

func patchEffectivelyEmpty(event *calendar.Event) bool {
	return event == nil || reflect.DeepEqual(*event, calendar.Event{})
}

func removeStringField(fields []string, value string) []string {
	out := fields[:0]
	for _, field := range fields {
		if field != value {
			out = append(out, field)
		}
	}
	return out
}

func (c *CalendarUpdateCmd) applyExtendedProperties(kctx *kong.Context, patch *calendar.Event) bool {
	if !flagProvided(kctx, "private-prop") && !flagProvided(kctx, "shared-prop") {
		return false
	}
	patch.ExtendedProperties = buildExtendedProperties(c.PrivateProps, c.SharedProps)
	return true
}

func (c *CalendarUpdateCmd) applyEventTypeProperties(kctx *kong.Context, patch *calendar.Event, eventType string, eventTypeRequested, focusFlags, oooFlags, workingFlags bool) (bool, error) {
	changed := false
	if eventTypeRequested {
		patch.EventType = eventType
		changed = true
		if eventType == eventTypeDefault {
			patch.NullFields = append(patch.NullFields, "FocusTimeProperties", "OutOfOfficeProperties", "WorkingLocationProperties")
		}
	}
	if eventTypeRequested && !flagProvided(kctx, "transparency") &&
		(eventType == eventTypeFocusTime || eventType == eventTypeOutOfOffice) {
		patch.Transparency = transparencyOpaque
		changed = true
	}
	if eventTypeRequested && !flagProvided(kctx, "transparency") && eventType == eventTypeWorkingLocation {
		patch.Transparency = transparencyTransparent
		changed = true
	}
	if eventTypeRequested && !flagProvided(kctx, "visibility") && eventType == eventTypeWorkingLocation {
		patch.Visibility = visibilityPublic
		changed = true
	}

	switch eventType {
	case eventTypeFocusTime:
		if eventTypeRequested || focusFlags {
			props, err := c.buildUpdateFocusTimeProperties()
			if err != nil {
				return false, err
			}
			patch.FocusTimeProperties = props
			changed = true
		}
	case eventTypeOutOfOffice:
		if eventTypeRequested || oooFlags {
			props, err := c.buildUpdateOutOfOfficeProperties(flagProvided(kctx, "ooo-decline-message"))
			if err != nil {
				return false, err
			}
			patch.OutOfOfficeProperties = props
			changed = true
		}
	case eventTypeWorkingLocation:
		if eventTypeRequested || workingFlags {
			props, err := buildWorkingLocationProperties(workingLocationInput{
				Type:        c.WorkingLocationType,
				OfficeLabel: c.WorkingOfficeLabel,
				BuildingId:  c.WorkingBuildingId,
				FloorId:     c.WorkingFloorId,
				DeskId:      c.WorkingDeskId,
				CustomLabel: c.WorkingCustomLabel,
			})
			if err != nil {
				return false, err
			}
			patch.WorkingLocationProperties = props
			changed = true
		}
	}
	return changed, nil
}

func (c *CalendarUpdateCmd) buildUpdateFocusTimeProperties() (*calendar.EventFocusTimeProperties, error) {
	return buildFocusTimeProperties(focusTimeInput{
		AutoDecline:    c.FocusAutoDecline,
		DeclineMessage: c.FocusDeclineMessage,
		ChatStatus:     c.FocusChatStatus,
	})
}

func (c *CalendarUpdateCmd) buildUpdateOutOfOfficeProperties(declineProvided bool) (*calendar.EventOutOfOfficeProperties, error) {
	return buildOutOfOfficeProperties(outOfOfficeInput{
		AutoDecline:            c.OOOAutoDecline,
		DeclineMessage:         c.OOODeclineMessage,
		DeclineMessageProvided: declineProvided,
	})
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
	calendarID, err := prepareCalendarID(c.CalendarID, false)
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
