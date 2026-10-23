package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
)

// calendarUpdateCalendarRecorder captures every mutating request the command
// makes so tests can assert method, path and the exact JSON body.
type calendarUpdateCalendarRecorder struct {
	method string
	path   string
	body   string
}

func newCalendarUpdateCalendarServer(t *testing.T, patchResponse map[string]any) (*calendar.Service, *[]calendarUpdateCalendarRecorder, func()) {
	t.Helper()

	var calls []calendarUpdateCalendarRecorder
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH (not PUT)", r.Method)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		calls = append(calls, calendarUpdateCalendarRecorder{
			method: r.Method,
			path:   path,
			body:   string(raw),
		})

		if patchResponse == nil {
			patchResponse = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(patchResponse)
	}))
	return svc, &calls, closeSvc
}

func strptr(value string) *string { return &value }

func TestCalendarUpdateCalendarCmd_PatchBody(t *testing.T) {
	const calendarPath = "/calendars/structure@group.calendar.google.com"
	cases := []struct {
		name     string
		cmd      CalendarUpdateCalendarCmd
		wantBody string
	}{
		{
			name: "timezone patches only the calendar resource",
			cmd: CalendarUpdateCalendarCmd{
				CalendarID: "structure@group.calendar.google.com",
				TimeZone:   strptr("America/Los_Angeles"),
			},
			wantBody: `{"timeZone":"America/Los_Angeles"}`,
		},
		{
			name: "omitted fields stay out of the body",
			cmd: CalendarUpdateCalendarCmd{
				CalendarID:  "structure@group.calendar.google.com",
				Summary:     strptr("Structure"),
				Description: strptr(""),
			},
			wantBody: `{"description":"","summary":"Structure"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, calls, closeSvc := newCalendarUpdateCalendarServer(t, map[string]any{
				"id":       "structure@group.calendar.google.com",
				"summary":  "Structure",
				"timeZone": "America/Los_Angeles",
			})
			defer closeSvc()

			cmd := tc.cmd
			var out bytes.Buffer
			ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
			if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", Force: true}); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if len(*calls) != 1 {
				t.Fatalf("got %d patches (%v), want 1", len(*calls), *calls)
			}
			call := (*calls)[0]
			if call.path != calendarPath {
				t.Fatalf("path = %q, want %q", call.path, calendarPath)
			}
			if call.method != http.MethodPatch {
				t.Fatalf("method = %q, want PATCH", call.method)
			}
			if strings.TrimSpace(call.body) != tc.wantBody {
				t.Fatalf("body = %s, want %s", call.body, tc.wantBody)
			}
		})
	}
}

func TestCalendarUpdateCalendarCmd_ResultReportsSummaryAndTimeZone(t *testing.T) {
	svc, _, closeSvc := newCalendarUpdateCalendarServer(t, map[string]any{
		"id":       "structure@group.calendar.google.com",
		"summary":  "Structure",
		"timeZone": "America/Los_Angeles",
	})
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{
		CalendarID: "structure@group.calendar.google.com",
		TimeZone:   strptr("America/Los_Angeles"),
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", Force: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var payload struct {
		CalendarID string `json:"calendarId"`
		Calendar   struct {
			Summary  string `json:"summary"`
			TimeZone string `json:"timeZone"`
		} `json:"calendar"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\nout=%q", err, out.String())
	}
	if payload.CalendarID != "structure@group.calendar.google.com" {
		t.Fatalf("calendarId = %q", payload.CalendarID)
	}
	if payload.Calendar.Summary != "Structure" || payload.Calendar.TimeZone != "America/Los_Angeles" {
		t.Fatalf("unexpected calendar result: %#v", payload.Calendar)
	}
}

func TestCalendarUpdateCalendarCmd_RequiresAField(t *testing.T) {
	svc, _, closeSvc := newCalendarUpdateCalendarServer(t, map[string]any{})
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	err := (&CalendarUpdateCalendarCmd{CalendarID: "structure@group.calendar.google.com"}).
		Run(ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil || !strings.Contains(err.Error(), "provide at least one of") {
		t.Fatalf("err = %v, want usage error", err)
	}
}

func TestCalendarUpdateCalendarCmd_RejectsEmptyTimeZone(t *testing.T) {
	svc, calls, closeSvc := newCalendarUpdateCalendarServer(t, map[string]any{})
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{
		CalendarID: "structure@group.calendar.google.com",
		TimeZone:   strptr("  "),
	}
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil || !strings.Contains(err.Error(), "--timezone cannot be empty") {
		t.Fatalf("err = %v, want empty-timezone usage error", err)
	}
	// An empty timeZone would vanish from the PATCH body, so a request here
	// would be a confirmed no-op.
	if len(*calls) != 0 {
		t.Fatalf("expected no API calls, got %v", *calls)
	}
}

func TestCalendarUpdateCalendarCmd_DryRunDoesNotOpenService(t *testing.T) {
	var out bytes.Buffer
	ctx := withCalendarTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*calendar.Service, error) {
			t.Fatal("calendar service should not be created during dry-run")
			return nil, errors.New("unexpected calendar service call")
		},
	)

	cmd := CalendarUpdateCalendarCmd{
		CalendarID: "structure@group.calendar.google.com",
		TimeZone:   strptr("America/Los_Angeles"),
	}
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", DryRun: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}

	var payload struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Calendar struct {
				TimeZone string `json:"timeZone"`
			} `json:"calendar"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\nout=%q", err, out.String())
	}
	if !payload.DryRun || payload.Op != "calendar.update-calendar" {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}
	if payload.Request.Calendar.TimeZone != "America/Los_Angeles" {
		t.Fatalf("unexpected dry-run request: %#v", payload.Request)
	}
}

func TestCalendarUpdateCalendarCmd_NonInteractiveWithoutForceRefuses(t *testing.T) {
	svc, calls, closeSvc := newCalendarUpdateCalendarServer(t, map[string]any{})
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{
		CalendarID: "structure@group.calendar.google.com",
		TimeZone:   strptr("America/Los_Angeles"),
	}
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", NoInput: true})
	if err == nil || !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("err = %v, want refusal without --force", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("expected no API calls, got %v", *calls)
	}
}

// TestCalendarUpdateCalendarCmd_CheckReportsBothTimeZones encodes the
// motivating incident: the calendar's own default timezone was left on
// Europe/Copenhagen after a trip. The read path must expose it without writes.
func TestCalendarUpdateCalendarCmd_CheckReportsBothTimeZones(t *testing.T) {
	const calendarID = "structure@group.calendar.google.com"

	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/"+calendarID:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       calendarID,
				"summary":  "Structure",
				"timeZone": "Europe/Copenhagen",
			})
		case r.Method == http.MethodGet && path == "/users/me/calendarList/"+calendarID:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       calendarID,
				"summary":  "Structure",
				"timeZone": "Europe/Copenhagen",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{CalendarID: calendarID, Check: true}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var payload struct {
		CalendarTimeZone     string `json:"calendarTimeZone"`
		CalendarListTimeZone string `json:"calendarListTimeZone"`
		TimeZonesMatch       bool   `json:"timeZonesMatch"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\nout=%q", err, out.String())
	}
	if payload.CalendarTimeZone != "Europe/Copenhagen" {
		t.Fatalf("calendarTimeZone = %q, want Europe/Copenhagen", payload.CalendarTimeZone)
	}
	if payload.CalendarListTimeZone != "Europe/Copenhagen" {
		t.Fatalf("calendarListTimeZone = %q", payload.CalendarListTimeZone)
	}
	if !payload.TimeZonesMatch {
		t.Fatal("expected matching timezones in this fixture")
	}
}

