package cmd

import (
	"io"
	"testing"

	"github.com/alecthomas/kong"
	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/googleauth"
)

func parseKongContext(t *testing.T, cmd any, args []string) *kong.Context {
	t.Helper()

	parser, err := kong.New(
		cmd,
		kong.Vars(kong.Vars{
			"auth_services": googleauth.UserServiceCSV(),
		}),
		kong.Writers(io.Discard, io.Discard),
	)
	if err != nil {
		t.Fatalf("kong new: %v", err)
	}

	kctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("kong parse: %v", err)
	}

	return kctx
}

func hasForceSendField(fields []string, field string) bool {
	for _, f := range fields {
		if f == field {
			return true
		}
	}
	return false
}

func TestCalendarUpdatePatchClearsRecurrence(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	kctx := parseKongContext(t, cmd, []string{"cal1", "evt1", "--rrule", " "})

	patch, _, err := cmd.buildUpdatePatch(kctx)
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if patch == nil {
		t.Fatal("expected patch")
	}
	if patch.Recurrence == nil || len(patch.Recurrence) != 0 {
		t.Fatalf("expected empty recurrence, got %#v", patch.Recurrence)
	}
	if !hasForceSendField(patch.ForceSendFields, "Recurrence") {
		t.Fatalf("expected Recurrence in ForceSendFields")
	}
}

func TestCalendarUpdatePatchClearsReminders(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	kctx := parseKongContext(t, cmd, []string{"cal1", "evt1", "--reminder", " "})

	patch, _, err := cmd.buildUpdatePatch(kctx)
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if patch == nil {
		t.Fatal("expected patch")
	}
	if patch.Reminders == nil || !patch.Reminders.UseDefault {
		t.Fatalf("expected reminders.UseDefault=true, got %#v", patch.Reminders)
	}
	if !hasForceSendField(patch.ForceSendFields, "Reminders") {
		t.Fatalf("expected Reminders in ForceSendFields")
	}
}

func TestApplyCreateEventType_Default(t *testing.T) {
	cmd := &CalendarCreateCmd{}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeDefault)
	if err != nil {
		t.Fatalf("applyCreateEventType: %v", err)
	}
	if event.EventType != eventTypeDefault {
		t.Fatalf("expected event type %q, got %q", eventTypeDefault, event.EventType)
	}
}

func TestApplyCreateEventType_FocusTime(t *testing.T) {
	cmd := &CalendarCreateCmd{
		FocusAutoDecline:  "all",
		FocusDeclineMessage: "I'm busy",
		FocusChatStatus:   "doNotDisturb",
	}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeFocusTime)
	if err != nil {
		t.Fatalf("applyCreateEventType: %v", err)
	}
	if event.EventType != eventTypeFocusTime {
		t.Fatalf("expected event type %q, got %q", eventTypeFocusTime, event.EventType)
	}
	if event.FocusTimeProperties == nil {
		t.Fatal("expected FocusTimeProperties to be set")
	}
	if event.FocusTimeProperties.AutoDeclineMode != "declineAllConflictingInvitations" {
		t.Fatalf("unexpected AutoDeclineMode: %q", event.FocusTimeProperties.AutoDeclineMode)
	}
	if event.FocusTimeProperties.DeclineMessage != "I'm busy" {
		t.Fatalf("unexpected DeclineMessage: %q", event.FocusTimeProperties.DeclineMessage)
	}
	if event.FocusTimeProperties.ChatStatus != "doNotDisturb" {
		t.Fatalf("unexpected ChatStatus: %q", event.FocusTimeProperties.ChatStatus)
	}
}

