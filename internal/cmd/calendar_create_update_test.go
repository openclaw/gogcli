package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func newCalendarServiceFromServer(t *testing.T, srv *httptest.Server) *calendar.Service {
	t.Helper()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestCalendarCreateCmd_RunJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev1",
				"summary": "Meeting",
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
	ctx, output := newCalendarTestJSONContext(t, svc)

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"--summary", "Meeting",
		"--from", "2025-01-02T10:00:00Z",
		"--to", "2025-01-02T11:00:00Z",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !strings.Contains(output.String(), "\"event\"") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestCalendarCreateCmd_WithMeetAndAttachments(t *testing.T) {
	var sawConference, sawAttachments bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			sawConference = body.ConferenceData != nil
			sawAttachments = len(body.Attachments) > 0
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev2",
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
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"--summary", "Meet",
		"--from", "2025-01-02T10:00:00Z",
		"--to", "2025-01-02T11:00:00Z",
		"--with-meet",
		"--attachment", "https://example.com/file",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !sawConference || !sawAttachments {
		t.Fatalf("expected conference+attachments, sawConference=%v sawAttachments=%v", sawConference, sawAttachments)
	}
}

func TestCalendarCreateCmd_RecurringOffsetTimezoneFallback(t *testing.T) {
	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/calendars/") && strings.HasSuffix(r.URL.Path, "/events"):
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev3",
			})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/calendars/") && !strings.Contains(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "primary",
				"timeZone": "UTC",
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
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"primary",
		"--summary", "Recurring Test",
		"--from", "2026-02-13T08:00:00+02:00",
		"--to", "2026-02-13T09:00:00+02:00",
		"--rrule", "FREQ=WEEKLY;BYDAY=TU,TH",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.Start == nil || gotEvent.Start.TimeZone != "Etc/GMT-2" {
		t.Fatalf("expected start timezone fallback Etc/GMT-2, got %#v", gotEvent.Start)
	}
	if gotEvent.End == nil || gotEvent.End.TimeZone != "Etc/GMT-2" {
		t.Fatalf("expected end timezone fallback Etc/GMT-2, got %#v", gotEvent.End)
	}
	if len(gotEvent.Recurrence) != 1 || gotEvent.Recurrence[0] != "FREQ=WEEKLY;BYDAY=TU,TH" {
		t.Fatalf("unexpected recurrence payload: %#v", gotEvent.Recurrence)
	}
}

func TestCalendarCreateCmd_ExplicitTimezones(t *testing.T) {
	plan, err := buildCalendarCreatePlan(defaultConfigStoreForTest(t), calendarCreateInput{
		CalendarID:    "primary",
		Summary:       "Flight",
		From:          "2026-08-13T13:40:00+02:00",
		To:            "2026-08-13T17:00:00-04:00",
		StartTimezone: "Europe/Rome",
		EndTimezone:   "America/New_York",
		SendUpdates:   "none",
		Transparency:  "opaque",
		Visibility:    "default",
	}, calendarCreateFields{})
	if err != nil {
		t.Fatalf("buildCalendarCreatePlan: %v", err)
	}
	if plan.Event.Start == nil || plan.Event.Start.TimeZone != "Europe/Rome" {
		t.Fatalf("expected start timezone Europe/Rome, got %#v", plan.Event.Start)
	}
	if plan.Event.End == nil || plan.Event.End.TimeZone != "America/New_York" {
		t.Fatalf("expected end timezone America/New_York, got %#v", plan.Event.End)
	}
}

