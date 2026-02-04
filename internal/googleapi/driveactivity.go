package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/driveactivity/v2"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewDriveActivity(ctx context.Context, email string) (*driveactivity.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceDriveActivity, email)
	if err != nil {
		return nil, fmt.Errorf("drive activity options: %w", err)
	}

	svc, err := driveactivity.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create drive activity service: %w", err)
	}

	return svc, nil
}
