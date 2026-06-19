package cmd

import (
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/config"
)

type focusTimeInput struct {
	AutoDecline    string
	DeclineMessage string
	ChatStatus     string
}

type outOfOfficeInput struct {
	AutoDecline            string
	DeclineMessage         string
	DeclineMessageProvided bool
}

type calendarCreateFields struct {
	Location       bool
	LocationSearch bool
	PlaceID        bool
	WithMeet       bool
	WithZoom       bool
}

type calendarCreateInput struct {
	CalendarID            string
	Summary               string
	From                  string
	To                    string
	StartTimezone         string
	EndTimezone           string
	Timezone              string
	Description           string
	Location              string
	Attendees             string
	AllDay                bool
	Recurrence            []string
	Reminders             []string
	ColorID               string
	Visibility            string
	Transparency          string
	SendUpdates           string
	GuestsCanInviteOthers *bool
	GuestsCanModify       *bool
	GuestsCanSeeOthers    *bool
	WithMeet              bool
	WithZoom              bool
	SourceURL             string
	SourceTitle           string
	Attachments           []string
	PrivateProps          []string
	SharedProps           []string
	EventType             string
	FocusAutoDecline      string
	FocusDeclineMessage   string
	FocusChatStatus       string
	OOOAutoDecline        string
	OOODeclineMessage     string
	WorkingLocationType   string
	WorkingOfficeLabel    string
	WorkingBuildingID     string
	WorkingFloorID        string
	WorkingDeskID         string
	WorkingCustomLabel    string
	LocationSearch        string
	PlaceID               string
	PlaceLanguage         string
	PlaceRegion           string
	ResolvedPlace         *calendarPlace
}

type calendarCreatePlan struct {
	CalendarID  string
	SendUpdates string
	WithMeet    bool
	WithZoom    bool
	PlaceLookup *calendarPlaceLookupRequest
	Event       *calendar.Event
}

type calendarUpdateFields struct {
	Summary               bool
	Description           bool
	Location              bool
	LocationSearch        bool
	PlaceID               bool
	From                  bool
	To                    bool
	StartTimezone         bool
	EndTimezone           bool
	AllDay                bool
	Attendees             bool
	AddAttendee           bool
	Attachments           bool
	Recurrence            bool
	Reminders             bool
	ColorID               bool
	Visibility            bool
	Transparency          bool
	GuestsCanInviteOthers bool
	GuestsCanModify       bool
	GuestsCanSeeOthers    bool
	WithMeet              bool
	RegenerateMeet        bool
	WithZoom              bool
	RegenerateZoom        bool
	RemoveZoom            bool
	PrivateProps          bool
	SharedProps           bool
	FocusAutoDecline      bool
	FocusDeclineMessage   bool
	FocusChatStatus       bool
	OOOAutoDecline        bool
	OOODeclineMessage     bool
	WorkingLocationType   bool
	WorkingOfficeLabel    bool
	WorkingBuildingID     bool
	WorkingFloorID        bool
	WorkingDeskID         bool
	WorkingCustomLabel    bool
}

type calendarUpdateInput struct {
	CalendarID            string
	EventID               string
	Summary               string
	From                  string
	To                    string
	StartTimezone         string
	EndTimezone           string
	Description           string
	Location              string
	LocationSearch        string
	PlaceID               string
	PlaceLanguage         string
	PlaceRegion           string
	Attendees             string
	AddAttendee           string
	Attachments           []string
	AllDay                bool
	Recurrence            []string
	Reminders             []string
	ColorID               string
	Visibility            string
	Transparency          string
	GuestsCanInviteOthers *bool
	GuestsCanModify       *bool
	GuestsCanSeeOthers    *bool
	Scope                 string
	OriginalStartTime     string
	PrivateProps          []string
	SharedProps           []string
	EventType             string
	FocusAutoDecline      string
	FocusDeclineMessage   string
	FocusChatStatus       string
	OOOAutoDecline        string
	OOODeclineMessage     string
	WorkingLocationType   string
	WorkingOfficeLabel    string
	WorkingBuildingID     string
	WorkingFloorID        string
	WorkingDeskID         string
	WorkingCustomLabel    string
	SendUpdates           string
	ResolvedPlace         *calendarPlace
}

type calendarUpdatePlan struct {
	CalendarID         string
	EventID            string
	Scope              string
	OriginalStartTime  string
	SendUpdates        string
	AddAttendee        string
	WantsAddAttendee   bool
	RecurrenceProvided bool
	Fields             calendarUpdateFields
	PlaceLookup        *calendarPlaceLookupRequest
	Patch              *calendar.Event
	Changed            bool
}

func (f calendarUpdateFields) focusEventType() bool {
	return f.FocusAutoDecline || f.FocusDeclineMessage || f.FocusChatStatus
}

