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

	"github.com/steipete/gogcli/internal/googleauth"
)

const defaultPhotosBaseURL = "https://photoslibrary.googleapis.com/v1"

var (
	errEmptyPhotosMediaItemID = errors.New("empty media item id")
	errPhotosAPI              = errors.New("photos API error")
)

type PhotosClient struct {
	baseURL string
	client  *http.Client
}

type PhotosClientOption func(*PhotosClient)

func WithPhotosBaseURL(baseURL string) PhotosClientOption {
	return func(c *PhotosClient) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		}
	}
}

func NewPhotosClient(client *http.Client, opts ...PhotosClientOption) *PhotosClient {
	if client == nil {
		client = http.DefaultClient
	}

	c := &PhotosClient{
		baseURL: defaultPhotosBaseURL,
		client:  client,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func NewPhotosClientForAccount(ctx context.Context, email string, opts ...PhotosClientOption) (*PhotosClient, error) {
	client, err := NewHTTPClient(ctx, googleauth.ServicePhotos, email)
	if err != nil {
		return nil, err
	}

	return NewPhotosClient(client, opts...), nil
}

type PhotosListOptions struct {
	PageSize  int64
	PageToken string
}

type PhotosSearchOptions struct {
	PageSize             int64
	PageToken            string
	AlbumID              string
	MediaType            string
	StartDate            *PhotosDate
	EndDate              *PhotosDate
	IncludeArchivedMedia bool
	OrderBy              string
}

type PhotosDate struct {
	Year  int `json:"year,omitempty"`
	Month int `json:"month,omitempty"`
	Day   int `json:"day,omitempty"`
}

//nolint:tagliatelle // Google Photos API uses lowerCamelCase JSON fields.
type PhotosMediaItem struct {
	ID            string               `json:"id,omitempty"`
	Description   string               `json:"description,omitempty"`
	ProductURL    string               `json:"productUrl,omitempty"`
	BaseURL       string               `json:"baseUrl,omitempty"`
	MimeType      string               `json:"mimeType,omitempty"`
	Filename      string               `json:"filename,omitempty"`
	MediaMetadata *PhotosMediaMetadata `json:"mediaMetadata,omitempty"`
}

//nolint:tagliatelle // Google Photos API uses lowerCamelCase JSON fields.
type PhotosMediaMetadata struct {
	CreationTime string       `json:"creationTime,omitempty"`
	Width        string       `json:"width,omitempty"`
	Height       string       `json:"height,omitempty"`
	Photo        any          `json:"photo,omitempty"`
	Video        *PhotosVideo `json:"video,omitempty"`
}

type PhotosVideo struct {
	Status string `json:"status,omitempty"`
}

//nolint:tagliatelle // Google Photos API uses lowerCamelCase JSON fields.
type PhotosMediaItemsResponse struct {
	MediaItems    []*PhotosMediaItem `json:"mediaItems,omitempty"`
	NextPageToken string             `json:"nextPageToken,omitempty"`
}

func (c *PhotosClient) ListMediaItems(ctx context.Context, opts PhotosListOptions) (*PhotosMediaItemsResponse, error) {
	u, err := url.Parse(c.baseURL + "/mediaItems")
	if err != nil {
		return nil, fmt.Errorf("build Photos list URL: %w", err)
	}

	q := u.Query()
	if opts.PageSize > 0 {
		q.Set("pageSize", fmt.Sprint(opts.PageSize))
	}

	if strings.TrimSpace(opts.PageToken) != "" {
		q.Set("pageToken", strings.TrimSpace(opts.PageToken))
	}

	u.RawQuery = q.Encode()

	var out PhotosMediaItemsResponse
	if err := c.doJSON(ctx, http.MethodGet, u.String(), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *PhotosClient) SearchMediaItems(ctx context.Context, opts PhotosSearchOptions) (*PhotosMediaItemsResponse, error) {
	body := map[string]any{}
	if opts.PageSize > 0 {
		body["pageSize"] = opts.PageSize
	}

	if strings.TrimSpace(opts.PageToken) != "" {
		body["pageToken"] = strings.TrimSpace(opts.PageToken)
	}

	if strings.TrimSpace(opts.AlbumID) != "" {
		body["albumId"] = strings.TrimSpace(opts.AlbumID)
	}

	if strings.TrimSpace(opts.OrderBy) != "" {
		body["orderBy"] = strings.TrimSpace(opts.OrderBy)
	}

	filters := map[string]any{}
	if opts.IncludeArchivedMedia {
		filters["includeArchivedMedia"] = true
	}

	if mt := strings.ToUpper(strings.TrimSpace(opts.MediaType)); mt != "" && mt != "ALL_MEDIA" {
		filters["mediaTypeFilter"] = map[string]any{"mediaTypes": []string{mt}}
	}

	if opts.StartDate != nil || opts.EndDate != nil {
		dr := map[string]any{}
		if opts.StartDate != nil {
			dr["startDate"] = opts.StartDate
		}

		if opts.EndDate != nil {
			dr["endDate"] = opts.EndDate
		}

		filters["dateFilter"] = map[string]any{"ranges": []map[string]any{dr}}
	}

	if len(filters) > 0 {
		body["filters"] = filters
	}

	var out PhotosMediaItemsResponse
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/mediaItems:search", body, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *PhotosClient) GetMediaItem(ctx context.Context, id string) (*PhotosMediaItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errEmptyPhotosMediaItemID
	}

	var out PhotosMediaItem
	if err := c.doJSON(ctx, http.MethodGet, c.baseURL+"/mediaItems/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *PhotosClient) doJSON(ctx context.Context, method, endpoint string, body any, out any) error {
	var reader io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Photos API request: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("build Photos API request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send Photos API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read Photos API response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return photosAPIError(resp.StatusCode, respBody)
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode Photos API response: %w", err)
	}

	return nil
}

func photosAPIError(statusCode int, body []byte) error {
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
				Err:    fmt.Errorf("%w (%d %s): %s", errPhotosAPI, statusCode, parsed.Error.Status, parsed.Error.Message),
			}
		}

		return &HTTPStatusError{
			Code: statusCode,
			Err:  fmt.Errorf("%w (%d): %s", errPhotosAPI, statusCode, parsed.Error.Message),
		}
	}

	return &HTTPStatusError{
		Code: statusCode,
		Err:  fmt.Errorf("%w (%d): %s", errPhotosAPI, statusCode, strings.TrimSpace(string(body))),
	}
}