func TestCalendarUpdateCmd_RecurrenceFillsMissingTimezone(t *testing.T) {
	var (
		gotPatch      calendar.Event
		currentLoaded bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			currentLoaded = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
				"start": map[string]any{
					"dateTime": "2026-03-03T20:00:00+01:00",
				},
				"end": map[string]any{
					"dateTime": "2026-03-03T20:30:00+01:00",
				},
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			_ = json.NewDecoder(r.Body).Decode(&gotPatch)
			if gotPatch.Start == nil || gotPatch.End == nil ||
				gotPatch.Start.TimeZone == "" || gotPatch.End.TimeZone == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    400,
						"message": "Missing time zone definition for start time.",
					},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
			})
			return
		case r.Method == http.MethodGet && path == "/users/me/calendarList/cal@example.com":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "cal@example.com",
				"timeZone": "UTC",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--rrule", "RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if !currentLoaded {
		t.Fatalf("expected existing event fetch for recurring timezone enrichment")
	}
	if gotPatch.Start == nil || gotPatch.Start.TimeZone != "Etc/GMT-1" {
		t.Fatalf("expected start timezone Etc/GMT-1, got %#v", gotPatch.Start)
	}
	if gotPatch.End == nil || gotPatch.End.TimeZone != "Etc/GMT-1" {
		t.Fatalf("expected end timezone Etc/GMT-1, got %#v", gotPatch.End)
	}
	if len(gotPatch.Recurrence) != 1 || gotPatch.Recurrence[0] != "RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR" {
		t.Fatalf("unexpected recurrence payload: %#v", gotPatch.Recurrence)
	}
}

func TestCalendarUpdateCmd_RunJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev",
				"summary": "Updated",
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
	ctx, output := newCalendarTestJSONContext(t, svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--summary", "Updated",
		"--scope", "all",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !strings.Contains(output.String(), "\"event\"") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestCalendarUpdateCmd_AttachmentsReplaceAndClear(t *testing.T) {
	var patchCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method != http.MethodPatch || path != "/calendars/cal@example.com/events/ev" {
			http.NotFound(w, r)
			return
		}
		patchCalls++
		if got := r.URL.Query().Get("supportsAttachments"); got != "true" {
			t.Fatalf("supportsAttachments = %q, want true", got)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode patch body: %v", err)
		}
		raw, ok := body["attachments"]
		if !ok {
			t.Fatalf("patch %d missing attachments: %#v", patchCalls, body)
		}
		var attachments []*calendar.EventAttachment
		if err := json.Unmarshal(raw, &attachments); err != nil {
			t.Fatalf("decode attachments: %v", err)
		}
		switch patchCalls {
		case 1:
			if len(attachments) != 2 ||
				attachments[0].FileUrl != "https://drive.google.com/file/d/one" ||
				attachments[1].FileUrl != "https://drive.google.com/file/d/two" {
				t.Fatalf("unexpected replacement attachments: %#v", attachments)
			}
		case 2:
			if attachments == nil || len(attachments) != 0 {
				t.Fatalf("expected explicit empty attachments array, got %#v", attachments)
			}
		default:
			t.Fatalf("unexpected patch call %d", patchCalls)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	if err := runKong(t, &CalendarUpdateCmd{}, []string{
		"cal@example.com", "ev",
		"--attachment", "https://drive.google.com/file/d/one",
		"--attachment", "https://drive.google.com/file/d/two",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("replace attachments: %v", err)
	}
	if err := runKong(t, &CalendarUpdateCmd{}, []string{
		"cal@example.com", "ev", "--attachment=",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("clear attachments: %v", err)
	}
	if patchCalls != 2 {
		t.Fatalf("patch calls = %d, want 2", patchCalls)
	}
}

func TestCalendarUpdateCmd_AttachmentClearDryRunReportsSupport(t *testing.T) {
	var output bytes.Buffer
	ctx := withCalendarTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &output, io.Discard),
		func(context.Context, string) (*calendar.Service, error) {
			t.Fatal("calendar service should not be created during dry-run")
			return nil, context.Canceled
		},
	)
	err := runKong(t, &CalendarUpdateCmd{}, []string{
		"primary", "ev", "--attachment=",
	}, ctx, &RootFlags{Account: "a@b.com", DryRun: true})
	if err != nil && ExitCode(err) != 0 {
		t.Fatalf("dry-run: %v", err)
	}

	var payload struct {
		Request struct {
			SupportsAttachments bool `json:"supports_attachments"`
			Patch               struct {
				Attachments []*calendar.EventAttachment `json:"attachments"`
			} `json:"patch"`
		} `json:"request"`
	}
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal dry-run: %v\n%s", err, output.String())
	}
	if !payload.Request.SupportsAttachments {
		t.Fatal("expected supports_attachments for clear")
	}
	if payload.Request.Patch.Attachments == nil || len(payload.Request.Patch.Attachments) != 0 {
		t.Fatalf("expected explicit empty attachment list: %#v", payload.Request.Patch.Attachments)
	}
}

func TestCalendarUpdateCmd_WithMeet(t *testing.T) {
	var (
		sawConferenceData bool
		sawVersion        bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			sawVersion = r.URL.Query().Get("conferenceDataVersion") == "1"
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			sawConferenceData = body.ConferenceData != nil &&
				body.ConferenceData.CreateRequest != nil &&
				body.ConferenceData.CreateRequest.ConferenceSolutionKey != nil &&
				body.ConferenceData.CreateRequest.ConferenceSolutionKey.Type == "hangoutsMeet"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "ev",
				"hangoutLink": "https://meet.google.com/aaa-bbbb-ccc",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--with-meet",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !sawConferenceData {
		t.Fatalf("expected conferenceData create request in patch body")
	}
	if !sawVersion {
		t.Fatalf("expected conferenceDataVersion=1 on patch request")
	}
}

func TestCalendarUpdateCmd_WithMeetExistingConferenceIsIdempotent(t *testing.T) {
	var sawPatch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "ev",
				"hangoutLink": "https://meet.google.com/existing",
				"conferenceData": map[string]any{
					"entryPoints": []map[string]any{
						{"entryPointType": calendarEntryPointTypeVideo, "uri": "https://meet.google.com/existing"},
					},
				},
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			sawPatch = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--with-meet",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if sawPatch {
		t.Fatalf("expected existing Meet link to skip patch")
	}
}

