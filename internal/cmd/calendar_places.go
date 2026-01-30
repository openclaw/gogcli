package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/steipete/goplaces"

	placescfg "github.com/steipete/gogcli/internal/places"
)

const placeIDProp = "gog.place_id"

type placeLookup struct {
	SearchText string
	PlaceID    string
	Language   string
	Region     string
}

type calendarPlace struct {
	PlaceID string
	Name    string
	Address string
}

func validatePlaceLocationFlags(locationSet, locationSearchProvided bool, locationSearch string, placeIDProvided bool, placeID string) error {
	searchText := strings.TrimSpace(locationSearch)
	rawPlaceID := strings.TrimSpace(placeID)

	if locationSearchProvided && searchText == "" {
		return usage("empty --location-search")
	}
	if placeIDProvided && rawPlaceID == "" {
		return usage("empty --place-id")
	}
	if searchText != "" && rawPlaceID != "" {
		return usage("use either --location-search or --place-id (not both)")
	}
	if locationSet && (searchText != "" || rawPlaceID != "") {
		return usage("cannot combine --location with --location-search or --place-id")
	}
	return nil
}

func resolveCalendarPlace(ctx context.Context, lookup placeLookup) (*calendarPlace, error) {
	searchText := strings.TrimSpace(lookup.SearchText)
	placeID := strings.TrimSpace(lookup.PlaceID)
	if searchText == "" && placeID == "" {
		return nil, nil //nolint:nilnil // intentional: no lookup needed
	}

	client, err := newPlacesClientFromConfig()
	if err != nil {
		return nil, err
	}

	language := strings.TrimSpace(lookup.Language)
	region := strings.TrimSpace(lookup.Region)

	if searchText != "" {
		resp, resolveErr := client.Resolve(ctx, goplaces.LocationResolveRequest{
			LocationText: searchText,
			Limit:        1,
			Language:     language,
			Region:       region,
		})
		if resolveErr != nil {
			return nil, resolveErr
		}
		if len(resp.Results) == 0 {
			return nil, fmt.Errorf("no places matched %q", searchText)
		}
		match := resp.Results[0]
		if strings.TrimSpace(match.PlaceID) == "" {
			return nil, fmt.Errorf("places lookup returned empty place id for %q", searchText)
		}
		return &calendarPlace{
			PlaceID: match.PlaceID,
			Name:    match.Name,
			Address: match.Address,
		}, nil
	}

	details, err := client.DetailsWithOptions(ctx, goplaces.DetailsRequest{
		PlaceID:  placeID,
		Language: language,
		Region:   region,
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(details.PlaceID) == "" {
		details.PlaceID = placeID
	}
	return &calendarPlace{
		PlaceID: details.PlaceID,
		Name:    details.Name,
		Address: details.Address,
	}, nil
}

func newPlacesClientFromConfig() (placesClient, error) {
	state, err := placescfg.LoadAPIKey()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(state.Key) == "" {
		return nil, usage("Places API key required for --location-search or --place-id. Set GOOGLE_PLACES_API_KEY, configure it in 'gog auth manage', or run 'gog config set places_api_key <key>'")
	}
	opts := goplaces.Options{
		APIKey:        state.Key,
		BaseURL:       strings.TrimSpace(os.Getenv("GOOGLE_PLACES_BASE_URL")),
		RoutesBaseURL: strings.TrimSpace(os.Getenv("GOOGLE_ROUTES_BASE_URL")),
		Timeout:       10 * time.Second,
	}
	return newPlacesClient(opts), nil
}

func formatPlaceLocation(place *calendarPlace) (string, error) {
	if place == nil {
		return "", nil
	}
	name := strings.TrimSpace(place.Name)
	address := strings.TrimSpace(place.Address)
	switch {
	case name != "" && address != "":
		return fmt.Sprintf("%s, %s", name, address), nil
	case name != "":
		return name, nil
	case address != "":
		return address, nil
	default:
		return "", fmt.Errorf("place has no name or address")
	}
}

func placePrivateProps(place *calendarPlace) map[string]string {
	if place == nil {
		return nil
	}
	placeID := strings.TrimSpace(place.PlaceID)
	if placeID == "" {
		return nil
	}
	return map[string]string{placeIDProp: placeID}
}

func placeLabel(place *calendarPlace) string {
	if place == nil {
		return ""
	}
	if strings.TrimSpace(place.Name) != "" {
		return strings.TrimSpace(place.Name)
	}
	if strings.TrimSpace(place.Address) != "" {
		return strings.TrimSpace(place.Address)
	}
	return strings.TrimSpace(place.PlaceID)
}
