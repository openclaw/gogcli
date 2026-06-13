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

const defaultPhotosPickerBaseURL = "https://photospicker.googleapis.com/v1"

var (
	errEmptyPhotosPickerSessionID   = errors.New("empty Photos Picker session id")
	errEmptyPhotosPickerMediaItemID = errors.New("empty Photos Picker media item id")
	errPhotosPickerMediaItemMissing = errors.New("media item not found in Photos Picker session")
	errPhotosPickerRepeatedPage     = errors.New("repeated page token from Photos Picker")
	errPhotosPickerMediaFileMissing = errors.New("media item has no media file")
	errPhotosPickerBaseURLMissing   = errors.New("media item has no base URL")
	errPhotosPickerVideoNotReady    = errors.New("video is not ready")
	errPhotosPickerDownloadFailed   = errors.New("media download failed")
)

type PhotosPickerClient struct {
	baseURL string
	client  *http.Client
}

type PhotosPickerClientOption func(*PhotosPickerClient)

func WithPhotosPickerBaseURL(baseURL string) PhotosPickerClientOption {
	return func(c *PhotosPickerClient) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		}
	}
}

func NewPhotosPickerClient(client *http.Client, opts ...PhotosPickerClientOption) *PhotosPickerClient {
	if client == nil {
		client = http.DefaultClient
	}

	c := &PhotosPickerClient{
		baseURL: defaultPhotosPickerBaseURL,
		client:  client,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func NewPhotosPickerClientForAccount(ctx context.Context, email string, opts ...PhotosPickerClientOption) (*PhotosPickerClient, error) {
	client, err := NewHTTPClient(ctx, googleauth.ServicePhotosPicker, email)
	if err != nil {
		return nil, err
	}

	return NewPhotosPickerClient(client, opts...), nil
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerSession struct {
	ID            string                     `json:"id,omitempty"`
	PickerURI     string                     `json:"pickerUri,omitempty"`
	PollingConfig *PhotosPickerPollingConfig `json:"pollingConfig,omitempty"`
	ExpireTime    string                     `json:"expireTime,omitempty"`
	PickingConfig *PhotosPickerPickingConfig `json:"pickingConfig,omitempty"`
	MediaItemsSet bool                       `json:"mediaItemsSet,omitempty"`
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerPollingConfig struct {
	PollInterval string `json:"pollInterval,omitempty"`
	TimeoutIn    string `json:"timeoutIn,omitempty"`
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerPickingConfig struct {
	MaxItemCount int64 `json:"maxItemCount,string,omitempty"`
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerMediaItemsResponse struct {
	MediaItems    []*PhotosPickerMediaItem `json:"mediaItems,omitempty"`
	NextPageToken string                   `json:"nextPageToken,omitempty"`
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerMediaItem struct {
	ID         string                 `json:"id,omitempty"`
	CreateTime string                 `json:"createTime,omitempty"`
	Type       string                 `json:"type,omitempty"`
	MediaFile  *PhotosPickerMediaFile `json:"mediaFile,omitempty"`
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerMediaFile struct {
	BaseURL           string                         `json:"baseUrl,omitempty"`
	MimeType          string                         `json:"mimeType,omitempty"`
	Filename          string                         `json:"filename,omitempty"`
	MediaFileMetadata *PhotosPickerMediaFileMetadata `json:"mediaFileMetadata,omitempty"`
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerMediaFileMetadata struct {
	Width         int64                      `json:"width,omitempty"`
	Height        int64                      `json:"height,omitempty"`
	CameraMake    string                     `json:"cameraMake,omitempty"`
	CameraModel   string                     `json:"cameraModel,omitempty"`
	PhotoMetadata *PhotosPickerPhotoMetadata `json:"photoMetadata,omitempty"`
	VideoMetadata *PhotosPickerVideoMetadata `json:"videoMetadata,omitempty"`
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerPhotoMetadata struct {
	FocalLength     float64 `json:"focalLength,omitempty"`
	ApertureFNumber float64 `json:"apertureFNumber,omitempty"`
	ISOEquivalent   int64   `json:"isoEquivalent,omitempty"`
	ExposureTime    string  `json:"exposureTime,omitempty"`
}

//nolint:tagliatelle // Google Photos Picker API uses lowerCamelCase JSON fields.
type PhotosPickerVideoMetadata struct {
	FPS              float64 `json:"fps,omitempty"`
	ProcessingStatus string  `json:"processingStatus,omitempty"`
}

func (c *PhotosPickerClient) CreateSession(ctx context.Context, maxItemCount int64) (*PhotosPickerSession, error) {
	body := &PhotosPickerSession{}
	if maxItemCount > 0 {
		body.PickingConfig = &PhotosPickerPickingConfig{MaxItemCount: maxItemCount}
	}

	var out PhotosPickerSession
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/sessions", body, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *PhotosPickerClient) GetSession(ctx context.Context, sessionID string) (*PhotosPickerSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errEmptyPhotosPickerSessionID
	}

	var out PhotosPickerSession
	if err := c.doJSON(ctx, http.MethodGet, c.baseURL+"/sessions/"+url.PathEscape(sessionID), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *PhotosPickerClient) DeleteSession(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errEmptyPhotosPickerSessionID
	}

	return c.doJSON(ctx, http.MethodDelete, c.baseURL+"/sessions/"+url.PathEscape(sessionID), nil, nil)
}

func (c *PhotosPickerClient) ListMediaItems(
	ctx context.Context,
	sessionID string,
	pageSize int64,
	pageToken string,
) (*PhotosPickerMediaItemsResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errEmptyPhotosPickerSessionID
	}

	u, err := url.Parse(c.baseURL + "/mediaItems")
	if err != nil {
		return nil, fmt.Errorf("build Photos Picker media list URL: %w", err)
	}

	q := u.Query()
	q.Set("sessionId", sessionID)

	if pageSize > 0 {
		q.Set("pageSize", fmt.Sprint(pageSize))
	}

	if strings.TrimSpace(pageToken) != "" {
		q.Set("pageToken", strings.TrimSpace(pageToken))
	}
	u.RawQuery = q.Encode()

	var out PhotosPickerMediaItemsResponse
	if err := c.doJSON(ctx, http.MethodGet, u.String(), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *PhotosPickerClient) FindMediaItem(
	ctx context.Context,
	sessionID string,
	mediaItemID string,
) (*PhotosPickerMediaItem, error) {
	mediaItemID = strings.TrimSpace(mediaItemID)
	if mediaItemID == "" {
		return nil, errEmptyPhotosPickerMediaItemID
	}

	pageToken := ""
	seenTokens := map[string]struct{}{}

	for {
		resp, err := c.ListMediaItems(ctx, sessionID, 100, pageToken)
		if err != nil {
			return nil, err
		}

		for _, item := range resp.MediaItems {
			if item != nil && item.ID == mediaItemID {
				return item, nil
			}
		}

		next := strings.TrimSpace(resp.NextPageToken)
		if next == "" {
			return nil, fmt.Errorf("%w: %s", errPhotosPickerMediaItemMissing, mediaItemID)
		}

		if _, exists := seenTokens[next]; exists {
			return nil, errPhotosPickerRepeatedPage
		}
		seenTokens[next] = struct{}{}
		pageToken = next
	}
}

func (c *PhotosPickerClient) DownloadMedia(ctx context.Context, item *PhotosPickerMediaItem) (*http.Response, error) {
	if item == nil || item.MediaFile == nil {
		return nil, errPhotosPickerMediaFileMissing
	}

	baseURL := strings.TrimSpace(item.MediaFile.BaseURL)
	if baseURL == "" {
		return nil, errPhotosPickerBaseURLMissing
	}

	suffix := "=d"

	if strings.EqualFold(strings.TrimSpace(item.Type), "VIDEO") {
		if metadata := item.MediaFile.MediaFileMetadata; metadata != nil && metadata.VideoMetadata != nil {
			status := strings.ToUpper(strings.TrimSpace(metadata.VideoMetadata.ProcessingStatus))
			if status != "" && status != "READY" {
				return nil, fmt.Errorf("%w: %s", errPhotosPickerVideoNotReady, status)
			}
		}
		suffix = "=dv"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+suffix, nil)
	if err != nil {
		return nil, fmt.Errorf("build Photos Picker download request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download Photos Picker media item: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	return nil, &HTTPStatusError{
		Code: resp.StatusCode,
		Err: fmt.Errorf(
			"%w: HTTP %d: %s",
			errPhotosPickerDownloadFailed,
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		),
	}
}

func (c *PhotosPickerClient) doJSON(ctx context.Context, method string, endpoint string, body any, out any) error {
	var reader io.Reader

	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Photos Picker API request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("build Photos Picker API request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send Photos Picker API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read Photos Picker API response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return photosAPIError(resp.StatusCode, respBody)
	}

	if out == nil || len(bytes.TrimSpace(respBody)) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode Photos Picker API response: %w", err)
	}

	return nil
}