func TestApplyCreateEventType_FocusTimeDefaults(t *testing.T) {
	cmd := &CalendarCreateCmd{}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeFocusTime)
	if err != nil {
		t.Fatalf("applyCreateEventType: %v", err)
	}
	if event.FocusTimeProperties == nil {
		t.Fatal("expected FocusTimeProperties to be set")
	}
	// Default auto decline mode should be "all" -> "declineAllConflictingInvitations"
	if event.FocusTimeProperties.AutoDeclineMode != "declineAllConflictingInvitations" {
		t.Fatalf("unexpected default AutoDeclineMode: %q", event.FocusTimeProperties.AutoDeclineMode)
	}
	// Default chat status should be "doNotDisturb"
	if event.FocusTimeProperties.ChatStatus != defaultFocusChatStatus {
		t.Fatalf("unexpected default ChatStatus: %q", event.FocusTimeProperties.ChatStatus)
	}
}

func TestApplyCreateEventType_OutOfOffice(t *testing.T) {
	cmd := &CalendarCreateCmd{
		OOOAutoDecline:  "new",
		OOODeclineMessage: "On vacation",
	}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeOutOfOffice)
	if err != nil {
		t.Fatalf("applyCreateEventType: %v", err)
	}
	if event.EventType != eventTypeOutOfOffice {
		t.Fatalf("expected event type %q, got %q", eventTypeOutOfOffice, event.EventType)
	}
	if event.OutOfOfficeProperties == nil {
		t.Fatal("expected OutOfOfficeProperties to be set")
	}
	if event.OutOfOfficeProperties.AutoDeclineMode != "declineOnlyNewConflictingInvitations" {
		t.Fatalf("unexpected AutoDeclineMode: %q", event.OutOfOfficeProperties.AutoDeclineMode)
	}
	if event.OutOfOfficeProperties.DeclineMessage != "On vacation" {
		t.Fatalf("unexpected DeclineMessage: %q", event.OutOfOfficeProperties.DeclineMessage)
	}
}

func TestApplyCreateEventType_OutOfOfficeDefaults(t *testing.T) {
	cmd := &CalendarCreateCmd{}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeOutOfOffice)
	if err != nil {
		t.Fatalf("applyCreateEventType: %v", err)
	}
	if event.OutOfOfficeProperties == nil {
		t.Fatal("expected OutOfOfficeProperties to be set")
	}
	if event.OutOfOfficeProperties.DeclineMessage != defaultOOODeclineMsg {
		t.Fatalf("unexpected default DeclineMessage: %q", event.OutOfOfficeProperties.DeclineMessage)
	}
}

func TestApplyCreateEventType_WorkingLocation_Home(t *testing.T) {
	cmd := &CalendarCreateCmd{
		WorkingLocationType: "home",
	}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeWorkingLocation)
	if err != nil {
		t.Fatalf("applyCreateEventType: %v", err)
	}
	if event.EventType != eventTypeWorkingLocation {
		t.Fatalf("expected event type %q, got %q", eventTypeWorkingLocation, event.EventType)
	}
	if event.WorkingLocationProperties == nil {
		t.Fatal("expected WorkingLocationProperties to be set")
	}
	if event.WorkingLocationProperties.Type != "homeOffice" {
		t.Fatalf("unexpected working location type: %q", event.WorkingLocationProperties.Type)
	}
}

func TestApplyCreateEventType_WorkingLocation_Office(t *testing.T) {
	cmd := &CalendarCreateCmd{
		WorkingLocationType: "office",
		WorkingOfficeLabel:  "HQ",
		WorkingBuildingId:   "building-123",
		WorkingFloorId:      "floor-2",
		WorkingDeskId:       "desk-42",
	}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeWorkingLocation)
	if err != nil {
		t.Fatalf("applyCreateEventType: %v", err)
	}
	if event.WorkingLocationProperties == nil {
		t.Fatal("expected WorkingLocationProperties to be set")
	}
	if event.WorkingLocationProperties.Type != "officeLocation" {
		t.Fatalf("unexpected working location type: %q", event.WorkingLocationProperties.Type)
	}
	office := event.WorkingLocationProperties.OfficeLocation
	if office == nil {
		t.Fatal("expected OfficeLocation to be set")
	}
	if office.Label != "HQ" {
		t.Fatalf("unexpected office label: %q", office.Label)
	}
	if office.BuildingId != "building-123" {
		t.Fatalf("unexpected building id: %q", office.BuildingId)
	}
	if office.FloorId != "floor-2" {
		t.Fatalf("unexpected floor id: %q", office.FloorId)
	}
	if office.DeskId != "desk-42" {
		t.Fatalf("unexpected desk id: %q", office.DeskId)
	}
}