func TestCalendarUpdateCmd_RegenerateMeetReplacesConference(t *testing.T) {
	var sawPatch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev" {
			sawPatch = true
			if r.URL.Query().Get("conferenceDataVersion") != "1" {
				t.Fatalf("expected conferenceDataVersion=1")
			}
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.ConferenceData == nil || body.ConferenceData.CreateRequest == nil {
				t.Fatalf("expected conferenceData create request")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "ev",
				"hangoutLink": "https://meet.google.com/replaced",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--regenerate-meet",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !sawPatch {
		t.Fatalf("expected patch")
	}
}

func TestCalendarUpdateCmd_WithMeetScopeFutureExistingConferenceIsIdempotent(t *testing.T) {
	var patchCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev_instance":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               "ev_instance",
				"recurringEventId": "ev",
				"hangoutLink":      "https://meet.google.com/existing",
			})
			return
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ev",
				"recurrence": []string{"RRULE:FREQ=DAILY"},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/cal@example.com/events/ev/instances"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":          "ev_instance",
						"hangoutLink": "https://meet.google.com/existing",
						"originalStartTime": map[string]any{
							"dateTime": "2025-01-02T10:00:00Z",
						},
					},
				},
			})
			return
		case r.Method == http.MethodPatch:
			patchCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "patched"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev_instance",
		"--with-meet",
		"--scope", "future",
		"--original-start", "2025-01-02T10:00:00Z",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if patchCalled {
		t.Fatalf("expected existing Meet link to skip future-scope patch/truncation")
	}
}

func TestCalendarUpdateCmd_AddAttendee(t *testing.T) {
	var patchedAttendees int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
				"attendees": []map[string]any{
					{"email": "a@example.com"},
				},
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			patchedAttendees = len(body.Attendees)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--add-attendee", "b@example.com",
		"--scope", "all",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if patchedAttendees < 2 {
		t.Fatalf("expected merged attendees, got %d", patchedAttendees)
	}
}

