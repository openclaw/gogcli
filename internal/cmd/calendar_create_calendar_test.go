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

func TestCalendarCreateCalendarCmd_RunJSON(t *testing.T) {
	var got calendar.Calendar
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method != http.MethodPost || path != "/calendars" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "created@example.com",
			"summary":     got.Summary,
			"description": got.Description,
			"timeZone":    got.TimeZone,
			"location":    got.Location,
		})
	}))
	defer closeSvc()
	stubCalendarServiceForTest(t, svc)

	cmd := &CalendarCreateCalendarCmd{
		Summary:     "Team Calendar",
		Description: "Planning",
		TimeZone:    "Europe/London",
		Location:    "London",
	}
	out := captureStdout(t, func() {
		if err := cmd.Run(newCalendarJSONContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if got.Summary != "Team Calendar" || got.Description != "Planning" || got.TimeZone != "Europe/London" || got.Location != "London" {
		t.Fatalf("unexpected request: %#v", got)
	}
	var payload struct {
		Calendar calendar.Calendar `json:"calendar"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v\nout=%q", err, out)
	}
	if payload.Calendar.Id != "created@example.com" || payload.Calendar.Summary != "Team Calendar" {
		t.Fatalf("unexpected output: %#v", payload.Calendar)
	}
}

func TestCalendarCreateCalendarCmd_RunTextIncludesLocation(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method != http.MethodPost || path != "/calendars" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "created@example.com",
			"summary":  "Team Calendar",
			"timeZone": "UTC",
			"location": "Remote",
		})
	}))
	defer closeSvc()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	var outBuf bytes.Buffer
	ctx := newCalendarOutputContext(t, &outBuf, io.Discard)
	err := (&CalendarCreateCalendarCmd{Summary: "Team Calendar", TimeZone: "UTC"}).Run(ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := outBuf.String()
	for _, want := range []string{"id\tcreated@example.com", "summary\tTeam Calendar", "timezone\tUTC", "location\tRemote"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestCalendarCreateCalendarCmd_DryRunDoesNotOpenService(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		t.Fatal("newCalendarService should not be called during dry-run")
		return nil, errors.New("unexpected calendar service call")
	}

	out := captureStdout(t, func() {
		err := (&CalendarCreateCalendarCmd{
			Summary:  "Dry Run",
			TimeZone: "UTC",
		}).Run(newCalendarJSONContext(t), &RootFlags{Account: "a@b.com", DryRun: true})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("expected dry-run exit 0, got %v", err)
		}
	})

	var payload struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Calendar calendar.Calendar `json:"calendar"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode dry-run: %v\nout=%q", err, out)
	}
	if !payload.DryRun || payload.Op != "calendar.create-calendar" || payload.Request.Calendar.Summary != "Dry Run" {
		t.Fatalf("unexpected dry-run output: %#v", payload)
	}
}

func TestCalendarCreateCalendarCmd_InvalidTimezone(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })
	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		t.Fatal("newCalendarService should not be called for invalid timezone")
		return nil, errors.New("unexpected calendar service call")
	}

	err := (&CalendarCreateCalendarCmd{
		Summary:  "Bad TZ",
		TimeZone: "Nope/Zone",
	}).Run(newCalendarJSONContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), `invalid timezone "Nope/Zone"`) {
		t.Fatalf("expected invalid timezone error, got %v", err)
	}
}