func TestApplyCreateEventType_WorkingLocation_Custom(t *testing.T) {
	cmd := &CalendarCreateCmd{
		WorkingLocationType: "custom",
		WorkingCustomLabel:  "Coffee Shop",
	}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeWorkingLocation)
	if err != nil {
		t.Fatalf("applyCreateEventType: %v", err)
	}
	if event.WorkingLocationProperties == nil {
		t.Fatal("expected WorkingLocationProperties to be set")
	}
	if event.WorkingLocationProperties.Type != "customLocation" {
		t.Fatalf("unexpected working location type: %q", event.WorkingLocationProperties.Type)
	}
	if event.WorkingLocationProperties.CustomLocation == nil {
		t.Fatal("expected CustomLocation to be set")
	}
	if event.WorkingLocationProperties.CustomLocation.Label != "Coffee Shop" {
		t.Fatalf("unexpected custom label: %q", event.WorkingLocationProperties.CustomLocation.Label)
	}
}

func TestApplyCreateEventType_WorkingLocation_MissingType(t *testing.T) {
	cmd := &CalendarCreateCmd{}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeWorkingLocation)
	if err == nil {
		t.Fatal("expected error for missing working location type")
	}
}

func TestApplyCreateEventType_InvalidAutoDeclineMode(t *testing.T) {
	cmd := &CalendarCreateCmd{
		FocusAutoDecline: "invalid",
	}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeFocusTime)
	if err == nil {
		t.Fatal("expected error for invalid auto decline mode")
	}
}

func TestApplyCreateEventType_InvalidChatStatus(t *testing.T) {
	cmd := &CalendarCreateCmd{
		FocusChatStatus: "invalid",
	}
	event := &calendar.Event{}

	err := cmd.applyCreateEventType(event, eventTypeFocusTime)
	if err == nil {
		t.Fatal("expected error for invalid chat status")
	}
}

