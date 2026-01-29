package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/steipete/goplaces"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type stubPlacesClient struct {
	resolve func(ctx context.Context, req goplaces.LocationResolveRequest) (goplaces.LocationResolveResponse, error)
	details func(ctx context.Context, req goplaces.DetailsRequest) (goplaces.PlaceDetails, error)
}

func (s *stubPlacesClient) Resolve(ctx context.Context, req goplaces.LocationResolveRequest) (goplaces.LocationResolveResponse, error) {
	return s.resolve(ctx, req)
}

func (s *stubPlacesClient) DetailsWithOptions(ctx context.Context, req goplaces.DetailsRequest) (goplaces.PlaceDetails, error) {
	return s.details(ctx, req)
}

func TestCalendarCreateCmd_LocationSearch(t *testing.T) {
	origNew := newCalendarService
	origPlaces := newPlacesClient
	t.Cleanup(func() {
		newCalendarService = origNew
		newPlacesClient = origPlaces
	})
	t.Setenv("GOOGLE_PLACES_API_KEY", "test-key")

	stub := &stubPlacesClient{
		resolve: func(ctx context.Context, req goplaces.LocationResolveRequest) (goplaces.LocationResolveResponse, error) {
			return goplaces.LocationResolveResponse{
				Results: []goplaces.ResolvedLocation{
					{
						PlaceID: "place-123",
						Name:    "Elysian Coffee",
						Address: "Vancouver, BC",
					},
				},
			}, nil
		},
		details: func(ctx context.Context, req goplaces.DetailsRequest) (goplaces.PlaceDetails, error) {
			return goplaces.PlaceDetails{}, nil
		},
	}
	newPlacesClient = func(opts goplaces.Options) placesClient {
		return stub
	}

	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal/events" {
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev1"})
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

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal",
		"--summary", "Coffee",
		"--from", "2025-01-02T10:00:00Z",
		"--to", "2025-01-02T11:00:00Z",
		"--location-search", "Elysian Coffee Vancouver",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.Location != "Elysian Coffee, Vancouver, BC" {
		t.Fatalf("unexpected location: %q", gotEvent.Location)
	}
	if gotEvent.ExtendedProperties == nil || gotEvent.ExtendedProperties.Private[placeIDProp] != "place-123" {
		t.Fatalf("expected place id in extended properties, got %#v", gotEvent.ExtendedProperties)
	}
}

func TestCalendarUpdateCmd_PlaceID(t *testing.T) {
	origNew := newCalendarService
	origPlaces := newPlacesClient
	t.Cleanup(func() {
		newCalendarService = origNew
		newPlacesClient = origPlaces
	})
	t.Setenv("GOOGLE_PLACES_API_KEY", "test-key")

	stub := &stubPlacesClient{
		resolve: func(ctx context.Context, req goplaces.LocationResolveRequest) (goplaces.LocationResolveResponse, error) {
			return goplaces.LocationResolveResponse{}, nil
		},
		details: func(ctx context.Context, req goplaces.DetailsRequest) (goplaces.PlaceDetails, error) {
			return goplaces.PlaceDetails{
				PlaceID: "place-999",
				Name:    "Elysian Coffee",
				Address: "Vancouver, BC",
			}, nil
		},
	}
	newPlacesClient = func(opts goplaces.Options) placesClient {
		return stub
	}

	var gotPatch calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
				"extendedProperties": map[string]any{
					"private": map[string]any{
						"keep": "yes",
					},
				},
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal/events/ev":
			_ = json.NewDecoder(r.Body).Decode(&gotPatch)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal",
		"ev",
		"--place-id", "place-999",
		"--scope", "all",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotPatch.Location != "Elysian Coffee, Vancouver, BC" {
		t.Fatalf("unexpected patch location: %q", gotPatch.Location)
	}
	if gotPatch.ExtendedProperties == nil || gotPatch.ExtendedProperties.Private[placeIDProp] != "place-999" {
		t.Fatalf("expected place id in patch extended properties, got %#v", gotPatch.ExtendedProperties)
	}
	if gotPatch.ExtendedProperties.Private["keep"] != "yes" {
		t.Fatalf("expected existing private prop preserved, got %#v", gotPatch.ExtendedProperties.Private)
	}
}
