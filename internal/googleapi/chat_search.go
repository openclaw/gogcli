package googleapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/api/chat/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/googleauth"
)

type ChatSearchClient struct {
	client  *http.Client
	baseURL string
}

func NewChatSearchClient(client *http.Client, baseURL string) *ChatSearchClient {
	if client == nil {
		client = NewBoundedHTTPClient()
	}

	if baseURL == "" {
		baseURL = "https://chat.googleapis.com/v1"
	}

	return &ChatSearchClient{client: client, baseURL: strings.TrimRight(baseURL, "/")}
}

func NewChatSearchClientForAccount(ctx context.Context, email string) (*ChatSearchClient, error) {
	client, err := NewHTTPClient(ctx, googleauth.ServiceChat, email)
	if err != nil {
		return nil, err
	}

	return NewChatSearchClient(client, ""), nil
}

// The generated Discovery type loses presence for the proto's optional read
// field. Decode it here so unavailable read-state metadata stays unknown.
//
//nolint:tagliatelle // Match the Google Chat API's lowerCamelCase wire fields.
type ChatSearchResult struct {
	Message          *chat.Message `json:"message,omitempty"`
	Read             *bool         `json:"read,omitempty"`
	SpaceMuteSetting string        `json:"spaceMuteSetting,omitempty"`
}

//nolint:tagliatelle // Match the Google Chat API's lowerCamelCase wire fields.
type ChatSearchResponse struct {
	Results       []*ChatSearchResult `json:"results,omitempty"`
	NextPageToken string              `json:"nextPageToken,omitempty"`
}

func (c *ChatSearchClient) Search(ctx context.Context, query *chat.SearchMessagesRequest) (*ChatSearchResponse, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("encode Chat search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/spaces/-/messages:search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build Chat search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search Chat messages: %w", err)
	}
	defer resp.Body.Close()

	if err := gapi.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("search Chat messages: %w", err)
	}

	var result ChatSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Chat search response: %w", err)
	}

	return &result, nil
}