func TestCalendarUpdateCalendarCmd_CheckRejectsSettingFlags(t *testing.T) {
	svc, _, closeSvc := newCalendarUpdateCalendarServer(t, map[string]any{})
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{
		CalendarID: "structure@group.calendar.google.com",
		Check:      true,
		TimeZone:   strptr("America/Los_Angeles"),
	}
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil || !strings.Contains(err.Error(), "--check is read-only") {
		t.Fatalf("err = %v, want --check usage error", err)
	}
}

func TestCalendarUpdateCalendarCmd_CheckTextOutputNamesBothCopies(t *testing.T) {
	const calendarID = "structure@group.calendar.google.com"

	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		w.Header().Set("Content-Type", "application/json")
		switch path {
		case "/calendars/" + calendarID:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       calendarID,
				"summary":  "Structure",
				"timeZone": "Europe/Copenhagen",
			})
		case "/users/me/calendarList/" + calendarID:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       calendarID,
				"summary":  "Structure",
				"timeZone": "America/Los_Angeles",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{CalendarID: calendarID, Check: true}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := out.String()
	// Keys are snake_case throughout the block so a TSV consumer needs one rule.
	for _, want := range []string{
		"calendar_id\t" + calendarID,
		"calendar_summary\tStructure",
		"calendar_timezone\tEurope/Copenhagen",
		"calendar_list_summary\tStructure",
		"calendar_list_timezone\tAmerica/Los_Angeles",
		"timezones_match\tfalse",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got %q", want, got)
		}
	}
	if strings.Contains(got, "calendarList_") {
		t.Fatalf("expected snake_case keys only, got %q", got)
	}
}

