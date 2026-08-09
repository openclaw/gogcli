package cmd

import (
	"context"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/openclaw/gogcli/internal/config"
)

func getYouTubeAPIKey(ctx context.Context) (string, error) {
	cfg, err := loadConfig(ctx)
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
	key, err := getYouTubeAPIKey(ctx)
	if err != nil {
		return nil, err
	}
	runtime, serviceErr := runtimeWithService(ctx, "youtube API key")
	if serviceErr != nil || runtime.Services.YouTubeAPIKey == nil {
		return nil, serviceError(serviceErr, "youtube API key")
	}
	return runtime.Services.YouTubeAPIKey(ctx, key)
}

func getYouTubeServiceForAccount(ctx context.Context, account string) (*youtube.Service, error) {
	runtime, err := runtimeWithService(ctx, "youtube account")
	if err != nil || runtime.Services.YouTubeAccount == nil {
		return nil, serviceError(err, "youtube account")
	}
	return runtime.Services.YouTubeAccount(ctx, account)
}

func getYouTubeCommentsServiceForAccount(ctx context.Context, account string) (*youtube.Service, error) {
	runtime, err := runtimeWithService(ctx, "youtube comments")
	if err != nil || runtime.Services.YouTubeComments == nil {
		return nil, serviceError(err, "youtube comments")
	}
	return runtime.Services.YouTubeComments(ctx, account)
}

func getYouTubeWriteServiceForAccount(ctx context.Context, account string) (*youtube.Service, error) {
	runtime, err := runtimeWithService(ctx, "youtube write")
	if err != nil || runtime.Services.YouTubeWrite == nil {
		return nil, serviceError(err, "youtube write")
	}
	return runtime.Services.YouTubeWrite(ctx, account)
}