func TestResolveCreateAllDay(t *testing.T) {
	tests := []struct {
		name      string
		from      string
		to        string
		allDay    bool
		eventType string
		want      bool
		wantErr   bool
	}{
		{
			name:      "default event, allDay=false",
			from:      "2025-01-01T10:00:00Z",
			to:        "2025-01-01T11:00:00Z",
			allDay:    false,
			eventType: eventTypeDefault,
			want:      false,
			wantErr:   false,
		},
		{
			name:      "default event, allDay=true",
			from:      "2025-01-01",
			to:        "2025-01-02",
			allDay:    true,
			eventType: eventTypeDefault,
			want:      true,
			wantErr:   false,
		},
		{
			name:      "working location with date-only",
			from:      "2025-01-01",
			to:        "2025-01-02",
			allDay:    false,
			eventType: eventTypeWorkingLocation,
			want:      true,
			wantErr:   false,
		},
		{
			name:      "working location with datetime (error)",
			from:      "2025-01-01T10:00:00Z",
			to:        "2025-01-02T10:00:00Z",
			allDay:    false,
			eventType: eventTypeWorkingLocation,
			want:      false,
			wantErr:   true,
		},
		{
			name:      "focus time, allDay=false",
			from:      "2025-01-01T10:00:00Z",
			to:        "2025-01-01T11:00:00Z",
			allDay:    false,
			eventType: eventTypeFocusTime,
			want:      false,
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveCreateAllDay(tc.from, tc.to, tc.allDay, tc.eventType)
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolveCreateAllDay() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("resolveCreateAllDay() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplyEventTypeTransparencyDefault(t *testing.T) {
	tests := []struct {
		name         string
		transparency string
		eventType    string
		want         string
	}{
		{
			name:         "focus time with no transparency",
			transparency: "",
			eventType:    eventTypeFocusTime,
			want:         transparencyOpaque,
		},
		{
			name:         "out of office with no transparency",
			transparency: "",
			eventType:    eventTypeOutOfOffice,
			want:         transparencyOpaque,
		},
		{
			name:         "focus time with transparency set",
			transparency: "transparent",
			eventType:    eventTypeFocusTime,
			want:         "transparent",
		},
		{
			name:         "default event type",
			transparency: "",
			eventType:    eventTypeDefault,
			want:         "",
		},
		{
			name:         "working location",
			transparency: "",
			eventType:    eventTypeWorkingLocation,
			want:         "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := applyEventTypeTransparencyDefault(tc.transparency, tc.eventType)
			if got != tc.want {
				t.Fatalf("applyEventTypeTransparencyDefault() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCalendarCreateCmd_ResolveCreateEventType(t *testing.T) {
	tests := []struct {
		name    string
		cmd     CalendarCreateCmd
		want    string
		wantErr bool
	}{
		{
			name: "explicit focus-time",
			cmd:  CalendarCreateCmd{EventType: "focus-time"},
			want: eventTypeFocusTime,
		},
		{
			name: "explicit out-of-office",
			cmd:  CalendarCreateCmd{EventType: "ooo"},
			want: eventTypeOutOfOffice,
		},
		{
			name: "inferred from focus flags",
			cmd:  CalendarCreateCmd{FocusAutoDecline: "all"},
			want: eventTypeFocusTime,
		},
		{
			name: "inferred from ooo flags",
			cmd:  CalendarCreateCmd{OOODeclineMessage: "Gone"},
			want: eventTypeOutOfOffice,
		},
		{
			name: "inferred from working location flags",
			cmd:  CalendarCreateCmd{WorkingLocationType: "home"},
			want: eventTypeWorkingLocation,
		},
		{
			name:    "mixed flags error",
			cmd:     CalendarCreateCmd{FocusAutoDecline: "all", OOODeclineMessage: "Gone"},
			wantErr: true,
		},
		{
			name:    "focus-time with ooo flags error",
			cmd:     CalendarCreateCmd{EventType: "focus-time", OOODeclineMessage: "Gone"},
			wantErr: true,
		},
		{
			name: "no event type flags",
			cmd:  CalendarCreateCmd{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.cmd.resolveCreateEventType()
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolveCreateEventType() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("resolveCreateEventType() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCalendarCreateCmd_DefaultSummaryForEventType(t *testing.T) {
	tests := []struct {
		name      string
		cmd       CalendarCreateCmd
		eventType string
		want      string
	}{
		{
			name:      "focus time",
			cmd:       CalendarCreateCmd{},
			eventType: eventTypeFocusTime,
			want:      defaultFocusSummary,
		},
		{
			name:      "out of office",
			cmd:       CalendarCreateCmd{},
			eventType: eventTypeOutOfOffice,
			want:      defaultOOOSummary,
		},
		{
			name:      "working location home",
			cmd:       CalendarCreateCmd{WorkingLocationType: "home"},
			eventType: eventTypeWorkingLocation,
			want:      "Working from home",
		},
		{
			name:      "working location office",
			cmd:       CalendarCreateCmd{WorkingLocationType: "office", WorkingOfficeLabel: "HQ"},
			eventType: eventTypeWorkingLocation,
			want:      "Working from HQ",
		},
		{
			name:      "working location custom",
			cmd:       CalendarCreateCmd{WorkingLocationType: "custom", WorkingCustomLabel: "Cafe"},
			eventType: eventTypeWorkingLocation,
			want:      "Working from Cafe",
		},
		{
			name:      "default event type",
			cmd:       CalendarCreateCmd{},
			eventType: eventTypeDefault,
			want:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cmd.defaultSummaryForEventType(tc.eventType)
			if got != tc.want {
				t.Fatalf("defaultSummaryForEventType() = %q, want %q", got, tc.want)
			}
		})
	}
}
