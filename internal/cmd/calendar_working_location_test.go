package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestWorkingLocationProperties(t *testing.T) {
	cmd := &CalendarWorkingLocationCmd{Type: "home"}
	props, err := cmd.buildWorkingLocationProperties()
	if err != nil {
		t.Fatalf("buildWorkingLocationProperties: %v", err)
	}
	if props.Type != "homeOffice" {
		t.Fatalf("unexpected type: %q", props.Type)
	}

	cmd = &CalendarWorkingLocationCmd{Type: "office", OfficeLabel: "HQ", BuildingId: "b1", FloorId: "f1", DeskId: "d1"}
	props, err = cmd.buildWorkingLocationProperties()
	if err != nil {
		t.Fatalf("buildWorkingLocationProperties office: %v", err)
	}
	if props.OfficeLocation == nil || props.OfficeLocation.Label != "HQ" {
		t.Fatalf("unexpected office props: %#v", props)
	}

	cmd = &CalendarWorkingLocationCmd{Type: "custom", CustomLabel: "Cafe"}
	props, err = cmd.buildWorkingLocationProperties()
	if err != nil {
		t.Fatalf("buildWorkingLocationProperties custom: %v", err)
	}
	if props.CustomLocation == nil || props.CustomLocation.Label != "Cafe" {
		t.Fatalf("unexpected custom props: %#v", props)
	}

	cmd = &CalendarWorkingLocationCmd{Type: "custom"}
	if _, err = cmd.buildWorkingLocationProperties(); err == nil {
		t.Fatalf("expected error for missing custom label")
	}
}

func TestWorkingLocationSummary(t *testing.T) {
	cmd := &CalendarWorkingLocationCmd{Type: "home"}
	if cmd.generateSummary() != "Working from home" {
		t.Fatalf("unexpected home summary")
	}
	cmd = &CalendarWorkingLocationCmd{Type: "office", OfficeLabel: "HQ"}
	if cmd.generateSummary() != "Working from HQ" {
		t.Fatalf("unexpected office summary")
	}
	cmd = &CalendarWorkingLocationCmd{Type: "custom", CustomLabel: "Cafe"}
	if cmd.generateSummary() != "Working from Cafe" {
		t.Fatalf("unexpected custom summary")
	}
}

func TestCalendarWorkingLocation_RunJSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			if err := json.NewDecoder(r.Body).Decode(&gotEvent); err != nil {
				t.Fatalf("decode event: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev1",
				"summary": "Working from HQ",
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarWorkingLocationCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{
			"cal@example.com",
			"--from", "2025-01-01",
			"--to", "2025-01-02",
			"--type", "office",
			"--office-label", "HQ",
			"--building-id", "b1",
			"--floor-id", "f1",
			"--desk-id", "d1",
		}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})
	if !strings.Contains(out, "\"event\"") {
		t.Fatalf("unexpected output: %q", out)
	}

	if gotEvent.EventType != "workingLocation" {
		t.Fatalf("unexpected event type: %q", gotEvent.EventType)
	}
	if gotEvent.Summary != "Working from HQ" {
		t.Fatalf("unexpected summary: %q", gotEvent.Summary)
	}
	props := gotEvent.WorkingLocationProperties
	if props == nil || props.Type != "officeLocation" || props.OfficeLocation == nil {
		t.Fatalf("unexpected working location props: %#v", props)
	}
	if props.OfficeLocation.Label != "HQ" || props.OfficeLocation.BuildingId != "b1" || props.OfficeLocation.FloorId != "f1" || props.OfficeLocation.DeskId != "d1" {
		t.Fatalf("unexpected office props: %#v", props.OfficeLocation)
	}
}
