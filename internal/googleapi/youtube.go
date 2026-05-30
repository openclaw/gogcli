package googleapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/api/option"
	httptransport "google.golang.org/api/transport/http"
	youtube "google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/googleauth"
)

var errYouTubeAPIKeyRequired = errors.New("youtube: API key required (config set youtube_api_key KEY or GOG_YOUTUBE_API_KEY)")

const scopeYouTubeForceSSL = "https://www.googleapis.com/auth/youtube.force-ssl"

// NewYouTubeWithAPIKey creates a YouTube Data API v3 service client using an API key.
// Use for public data: list by channelId, videoId, playlistId, etc.
// API key can be set via config (youtube_api_key) or GOG_YOUTUBE_API_KEY.
func NewYouTubeWithAPIKey(ctx context.Context, apiKey string) (*youtube.Service, error) {
	if apiKey == "" {
		return nil, errYouTubeAPIKeyRequired
	}

	client, err := newYouTubeAPIKeyHTTPClient(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	svc, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("youtube service with API key: %w", err)
	}

	return svc, nil
}

func newYouTubeAPIKeyHTTPClient(ctx context.Context, apiKey string) (*http.Client, error) {
	transport, err := httptransport.NewTransport(ctx, newBaseTransport(), option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("youtube API key transport: %w", err)
	}

	return &http.Client{Transport: NewRetryTransport(transport)}, nil
}

// NewYouTubeForAccount creates a YouTube Data API v3 service client using OAuth for the given account.
// Use for "mine" operations (authenticated user's channel, playlists, activities).
func NewYouTubeForAccount(ctx context.Context, email string) (*youtube.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceYouTube, email)
	if err != nil {
		return nil, fmt.Errorf("youtube OAuth options: %w", err)
	}

	svc, err := youtube.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("youtube service for account: %w", err)
	}

	return svc, nil
}

// NewYouTubeCommentsForAccount creates a YouTube Data API v3 client for comment reads.
// Google requires youtube.force-ssl for commentThreads.list; youtube.readonly is insufficient.
func NewYouTubeCommentsForAccount(ctx context.Context, email string) (*youtube.Service, error) {
	opts, err := optionsForAccountScopes(ctx, string(googleauth.ServiceYouTube), email, []string{scopeYouTubeForceSSL})
	if err != nil {
		return nil, fmt.Errorf("youtube comments OAuth options: %w", err)
	}

	svc, err := youtube.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("youtube comments service for account: %w", err)
	}

	return svc, nil
}
