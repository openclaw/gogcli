package googleapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Photos Library API writes that remain available after the 2025 scope
// changes: uploading bytes, creating media items (optionally into an
// app-created album), and creating/listing app-created albums. All of them
// need photoslibrary.appendonly; none can touch media or albums the app did
// not create.

const photosBatchCreateMax = 50

var (
	errEmptyPhotosUpload      = errors.New("empty upload")
	errEmptyPhotosAlbumTitle  = errors.New("empty album title")
	errEmptyPhotosUploadToken = errors.New("Photos upload returned an empty upload token")
	errTooManyPhotosNewItems  = fmt.Errorf("at most %d media items per batch", photosBatchCreateMax)
)

//nolint:tagliatelle // Google Photos API uses lowerCamelCase JSON fields.
type PhotosAlbum struct {
	ID                    string `json:"id,omitempty"`
	Title                 string `json:"title,omitempty"`
	ProductURL            string `json:"productUrl,omitempty"`
	IsWriteable           bool   `json:"isWriteable,omitempty"`
	MediaItemsCount       string `json:"mediaItemsCount,omitempty"`
	CoverPhotoBaseURL     string `json:"coverPhotoBaseUrl,omitempty"`
	CoverPhotoMediaItemID string `json:"coverPhotoMediaItemId,omitempty"`
}

//nolint:tagliatelle // Google Photos API uses lowerCamelCase JSON fields.
type PhotosAlbumsResponse struct {
	Albums        []*PhotosAlbum `json:"albums,omitempty"`
	NextPageToken string         `json:"nextPageToken,omitempty"`
}

// PhotosNewMediaItem is one upload token to turn into a media item.
type PhotosNewMediaItem struct {
	UploadToken string
	FileName    string
	Description string
}

//nolint:tagliatelle // Google Photos API uses lowerCamelCase JSON fields.
type PhotosNewMediaItemResult struct {
	UploadToken string           `json:"uploadToken,omitempty"`
	Status      *PhotosStatus    `json:"status,omitempty"`
	MediaItem   *PhotosMediaItem `json:"mediaItem,omitempty"`
}

type PhotosStatus struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

//nolint:tagliatelle // Google Photos API uses lowerCamelCase JSON fields.
type PhotosBatchCreateResponse struct {
	NewMediaItemResults []*PhotosNewMediaItemResult `json:"newMediaItemResults,omitempty"`
}

// UploadBytes sends raw media bytes and returns the upload token that
// BatchCreateMediaItems turns into a media item. Tokens expire after a day.
func (c *PhotosClient) UploadBytes(ctx context.Context, body io.Reader, size int64, mimeType, fileName string) (string, error) {
	if body == nil || size == 0 {
		return "", errEmptyPhotosUpload
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/uploads", body)
	if err != nil {
		return "", fmt.Errorf("build Photos upload request: %w", err)
	}

	if size > 0 {
		req.ContentLength = size
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Goog-Upload-Protocol", "raw")

	if mt := strings.TrimSpace(mimeType); mt != "" {
		req.Header.Set("X-Goog-Upload-Content-Type", mt)
	}

	if name := strings.TrimSpace(fileName); name != "" {
		req.Header.Set("X-Goog-Upload-File-Name", name)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send Photos upload: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("read Photos upload response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", photosAPIError(resp.StatusCode, respBody)
	}

	token := strings.TrimSpace(string(respBody))
	if token == "" {
		return "", errEmptyPhotosUploadToken
	}

	return token, nil
}

// BatchCreateMediaItems creates up to 50 media items from upload tokens,
// optionally adding them to an app-created album. Per-item failures are
// reported in the results, not as an error.
func (c *PhotosClient) BatchCreateMediaItems(ctx context.Context, albumID string, items []PhotosNewMediaItem) (*PhotosBatchCreateResponse, error) {
	if len(items) == 0 {
		return &PhotosBatchCreateResponse{}, nil
	}

	if len(items) > photosBatchCreateMax {
		return nil, errTooManyPhotosNewItems
	}

	newItems := make([]map[string]any, 0, len(items))
	for _, item := range items {
		simple := map[string]any{"uploadToken": item.UploadToken}
		if name := strings.TrimSpace(item.FileName); name != "" {
			simple["fileName"] = name
		}

		entry := map[string]any{"simpleMediaItem": simple}
		if desc := strings.TrimSpace(item.Description); desc != "" {
			entry["description"] = desc
		}

		newItems = append(newItems, entry)
	}

	body := map[string]any{"newMediaItems": newItems}
	if id := strings.TrimSpace(albumID); id != "" {
		body["albumId"] = id
	}

	var out PhotosBatchCreateResponse
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/mediaItems:batchCreate", body, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// CreateAlbum creates an app-created album. Only app-created albums can
// receive uploads through the API.
func (c *PhotosClient) CreateAlbum(ctx context.Context, title string) (*PhotosAlbum, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errEmptyPhotosAlbumTitle
	}

	var out PhotosAlbum

	body := map[string]any{"album": map[string]any{"title": title}}
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/albums", body, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// ListAlbums lists albums visible to this app, which since the 2025 scope
// changes means app-created albums only.
func (c *PhotosClient) ListAlbums(ctx context.Context, pageSize int64, pageToken string) (*PhotosAlbumsResponse, error) {
	u, err := url.Parse(c.baseURL + "/albums")
	if err != nil {
		return nil, fmt.Errorf("build Photos albums URL: %w", err)
	}

	q := u.Query()
	if pageSize > 0 {
		q.Set("pageSize", fmt.Sprint(pageSize))
	}

	if strings.TrimSpace(pageToken) != "" {
		q.Set("pageToken", strings.TrimSpace(pageToken))
	}

	u.RawQuery = q.Encode()

	var out PhotosAlbumsResponse
	if err := c.doJSON(ctx, http.MethodGet, u.String(), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
