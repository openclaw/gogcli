package cmd

import (
	"context"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
)

func getYouTubeAPIKey() (string, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return "", err
	}
	key := config.GetValue(cfg, config.KeyYoutubeAPIKey)
	if key == "" {
		return "", usage("YouTube API key required: set config youtube_api_key KEY or GOG_YOUTUBE_API_KEY")
	}
	return key, nil
}

func getYouTubeServiceWithAPIKey(ctx context.Context) (*youtube.Service, error) {
	key, err := getYouTubeAPIKey()
	if err != nil {
		return nil, err
	}
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.YouTubeAPIKey != nil {
		return runtime.Services.YouTubeAPIKey(ctx, key)
	}
	return googleapi.NewYouTubeWithAPIKey(ctx, key)
}

func getYouTubeServiceForAccount(ctx context.Context, account string) (*youtube.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.YouTubeAccount != nil {
		return runtime.Services.YouTubeAccount(ctx, account)
	}
	return googleapi.NewYouTubeForAccount(ctx, account)
}

func getYouTubeCommentsServiceForAccount(ctx context.Context, account string) (*youtube.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.YouTubeComments != nil {
		return runtime.Services.YouTubeComments(ctx, account)
	}
	return googleapi.NewYouTubeCommentsForAccount(ctx, account)
}

func getYouTubeWriteServiceForAccount(ctx context.Context, account string) (*youtube.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.YouTubeWrite != nil {
		return runtime.Services.YouTubeWrite(ctx, account)
	}
	return googleapi.NewYouTubeWriteForAccount(ctx, account)
}