func TestCalendarCreateCmd_EventTypeFocusTimeDefaults(t *testing.T) {
	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev1",
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
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"--event-type", "focus-time",
		"--from", "2025-01-02T10:00:00Z",
		"--to", "2025-01-02T11:00:00Z",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.EventType != eventTypeFocusTime {
		t.Fatalf("expected focusTime event type, got %q", gotEvent.EventType)
	}
	if gotEvent.Summary != defaultFocusSummary {
		t.Fatalf("expected default summary, got %q", gotEvent.Summary)
	}
	if gotEvent.Transparency != transparencyOpaque {
		t.Fatalf("expected opaque transparency, got %q", gotEvent.Transparency)
	}
	if gotEvent.FocusTimeProperties == nil {
		t.Fatalf("expected focus time properties")
	}
	if gotEvent.FocusTimeProperties.AutoDeclineMode != "declineAllConflictingInvitations" {
		t.Fatalf("unexpected autoDeclineMode: %q", gotEvent.FocusTimeProperties.AutoDeclineMode)
	}
	if gotEvent.FocusTimeProperties.ChatStatus != defaultFocusChatStatus {
		t.Fatalf("unexpected chat status: %q", gotEvent.FocusTimeProperties.ChatStatus)
	}
}

func TestCalendarCreateCmd_EventTypeWorkingLocation(t *testing.T) {
	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev1",
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
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"--event-type", "working-location",
		"--working-location-type", "office",
		"--working-office-label", "HQ",
		"--from", "2025-01-01",
		"--to", "2025-01-02",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.EventType != eventTypeWorkingLocation {
		t.Fatalf("expected workingLocation event type, got %q", gotEvent.EventType)
	}
	if gotEvent.Summary != "Working from HQ" {
		t.Fatalf("expected working location summary, got %q", gotEvent.Summary)
	}
	if gotEvent.Start == nil || gotEvent.Start.Date != "2025-01-01" {
		t.Fatalf("unexpected start date: %#v", gotEvent.Start)
	}
	if gotEvent.End == nil || gotEvent.End.Date != "2025-01-02" {
		t.Fatalf("unexpected end date: %#v", gotEvent.End)
	}
	if gotEvent.WorkingLocationProperties == nil || gotEvent.WorkingLocationProperties.Type != "officeLocation" {
		t.Fatalf("unexpected working location props: %#v", gotEvent.WorkingLocationProperties)
	}
	if gotEvent.Transparency != transparencyTransparent {
		t.Fatalf("expected transparent working location, got %q", gotEvent.Transparency)
	}
	if gotEvent.Visibility != "public" {
		t.Fatalf("expected public working location visibility, got %q", gotEvent.Visibility)
	}
}

func TestCalendarUpdateCmd_EventTypeOOO(t *testing.T) {
	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev" {
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
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
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--event-type", "out-of-office",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.EventType != eventTypeOutOfOffice {
		t.Fatalf("expected outOfOffice event type, got %q", gotEvent.EventType)
	}
	if gotEvent.Transparency != transparencyOpaque {
		t.Fatalf("expected opaque transparency, got %q", gotEvent.Transparency)
	}
	if gotEvent.OutOfOfficeProperties == nil {
		t.Fatalf("expected out-of-office properties")
	}
	if gotEvent.OutOfOfficeProperties.AutoDeclineMode != "declineAllConflictingInvitations" {
		t.Fatalf("unexpected autoDeclineMode: %q", gotEvent.OutOfOfficeProperties.AutoDeclineMode)
	}
	if gotEvent.OutOfOfficeProperties.DeclineMessage != defaultOOODeclineMsg {
		t.Fatalf("unexpected decline message: %q", gotEvent.OutOfOfficeProperties.DeclineMessage)
	}
}

func TestCalendarUpdateCmd_EventTypeWorkingLocationDefaults(t *testing.T) {
	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev" {
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
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
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--event-type", "working-location",
		"--working-location-type", "office",
		"--working-office-label", "HQ",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.EventType != eventTypeWorkingLocation {
		t.Fatalf("expected workingLocation event type, got %q", gotEvent.EventType)
	}
	if gotEvent.Transparency != transparencyTransparent {
		t.Fatalf("expected transparent working location, got %q", gotEvent.Transparency)
	}
	if gotEvent.Visibility != "public" {
		t.Fatalf("expected public working location visibility, got %q", gotEvent.Visibility)
	}
}

