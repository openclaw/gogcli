package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/cloudresourcemanager/v3"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewCloudResourceManager(ctx context.Context, email string) (*cloudresourcemanager.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceCloudResource, email)
	if err != nil {
		return nil, fmt.Errorf("cloud resource manager options: %w", err)
	}

	svc, err := cloudresourcemanager.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create cloud resource manager service: %w", err)
	}

	return svc, nil
}
