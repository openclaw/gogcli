package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewYouTube(ctx context.Context, email string) (*youtube.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceYouTube, email)
	if err != nil {
		return nil, fmt.Errorf("youtube options: %w", err)
	}
	svc, err := youtube.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create youtube service: %w", err)
	}
	return svc, nil
}
