package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const (
	placeIDPrivateProp      = "gog.place_id"
	placeMapsURIPrivateProp = "gog.place_maps_uri"
)

type calendarPlace struct {
	ID               string
	Name             string
	FormattedAddress string
	GoogleMapsURI    string
}

func (c *CalendarCreateCmd) resolvePlace(ctx context.Context, fields calendarCreateFields) error {
	place, err := resolveCalendarPlace(ctx, calendarPlaceLookup{
		LocationSet:       fields.Location || strings.TrimSpace(c.Location) != "",
		LocationSearch:    c.LocationSearch,
		LocationSearchSet: fields.LocationSearch,
		PlaceID:           c.PlaceID,
		PlaceIDSet:        fields.PlaceID,
		LanguageCode:      c.PlaceLanguage,
		RegionCode:        c.PlaceRegion,
	})
	if err != nil {
		return err
	}
	c.resolvedPlace = place
	return nil
}

func (c *CalendarUpdateCmd) resolvePlace(ctx context.Context, fields calendarUpdateFields) error {
	place, err := resolveCalendarPlace(ctx, calendarPlaceLookup{
		LocationSet:       fields.Location,
		LocationSearch:    c.LocationSearch,
		LocationSearchSet: fields.LocationSearch,
		PlaceID:           c.PlaceID,
		PlaceIDSet:        fields.PlaceID,
		LanguageCode:      c.PlaceLanguage,
		RegionCode:        c.PlaceRegion,
	})
	if err != nil {
		return err
	}
	c.resolvedPlace = place
	return nil
}

type calendarPlaceLookup struct {
	LocationSet       bool
	LocationSearch    string
	LocationSearchSet bool
	PlaceID           string
	PlaceIDSet        bool
	LanguageCode      string
	RegionCode        string
}

type calendarPlaceLookupRequest struct {
	Mode         string `json:"mode"`
	Query        string `json:"query,omitempty"`
	PlaceID      string `json:"place_id,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
	RegionCode   string `json:"region_code,omitempty"`
}

func (r *calendarPlaceLookupRequest) dryRunPayload() map[string]string {
	if r == nil {
		return nil
	}
	payload := map[string]string{"mode": r.Mode}
	if r.Query != "" {
		payload["query"] = r.Query
	}
	if r.PlaceID != "" {
		payload["place_id"] = r.PlaceID
	}
	if r.LanguageCode != "" {
		payload["language_code"] = r.LanguageCode
	}
	if r.RegionCode != "" {
		payload["region_code"] = r.RegionCode
	}
	return payload
}

func validateCalendarPlaceLookup(lookup calendarPlaceLookup) (*calendarPlaceLookupRequest, error) {
	search := strings.TrimSpace(lookup.LocationSearch)
	placeID := strings.TrimSpace(lookup.PlaceID)
	searchSet := lookup.LocationSearchSet || search != ""
	placeIDSet := lookup.PlaceIDSet || placeID != ""

	if searchSet && search == "" {
		return nil, usage("empty --location-search")
	}
	if placeIDSet && placeID == "" {
		return nil, usage("empty --place-id")
	}
	if search != "" && placeID != "" {
		return nil, usage("use either --location-search or --place-id, not both")
	}
	if lookup.LocationSet && (search != "" || placeID != "") {
		return nil, usage("cannot combine --location with --location-search or --place-id")
	}
	if search == "" && placeID == "" {
		return nil, nil //nolint:nilnil // no lookup requested
	}
	request := &calendarPlaceLookupRequest{
		LanguageCode: strings.TrimSpace(lookup.LanguageCode),
		RegionCode:   strings.TrimSpace(lookup.RegionCode),
	}
	if search != "" {
		request.Mode = "text_search"
		request.Query = search
	} else {
		placeID = strings.TrimPrefix(placeID, "places/")
		if placeID == "" {
			return nil, usage("empty --place-id")
		}
		request.Mode = "details"
		request.PlaceID = placeID
	}
	return request, nil
}

func resolveCalendarPlace(ctx context.Context, lookup calendarPlaceLookup) (*calendarPlace, error) {
	request, err := validateCalendarPlaceLookup(lookup)
	if err != nil {
		return nil, err
	}
	if request == nil {
		return nil, nil //nolint:nilnil // no lookup requested
	}

	apiKey, err := placesAPIKey(ctx)
	if err != nil {
		return nil, err
	}
	client := googleapi.NewPlacesClient(apiKey, googleapi.WithPlacesBaseURL(os.Getenv("GOG_PLACES_BASE_URL")))
	opts := googleapi.PlacesLookupOptions{
		LanguageCode: strings.TrimSpace(lookup.LanguageCode),
		RegionCode:   strings.TrimSpace(lookup.RegionCode),
	}

	var place *googleapi.Place
	if request.Query != "" {
		place, err = client.TextSearch(ctx, request.Query, opts)
	} else {
		place, err = client.Details(ctx, request.PlaceID, opts)
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(place.ID) == "" && request.PlaceID != "" {
		place.ID = request.PlaceID
	}
	return &calendarPlace{
		ID:               strings.TrimSpace(place.ID),
		Name:             strings.TrimSpace(place.Name),
		FormattedAddress: strings.TrimSpace(place.FormattedAddress),
		GoogleMapsURI:    strings.TrimSpace(place.GoogleMapsURI),
	}, nil
}

func placesAPIKey(ctx context.Context) (string, error) {
	cfg, err := loadConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("read config for Places API key: %w", err)
	}
	if key := strings.TrimSpace(config.GetValue(cfg, config.KeyPlacesAPIKey)); key != "" {
		return key, nil
	}
	return "", usage("Google Maps/Places API key required. Set GOG_PLACES_API_KEY, GOOGLE_PLACES_API_KEY, or run 'gog config set places_api_key <key>'")
}

func formatCalendarPlaceLocation(place *calendarPlace) string {
	if place == nil {
		return ""
	}
	name := strings.TrimSpace(place.Name)
	address := strings.TrimSpace(place.FormattedAddress)
	switch {
	case name != "" && address != "":
		return name + ", " + address
	case name != "":
		return name
	case address != "":
		return address
	default:
		return strings.TrimSpace(place.ID)
	}
}

func applyCalendarPlaceProperties(event *calendar.Event, place *calendarPlace) {
	if event == nil || place == nil {
		return
	}
	if event.ExtendedProperties == nil {
		event.ExtendedProperties = &calendar.EventExtendedProperties{}
	}
	if event.ExtendedProperties.Private == nil {
		event.ExtendedProperties.Private = map[string]string{}
	}
	if id := strings.TrimSpace(place.ID); id != "" {
		event.ExtendedProperties.Private[placeIDPrivateProp] = id
	}
	if mapsURI := strings.TrimSpace(place.GoogleMapsURI); mapsURI != "" {
		event.ExtendedProperties.Private[placeMapsURIPrivateProp] = mapsURI
	}
}
