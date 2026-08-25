package cmd

import (
	"strings"

	"google.golang.org/api/calendar/v3"
)

func buildCalendarUpdatePatch(input calendarUpdateInput, fields calendarUpdateFields) (*calendar.Event, bool, error) {
	patch := &calendar.Event{}
	changed := false

	eventType, eventTypeRequested, focusFlags, oooFlags, workingFlags, err := resolveUpdateEventType(input, fields)
	if err != nil {
		return nil, false, err
	}

	if applyUpdateTextFields(input, fields, patch) {
		changed = true
	}

	timeChanged, err := applyUpdateTimeFields(input, fields, patch, eventType)
	if err != nil {
		return nil, false, err
	}
	if timeChanged {
		changed = true
	}

	if applyUpdateAttendees(input, fields, patch) {
		changed = true
	}
	if applyUpdateAttachments(input, fields, patch) {
		changed = true
	}
	if applyUpdateRecurrence(input, fields, patch) {
		changed = true
	}

	remindersChanged, err := applyUpdateReminders(input, fields, patch)
	if err != nil {
		return nil, false, err
	}
	if remindersChanged {
		changed = true
	}

	displayChanged, err := applyUpdateDisplayOptions(input, fields, patch)
	if err != nil {
		return nil, false, err
	}
	if displayChanged {
		changed = true
	}

	if applyUpdateGuestOptions(input, fields, patch) {
		changed = true
	}
	if applyUpdateConferenceData(fields, patch) {
		changed = true
	}
	if applyUpdateExtendedProperties(input, fields, patch) {
		changed = true
	}
	if input.ResolvedPlace != nil {
		applyCalendarPlaceProperties(patch, input.ResolvedPlace)
		changed = true
	}

	eventTypeChanged, err := applyUpdateEventTypeProperties(input, fields, patch, eventType, eventTypeRequested, focusFlags, oooFlags, workingFlags)
	if err != nil {
		return nil, false, err
	}
	if eventTypeChanged {
		changed = true
	}

	return patch, changed, nil
}

func resolveUpdateEventType(input calendarUpdateInput, fields calendarUpdateFields) (string, bool, bool, bool, bool, error) {
	focusFlags := fields.focusEventType()
	oooFlags := fields.outOfOfficeEventType()
	workingFlags := fields.workingLocationEventType()
	eventType, err := resolveEventType(input.EventType, focusFlags, oooFlags, workingFlags)
	if err != nil {
		return "", false, false, false, false, err
	}
	return eventType, eventType != "", focusFlags, oooFlags, workingFlags, nil
}

