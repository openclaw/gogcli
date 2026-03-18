package cmd

import (
	"strings"

	"google.golang.org/api/calendar/v3"
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

type calendarCreatePlan struct {
	CalendarID  string
	SendUpdates string
	WithMeet    bool
	Event       *calendar.Event
}

func buildCalendarCreatePlan(c *CalendarCreateCmd) (*calendarCreatePlan, error) {
	eventType, err := c.resolveCreateEventType()
	if err != nil {
		return nil, err
	}

	summary := strings.TrimSpace(c.Summary)
	if summary == "" {
		summary = c.defaultSummaryForEventType(eventType)
	}
	if summary == "" || strings.TrimSpace(c.From) == "" || strings.TrimSpace(c.To) == "" {
		return nil, usage("required: --summary, --from, --to")
	}

	colorID, err := validateColorId(c.ColorId)
	if err != nil {
		return nil, err
	}
	visibility, err := validateVisibility(c.Visibility)
	if err != nil {
		return nil, err
	}
	transparency, err := validateTransparency(c.Transparency)
	if err != nil {
		return nil, err
	}
	sendUpdates, err := validateSendUpdates(c.SendUpdates)
	if err != nil {
		return nil, err
	}
	reminders, err := buildReminders(c.Reminders)
	if err != nil {
		return nil, err
	}
	allDay, err := resolveCreateAllDay(c.From, c.To, c.AllDay, eventType)
	if err != nil {
		return nil, err
	}

	event := &calendar.Event{
		Summary:            summary,
		Description:        strings.TrimSpace(c.Description),
		Location:           strings.TrimSpace(c.Location),
		Start:              buildEventDateTime(c.From, allDay),
		End:                buildEventDateTime(c.To, allDay),
		Attendees:          buildAttendees(c.Attendees),
		Recurrence:         buildRecurrence(c.Recurrence),
		Reminders:          reminders,
		ColorId:            colorID,
		Visibility:         applyEventTypeVisibilityDefault(visibility, eventType),
		Transparency:       applyEventTypeTransparencyDefault(transparency, eventType),
		ConferenceData:     buildConferenceData(c.WithMeet),
		Attachments:        buildAttachments(c.Attachments),
		ExtendedProperties: buildExtendedProperties(c.PrivateProps, c.SharedProps),
	}
	if c.GuestsCanInviteOthers != nil {
		event.GuestsCanInviteOthers = c.GuestsCanInviteOthers
	}
	if c.GuestsCanModify != nil {
		event.GuestsCanModify = *c.GuestsCanModify
	}
	if c.GuestsCanSeeOthers != nil {
		event.GuestsCanSeeOtherGuests = c.GuestsCanSeeOthers
	}
	if strings.TrimSpace(c.SourceUrl) != "" {
		event.Source = &calendar.EventSource{
			Url:   strings.TrimSpace(c.SourceUrl),
			Title: strings.TrimSpace(c.SourceTitle),
		}
	}

	if err := c.applyCreateEventType(event, eventType); err != nil {
		return nil, err
	}

	return &calendarCreatePlan{
		CalendarID:  strings.TrimSpace(c.CalendarID),
		SendUpdates: sendUpdates,
		WithMeet:    c.WithMeet,
		Event:       event,
	}, nil
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
