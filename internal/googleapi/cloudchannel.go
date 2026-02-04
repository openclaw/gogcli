package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/cloudchannel/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewCloudChannel(ctx context.Context, email string) (*cloudchannel.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceCloudChannel, email)
	if err != nil {
		return nil, fmt.Errorf("cloud channel options: %w", err)
	}

	svc, err := cloudchannel.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create cloud channel service: %w", err)
	}

	return svc, nil
}
