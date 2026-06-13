package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestResolveCalendarPlaceTextSearch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	t.Setenv("GOG_PLACES_API_KEY", "test-key")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/places:searchText" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Goog-Api-Key"); got != "test-key" {
			t.Fatalf("api key = %q", got)
		}
		if got := r.Header.Get("X-Goog-FieldMask"); !strings.Contains(got, "places.id") {
			t.Fatalf("field mask = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"places":[{"id":"ChIJ123","displayName":{"text":"Cafe"},"formattedAddress":"1 Main St","googleMapsUri":"https://maps.example/cafe"}]}`))
	}))
	defer srv.Close()
	t.Setenv("GOG_PLACES_BASE_URL", srv.URL)

	place, err := resolveCalendarPlace(context.Background(), calendarPlaceLookup{LocationSearch: "cafe"})
	if err != nil {
		t.Fatalf("resolveCalendarPlace: %v", err)
	}
	if place.ID != "ChIJ123" || place.Name != "Cafe" || place.FormattedAddress != "1 Main St" || place.GoogleMapsURI == "" {
		t.Fatalf("unexpected place: %#v", place)
	}
}

func TestResolveCalendarPlaceValidation(t *testing.T) {
	_, err := resolveCalendarPlace(context.Background(), calendarPlaceLookup{LocationSet: true, LocationSearch: "cafe"})
	if err == nil || !strings.Contains(err.Error(), "cannot combine") {
		t.Fatalf("expected conflict error, got %v", err)
	}

	_, err = validateCalendarPlaceLookup(calendarPlaceLookup{PlaceID: "places/"})
	if err == nil || !strings.Contains(err.Error(), "empty --place-id") {
		t.Fatalf("expected empty prefixed place id error, got %v", err)
	}
}

func TestCalendarCreateDryRunLocationSearchSkipsPlacesAPI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	t.Setenv("GOG_PLACES_API_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run must not call Places API: %s", r.URL.Path)
	}))
	defer srv.Close()
	t.Setenv("GOG_PLACES_BASE_URL", srv.URL)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--dry-run",
				"--no-input",
				"calendar", "create", "primary",
				"--summary", "Coffee",
				"--from", "2026-05-10T10:00:00Z",
				"--to", "2026-05-10T11:00:00Z",
				"--location-search", "Cafe",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var got struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			PlaceLookup map[string]string `json:"place_lookup"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v\nout=%s", err, out)
	}
	if !got.DryRun || got.Request.PlaceLookup["query"] != "Cafe" || got.Request.PlaceLookup["mode"] != "text_search" {
		t.Fatalf("unexpected dry-run payload: %#v", got)
	}
}

func TestCalendarCreateDryRunEmptyPlaceIDErrors(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{
				"--json",
				"--dry-run",
				"--no-input",
				"calendar", "create", "primary",
				"--summary", "Coffee",
				"--from", "2026-05-10T10:00:00Z",
				"--to", "2026-05-10T11:00:00Z",
				"--place-id", "",
			})
			if err == nil || !strings.Contains(err.Error(), "empty --place-id") {
				t.Fatalf("expected empty --place-id error, got %v", err)
			}
		})
	})
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected no dry-run output, got %q", out)
	}
}

func TestCalendarUpdateDryRunPlaceIDSkipsPlacesAPI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	t.Setenv("GOG_PLACES_API_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run must not call Places API: %s", r.URL.Path)
	}))
	defer srv.Close()
	t.Setenv("GOG_PLACES_BASE_URL", srv.URL)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--dry-run",
				"--no-input",
				"calendar", "update", "primary", "evt123",
				"--place-id", "places/ChIJ123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var got struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			PlaceLookup map[string]string `json:"place_lookup"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v\nout=%s", err, out)
	}
	if !got.DryRun || got.Request.PlaceLookup["place_id"] != "ChIJ123" || got.Request.PlaceLookup["mode"] != "details" {
		t.Fatalf("unexpected dry-run payload: %#v", got)
	}
}

func TestBuildCalendarCreatePlanAppliesResolvedPlace(t *testing.T) {
	plan, err := buildCalendarCreatePlan(defaultConfigStoreForTest(t), calendarCreateInput{
		CalendarID:  "primary",
		Summary:     "Coffee",
		From:        "2026-05-10T10:00:00Z",
		To:          "2026-05-10T11:00:00Z",
		SendUpdates: "none",
		ResolvedPlace: &calendarPlace{
			ID:               "ChIJ123",
			Name:             "Cafe",
			FormattedAddress: "1 Main St",
			GoogleMapsURI:    "https://maps.example/cafe",
		},
	}, calendarCreateFields{})
	if err != nil {
		t.Fatalf("buildCalendarCreatePlan: %v", err)
	}
	if plan.Event.Location != "Cafe, 1 Main St" {
		t.Fatalf("location = %q", plan.Event.Location)
	}
	props := plan.Event.ExtendedProperties
	if props == nil || props.Private[placeIDPrivateProp] != "ChIJ123" || props.Private[placeMapsURIPrivateProp] == "" {
		t.Fatalf("unexpected place props: %#v", props)
	}
}

func TestApplyCalendarPlacePropertiesMerges(t *testing.T) {
	event := &calendar.Event{ExtendedProperties: buildExtendedProperties([]string{"existing=value"}, nil)}
	applyCalendarPlaceProperties(event, &calendarPlace{ID: "ChIJ123"})

	if event.ExtendedProperties.Private["existing"] != "value" {
		t.Fatalf("existing private prop lost: %#v", event.ExtendedProperties.Private)
	}
	if event.ExtendedProperties.Private[placeIDPrivateProp] != "ChIJ123" {
		t.Fatalf("place private prop missing: %#v", event.ExtendedProperties.Private)
	}
}