func applyUpdateTextFields(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event) bool {
	changed := false
	if fields.Summary {
		patch.Summary = strings.TrimSpace(input.Summary)
		changed = true
	}
	if fields.Description {
		patch.Description = strings.TrimSpace(input.Description)
		if patch.Description == "" {
			patch.ForceSendFields = appendForceSendField(patch.ForceSendFields, "Description")
		}
		changed = true
	}
	if fields.Location {
		patch.Location = strings.TrimSpace(input.Location)
		changed = true
	}
	if input.ResolvedPlace != nil {
		patch.Location = formatCalendarPlaceLocation(input.ResolvedPlace)
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

func applyUpdateTimeFields(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event, eventType string) (bool, error) {
	changed := false
	if fields.StartTimezone && !fields.From {
		return false, usage("--start-timezone requires --from")
	}
	if fields.EndTimezone && !fields.To {
		return false, usage("--end-timezone requires --to")
	}
	if fields.From {
		allDay, err := resolveUpdateAllDay(input.From, input.AllDay, eventType)
		if err != nil {
			return false, err
		}
		patch.Start, err = buildEventDateTimeWithTimezone(input.From, allDay, input.StartTimezone, "--start-timezone")
		if err != nil {
			return false, err
		}
		changed = true
	}
	if fields.To {
		allDay, err := resolveUpdateAllDay(input.To, input.AllDay, eventType)
		if err != nil {
			return false, err
		}
		patch.End, err = buildEventDateTimeWithTimezone(input.To, allDay, input.EndTimezone, "--end-timezone")
		if err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func applyUpdateAttendees(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event) bool {
	if !fields.Attendees {
		return false
	}
	patch.Attendees = buildAttendees(input.Attendees)
	return true
}

func applyUpdateAttachments(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event) bool {
	if !fields.Attachments {
		return false
	}
	patch.Attachments = buildAttachments(input.Attachments)
	if len(patch.Attachments) == 0 {
		patch.Attachments = []*calendar.EventAttachment{}
		patch.ForceSendFields = appendForceSendField(patch.ForceSendFields, "Attachments")
	}
	return true
}

func applyUpdateRecurrence(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event) bool {
	if !fields.Recurrence {
		return false
	}
	recurrence := buildRecurrence(input.Recurrence)
	if recurrence == nil {
		patch.Recurrence = []string{}
		patch.ForceSendFields = append(patch.ForceSendFields, "Recurrence")
	} else {
		patch.Recurrence = recurrence
	}
	return true
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

func applyUpdateReminders(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event) (bool, error) {
	if !fields.Reminders && !input.NoReminders {
		return false, nil
	}
	reminders, err := buildReminders(input.Reminders, input.NoReminders)
	if err != nil {
		return false, err
	}
	if reminders == nil {
		// PATCH merges omitted fields, so remove old overrides before restoring defaults.
		patch.Reminders = &calendar.EventReminders{
			UseDefault: true,
			NullFields: []string{"Overrides"},
		}
		patch.ForceSendFields = append(patch.ForceSendFields, "Reminders")
	} else {
		patch.Reminders = reminders
	}
	return true, nil
}

func applyUpdateDisplayOptions(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event) (bool, error) {
	changed := false
	if fields.ColorID {
		colorID, err := validateColorId(input.ColorID)
		if err != nil {
			return false, err
		}
		patch.ColorId = colorID
		changed = true
	}
	if fields.Visibility {
		visibility, err := validateVisibility(input.Visibility)
		if err != nil {
			return false, err
		}
		patch.Visibility = visibility
		changed = true
	}
	if fields.Transparency {
		transparency, err := validateTransparency(input.Transparency)
		if err != nil {
			return false, err
		}
		patch.Transparency = transparency
		changed = true
	}
	return changed, nil
}

func applyUpdateGuestOptions(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event) bool {
	changed := false
	if fields.GuestsCanInviteOthers {
		if input.GuestsCanInviteOthers != nil {
			patch.GuestsCanInviteOthers = input.GuestsCanInviteOthers
		}
		patch.ForceSendFields = append(patch.ForceSendFields, "GuestsCanInviteOthers")
		changed = true
	}
	if fields.GuestsCanModify {
		if input.GuestsCanModify != nil {
			patch.GuestsCanModify = *input.GuestsCanModify
		}
		patch.ForceSendFields = append(patch.ForceSendFields, "GuestsCanModify")
		changed = true
	}
	if fields.GuestsCanSeeOthers {
		if input.GuestsCanSeeOthers != nil {
			patch.GuestsCanSeeOtherGuests = input.GuestsCanSeeOthers
		}
		patch.ForceSendFields = append(patch.ForceSendFields, "GuestsCanSeeOtherGuests")
		changed = true
	}
	return changed
}

func applyUpdateConferenceData(fields calendarUpdateFields, patch *calendar.Event) bool {
	if fields.RemoveZoom {
		patch.NullFields = append(patch.NullFields, "ConferenceData")
		return true
	}
	if fields.WithZoom || fields.RegenerateZoom {
		return true
	}
	if !fields.WithMeet && !fields.RegenerateMeet {
		return false
	}
	patch.ConferenceData = buildMeetConferenceData()
	return true
}

func applyUpdateExtendedProperties(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event) bool {
	if !fields.PrivateProps && !fields.SharedProps {
		return false
	}
	patch.ExtendedProperties = buildExtendedProperties(input.PrivateProps, input.SharedProps)
	return true
}

func applyUpdateEventTypeProperties(input calendarUpdateInput, fields calendarUpdateFields, patch *calendar.Event, eventType string, eventTypeRequested, focusFlags, oooFlags, workingFlags bool) (bool, error) {
	changed := false
	if eventTypeRequested {
		patch.EventType = eventType
		changed = true
		if eventType == eventTypeDefault {
			patch.NullFields = append(patch.NullFields, "FocusTimeProperties", "OutOfOfficeProperties", "WorkingLocationProperties")
		}
	}
	if eventTypeRequested && !fields.Transparency &&
		(eventType == eventTypeFocusTime || eventType == eventTypeOutOfOffice) {
		patch.Transparency = transparencyOpaque
		changed = true
	}
	if eventTypeRequested && !fields.Transparency && eventType == eventTypeWorkingLocation {
		patch.Transparency = transparencyTransparent
		changed = true
	}
	if eventTypeRequested && !fields.Visibility && eventType == eventTypeWorkingLocation {
		patch.Visibility = visibilityPublic
		changed = true
	}

	switch eventType {
	case eventTypeFocusTime:
		if eventTypeRequested || focusFlags {
			props, err := buildFocusTimeProperties(focusTimeInput{
				AutoDecline:    input.FocusAutoDecline,
				DeclineMessage: input.FocusDeclineMessage,
				ChatStatus:     input.FocusChatStatus,
			})
			if err != nil {
				return false, err
			}
			patch.FocusTimeProperties = props
			changed = true
		}
	case eventTypeOutOfOffice:
		if eventTypeRequested || oooFlags {
			props, err := buildOutOfOfficeProperties(outOfOfficeInput{
				AutoDecline:            input.OOOAutoDecline,
				DeclineMessage:         input.OOODeclineMessage,
				DeclineMessageProvided: fields.OOODeclineMessage,
			})
			if err != nil {
				return false, err
			}
			patch.OutOfOfficeProperties = props
			changed = true
		}
	case eventTypeWorkingLocation:
		if eventTypeRequested || workingFlags {
			props, err := buildWorkingLocationProperties(workingLocationInput{
				Type:        input.WorkingLocationType,
				OfficeLabel: input.WorkingOfficeLabel,
				BuildingId:  input.WorkingBuildingID,
				FloorId:     input.WorkingFloorID,
				DeskId:      input.WorkingDeskID,
				CustomLabel: input.WorkingCustomLabel,
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
