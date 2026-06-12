package cmd

import (
	"context"
	"errors"
	"testing"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/app"
)

type youtubeTestServices struct {
	APIKey   app.YouTubeServiceFactory
	Account  app.YouTubeServiceFactory
	Comments app.YouTubeServiceFactory
	Write    app.YouTubeServiceFactory
}

func fixedYouTubeTestService(svc *youtube.Service) app.YouTubeServiceFactory {
	return func(context.Context, string) (*youtube.Service, error) {
		return svc, nil
	}
}

func unexpectedYouTubeTestService(t *testing.T, message string) app.YouTubeServiceFactory {
	t.Helper()
	return func(context.Context, string) (*youtube.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected YouTube service call")
	}
}

func withYouTubeTestServices(ctx context.Context, services youtubeTestServices) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.YouTubeAPIKey = services.APIKey
		runtime.Services.YouTubeAccount = services.Account
		runtime.Services.YouTubeComments = services.Comments
		runtime.Services.YouTubeWrite = services.Write
	})
}