func TestCalendarUpdateCalendarCmd_SurfacesAPIErrorVerbatim(t *testing.T) {
	const calendarID = "shared@group.calendar.google.com"

	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 403,
				"message": "The user is not the owner of the calendar.",
				"errors": [{"domain": "global", "reason": "forbidden", "message": "The user is not the owner of the calendar."}]
			}
		}`))
	}))
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{
		CalendarID: calendarID,
		TimeZone:   strptr("America/Los_Angeles"),
	}
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "The user is not the owner of the calendar.") {
		t.Fatalf("err = %v, want the API message preserved verbatim", err)
	}
}

func TestCalendarUpdateCalendarCmd_Surfaces404Verbatim(t *testing.T) {
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 404,
				"message": "Not Found",
				"errors": [{"domain": "global", "reason": "notFound", "message": "Not Found"}]
			}
		}`))
	}))
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{
		CalendarID: "missing@group.calendar.google.com",
		TimeZone:   strptr("America/Los_Angeles"),
	}
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil || !strings.Contains(err.Error(), "Not Found") {
		t.Fatalf("err = %v, want 404 passed through verbatim", err)
	}
}

func TestCalendarUpdateCalendarCmd_TextOutputReportsSummaryAndTimeZone(t *testing.T) {
	svc, _, closeSvc := newCalendarUpdateCalendarServer(t, map[string]any{
		"id":       "structure@group.calendar.google.com",
		"summary":  "Structure",
		"timeZone": "America/Los_Angeles",
	})
	defer closeSvc()

	var out bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	cmd := CalendarUpdateCalendarCmd{
		CalendarID: "structure@group.calendar.google.com",
		TimeZone:   strptr("America/Los_Angeles"),
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", Force: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"calendar_id\tstructure@group.calendar.google.com",
		"summary\tStructure",
		"timezone\tAmerica/Los_Angeles",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got %q", want, got)
		}
	}
}

func TestCalendarUpdateCalendarCmd_RejectsInvalidSettingsBeforeIO(t *testing.T) {
	for _, test := range []struct{ name, flag, value, want string }{
		{name: "empty summary", flag: "--summary", value: "  ", want: "--summary cannot be empty"},
		{name: "invalid timezone", flag: "--timezone", value: "Not/AZone", want: "invalid timezone"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := executeWithTestRuntime(t, []string{"calendar", "update-calendar", "primary", "--dry-run", "--json", test.flag, test.value}, nil)
			if result.err == nil || !strings.Contains(result.err.Error(), test.want) {
				t.Fatalf("err=%v, want %s", result.err, test.want)
			}
		})
	}
}