func (f calendarUpdateFields) outOfOfficeEventType() bool {
	return f.OOOAutoDecline || f.OOODeclineMessage
}

func (f calendarUpdateFields) workingLocationEventType() bool {
	return f.WorkingLocationType ||
		f.WorkingOfficeLabel ||
		f.WorkingBuildingID ||
		f.WorkingFloorID ||
		f.WorkingDeskID ||
		f.WorkingCustomLabel
}

func (f calendarUpdateFields) zoomMutation() bool {
	return f.WithZoom || f.RegenerateZoom || f.RemoveZoom
}

func buildCalendarUpdatePlan(store *config.ConfigStore, input calendarUpdateInput, fields calendarUpdateFields) (*calendarUpdatePlan, error) {
	calendarID, err := prepareCalendarID(store, input.CalendarID, false)
	if err != nil {
		return nil, err
	}
	eventID := normalizeCalendarEventID(input.EventID)
	if eventID == "" {
		return nil, usage("empty eventId")
	}

	scope, err := resolveRecurringScope(input.Scope, input.OriginalStartTime)
	if err != nil {
		return nil, err
	}
	if fields.AllDay && (!fields.From || !fields.To) {
		return nil, usage("when changing --all-day, also provide --from and --to")
	}
	if fields.Attendees && fields.AddAttendee {
		return nil, usage("cannot use both --attendees and --add-attendee; use --attendees to replace all, or --add-attendee to add")
	}
	if fields.WithMeet && fields.RegenerateMeet {
		return nil, usage("use only one of --with-meet or --regenerate-meet")
	}
	if mutexErr := validateZoomConferenceFlagMutex(fields); mutexErr != nil {
		return nil, mutexErr
	}

	placeLookup, err := validateCalendarPlaceLookup(calendarPlaceLookup{
		LocationSet:       fields.Location,
		LocationSearch:    input.LocationSearch,
		LocationSearchSet: fields.LocationSearch,
		PlaceID:           input.PlaceID,
		PlaceIDSet:        fields.PlaceID,
		LanguageCode:      input.PlaceLanguage,
		RegionCode:        input.PlaceRegion,
	})
	if err != nil {
		return nil, err
	}
	sendUpdates, err := validateSendUpdates(input.SendUpdates)
	if err != nil {
		return nil, err
	}
	patch, changed, err := buildCalendarUpdatePatch(input, fields)
	if err != nil {
		return nil, err
	}

	addAttendee := strings.TrimSpace(input.AddAttendee)
	if fields.AddAttendee && addAttendee == "" {
		return nil, usage("empty --add-attendee")
	}
	if !changed && !fields.AddAttendee && placeLookup == nil {
		return nil, usage("no updates provided")
	}

	return &calendarUpdatePlan{
		CalendarID:         calendarID,
		EventID:            eventID,
		Scope:              scope,
		OriginalStartTime:  strings.TrimSpace(input.OriginalStartTime),
		SendUpdates:        sendUpdates,
		AddAttendee:        addAttendee,
		WantsAddAttendee:   fields.AddAttendee,
		RecurrenceProvided: fields.Recurrence,
		Fields:             fields,
		PlaceLookup:        placeLookup,
		Patch:              patch,
		Changed:            changed,
	}, nil
}

func (p *calendarUpdatePlan) dryRunRequest() map[string]any {
	request := map[string]any{
		"calendar_id":          p.CalendarID,
		"event_id":             p.EventID,
		"send_updates":         p.SendUpdates,
		"scope":                p.Scope,
		"original_start_time":  p.OriginalStartTime,
		"add_attendee":         p.AddAttendee,
		"patch":                p.Patch,
		"wants_add_attendee":   p.WantsAddAttendee,
		"conference_version_1": patchHasConferenceDataMutation(p.Patch),
		"supports_attachments": patchHasAttachmentsMutation(p.Patch),
	}
	if p.PlaceLookup != nil {
		request["place_lookup"] = p.PlaceLookup.dryRunPayload()
	}
	if zoomPayload := zoomUpdateDryRunPayload(p.Fields); zoomPayload != nil {
		request["zoom"] = zoomPayload
	}
	return request
}

