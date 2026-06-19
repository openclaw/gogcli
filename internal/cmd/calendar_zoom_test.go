package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/zoom"
)

func newCalendarServiceFromZoomTestServer(t *testing.T, ctx context.Context, srv *httptest.Server) *calendar.Service {
	t.Helper()
	svc, err := calendar.NewService(ctx,
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type fakeZoomCalendarClient struct {
	created int
	deleted []string
	err     error
}

func (f *fakeZoomCalendarClient) CreateMeeting(context.Context, string, zoom.CreateMeetingRequest) (*zoom.Meeting, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created++
	return &zoom.Meeting{
		ID:       int64(1000 + f.created),
		JoinURL:  "https://example.zoom.us/j/1001?pwd=secret",
		Password: "secret",
		IconURI:  "https://example.com/zoom.png",
	}, nil
}

func (f *fakeZoomCalendarClient) DeleteMeeting(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return f.err
}

func withFakeZoomClient(ctx context.Context, client *fakeZoomCalendarClient) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.Zoom = func(context.Context, string) (app.ZoomMeetingClient, error) {
			if client.err != nil && errors.Is(client.err, zoom.ErrCredentialsNotFound) {
				return nil, client.err
			}
			return client, nil
		}
	})
}

func newZoomCalendarTestJSONContext(t *testing.T, svc *calendar.Service, client *fakeZoomCalendarClient) context.Context {
	t.Helper()
	ctx, _ := newCalendarTestJSONContext(t, svc)
	return withFakeZoomClient(ctx, client)
}

func TestCalendarCreateCmd_WithZoomAndAttachments(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{}

	var sawZoomDescription, sawNoConferenceData, sawAttachments bool
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			// Zoom info lives in the event description, not conferenceData.
			// Google rejects conferenceData writes asserting key.type="addOn"
			// from non-Workspace-Marketplace OAuth clients.
			sawZoomDescription = descriptionHasZoomBlock(body.Description)
			sawNoConferenceData = body.ConferenceData == nil
			sawAttachments = len(body.Attachments) > 0
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com", "--summary", "Zoom", "--from", "2025-01-02T10:00:00Z", "--to", "2025-01-02T11:00:00Z",
		"--with-zoom", "--attachment", "https://example.com/file",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !sawZoomDescription || !sawNoConferenceData || !sawAttachments || zoomClient.created != 1 {
		t.Fatalf("expected zoom description+attachments, sawZoomDescription=%v sawNoConferenceData=%v sawAttachments=%v created=%d",
			sawZoomDescription, sawNoConferenceData, sawAttachments, zoomClient.created)
	}
}

func TestCalendarCreateCmd_DryRunWithZoomReportsZoomIntent(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json",
		"--dry-run",
		"--no-input",
		"calendar", "create", "primary",
		"--summary", "Zoom",
		"--from", "2026-05-18T10:00:00Z",
		"--to", "2026-05-18T10:30:00Z",
		"--with-zoom",
	}, &app.Runtime{Services: app.Services{
		Zoom: func(context.Context, string) (app.ZoomMeetingClient, error) {
			t.Fatal("dry-run must not create Zoom client")
			return nil, errors.New("unexpected Zoom client")
		},
	}})
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	var got struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			Zoom map[string]any `json:"zoom"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v\nout=%s", err, out)
	}
	if !got.DryRun || got.Request.Zoom["action"] != "create" || got.Request.Zoom["description_mode"] != true {
		t.Fatalf("unexpected dry-run zoom payload: %#v", got)
	}
}

func TestCalendarUpdateCmd_DryRunWithZoomReportsZoomIntent(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json",
		"--dry-run",
		"--no-input",
		"calendar", "update", "primary", "event-id",
		"--regenerate-zoom",
	}, &app.Runtime{Services: app.Services{
		Calendar: func(context.Context, string) (*calendar.Service, error) {
			t.Fatal("dry-run must not create Calendar service")
			return nil, errors.New("unexpected Calendar service")
		},
		Zoom: func(context.Context, string) (app.ZoomMeetingClient, error) {
			t.Fatal("dry-run must not create Zoom client")
			return nil, errors.New("unexpected Zoom client")
		},
	}})
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	var got struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			Zoom map[string]any `json:"zoom"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v\nout=%s", err, out)
	}
	if !got.DryRun || got.Request.Zoom["action"] != "regenerate" || got.Request.Zoom["description_mode"] != true {
		t.Fatalf("unexpected dry-run zoom payload: %#v", got)
	}
}

