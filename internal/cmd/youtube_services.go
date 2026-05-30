package cmd

import (
	"context"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
)

var (
	newYouTubeWithAPIKey         = googleapi.NewYouTubeWithAPIKey
	newYouTubeForAccount         = googleapi.NewYouTubeForAccount
	newYouTubeCommentsForAccount = googleapi.NewYouTubeCommentsForAccount
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
	return newYouTubeWithAPIKey(ctx, key)
}

func getYouTubeServiceForAccount(ctx context.Context, account string) (*youtube.Service, error) {
	return newYouTubeForAccount(ctx, account)
}

func getYouTubeCommentsServiceForAccount(ctx context.Context, account string) (*youtube.Service, error) {
	return newYouTubeCommentsForAccount(ctx, account)
}
