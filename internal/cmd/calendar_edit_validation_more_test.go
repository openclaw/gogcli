package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestCalendarCreateCmd_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	cases := []struct {
		name string
		cmd  CalendarCreateCmd
	}{
		{"missing calendar", CalendarCreateCmd{}},
		{"missing summary", CalendarCreateCmd{CalendarID: "cal1", From: "2025-01-01T00:00:00Z", To: "2025-01-01T01:00:00Z"}},
		{"invalid event type", CalendarCreateCmd{CalendarID: "cal1", Summary: "S", From: "2025-01-01T00:00:00Z", To: "2025-01-01T01:00:00Z", EventType: "nope"}},
		{"working location missing type", CalendarCreateCmd{CalendarID: "cal1", EventType: "working-location", From: "2025-01-01", To: "2025-01-02"}},
		{"working location with time", CalendarCreateCmd{CalendarID: "cal1", EventType: "working-location", From: "2025-01-01T00:00:00Z", To: "2025-01-02T00:00:00Z", WorkingLocationType: "home"}},
		{"invalid color", CalendarCreateCmd{CalendarID: "cal1", Summary: "S", From: "2025-01-01T00:00:00Z", To: "2025-01-01T01:00:00Z", ColorId: "12"}},
		{"invalid visibility", CalendarCreateCmd{CalendarID: "cal1", Summary: "S", From: "2025-01-01T00:00:00Z", To: "2025-01-01T01:00:00Z", Visibility: "nope"}},
		{"invalid transparency", CalendarCreateCmd{CalendarID: "cal1", Summary: "S", From: "2025-01-01T00:00:00Z", To: "2025-01-01T01:00:00Z", Transparency: "nope"}},
		{"invalid send updates", CalendarCreateCmd{CalendarID: "cal1", Summary: "S", From: "2025-01-01T00:00:00Z", To: "2025-01-01T01:00:00Z", SendUpdates: "nope"}},
		{"invalid reminders", CalendarCreateCmd{CalendarID: "cal1", Summary: "S", From: "2025-01-01T00:00:00Z", To: "2025-01-01T01:00:00Z", Reminders: []string{"bad"}}},
	}

	for _, tc := range cases {
		if err := tc.cmd.Run(ctx, flags); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestCalendarCreateCmd_WithExtras(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/events") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt1",
				"summary": "Created",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "a@b.com"}

	yes := true
	no := false
	cmd := &CalendarCreateCmd{
		CalendarID:            "cal1@example.com",
		Summary:               "Title",
		From:                  "2025-01-01T00:00:00Z",
		To:                    "2025-01-01T01:00:00Z",
		Description:           "Desc",
		Location:              "Room",
		Attendees:             "a@b.com",
		AllDay:                false,
		Recurrence:            []string{"RRULE:FREQ=DAILY;COUNT=2"},
		Reminders:             []string{"popup:30m"},
		ColorId:               "1",
		Visibility:            "private",
		Transparency:          "opaque",
		SendUpdates:           "all",
		GuestsCanInviteOthers: &yes,
		GuestsCanModify:       &no,
		GuestsCanSeeOthers:    &yes,
		WithMeet:              true,
		SourceUrl:             "https://example.com",
		SourceTitle:           "Import",
		Attachments:           []string{"https://example.com/file"},
		PrivateProps:          []string{"k=v"},
		SharedProps:           []string{"s=v"},
	}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "\"event\"") {
		t.Fatalf("unexpected json output: %q", out)
	}
}

func TestCalendarUpdateCmd_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	{
		cmd := &CalendarUpdateCmd{CalendarID: "", EventID: "evt"}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for missing calendarId")
		}
	}
	{
		cmd := &CalendarUpdateCmd{CalendarID: "cal", EventID: ""}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for missing eventId")
		}
	}
	{
		cmd := &CalendarUpdateCmd{CalendarID: "cal", EventID: "evt", Scope: "nope"}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--scope", "nope"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for invalid scope")
		}
	}
	{
		cmd := &CalendarUpdateCmd{CalendarID: "cal", EventID: "evt", Scope: scopeSingle}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--scope", "single"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for missing original-start")
		}
	}
	{
		cmd := &CalendarUpdateCmd{}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--all-day"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for all-day without from/to")
		}
	}
	{
		cmd := &CalendarUpdateCmd{}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--attendees", "a@b.com", "--add-attendee", "b@b.com"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for attendees + add-attendee")
		}
	}
	{
		cmd := &CalendarUpdateCmd{}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--add-attendee", " "})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for empty add-attendee")
		}
	}
	{
		cmd := &CalendarUpdateCmd{}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for no updates")
		}
	}
	{
		cmd := &CalendarUpdateCmd{CalendarID: "cal", EventID: "evt", EventType: "nope"}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--event-type", "nope"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for invalid event-type")
		}
	}
	{
		cmd := &CalendarUpdateCmd{CalendarID: "cal", EventID: "evt", EventType: "working-location"}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--event-type", "working-location"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for working-location without type")
		}
	}
	{
		cmd := &CalendarUpdateCmd{CalendarID: "cal", EventID: "evt", EventType: "working-location", WorkingLocationType: "home", From: "2025-01-01T00:00:00Z", To: "2025-01-02T00:00:00Z"}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--event-type", "working-location", "--working-location-type", "home", "--from", "2025-01-01T00:00:00Z", "--to", "2025-01-02T00:00:00Z"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for working-location with time range")
		}
	}
	{
		cmd := &CalendarUpdateCmd{CalendarID: "cal", EventID: "evt", Summary: "S", SendUpdates: "invalid"}
		kctx := parseKongContext(t, cmd, []string{"cal", "evt", "--summary", "S", "--send-updates", "invalid"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected error for invalid send-updates")
		}
	}
}

func TestCalendarDeleteCmd_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	cases := []struct {
		name string
		cmd  CalendarDeleteCmd
	}{
		{"missing calendar", CalendarDeleteCmd{}},
		{"missing event", CalendarDeleteCmd{CalendarID: "cal"}},
		{"invalid scope", CalendarDeleteCmd{CalendarID: "cal", EventID: "evt", Scope: "nope"}},
		{"scope single missing original", CalendarDeleteCmd{CalendarID: "cal", EventID: "evt", Scope: scopeSingle}},
		{"scope future missing original", CalendarDeleteCmd{CalendarID: "cal", EventID: "evt", Scope: scopeFuture}},
		{"invalid send-updates", CalendarDeleteCmd{CalendarID: "cal", EventID: "evt", SendUpdates: "invalid"}},
	}

	for _, tc := range cases {
		if err := tc.cmd.Run(ctx, flags); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}