func TestRedactEventZoomURLsRedactsDescriptionModePasswords(t *testing.T) {
	event := &calendar.Event{
		Description: buildZoomDescriptionBlock(&zoom.Meeting{
			ID:      1001,
			JoinURL: "https://example.zoom.us/j/1001?pwd=secret&from=addon",
		}),
	}

	redactEventZoomURLs(event, false)

	if strings.Contains(event.Description, "secret") {
		t.Fatalf("description leaked password: %s", event.Description)
	}
	if !strings.Contains(event.Description, "pwd=REDACTED") {
		t.Fatalf("description did not redact join URL password: %s", event.Description)
	}
	if !strings.Contains(event.Description, "Passcode: REDACTED") {
		t.Fatalf("description did not redact passcode line: %s", event.Description)
	}
}

func TestRedactEventZoomURLsIncludePasswordsPreservesDescriptionModePasswords(t *testing.T) {
	event := &calendar.Event{
		Description: buildZoomDescriptionBlock(&zoom.Meeting{
			ID:      1001,
			JoinURL: "https://example.zoom.us/j/1001?pwd=secret&from=addon",
		}),
	}

	redactEventZoomURLs(event, true)

	if !strings.Contains(event.Description, "secret") {
		t.Fatalf("description should preserve password with includePasswords: %s", event.Description)
	}
}

func TestRedactEventZoomURLsLeavesUnmanagedDescriptionText(t *testing.T) {
	event := &calendar.Event{
		Description: "Agenda\nPasscode: keep-me\nhttps://example.com/path?pwd=not-zoom",
	}

	redactEventZoomURLs(event, false)

	if !strings.Contains(event.Description, "keep-me") || !strings.Contains(event.Description, "not-zoom") {
		t.Fatalf("unmanaged description text should be preserved: %s", event.Description)
	}
}

func TestCalendarEventCmd_RedactsZoomDescriptionInJSON(t *testing.T) {
	desc := buildZoomDescriptionBlock(&zoom.Meeting{
		ID:      1001,
		JoinURL: "https://example.zoom.us/j/1001?pwd=secret",
	})
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "ev",
				"summary":     "Zoom",
				"description": desc,
				"start":       map[string]any{"dateTime": "2026-05-18T10:00:00Z"},
				"end":         map[string]any{"dateTime": "2026-05-18T10:30:00Z"},
			})
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cal@example.com", "timeZone": "UTC"})
		default:
			http.NotFound(w, r)
		}
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx, output := newCalendarTestJSONContext(t, svc)
	if err := runKong(t, &CalendarEventCmd{}, []string{"cal@example.com", "ev"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	out := output.String()

	if strings.Contains(out, "secret") {
		t.Fatalf("event output leaked zoom password: %s", out)
	}
	if !strings.Contains(out, "pwd=REDACTED") || !strings.Contains(out, "Passcode: REDACTED") {
		t.Fatalf("event output did not redact zoom password: %s", out)
	}
}

func TestListCalendarEventsJSONRedactsZoomDescription(t *testing.T) {
	desc := buildZoomDescriptionBlock(&zoom.Meeting{
		ID:      1001,
		JoinURL: "https://example.zoom.us/j/1001?pwd=secret",
	})
	svc, closeServer := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/cal1/events") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"items":[{"id":"e1","summary":"Zoom","description":%q,"start":{"dateTime":"2026-05-18T10:00:00Z"},"end":{"dateTime":"2026-05-18T10:30:00Z"}}]}`, desc)
			return
		}
		http.NotFound(w, r)
	}))
	defer closeServer()

	var output bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &output, io.Discard)
	if err := listCalendarEvents(ctx, svc, "cal1", "2026-05-18T00:00:00Z", "2026-05-19T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, false, "", ""); err != nil {
		t.Fatalf("listCalendarEvents: %v", err)
	}
	out := output.String()

	if strings.Contains(out, "secret") {
		t.Fatalf("events output leaked zoom password: %s", out)
	}
	if !strings.Contains(out, "pwd=REDACTED") || !strings.Contains(out, "Passcode: REDACTED") {
		t.Fatalf("events output did not redact zoom password: %s", out)
	}
}

