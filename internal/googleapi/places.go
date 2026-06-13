package googleapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultPlacesBaseURL = "https://places.googleapis.com/v1"

var (
	errEmptyPlacesTextSearch = errors.New("empty places text search")
	errNoPlacesMatched       = errors.New("no places matched")
	errEmptyPlaceID          = errors.New("empty place id")
	errMissingPlacesAPIKey   = errors.New("missing Places API key")
	errPlacesAPI             = errors.New("places API error")
)

type PlacesClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type PlacesClientOption func(*PlacesClient)

func WithPlacesBaseURL(baseURL string) PlacesClientOption {
	return func(c *PlacesClient) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		}
	}
}

func WithPlacesHTTPClient(client *http.Client) PlacesClientOption {
	return func(c *PlacesClient) {
		if client != nil {
			c.client = client
		}
	}
}

type Place struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name,omitempty"`
	FormattedAddress string `json:"formatted_address,omitempty"`
	GoogleMapsURI    string `json:"google_maps_uri,omitempty"`
}

type PlacesLookupOptions struct {
	LanguageCode string
	RegionCode   string
}

func NewPlacesClient(apiKey string, opts ...PlacesClientOption) *PlacesClient {
	c := &PlacesClient{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: defaultPlacesBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *PlacesClient) TextSearch(ctx context.Context, query string, opts PlacesLookupOptions) (*Place, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errEmptyPlacesTextSearch
	}

	body := map[string]string{"textQuery": query}
	if opts.LanguageCode != "" {
		body["languageCode"] = opts.LanguageCode
	}

	if opts.RegionCode != "" {
		body["regionCode"] = opts.RegionCode
	}

	var resp struct {
		Places []placeResponse `json:"places"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/places:searchText", body, "places.id,places.displayName,places.formattedAddress,places.googleMapsUri", &resp); err != nil {
		return nil, err
	}

	if len(resp.Places) == 0 {
		return nil, fmt.Errorf("%w: %q", errNoPlacesMatched, query)
	}

	return resp.Places[0].place(), nil
}

func (c *PlacesClient) Details(ctx context.Context, placeID string, opts PlacesLookupOptions) (*Place, error) {
	placeID = normalizePlaceID(placeID)
	if placeID == "" {
		return nil, errEmptyPlaceID
	}

	u, err := url.Parse(c.baseURL + "/places/" + url.PathEscape(placeID))
	if err != nil {
		return nil, fmt.Errorf("build Places details URL: %w", err)
	}

	q := u.Query()
	if opts.LanguageCode != "" {
		q.Set("languageCode", opts.LanguageCode)
	}

	if opts.RegionCode != "" {
		q.Set("regionCode", opts.RegionCode)
	}
	u.RawQuery = q.Encode()

	var resp placeResponse
	if err := c.doJSON(ctx, http.MethodGet, u.String(), nil, "id,displayName,formattedAddress,googleMapsUri", &resp); err != nil {
		return nil, err
	}

	place := resp.place()
	if place.ID == "" {
		place.ID = placeID
	}

	return place, nil
}

func (c *PlacesClient) doJSON(ctx context.Context, method, endpoint string, body any, fieldMask string, out any) error {
	if strings.TrimSpace(c.apiKey) == "" {
		return errMissingPlacesAPIKey
	}

	var reader io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Places API request: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("build Places API request: %w", err)
	}

	req.Header.Set("X-Goog-Api-Key", c.apiKey)
	req.Header.Set("X-Goog-FieldMask", fieldMask)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send Places API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read Places API response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return placesAPIError(resp.StatusCode, respBody)
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode Places API response: %w", err)
	}

	return nil
}

//nolint:tagliatelle // Google Places API uses lowerCamelCase JSON fields.
type placeResponse struct {
	ID          string `json:"id"`
	DisplayName struct {
		Text string `json:"text"`
	} `json:"displayName"`
	FormattedAddress string `json:"formattedAddress"`
	GoogleMapsURI    string `json:"googleMapsUri"`
}

func (p placeResponse) place() *Place {
	return &Place{
		ID:               strings.TrimSpace(p.ID),
		Name:             strings.TrimSpace(p.DisplayName.Text),
		FormattedAddress: strings.TrimSpace(p.FormattedAddress),
		GoogleMapsURI:    strings.TrimSpace(p.GoogleMapsURI),
	}
}

func normalizePlaceID(placeID string) string {
	placeID = strings.TrimSpace(placeID)
	placeID = strings.TrimPrefix(placeID, "places/")

	return strings.TrimSpace(placeID)
}

func placesAPIError(statusCode int, body []byte) error {
	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		if parsed.Error.Status != "" {
			return &HTTPStatusError{
				Code:   statusCode,
				Status: parsed.Error.Status,
				Err:    fmt.Errorf("%w %d %s: %s", errPlacesAPI, statusCode, parsed.Error.Status, parsed.Error.Message),
			}
		}

		return &HTTPStatusError{
			Code: statusCode,
			Err:  fmt.Errorf("%w %d: %s", errPlacesAPI, statusCode, parsed.Error.Message),
		}
	}

	return &HTTPStatusError{
		Code: statusCode,
		Err:  fmt.Errorf("%w %d: %s", errPlacesAPI, statusCode, strings.TrimSpace(string(body))),
	}
}