func buildCalendarCreatePlan(store *config.ConfigStore, input calendarCreateInput, fields calendarCreateFields) (*calendarCreatePlan, error) {
	if fields.WithMeet && fields.WithZoom {
		return nil, usage("use only one of --with-zoom or --with-meet")
	}
	placeLookup, err := validateCalendarPlaceLookup(calendarPlaceLookup{
		LocationSet:       fields.Location || strings.TrimSpace(input.Location) != "",
		LocationSearch:    input.LocationSearch,
		LocationSearchSet: fields.LocationSearch,
		PlaceID:           input.PlaceID,
		PlaceIDSet:        fields.PlaceID,
		LanguageCode:      input.PlaceLanguage,
		RegionCode:        input.PlaceRegion,
	})
	if err != nil {
		return nil, err
	}
	calendarID, err := prepareCalendarID(store, input.CalendarID, false)
	if err != nil {
		return nil, err
	}
	eventType, err := resolveCreateEventType(input)
	if err != nil {
		return nil, err
	}

	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = defaultCreateSummaryForEventType(input, eventType)
	}
	if summary == "" || strings.TrimSpace(input.From) == "" || strings.TrimSpace(input.To) == "" {
		return nil, usage("required: --summary, --from, --to")
	}

	colorID, err := validateColorId(input.ColorID)
	if err != nil {
		return nil, err
	}
	visibility, err := validateVisibility(input.Visibility)
	if err != nil {
		return nil, err
	}
	transparency, err := validateTransparency(input.Transparency)
	if err != nil {
		return nil, err
	}
	sendUpdates, err := validateSendUpdates(input.SendUpdates)
	if err != nil {
		return nil, err
	}
	reminders, err := buildReminders(input.Reminders)
	if err != nil {
		return nil, err
	}
	allDay, err := resolveCreateAllDay(input.From, input.To, input.AllDay, eventType)
	if err != nil {
		return nil, err
	}
	startTZ, startTZFlag, endTZ, endTZFlag, err := resolveUnifiedTimezone(input.Timezone, input.StartTimezone, input.EndTimezone)
	if err != nil {
		return nil, err
	}
	start, err := buildEventDateTimeWithTimezone(input.From, allDay, startTZ, startTZFlag)
	if err != nil {
		return nil, err
	}
	end, err := buildEventDateTimeWithTimezone(input.To, allDay, endTZ, endTZFlag)
	if err != nil {
		return nil, err
	}

	event := &calendar.Event{
		Summary:            summary,
		Description:        strings.TrimSpace(input.Description),
		Location:           strings.TrimSpace(input.Location),
		Start:              start,
		End:                end,
		Attendees:          buildAttendees(input.Attendees),
		Recurrence:         buildRecurrence(input.Recurrence),
		Reminders:          reminders,
		ColorId:            colorID,
		Visibility:         applyEventTypeVisibilityDefault(visibility, eventType),
		Transparency:       applyEventTypeTransparencyDefault(transparency, eventType),
		ConferenceData:     buildConferenceData(conferenceChoice{provider: conferenceProvider(input.WithMeet, input.WithZoom)}),
		Attachments:        buildAttachments(input.Attachments),
		ExtendedProperties: buildExtendedProperties(input.PrivateProps, input.SharedProps),
	}
	if input.GuestsCanInviteOthers != nil {
		event.GuestsCanInviteOthers = input.GuestsCanInviteOthers
	}
	if input.GuestsCanModify != nil {
		event.GuestsCanModify = *input.GuestsCanModify
	}
	if input.GuestsCanSeeOthers != nil {
		event.GuestsCanSeeOtherGuests = input.GuestsCanSeeOthers
	}
	if strings.TrimSpace(input.SourceURL) != "" {
		event.Source = &calendar.EventSource{
			Url:   strings.TrimSpace(input.SourceURL),
			Title: strings.TrimSpace(input.SourceTitle),
		}
	}
	if input.ResolvedPlace != nil {
		event.Location = formatCalendarPlaceLocation(input.ResolvedPlace)
		applyCalendarPlaceProperties(event, input.ResolvedPlace)
	}

	if err := applyCreateEventType(event, input, eventType); err != nil {
		return nil, err
	}

	return &calendarCreatePlan{
		CalendarID:  calendarID,
		SendUpdates: sendUpdates,
		WithMeet:    input.WithMeet,
		WithZoom:    input.WithZoom,
		PlaceLookup: placeLookup,
		Event:       event,
	}, nil
}

func (p *calendarCreatePlan) dryRunRequest() map[string]any {
	request := map[string]any{
		"calendar_id":          p.CalendarID,
		"send_updates":         p.SendUpdates,
		"conference_version_1": p.WithMeet,
		"supports_attachments": len(p.Event.Attachments) > 0,
		"event":                p.Event,
	}
	if p.WithZoom {
		request["zoom"] = zoomDryRunPayload("create")
	}
	if p.PlaceLookup != nil {
		request["place_lookup"] = p.PlaceLookup.dryRunPayload()
	}
	return request
}