func TestCalendarUpdateCmd_WithZoom(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{}

	var sawZoomPatch, sawNoConferenceData bool
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev", "summary": "Existing"})
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			// Description-mode: patch carries the Zoom block in the
			// description, not conferenceData. conferenceDataVersion is not
			// required because we are not mutating conferenceData.
			sawZoomPatch = descriptionHasZoomBlock(body.Description)
			sawNoConferenceData = body.ConferenceData == nil
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	if err := runKong(t, &CalendarUpdateCmd{}, []string{"cal@example.com", "ev", "--with-zoom"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !sawZoomPatch || !sawNoConferenceData || zoomClient.created != 1 {
		t.Fatalf("expected zoom patch/no-conference-data/create, sawZoomPatch=%v sawNoConferenceData=%v created=%d",
			sawZoomPatch, sawNoConferenceData, zoomClient.created)
	}
}

func TestCalendarUpdateCmd_WithZoomExistingConferenceIsIdempotent(t *testing.T) {
	testCalendarUpdateWithZoomExistingConferenceIsIdempotent(t, "all")
}

func TestCalendarUpdateCmd_WithZoomScopeFutureExistingConferenceIsIdempotent(t *testing.T) {
	testCalendarUpdateWithZoomExistingConferenceIsIdempotent(t, "future")
}

func testCalendarUpdateWithZoomExistingConferenceIsIdempotent(t *testing.T, scope string) {
	t.Helper()
	zoomClient := &fakeZoomCalendarClient{}
	var patchCalled bool
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			event := zoomEventJSON("ev", "1001")
			if scope == "future" {
				event["recurringEventId"] = "series"
			}
			_ = json.NewEncoder(w).Encode(event)
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/series":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "series", "recurrence": []string{"RRULE:FREQ=DAILY"}})
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev/instances":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"id": "ev", "originalStartTime": map[string]any{"dateTime": "2025-01-02T10:00:00Z"}}}})
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/series/instances":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"id": "ev", "originalStartTime": map[string]any{"dateTime": "2025-01-02T10:00:00Z"}}}})
		case r.Method == http.MethodPatch:
			patchCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "patched"})
		default:
			http.NotFound(w, r)
		}
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	args := []string{"cal@example.com", "ev", "--with-zoom"}
	if scope == "future" {
		args = append(args, "--scope", "future", "--original-start", "2025-01-02T10:00:00Z")
	}
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	if err := runKong(t, &CalendarUpdateCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if patchCalled || zoomClient.created != 0 {
		t.Fatalf("expected idempotent skip, patchCalled=%v created=%d", patchCalled, zoomClient.created)
	}
}

func TestCalendarUpdateCmd_RegenerateZoomReplacesConference(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{}
	var sawPatch bool
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			_ = json.NewEncoder(w).Encode(zoomEventJSON("ev", "999"))
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			sawPatch = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
		default:
			http.NotFound(w, r)
		}
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	if err := runKong(t, &CalendarUpdateCmd{}, []string{"cal@example.com", "ev", "--regenerate-zoom"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !sawPatch || zoomClient.created != 1 || len(zoomClient.deleted) != 1 || zoomClient.deleted[0] != "999" {
		t.Fatalf("expected delete/create/patch, sawPatch=%v created=%d deleted=%v", sawPatch, zoomClient.created, zoomClient.deleted)
	}
}

func TestCalendarUpdateCmd_RemoveZoom(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{}
	var cleared bool
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			_ = json.NewEncoder(w).Encode(zoomEventJSON("ev", "999"))
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, cleared = body["conferenceData"]
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
		default:
			http.NotFound(w, r)
		}
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	if err := runKong(t, &CalendarUpdateCmd{}, []string{"cal@example.com", "ev", "--remove-zoom"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !cleared || len(zoomClient.deleted) != 1 || zoomClient.deleted[0] != "999" {
		t.Fatalf("expected cleared/delete, cleared=%v deleted=%v", cleared, zoomClient.deleted)
	}
}

func TestCalendarUpdateCmd_RemoveZoomClearsDescriptionOnlyBlock(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{}
	var descriptionPresent bool
	var description string
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
				"description": buildZoomDescriptionBlock(&zoom.Meeting{
					ID:       999,
					JoinURL:  "https://example.zoom.us/j/999?pwd=secret",
					Password: "secret",
				}),
			})
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			rawDescription, ok := body["description"]
			descriptionPresent = ok
			if ok {
				description, _ = rawDescription.(string)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
		default:
			http.NotFound(w, r)
		}
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	if err := runKong(t, &CalendarUpdateCmd{}, []string{"cal@example.com", "ev", "--remove-zoom"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !descriptionPresent || description != "" || len(zoomClient.deleted) != 1 || zoomClient.deleted[0] != "999" {
		t.Fatalf("expected empty description/delete, descriptionPresent=%v description=%q deleted=%v", descriptionPresent, description, zoomClient.deleted)
	}
}

func TestDescriptionForPatchHonorsExplicitEmptyDescription(t *testing.T) {
	existing := &calendar.Event{Description: "Agenda\n\n" + buildZoomDescriptionBlock(&zoom.Meeting{
		ID:      999,
		JoinURL: "https://example.zoom.us/j/999",
	})}
	patch := &calendar.Event{
		Description:     "",
		ForceSendFields: []string{"Description"},
	}

	if got := descriptionForPatch(existing, patch); got != "" {
		t.Fatalf("descriptionForPatch = %q, want explicit empty description", got)
	}
}

func TestMergeEventPatchHonorsExplicitEmptyDescription(t *testing.T) {
	existing := &calendar.Event{Summary: "Planning", Description: "private agenda"}
	patch := &calendar.Event{
		Description:     "",
		ForceSendFields: []string{"Description"},
	}

	got := mergeEventPatch(existing, patch)
	if got.Description != "" {
		t.Fatalf("mergeEventPatch description = %q, want explicit empty description", got.Description)
	}
	if got.Summary != "Planning" {
		t.Fatalf("mergeEventPatch summary = %q, want existing summary", got.Summary)
	}
}

func TestCalendarUpdateCmd_WithZoomOnExistingMeetEventRejects(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{}
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev", "hangoutLink": "https://meet.google.com/aaa-bbbb-ccc"})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	err := runKong(t, &CalendarUpdateCmd{}, []string{"cal@example.com", "ev", "--with-zoom"}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "event already has a Meet conference") {
		t.Fatalf("error = %v, want existing Meet rejection", err)
	}
}

func TestCalendarUpdateCmd_WithZoomNoCredentialsErrors(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{err: zoom.ErrCredentialsNotFound}
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	err := runKong(t, &CalendarUpdateCmd{}, []string{"cal@example.com", "ev", "--with-zoom"}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "Zoom credentials not found") {
		t.Fatalf("error = %v, want credentials message", err)
	}
}

func TestCalendarUpdateCmd_RegenerateZoomWithUnparseablePriorMeetingWarns(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{}
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
				"conferenceData": map[string]any{
					"conferenceSolution": map[string]any{"key": map[string]any{"type": "addOn"}, "name": "Zoom Meeting"},
					"entryPoints":        []map[string]any{{"entryPointType": calendarEntryPointTypeVideo, "uri": "https://example.zoom.us/not-a-meeting"}},
				},
			})
		case r.Method == http.MethodPatch:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
		default:
			http.NotFound(w, r)
		}
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, context.Background(), srv)
	ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
	err := runKong(t, &CalendarUpdateCmd{}, []string{"cal@example.com", "ev", "--regenerate-zoom"}, ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if zoomClient.created != 1 || len(zoomClient.deleted) != 0 {
		t.Fatalf("expected create without delete, created=%d deleted=%v", zoomClient.created, zoomClient.deleted)
	}
}

