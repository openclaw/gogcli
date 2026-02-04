package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/groupssettings/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewGroupsSettings(ctx context.Context, email string) (*groupssettings.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceAdminDirectory, email)
	if err != nil {
		return nil, fmt.Errorf("groups settings options: %w", err)
	}

	svc, err := groupssettings.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create groups settings service: %w", err)
	}

	return svc, nil
}