func resolveCreateEventType(input calendarCreateInput) (string, error) {
	focusFlags := strings.TrimSpace(input.FocusAutoDecline) != "" ||
		strings.TrimSpace(input.FocusDeclineMessage) != "" ||
		strings.TrimSpace(input.FocusChatStatus) != ""
	oooFlags := strings.TrimSpace(input.OOOAutoDecline) != "" ||
		strings.TrimSpace(input.OOODeclineMessage) != ""
	workingFlags := strings.TrimSpace(input.WorkingLocationType) != "" ||
		strings.TrimSpace(input.WorkingOfficeLabel) != "" ||
		strings.TrimSpace(input.WorkingBuildingID) != "" ||
		strings.TrimSpace(input.WorkingFloorID) != "" ||
		strings.TrimSpace(input.WorkingDeskID) != "" ||
		strings.TrimSpace(input.WorkingCustomLabel) != ""

	return resolveEventType(input.EventType, focusFlags, oooFlags, workingFlags)
}

func defaultCreateSummaryForEventType(input calendarCreateInput, eventType string) string {
	switch eventType {
	case eventTypeFocusTime:
		return defaultFocusSummary
	case eventTypeOutOfOffice:
		return defaultOOOSummary
	case eventTypeWorkingLocation:
		return workingLocationSummary(workingLocationInput{
			Type:        input.WorkingLocationType,
			OfficeLabel: input.WorkingOfficeLabel,
			CustomLabel: input.WorkingCustomLabel,
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

func applyCreateEventType(event *calendar.Event, input calendarCreateInput, eventType string) error {
	switch eventType {
	case eventTypeDefault:
		event.EventType = eventTypeDefault
	case eventTypeFocusTime:
		props, err := buildFocusTimeProperties(focusTimeInput{
			AutoDecline:    input.FocusAutoDecline,
			DeclineMessage: input.FocusDeclineMessage,
			ChatStatus:     input.FocusChatStatus,
		})
		if err != nil {
			return err
		}
		event.EventType = eventTypeFocusTime
		event.FocusTimeProperties = props
	case eventTypeOutOfOffice:
		props, err := buildOutOfOfficeProperties(outOfOfficeInput{
			AutoDecline:            input.OOOAutoDecline,
			DeclineMessage:         input.OOODeclineMessage,
			DeclineMessageProvided: false,
		})
		if err != nil {
			return err
		}
		event.EventType = eventTypeOutOfOffice
		event.OutOfOfficeProperties = props
	case eventTypeWorkingLocation:
		props, err := buildWorkingLocationProperties(workingLocationInput{
			Type:        input.WorkingLocationType,
			OfficeLabel: input.WorkingOfficeLabel,
			BuildingId:  input.WorkingBuildingID,
			FloorId:     input.WorkingFloorID,
			DeskId:      input.WorkingDeskID,
			CustomLabel: input.WorkingCustomLabel,
		})
		if err != nil {
			return err
		}
		event.EventType = eventTypeWorkingLocation
		event.WorkingLocationProperties = props
	}
	return nil
}

func conferenceProvider(withMeet, withZoom bool) string {
	switch {
	case withMeet:
		return conferenceProviderMeet
	case withZoom:
		return conferenceProviderZoom
	default:
		return ""
	}
}

func buildFocusTimeProperties(input focusTimeInput) (*calendar.EventFocusTimeProperties, error) {
	autoDecline := strings.TrimSpace(input.AutoDecline)
	if autoDecline == "" {
		autoDecline = defaultFocusAutoDecline
	}
	autoDeclineMode, err := validateAutoDeclineMode(autoDecline)
	if err != nil {
		return nil, err
	}

	chatStatus := strings.TrimSpace(input.ChatStatus)
	if chatStatus == "" {
		chatStatus = defaultFocusChatStatus
	}
	chatStatusValue, err := validateChatStatus(chatStatus)
	if err != nil {
		return nil, err
	}

	return &calendar.EventFocusTimeProperties{
		AutoDeclineMode: autoDeclineMode,
		DeclineMessage:  strings.TrimSpace(input.DeclineMessage),
		ChatStatus:      chatStatusValue,
	}, nil
}

func buildOutOfOfficeProperties(input outOfOfficeInput) (*calendar.EventOutOfOfficeProperties, error) {
	autoDecline := strings.TrimSpace(input.AutoDecline)
	if autoDecline == "" {
		autoDecline = defaultOOOAutoDecline
	}
	autoDeclineMode, err := validateAutoDeclineMode(autoDecline)
	if err != nil {
		return nil, err
	}

	declineMessage := strings.TrimSpace(input.DeclineMessage)
	if declineMessage == "" && !input.DeclineMessageProvided {
		declineMessage = defaultOOODeclineMsg
	}

	return &calendar.EventOutOfOfficeProperties{
		AutoDeclineMode: autoDeclineMode,
		DeclineMessage:  declineMessage,
	}, nil
}