func TestCalendarUpdateCmd_FlagMutex_WithZoomRegenerateZoom(t *testing.T) {
	assertCalendarUpdateZoomMutex(t, "--with-zoom", "--regenerate-zoom")
}

func TestCalendarUpdateCmd_FlagMutex_WithZoomRemoveZoom(t *testing.T) {
	assertCalendarUpdateZoomMutex(t, "--with-zoom", "--remove-zoom")
}

func TestCalendarUpdateCmd_FlagMutex_RegenerateZoomRemoveZoom(t *testing.T) {
	assertCalendarUpdateZoomMutex(t, "--regenerate-zoom", "--remove-zoom")
}

func TestCalendarUpdateCmd_FlagMutex_WithZoomWithMeet(t *testing.T) {
	assertCalendarUpdateZoomMutex(t, "--with-zoom", "--with-meet")
}

func TestCalendarUpdateCmd_FlagMutex_WithZoomRegenerateMeet(t *testing.T) {
	assertCalendarUpdateZoomMutex(t, "--with-zoom", "--regenerate-meet")
}

func TestCalendarUpdateCmd_FlagMutex_RegenerateZoomWithMeet(t *testing.T) {
	assertCalendarUpdateZoomMutex(t, "--regenerate-zoom", "--with-meet")
}

func TestCalendarUpdateCmd_FlagMutex_RegenerateZoomRegenerateMeet(t *testing.T) {
	assertCalendarUpdateZoomMutex(t, "--regenerate-zoom", "--regenerate-meet")
}

func assertCalendarUpdateZoomMutex(t *testing.T, flags ...string) {
	t.Helper()
	args := append([]string{"cal@example.com", "ev"}, flags...)
	err := runKong(t, &CalendarUpdateCmd{}, args, newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "use only one of") {
		t.Fatalf("error = %v, want mutex for %v", err, flags)
	}
}

func zoomEventJSON(id, meetingID string) map[string]any {
	return map[string]any{
		"id": id,
		"conferenceData": map[string]any{
			"conferenceSolution": map[string]any{"key": map[string]any{"type": "addOn"}, "name": "Zoom Meeting"},
			"entryPoints": []map[string]any{{
				"entryPointType": calendarEntryPointTypeVideo,
				"uri":            "https://example.zoom.us/j/" + meetingID + "?pwd=secret",
			}},
		},
	}
}