func TestCalendarUpdateCmd_SendUpdates(t *testing.T) {
	var gotSendUpdates string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			// resolveCalendarID() lists calendars and matches by Summary.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":       "cal",
						"summary":  "cal",
						"timeZone": "UTC",
					},
				},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/") && !strings.Contains(path, "/events"):
			// getCalendarLocation() fetches the calendar timezone.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "cal",
				"timeZone": "UTC",
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal/events/ev":
			gotSendUpdates = r.URL.Query().Get("sendUpdates")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev",
				"summary": "Updated",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal",
		"ev",
		"--summary", "Updated",
		"--send-updates", "all",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotSendUpdates != "all" {
		t.Fatalf("expected sendUpdates=all, got %q", gotSendUpdates)
	}
}

func TestCalendarCreateCmd_ReminderPopupZeroForceSendsMinutes(t *testing.T) {
	var gotEvent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":       "cal",
						"summary":  "cal",
						"timeZone": "UTC",
					},
				},
			})
			return
		case r.Method == http.MethodPost && path == "/calendars/cal/events":
			if err := json.NewDecoder(r.Body).Decode(&gotEvent); err != nil {
				t.Fatalf("decode event: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev",
				"summary": "Zero Reminder",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal",
		"--summary", "Zero Reminder",
		"--from", "2025-01-01T10:00:00Z",
		"--to", "2025-01-01T11:00:00Z",
		"--reminder", "popup:0m",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	reminders, ok := gotEvent["reminders"].(map[string]any)
	if !ok {
		t.Fatalf("expected reminders payload, got %#v", gotEvent["reminders"])
	}
	overrides, ok := reminders["overrides"].([]any)
	if !ok || len(overrides) != 1 {
		t.Fatalf("expected one override, got %#v", reminders["overrides"])
	}
	override, ok := overrides[0].(map[string]any)
	if !ok {
		t.Fatalf("expected override object, got %#v", overrides[0])
	}
	if method, _ := override["method"].(string); method != "popup" {
		t.Fatalf("expected popup reminder, got %#v", override)
	}
	minutes, ok := override["minutes"].(float64)
	if !ok || minutes != 0 {
		t.Fatalf("expected force-sent minutes=0, got %#v", override["minutes"])
	}
}

func TestCalendarUpdateCmd_AddAttendeeNoOp(t *testing.T) {
	var patchCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "cal",
						"summary": "cal",
					},
				},
			})
			return
		case r.Method == http.MethodGet && path == "/calendars/cal/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
				"attendees": []map[string]any{
					{"email": "existing@example.com", "responseStatus": "accepted"},
				},
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal/events/ev":
			patchCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	cmd := &CalendarUpdateCmd{}
	err := runKong(t, cmd, []string{
		"cal",
		"ev",
		"--add-attendee", "EXISTING@example.com",
	}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatalf("expected error for no-op add-attendee")
	}
	if !strings.Contains(err.Error(), "no updates provided") {
		t.Fatalf("expected no updates error, got %v", err)
	}
	if patchCalled {
		t.Fatalf("expected no PATCH call for no-op add-attendee")
	}
}

func TestCalendarUpdateCmd_ScopeFuture(t *testing.T) {
	var (
		truncated               bool
		instancePatchUpdatesVal string
		parentPatchUpdatesVal   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ev",
				"recurrence": []string{"RRULE:FREQ=DAILY"},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/cal@example.com/events/ev/instances"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "ev_1",
						"originalStartTime": map[string]any{
							"dateTime": "2025-01-02T10:00:00Z",
						},
					},
				},
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev_1":
			instancePatchUpdatesVal = r.URL.Query().Get("sendUpdates")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev_1"})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			truncated = true
			parentPatchUpdatesVal = r.URL.Query().Get("sendUpdates")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newCalendarServiceFromServer(t, srv)
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--summary", "Updated",
		"--scope", "future",
		"--original-start", "2025-01-02T10:00:00Z",
		"--send-updates", "all",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !truncated {
		t.Fatalf("expected recurrence truncation")
	}
	if instancePatchUpdatesVal != "all" {
		t.Fatalf("expected instance patch sendUpdates=all, got %q", instancePatchUpdatesVal)
	}
	if parentPatchUpdatesVal != "all" {
		t.Fatalf("expected parent patch sendUpdates=all, got %q", parentPatchUpdatesVal)
	}
}
